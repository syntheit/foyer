package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"golang.org/x/sync/errgroup"

	"github.com/dmiller/foyer/internal/auth"
	"github.com/dmiller/foyer/internal/vmcontrol"
)

// vmActionRateWindow / vmActionRateMax: per-user rate limit, in addition to
// the controller's per-VM limit. Defense in depth.
const (
	vmActionRateWindow = time.Minute
	vmActionRateMax    = 10
)

var vmRateMu sync.Mutex
var vmRateBuckets = map[int64][]time.Time{}

func userActionAllowed(userID int64) bool {
	vmRateMu.Lock()
	defer vmRateMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-vmActionRateWindow)
	hits := vmRateBuckets[userID]
	live := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			live = append(live, t)
		}
	}
	if len(live) >= vmActionRateMax {
		vmRateBuckets[userID] = live
		return false
	}
	vmRateBuckets[userID] = append(live, now)
	return true
}

// sweepVMRateBuckets drops bucket entries whose hits have all expired.
// Without this the map grows by O(unique-users-who-ever-acted) over the
// process's lifetime.
func sweepVMRateBuckets() {
	vmRateMu.Lock()
	defer vmRateMu.Unlock()
	cutoff := time.Now().Add(-vmActionRateWindow)
	for uid, hits := range vmRateBuckets {
		anyLive := false
		for _, t := range hits {
			if t.After(cutoff) {
				anyLive = true
				break
			}
		}
		if !anyLive {
			delete(vmRateBuckets, uid)
		}
	}
}

// StartRateBucketSweeper runs a periodic GC pass. Call once at startup.
func StartRateBucketSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sweepVMRateBuckets()
			}
		}
	}()
}

// userIDFor returns the DB id for the authenticated user, or 0 on error.
func userIDFor(db *sql.DB, username string) int64 {
	var id int64
	if err := db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&id); err != nil {
		return 0
	}
	return id
}

func writeAudit(db *sql.DB, userID int64, actor, action, target, ip string, success bool, message string) {
	_, err := db.Exec(
		`INSERT INTO audit_log (user_id, actor, action, target, ip, success, message)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sql.NullInt64{Int64: userID, Valid: userID > 0}, actor, action, target, ip, success, message,
	)
	if err != nil {
		slog.Error("audit log write failed", "error", err)
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// hasAssignment returns true if the given user is assigned the named VM.
// Checks against the canonical vm_assignments table; never trusts the URL.
func hasAssignment(db *sql.DB, userID int64, vmName string) bool {
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM vm_assignments WHERE user_id = ? AND vm_name = ?",
		userID, vmName,
	).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// --- User-facing VM endpoints ---

func listMyVMsHandler(db *sql.DB, vmClient *vmcontrol.Client) http.HandlerFunc {
	type vmRow struct {
		Name      string `json:"name"`
		State     string `json:"state"`
		CreatedAt string `json:"assigned_at"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		userID := userIDFor(db, username)

		rows, err := db.Query(
			"SELECT vm_name, created_at FROM vm_assignments WHERE user_id = ? ORDER BY created_at",
			userID,
		)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out, err := scanRows(rows, func(rows *sql.Rows) (vmRow, error) {
			var v vmRow
			err := rows.Scan(&v.Name, &v.CreatedAt)
			return v, err
		})
		if err != nil {
			slog.Error("vm list scan", "error", err)
		}

		// Probe each VM's state in parallel; the controller round-trips one
		// virsh exec per call, so serial would be O(N × ~30ms) on a busy host.
		g, ctx := errgroup.WithContext(r.Context())
		g.SetLimit(8)
		for i := range out {
			i := i
			g.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if resp, err := vmClient.Call(vmcontrol.ActionInfo, out[i].Name); err == nil && resp.OK {
					if m, ok := resp.Data.(map[string]interface{}); ok {
						if s, ok := m["State"].(string); ok {
							out[i].State = s
						}
					}
				}
				return nil
			})
		}
		_ = g.Wait()
		writeJSON(w, out)
	}
}

func getMyVMStatsHandler(db *sql.DB, vmClient *vmcontrol.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		userID := userIDFor(db, username)

		vmName := chi.URLParam(r, "vm")
		// Defense in depth: validate name format before hitting the DB or controller.
		if !vmcontrol.ValidVMName(vmName) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if !hasAssignment(db, userID, vmName) {
			// 404 (not 403) so we don't leak which VM names exist.
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		resp, err := vmClient.Call(vmcontrol.ActionStats, vmName)
		if err != nil {
			http.Error(w, "stats unavailable", http.StatusBadGateway)
			return
		}
		writeJSON(w, resp.Data)
	}
}

func getMyVMHistoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		userID := userIDFor(db, username)

		vmName := chi.URLParam(r, "vm")
		if !vmcontrol.ValidVMName(vmName) || !hasAssignment(db, userID, vmName) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		rows, err := db.Query(
			`SELECT sampled_at, cpu_percent, mem_rss_kib, mem_max_kib, disk_alloc_b, disk_capacity_b, net_rx_bytes, net_tx_bytes
			 FROM vm_metric_samples
			 WHERE vm_name = ? AND sampled_at >= datetime('now', '-90 days')
			 ORDER BY sampled_at DESC
			 LIMIT 5000`,
			vmName,
		)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		samples, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var sampledAt string
			var cpu sql.NullFloat64
			var memRSS, memMax, diskAlloc, diskCap, netRx, netTx sql.NullInt64
			err := rows.Scan(&sampledAt, &cpu, &memRSS, &memMax, &diskAlloc, &diskCap, &netRx, &netTx)
			return map[string]interface{}{
				"sampled_at":      sampledAt,
				"cpu_percent":     cpu.Float64,
				"mem_rss_kib":     memRSS.Int64,
				"mem_max_kib":     memMax.Int64,
				"disk_alloc_b":    diskAlloc.Int64,
				"disk_capacity_b": diskCap.Int64,
				"net_rx_bytes":    netRx.Int64,
				"net_tx_bytes":    netTx.Int64,
			}, err
		})
		if err != nil {
			slog.Error("vm history scan", "error", err)
		}
		writeJSON(w, samples)
	}
}

func vmPowerHandler(db *sql.DB, vmClient *vmcontrol.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		userID := userIDFor(db, username)
		ip := clientIP(r)

		vmName := chi.URLParam(r, "vm")
		if !vmcontrol.ValidVMName(vmName) || !hasAssignment(db, userID, vmName) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req struct {
			Action string `json:"action"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		// Restrict users to graceful operations only.
		if req.Action != vmcontrol.ActionReboot && req.Action != vmcontrol.ActionShutdown {
			writeAudit(db, userID, username, req.Action, vmName, ip, false, "action not allowed")
			http.Error(w, "action not allowed", http.StatusBadRequest)
			return
		}

		if !userActionAllowed(userID) {
			writeAudit(db, userID, username, req.Action, vmName, ip, false, "rate limit")
			http.Error(w, "rate limit", http.StatusTooManyRequests)
			return
		}

		_, err := vmClient.Call(req.Action, vmName)
		if err != nil {
			writeAudit(db, userID, username, req.Action, vmName, ip, false, err.Error())
			http.Error(w, "operation failed", http.StatusBadGateway)
			return
		}
		writeAudit(db, userID, username, req.Action, vmName, ip, true, "")
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Admin endpoints (assignment management + listing all VMs) ---

func listAllVMsHandler(vmClient *vmcontrol.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := vmClient.Call(vmcontrol.ActionList, "")
		if err != nil {
			http.Error(w, "vm controller unavailable", http.StatusBadGateway)
			return
		}
		writeJSON(w, resp.Data)
	}
}

func listAssignmentsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT a.id, a.vm_name, a.created_at, u.id, u.username
			FROM vm_assignments a
			JOIN users u ON u.id = a.user_id
			ORDER BY a.vm_name, u.username
		`)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out, err := scanRows(rows, func(rows *sql.Rows) (map[string]interface{}, error) {
			var id, userID int
			var vmName, createdAt, username string
			err := rows.Scan(&id, &vmName, &createdAt, &userID, &username)
			return map[string]interface{}{
				"id":         id,
				"vm_name":    vmName,
				"created_at": createdAt,
				"user_id":    userID,
				"username":   username,
			}, err
		})
		if err != nil {
			slog.Error("assignments scan", "error", err)
		}
		writeJSON(w, out)
	}
}

func createAssignmentHandler(db *sql.DB, vmClient *vmcontrol.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			VMName   string `json:"vm_name"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		req.VMName = strings.TrimSpace(req.VMName)
		if req.Username == "" || !vmcontrol.ValidVMName(req.VMName) {
			http.Error(w, "username and valid vm_name are required", http.StatusBadRequest)
			return
		}

		// Verify the VM is real (not a typo) by asking the controller.
		resp, err := vmClient.Call(vmcontrol.ActionList, "")
		if err != nil {
			http.Error(w, "vm controller unavailable", http.StatusBadGateway)
			return
		}
		names, _ := resp.Data.([]interface{})
		found := false
		for _, n := range names {
			if s, ok := n.(string); ok && s == req.VMName {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "vm not found", http.StatusNotFound)
			return
		}

		var userID int64
		if err := db.QueryRow("SELECT id FROM users WHERE username = ?", req.Username).Scan(&userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if _, err := db.Exec(
			"INSERT INTO vm_assignments (user_id, vm_name) VALUES (?, ?)",
			userID, req.VMName,
		); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				http.Error(w, "already assigned", http.StatusConflict)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func deleteAssignmentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		res, err := db.Exec("DELETE FROM vm_assignments WHERE id = ?", id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// adminVMPowerHandler bypasses the per-user rate limit so admins can drive
// recovery flows on any VM regardless of assignment.
func adminVMPowerHandler(db *sql.DB, vmClient *vmcontrol.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, claims, _ := jwtauth.FromContext(r.Context())
		username := auth.GetUsername(claims)
		userID := userIDFor(db, username)
		ip := clientIP(r)

		vmName := chi.URLParam(r, "vm")
		if !vmcontrol.ValidVMName(vmName) {
			http.Error(w, "invalid vm", http.StatusBadRequest)
			return
		}

		var req struct {
			Action string `json:"action"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Action != vmcontrol.ActionReboot && req.Action != vmcontrol.ActionShutdown {
			http.Error(w, "action not allowed", http.StatusBadRequest)
			return
		}

		if _, err := vmClient.Call(req.Action, vmName); err != nil {
			writeAudit(db, userID, username, req.Action+":admin", vmName, ip, false, err.Error())
			http.Error(w, "operation failed", http.StatusBadGateway)
			return
		}
		writeAudit(db, userID, username, req.Action+":admin", vmName, ip, true, "")
		w.WriteHeader(http.StatusNoContent)
	}
}
