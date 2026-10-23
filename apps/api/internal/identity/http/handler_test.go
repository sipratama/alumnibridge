package identityhttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sipratama/alumnibridge/apps/api/internal/httpserver"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

type allowedVerifier struct{}

func (allowedVerifier) Verify(_ context.Context, _ string) (identity.Principal, error) {
	return identity.Principal{Subject: "verified-subject"}, nil
}

type recordingStore struct {
	provisions  int
	onboardings int
}

func (s *recordingStore) Provision(_ context.Context, subject, _ string) (identity.User, error) {
	if subject != "verified-subject" {
		panic("unverified subject reached store")
	}
	s.provisions++
	return identity.User{ID: uuid.New(), Memberships: []identity.Membership{}}, nil
}
func (s *recordingStore) Onboard(_ context.Context, _ uuid.UUID, _ identity.OnboardingInput) (identity.User, error) {
	s.onboardings++
	return identity.User{}, nil
}

type healthyDB struct{}

func (healthyDB) Ping(context.Context) error { return nil }

func TestOnboardingRejectsClientIdentityAndAdminRole(t *testing.T) {
	store := &recordingStore{}
	router := httpserver.New(healthyDB{}, slog.Default())
	Register(router, identity.NewService(store), allowedVerifier{})
	for _, body := range []string{
		`{"role":"student","display_name":"Ada","institution":"uhamka","study_program":"mathematics-education","user_id":"` + uuid.NewString() + `"}`,
		`{"role":"admin","display_name":"Ada","institution":"uhamka","study_program":"mathematics-education"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/onboarding", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d for %s", response.Code, body)
		}
	}
	if store.provisions != 0 || store.onboardings != 0 {
		t.Fatal("invalid request touched the store")
	}
}
