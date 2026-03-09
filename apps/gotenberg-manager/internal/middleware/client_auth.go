package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const clientIDKey contextKey = "clientID"

// ClientAuth checks for a signed session cookie and injects the client ID into context.
// If the cookie is missing or invalid, it redirects to the login page.
func ClientAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("portal_session")
			if err != nil {
				http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
				return
			}

			clientID, err := validateSessionToken(cookie.Value, secret)
			if err != nil {
				// Clear invalid cookie
				http.SetCookie(w, &http.Cookie{
					Name:     "portal_session",
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
				})
				http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), clientIDKey, clientID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIDFromContext retrieves the client ID stored in the request context.
func ClientIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(clientIDKey).(string)
	return id
}

// CreateSessionToken creates a signed session token: base64(clientID|timestamp)|hmac
func CreateSessionToken(clientID, secret string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	payload := clientID + "|" + ts
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	sig := computeHMAC(encoded, secret)
	return encoded + "." + sig
}

// SetSessionCookie sets the portal session cookie on the response.
func SetSessionCookie(w http.ResponseWriter, clientID, secret string) {
	token := CreateSessionToken(clientID, secret)
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_session",
		Value:    token,
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie clears the portal session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func validateSessionToken(token, secret string) (string, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token format")
	}

	encoded, sig := parts[0], parts[1]
	expectedSig := computeHMAC(encoded, secret)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid signature")
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("invalid encoding")
	}

	payloadParts := strings.SplitN(string(decoded), "|", 2)
	if len(payloadParts) != 2 {
		return "", fmt.Errorf("invalid payload")
	}

	return payloadParts[0], nil
}

func computeHMAC(data, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
