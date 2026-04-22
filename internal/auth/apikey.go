package auth

import (
	"crypto/subtle"
	"net/http"
)

// APIKeyMiddleware checks for a valid API key in the X-API-Key header
// using constant-time comparison to prevent timing attacks.
func APIKeyMiddleware(validKeys []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" || !matchesAnyKey(key, validKeys) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func matchesAnyKey(candidate string, validKeys []string) bool {
	matched := false
	for _, k := range validKeys {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(k)) == 1 {
			matched = true
		}
	}
	return matched
}
