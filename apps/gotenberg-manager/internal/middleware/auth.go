package middleware

import (
	"net/http"
	"strings"
)

// AdminAuth validates the admin bearer token
func AdminAuth(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if token == auth {
				// No "Bearer " prefix found
				http.Error(w, `{"error":"invalid Authorization format, use Bearer <token>"}`, http.StatusUnauthorized)
				return
			}

			if token != adminToken {
				http.Error(w, `{"error":"invalid admin token"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
