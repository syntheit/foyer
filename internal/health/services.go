package health

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/dmiller/foyer/internal/config"
)

type ServicesStats struct {
	Jellyfin  *JellyfinSvcStats  `json:"jellyfin,omitempty"`
	Minecraft *MinecraftSvcStats `json:"minecraft,omitempty"`
}

type JellyfinSvcStats struct {
	ActiveStreams int `json:"active_streams"`
}

type MinecraftSvcStats struct {
	Online     bool   `json:"online"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	Version    string `json:"version,omitempty"`
}

// CollectServices probes each configured service in parallel. Failures yield
// nil/Online=false so a single bad upstream doesn't blank the whole snapshot.
func CollectServices(cfg *config.Config, httpClient *http.Client) ServicesStats {
	var stats ServicesStats
	var mu sync.Mutex
	var wg sync.WaitGroup

	if cfg.Jellyfin != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s, err := pollJellyfin(cfg.Jellyfin, httpClient); err == nil {
				mu.Lock()
				stats.Jellyfin = s
				mu.Unlock()
			}
		}()
	}

	if cfg.Minecraft != nil && cfg.Minecraft.Address != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := pingMinecraft(cfg.Minecraft.Address, 2*time.Second)
			mu.Lock()
			stats.Minecraft = s
			mu.Unlock()
		}()
	}

	wg.Wait()
	return stats
}

func pollJellyfin(cfg *config.JellyfinConfig, client *http.Client) (*JellyfinSvcStats, error) {
	req, err := http.NewRequest("GET", cfg.URL+"/Sessions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Token", cfg.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sessions []struct {
		NowPlayingItem interface{} `json:"NowPlayingItem"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, err
	}
	active := 0
	for _, s := range sessions {
		if s.NowPlayingItem != nil {
			active++
		}
	}
	return &JellyfinSvcStats{ActiveStreams: active}, nil
}

// pingMinecraft speaks the Java edition Server List Ping protocol — no auth.
// https://wiki.vg/Server_List_Ping
func pingMinecraft(addr string, timeout time.Duration) *MinecraftSvcStats {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return &MinecraftSvcStats{Online: false}
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return &MinecraftSvcStats{Online: false}
	}
	port, _ := strconv.Atoi(portStr)

	hs := make([]byte, 0, 32)
	hs = appendVarInt(hs, 0)
	hs = appendVarInt(hs, 763) // any modern protocol version; servers ignore exact value
	hs = appendString(hs, host)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(port))
	hs = append(hs, portBuf[:]...)
	hs = appendVarInt(hs, 1)

	if err := writePacket(conn, hs); err != nil {
		return &MinecraftSvcStats{Online: false}
	}
	if err := writePacket(conn, []byte{0x00}); err != nil {
		return &MinecraftSvcStats{Online: false}
	}

	if _, err := readVarInt(conn); err != nil {
		return &MinecraftSvcStats{Online: false}
	}
	pktID, err := readVarInt(conn)
	if err != nil || pktID != 0 {
		return &MinecraftSvcStats{Online: false}
	}
	strLen, err := readVarInt(conn)
	if err != nil || strLen <= 0 || strLen > 64*1024 {
		return &MinecraftSvcStats{Online: false}
	}
	buf := make([]byte, strLen)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return &MinecraftSvcStats{Online: false}
	}

	var resp struct {
		Version struct {
			Name string `json:"name"`
		} `json:"version"`
		Players struct {
			Max    int `json:"max"`
			Online int `json:"online"`
		} `json:"players"`
	}
	if err := json.Unmarshal(buf, &resp); err != nil {
		return &MinecraftSvcStats{Online: false}
	}
	return &MinecraftSvcStats{
		Online:     true,
		Players:    resp.Players.Online,
		MaxPlayers: resp.Players.Max,
		Version:    resp.Version.Name,
	}
}

func writePacket(w io.Writer, payload []byte) error {
	prefix := appendVarInt(nil, len(payload))
	if _, err := w.Write(prefix); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func appendVarInt(b []byte, v int) []byte {
	for {
		if v&^0x7F == 0 {
			return append(b, byte(v))
		}
		b = append(b, byte((v&0x7F)|0x80))
		v = int(uint(v) >> 7)
	}
}

func appendString(b []byte, s string) []byte {
	b = appendVarInt(b, len(s))
	return append(b, s...)
}

// net.Conn.Read can short-read by spec, so a one-byte read isn't safe even for
// the next-byte case here.
func readVarInt(r io.Reader) (int, error) {
	var v int
	var shift uint
	var buf [1]byte
	for i := 0; i < 5; i++ {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		v |= int(buf[0]&0x7F) << shift
		if buf[0]&0x80 == 0 {
			return v, nil
		}
		shift += 7
	}
	return 0, errors.New("varint exceeds 5 bytes")
}
