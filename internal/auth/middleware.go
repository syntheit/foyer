package auth

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/jwtauth/v5"
)

// RequireAuth is a middleware that ensures the request has a valid JWT token.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, claims, err := jwtauth.FromContext(r.Context())
		if err != nil {
			slog.Debug("auth failed", "error", err, "path", r.URL.Path, "token_nil", token == nil)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if claims == nil {
			slog.Debug("auth failed: nil claims", "path", r.URL.Path)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is a middleware that ensures the user has admin role.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, claims, err := jwtauth.FromContext(r.Context())
		if err != nil || claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !IsAdmin(claims) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
