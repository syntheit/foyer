package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/dmiller/foyer/internal/auth"
	"github.com/dmiller/foyer/internal/health"
	"github.com/go-chi/jwtauth/v5"
)

const broadcastInterval = 5 * time.Second

type client struct {
	conn *websocket.Conn
	done chan struct{}
}

type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	domain  string
	devMode bool
}

func NewHub(domain string, devMode bool) *Hub {
	return &Hub{
		clients: make(map[*client]struct{}),
		domain:  domain,
		devMode: devMode,
	}
}

// Run broadcasts stats to all connected clients at a regular interval.
func (h *Hub) Run(ctx context.Context, collector *health.Collector) {
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := collector.Current()
			msg := struct {
				Type string `json:"type"`
				health.Stats
			}{
				Type:  "stats",
				Stats: stats,
			}
			data, err := json.Marshal(msg)
			if err != nil {
				slog.Error("failed to marshal stats", "error", err)
				continue
			}
			h.broadcast(ctx, data)
		}
	}
}

func (h *Hub) broadcast(ctx context.Context, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.conn.Write(writeCtx, websocket.MessageText, data)
		cancel()
		if err != nil {
			slog.Debug("removing disconnected client", "error", err)
			c.conn.CloseNow()
			delete(h.clients, c)
		}
	}
}

// Handler upgrades HTTP connections to WebSocket and adds them to the hub.
func (h *Hub) Handler(tokenAuth *jwtauth.JWTAuth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify JWT from cookie
		token, err := jwtauth.VerifyRequest(tokenAuth, r, auth.TokenFromCookie)
		if err != nil || token == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Restrict origins to the configured domain to prevent CSWSH
		origins := []string{h.domain}
		if h.devMode {
			origins = append(origins, "localhost:*")
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: origins,
		})
		if err != nil {
			slog.Error("websocket accept failed", "error", err)
			return
		}

		c := &client{
			conn: conn,
			done: make(chan struct{}),
		}

		h.mu.Lock()
		h.clients[c] = struct{}{}
		h.mu.Unlock()

		// Block until the client disconnects.
		// Read in a loop to process control frames (ping/pong/close).
		for {
			_, _, err := c.conn.Read(r.Context())
			if err != nil {
				break
			}
		}

		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		conn.CloseNow()
	}
}
