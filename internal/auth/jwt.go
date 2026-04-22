package auth

import (
	"net/http"
	"time"

	"github.com/go-chi/jwtauth/v5"
)

const (
	cookieName    = "foyer_token"
	tokenExpiry   = 7 * 24 * time.Hour
	refreshAfter  = 24 * time.Hour
)

type Auth struct {
	tokenAuth    *jwtauth.JWTAuth
	cookieDomain string
	secure       bool
}

func New(secret string, cookieDomain string) *Auth {
	// Only set Secure flag when using a real domain (not localhost/dev)
	secure := cookieDomain != "" && cookieDomain != "localhost"
	return &Auth{
		tokenAuth:    jwtauth.New("HS256", []byte(secret), nil),
		cookieDomain: cookieDomain,
		secure:       secure,
	}
}

func (a *Auth) TokenAuth() *jwtauth.JWTAuth {
	return a.tokenAuth
}

func (a *Auth) CreateToken(username, role string) (string, error) {
	now := time.Now()
	claims := map[string]interface{}{
		"username": username,
		"role":     role,
		"iat":      now.Unix(),
		"exp":      now.Add(tokenExpiry).Unix(),
	}
	_, tokenStr, err := a.tokenAuth.Encode(claims)
	return tokenStr, err
}

func (a *Auth) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Domain:   a.cookieDomain,
		Path:     "/",
		MaxAge:   int(tokenExpiry.Seconds()),
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Auth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Domain:   a.cookieDomain,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ShouldRefresh checks if the token is older than refreshAfter and returns true if it should be refreshed.
func ShouldRefresh(claims map[string]interface{}) bool {
	iat, ok := claims["iat"].(float64)
	if !ok {
		return false
	}
	issuedAt := time.Unix(int64(iat), 0)
	return time.Since(issuedAt) > refreshAfter
}

func GetUsername(claims map[string]interface{}) string {
	username, _ := claims["username"].(string)
	return username
}

func GetRole(claims map[string]interface{}) string {
	role, _ := claims["role"].(string)
	return role
}

func IsAdmin(claims map[string]interface{}) bool {
	return GetRole(claims) == "admin"
}

// TokenFromCookie extracts the JWT token from the foyer_token cookie.
func TokenFromCookie(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// Verifier is a middleware that looks for a JWT in our custom cookie and
// the Authorization header, then sets the token in the request context.
func Verifier(ja *jwtauth.JWTAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return jwtauth.Verify(ja, TokenFromCookie, jwtauth.TokenFromHeader)(next)
	}
}
