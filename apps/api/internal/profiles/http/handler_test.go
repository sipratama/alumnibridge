package profileshttp

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (identity.Principal, error) {
	if token != "valid" {
		return identity.Principal{}, errors.New("invalid")
	}
	return identity.Principal{Subject: "subject"}, nil
}

type identityStore struct {
	identity.Store
	id uuid.UUID
}

func (s identityStore) Provision(_ context.Context, _, _ string) (identity.User, error) {
	return identity.User{ID: s.id}, nil
}

type profileStore struct {
	profiles.Store
	id uuid.UUID
}

func (s profileStore) Profile(_ context.Context, _ uuid.UUID) (profiles.Profile, error) {
	return profiles.Profile{DisplayName: "Jane Doe", Slug: "jane-doe"}, nil
}
func (s profileStore) Counts(_ context.Context, _ uuid.UUID) ([3]int, error) { return [3]int{}, nil }
func (s profileStore) DeleteExperience(_ context.Context, _, _ uuid.UUID) error {
	return profiles.ErrNotFound
}
func TestOwnerRoutes(t *testing.T) {
	id := uuid.New()
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		RegisterRoutes(r, profiles.New(profileStore{id: id}), identity.NewService(identityStore{id: id}), verifier{})
	})
	cases := []struct {
		method, path, body, token string
		want                      int
	}{
		{"GET", "/api/v1/users/me/profile", "", "", 401},
		{"GET", "/api/v1/users/me/profile", "", "Bearer valid", 200},
		{"PATCH", "/api/v1/users/me/profile", `{"slug":"admin"}`, "Bearer valid", 400},
		{"DELETE", "/api/v1/users/me/experiences/" + uuid.NewString(), "", "Bearer valid", 404},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", tc.token)
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s %s: got %d want %d: %s", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}
}
