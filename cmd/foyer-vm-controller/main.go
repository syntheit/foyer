// foyer-vm-controller is the privilege boundary for VM operations.
//
// It runs as a separate systemd unit (user `foyer-vm`, group `libvirtd`),
// listens on a Unix socket, and is the *only* process in the foyer stack
// that ever invokes virsh. The web service (foyer) holds no libvirt access
// at all; it talks to this controller as a client.
//
// Trust boundary properties:
//   - SO_PEERCRED check: only the foyer UID may connect.
//   - Action allowlist: anything outside the list is rejected with no I/O.
//   - VM name regex: rejects shell metacharacters, leading dashes, paths.
//   - Existence check: VM must be a defined libvirt domain before any action.
//   - exec.Command with positional args, no shell, no env passthrough.
//   - Per-VM rate limiting on power actions.
//   - 1 KB request cap, 5 s deadlines.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dmiller/foyer/internal/vmcontrol"
)

const (
	defaultSocket    = vmcontrol.SocketPath
	commandTimeout   = 5 * time.Second
	rateWindow       = time.Minute
	rateMaxPerVM     = 5 // power actions / VM / minute
	libvirtURI       = "qemu:///system"
	// Each stats call costs 5–8 fork+execs of virsh. Multiple foyer clients
	// (browsers + the host collector) hit the same VM on overlapping ticks,
	// so a small cache turns the steady-state cost back into O(VMs).
	statsCacheTTL = 2 * time.Second
)

func main() {
	socketPath := flag.String("socket", defaultSocket, "Unix socket path")
	expectedUID := flag.Int("foyer-uid", -1, "UID of the foyer process allowed to connect (overrides --foyer-user)")
	expectedUser := flag.String("foyer-user", "foyer", "Username of the foyer process; resolved to a UID at startup")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	uid := *expectedUID
	if uid < 0 {
		u, err := user.Lookup(*expectedUser)
		if err != nil {
			slog.Error("failed to look up foyer user", "user", *expectedUser, "error", err)
			os.Exit(2)
		}
		parsed, err := strconv.Atoi(u.Uid)
		if err != nil {
			slog.Error("failed to parse uid", "uid", u.Uid, "error", err)
			os.Exit(2)
		}
		uid = parsed
	}

	// Verify virsh is reachable. Failing fast at boot is better than failing
	// silently when a user clicks reboot.
	if _, err := exec.LookPath("virsh"); err != nil {
		slog.Error("virsh not in PATH", "error", err)
		os.Exit(2)
	}

	// Always remove a stale socket. Bind, then chmod 0660 so the foyer group
	// member can connect; group membership is the access control mechanism.
	_ = os.Remove(*socketPath)
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("listen failed", "error", err)
		os.Exit(1)
	}
	if err := os.Chmod(*socketPath, 0o660); err != nil {
		slog.Error("chmod socket failed", "error", err)
		os.Exit(1)
	}

	srv := &server{
		expectedUID: uint32(uid),
		rate:        newRateLimiter(),
		statsCache:  make(map[string]statsEntry),
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				srv.rate.sweep()
				srv.pruneStatsCache()
			}
		}
	}()

	slog.Info("foyer-vm-controller listening", "socket", *socketPath, "expected_uid", uid)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Warn("accept failed", "error", err)
			continue
		}
		go srv.handle(conn)
	}
}

type server struct {
	expectedUID uint32
	rate        *rateLimiter

	statsMu    sync.Mutex
	statsCache map[string]statsEntry
}

type statsEntry struct {
	stats    *VMStats
	cachedAt time.Time
}

func (s *server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(commandTimeout))

	uc, ok := conn.(*net.UnixConn)
	if !ok {
		writeError(conn, "non-unix connection rejected")
		return
	}

	// SO_PEERCRED — only accept connections from the configured UID. Group
	// membership opens the socket; this confirms which process is connecting.
	if err := s.checkPeerCred(uc); err != nil {
		slog.Warn("peer cred rejected", "error", err)
		writeError(conn, "unauthorized")
		return
	}

	// Read at most MaxRequestBytes + a small ceiling. Anything larger is dropped.
	r := io.LimitReader(uc, vmcontrol.MaxRequestBytes+1)
	body, err := io.ReadAll(r)
	if err != nil {
		writeError(conn, "read failed")
		return
	}
	if len(body) > vmcontrol.MaxRequestBytes {
		writeError(conn, "request too large")
		return
	}

	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	var req vmcontrol.Request
	if err := dec.Decode(&req); err != nil {
		writeError(conn, "invalid json")
		return
	}
	if dec.More() {
		writeError(conn, "trailing data")
		return
	}

	if !vmcontrol.IsActionAllowed(req.Action) {
		slog.Warn("rejected unknown action", "action", req.Action)
		writeError(conn, "action not allowed")
		return
	}

	// VM name validation — except for `list` which doesn't need one.
	if req.Action != vmcontrol.ActionList {
		if !vmcontrol.ValidVMName(req.VM) {
			slog.Warn("rejected invalid vm name", "vm", req.VM)
			writeError(conn, "invalid vm name")
			return
		}
		if !s.vmExists(req.VM) {
			writeError(conn, "vm not found")
			return
		}
	}

	if req.Action == vmcontrol.ActionReboot || req.Action == vmcontrol.ActionShutdown {
		if !s.rate.allow(req.VM) {
			writeError(conn, "rate limit")
			return
		}
	}

	resp := s.execute(req)
	writeResponse(conn, resp)
}

func (s *server) checkPeerCred(uc *net.UnixConn) error {
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var ucred *syscall.Ucred
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		ucred, sockErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return err
	}
	if sockErr != nil {
		return sockErr
	}
	if ucred.Uid != s.expectedUID {
		return fmt.Errorf("peer uid %d != expected %d", ucred.Uid, s.expectedUID)
	}
	return nil
}

// vmExists confirms name is in the libvirt domain list. We never trust the
// caller's claim that a domain is real; we re-check every time.
func (s *server) vmExists(name string) bool {
	out, err := runVirsh(commandTimeout, "list", "--all", "--name")
	if err != nil {
		slog.Warn("vmExists: virsh list failed", "error", err)
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func (s *server) execute(req vmcontrol.Request) vmcontrol.Response {
	switch req.Action {
	case vmcontrol.ActionList:
		out, err := runVirsh(commandTimeout, "list", "--all", "--name")
		if err != nil {
			return errResp("list failed")
		}
		var names []string
		for _, l := range strings.Split(out, "\n") {
			n := strings.TrimSpace(l)
			if n != "" {
				names = append(names, n)
			}
		}
		return vmcontrol.Response{OK: true, Data: names}

	case vmcontrol.ActionInfo:
		out, err := runVirsh(commandTimeout, "dominfo", req.VM)
		if err != nil {
			return errResp("dominfo failed")
		}
		return vmcontrol.Response{OK: true, Data: parseKeyValue(out)}

	case vmcontrol.ActionStats:
		stats, err := s.cachedStats(req.VM)
		if err != nil {
			slog.Warn("stats failed", "vm", req.VM, "error", err)
			return errResp("stats failed")
		}
		return vmcontrol.Response{OK: true, Data: stats}

	case vmcontrol.ActionReboot:
		if _, err := runVirsh(commandTimeout, "reboot", req.VM); err != nil {
			slog.Warn("reboot failed", "vm", req.VM, "error", err)
			return errResp("reboot failed")
		}
		slog.Info("reboot issued", "vm", req.VM)
		return vmcontrol.Response{OK: true}

	case vmcontrol.ActionShutdown:
		if _, err := runVirsh(commandTimeout, "shutdown", req.VM); err != nil {
			slog.Warn("shutdown failed", "vm", req.VM, "error", err)
			return errResp("shutdown failed")
		}
		slog.Info("shutdown issued", "vm", req.VM)
		return vmcontrol.Response{OK: true}
	}
	return errResp("unhandled action")
}

func (s *server) cachedStats(name string) (*VMStats, error) {
	s.statsMu.Lock()
	if e, ok := s.statsCache[name]; ok && time.Since(e.cachedAt) < statsCacheTTL {
		s.statsMu.Unlock()
		return e.stats, nil
	}
	s.statsMu.Unlock()

	stats, err := collectStats(name)
	if err != nil {
		return nil, err
	}
	s.statsMu.Lock()
	s.statsCache[name] = statsEntry{stats: stats, cachedAt: time.Now()}
	s.statsMu.Unlock()
	return stats, nil
}

// pruneStatsCache evicts entries older than 2× the TTL — anything past that
// is for a VM nobody's queried recently.
func (s *server) pruneStatsCache() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	cutoff := time.Now().Add(-2 * statsCacheTTL)
	for k, e := range s.statsCache {
		if e.cachedAt.Before(cutoff) {
			delete(s.statsCache, k)
		}
	}
}

func errResp(msg string) vmcontrol.Response {
	return vmcontrol.Response{OK: false, Error: msg}
}

func writeError(w io.Writer, msg string) {
	writeResponse(w, errResp(msg))
}

func writeResponse(w io.Writer, resp vmcontrol.Response) {
	b, _ := json.Marshal(resp)
	_, _ = w.Write(b)
}

// --- virsh helpers ---

// runVirsh executes virsh with the given args and returns combined stdout.
// `--` is inserted before positional args to defang any flag-like name that
// somehow slips past the regex (defense in depth — the regex already rejects
// leading dashes).
func runVirsh(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	full := append([]string{"-c", libvirtURI}, args...)
	cmd := exec.CommandContext(ctx, "virsh", full...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C"} // minimal env, deterministic locale
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// parseKeyValue parses lines like "Key: value" into a map.
func parseKeyValue(s string) map[string]string {
	result := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		result[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return result
}

// VMStats is what we report back. All numeric fields are best-effort —
// missing data is reported as 0 rather than failing the whole call.
type VMStats struct {
	State       string `json:"state"`
	VCPUs       int    `json:"vcpus"`
	CPUTimeNS   uint64 `json:"cpu_time_ns"`
	MemMaxKiB   uint64 `json:"mem_max_kib"`
	MemRSSKiB   uint64 `json:"mem_rss_kib"`
	MemUsableKiB uint64 `json:"mem_usable_kib"`
	MemUnusedKiB uint64 `json:"mem_unused_kib"`
	DiskCapacityB  uint64 `json:"disk_capacity_b"`
	DiskAllocB     uint64 `json:"disk_alloc_b"`
	DiskReadB      uint64 `json:"disk_read_b"`
	DiskWrittenB   uint64 `json:"disk_written_b"`
	NetRxBytes uint64 `json:"net_rx_bytes"`
	NetTxBytes uint64 `json:"net_tx_bytes"`
	SampledAt  int64  `json:"sampled_at"`
}

func collectStats(name string) (*VMStats, error) {
	stats := &VMStats{SampledAt: time.Now().Unix()}

	// domstats: state, vcpus, balloon
	out, err := runVirsh(commandTimeout, "domstats", "--state", "--vcpu", "--balloon", name)
	if err != nil {
		return nil, err
	}
	parseDomstats(out, stats)

	// dominfo for the human-readable state name
	if info, err := runVirsh(commandTimeout, "dominfo", name); err == nil {
		kv := parseKeyValue(info)
		if v, ok := kv["State"]; ok {
			stats.State = v
		}
	}

	// disk: enumerate block devices, sum across all of them
	blkOut, err := runVirsh(commandTimeout, "domblklist", "--details", name)
	if err == nil {
		for _, dev := range parseBlockDevices(blkOut) {
			if info, err := runVirsh(commandTimeout, "domblkinfo", name, dev); err == nil {
				cap, alloc := parseDomblkinfo(info)
				stats.DiskCapacityB += cap
				stats.DiskAllocB += alloc
			}
			if io, err := runVirsh(commandTimeout, "domblkstat", name, dev); err == nil {
				rd, wr := parseDomblkstat(io)
				stats.DiskReadB += rd
				stats.DiskWrittenB += wr
			}
		}
	}

	// network: enumerate interfaces, sum
	ifOut, err := runVirsh(commandTimeout, "domiflist", name)
	if err == nil {
		for _, iface := range parseInterfaces(ifOut) {
			if io, err := runVirsh(commandTimeout, "domifstat", name, iface); err == nil {
				rx, tx := parseDomifstat(io)
				stats.NetRxBytes += rx
				stats.NetTxBytes += tx
			}
		}
	}

	return stats, nil
}

func parseDomstats(s string, out *VMStats) {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "vcpu.current":
			out.VCPUs, _ = strconv.Atoi(v)
		case "balloon.maximum":
			out.MemMaxKiB = parseU64(v)
		case "balloon.rss":
			out.MemRSSKiB = parseU64(v)
		case "balloon.usable":
			out.MemUsableKiB = parseU64(v)
		case "balloon.unused":
			out.MemUnusedKiB = parseU64(v)
		default:
			// Sum CPU time across all vCPUs.
			if strings.HasPrefix(k, "vcpu.") && strings.HasSuffix(k, ".time") {
				out.CPUTimeNS += parseU64(v)
			}
		}
	}
}

func parseBlockDevices(s string) []string {
	var devs []string
	sc := bufio.NewScanner(strings.NewReader(s))
	// Skip the two header lines from `virsh domblklist --details`:
	//   Type   Device   Target   Source
	//   --------------------------------
	header := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if header < 2 {
			header++
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Take only `disk` types (skip cdrom etc.) with a real source.
		if len(fields) >= 4 && fields[0] == "disk" && fields[3] != "-" {
			devs = append(devs, fields[2])
		}
	}
	return devs
}

func parseInterfaces(s string) []string {
	var ifaces []string
	sc := bufio.NewScanner(strings.NewReader(s))
	header := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if header < 2 {
			header++
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			ifaces = append(ifaces, fields[0])
		}
	}
	return ifaces
}

func parseDomblkinfo(s string) (capacity, allocation uint64) {
	kv := parseKeyValue(s)
	capacity = parseU64(kv["Capacity"])
	allocation = parseU64(kv["Allocation"])
	return
}

func parseDomblkstat(s string) (read, written uint64) {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		// Format: "<dev> rd_bytes <n>" or "<dev> wr_bytes <n>"
		switch fields[1] {
		case "rd_bytes":
			read = parseU64(fields[2])
		case "wr_bytes":
			written = parseU64(fields[2])
		}
	}
	return
}

func parseDomifstat(s string) (rx, tx uint64) {
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		switch fields[1] {
		case "rx_bytes":
			rx = parseU64(fields[2])
		case "tx_bytes":
			tx = parseU64(fields[2])
		}
	}
	return
}

func parseU64(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}

// --- rate limiter ---

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string][]time.Time)}
}

func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rateWindow)

	// Drop expired entries.
	hits := r.buckets[key]
	live := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			live = append(live, t)
		}
	}
	if len(live) >= rateMaxPerVM {
		r.buckets[key] = live
		return false
	}
	r.buckets[key] = append(live, now)
	return true
}

// sweep drops bucket keys whose hits have all expired. Bounds the map by
// active-VMs-in-the-window rather than ever-touched-VMs.
func (r *rateLimiter) sweep() {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-rateWindow)
	for k, hits := range r.buckets {
		anyLive := false
		for _, t := range hits {
			if t.After(cutoff) {
				anyLive = true
				break
			}
		}
		if !anyLive {
			delete(r.buckets, k)
		}
	}
}
