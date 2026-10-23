package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

type stubVerifier struct {
	actor identity.Principal
	err   error
}

func (v stubVerifier) Verify(_ context.Context, _ string) (identity.Principal, error) {
	return v.actor, v.err
}

func TestAuthorizationMiddleware(t *testing.T) {
	for _, test := range []struct {
		name, header string
		verifier     stubVerifier
		want         int
	}{
		{"missing", "", stubVerifier{}, 401},
		{"malformed", "Basic abc", stubVerifier{}, 401},
		{"expired", "Bearer expired", stubVerifier{err: errors.New("expired")}, 401},
		{"forbidden", "Bearer valid", stubVerifier{actor: identity.Principal{Subject: "s"}}, 403},
		{"admin", "Bearer valid", stubVerifier{actor: identity.Principal{Subject: "s", Roles: map[string]bool{"alumnibridge-admin": true}}}, 204},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := Middleware(test.verifier)(RequireRole("alumnibridge-admin")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
			req := httptest.NewRequest("GET", "/admin", nil)
			if test.header != "" {
				req.Header.Set("Authorization", test.header)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != test.want {
				t.Fatalf("got %d want %d", resp.Code, test.want)
			}
		})
	}
}
