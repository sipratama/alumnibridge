package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

type principalKey struct{}

func Principal(ctx context.Context) (identity.Principal, bool) {
	actor, ok := ctx.Value(principalKey{}).(identity.Principal)
	return actor, ok
}

func Middleware(verifier Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			parts := strings.Fields(r.Header.Get("Authorization"))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
				unauthorized(w)
				return
			}
			actor, err := verifier.Verify(r.Context(), parts[1])
			if err != nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, actor)))
		})
	}
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := Principal(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			if !actor.Roles[role] {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter) { writeError(w, http.StatusUnauthorized, "unauthorized") }
func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
