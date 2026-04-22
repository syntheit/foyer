package auth

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshTimestampHeader = "X-SSH-Timestamp"
	sshSignatureHeader = "X-SSH-Signature"
	sshMaxClockSkew    = 5 * time.Minute
)

// SSHKeyStore holds parsed authorized public keys for signature verification.
type SSHKeyStore struct {
	keys []ssh.PublicKey
}

// NewSSHKeyStore parses authorized_keys formatted lines into public keys.
func NewSSHKeyStore(authorizedKeys []string) *SSHKeyStore {
	store := &SSHKeyStore{}
	for _, line := range authorizedKeys {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			slog.Warn("failed to parse SSH public key", "error", err)
			continue
		}
		store.keys = append(store.keys, pubKey)
		slog.Info("loaded SSH authorized key", "type", pubKey.Type(), "fingerprint", ssh.FingerprintSHA256(pubKey))
	}
	return store
}

// Verify checks if the given data was signed by any of the authorized keys.
func (s *SSHKeyStore) Verify(data []byte, sigBytes []byte) bool {
	sig := new(ssh.Signature)
	if err := ssh.Unmarshal(sigBytes, sig); err != nil {
		slog.Debug("failed to unmarshal SSH signature", "error", err)
		return false
	}

	for _, key := range s.keys {
		if err := key.Verify(data, sig); err == nil {
			return true
		}
	}
	return false
}

// HasKeys returns true if any authorized keys are configured.
func (s *SSHKeyStore) HasKeys() bool {
	return len(s.keys) > 0
}

// BuildSignedMessage constructs the message that must be signed for a request.
// Format: "{timestamp}\n{method} {path}"
func BuildSignedMessage(timestamp string, method string, path string) []byte {
	return []byte(fmt.Sprintf("%s\n%s %s", timestamp, method, path))
}

// CombinedAuthMiddleware checks for either a valid API key (X-API-Key header)
// or a valid SSH signature (X-SSH-Timestamp + X-SSH-Signature headers).
func CombinedAuthMiddleware(sshStore *SSHKeyStore, validAPIKeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try API key first (fast path)
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				if matchesAnyKey(apiKey, validAPIKeys) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Try SSH signature
			timestampStr := r.Header.Get(sshTimestampHeader)
			sigB64 := r.Header.Get(sshSignatureHeader)

			if timestampStr != "" && sigB64 != "" && sshStore.HasKeys() {
				// Validate timestamp (replay protection)
				ts, err := strconv.ParseInt(timestampStr, 10, 64)
				if err != nil {
					http.Error(w, "invalid timestamp", http.StatusBadRequest)
					return
				}
				skew := time.Since(time.Unix(ts, 0))
				if skew < 0 {
					skew = -skew
				}
				if skew > sshMaxClockSkew {
					http.Error(w, "timestamp expired", http.StatusUnauthorized)
					return
				}

				// Decode and verify signature
				sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
				if err != nil {
					http.Error(w, "invalid signature encoding", http.StatusBadRequest)
					return
				}

				message := BuildSignedMessage(timestampStr, r.Method, r.URL.Path)
				if sshStore.Verify(message, sigBytes) {
					next.ServeHTTP(w, r)
					return
				}

				http.Error(w, "signature verification failed", http.StatusUnauthorized)
				return
			}

			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}
