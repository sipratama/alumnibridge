package identityhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

type Handler struct{ service *identity.Service }

func Register(router chi.Router, service *identity.Service, verifier auth.Verifier) {
	router.Route("/api/v1", func(r chi.Router) { RegisterRoutes(r, service, verifier) })
}

func RegisterRoutes(router chi.Router, service *identity.Service, verifier auth.Verifier) {
	h := &Handler{service: service}
	router.Group(func(r chi.Router) {
		r.Use(auth.Middleware(verifier))
		r.Get("/auth/me", h.me)
		r.Post("/auth/onboarding", h.onboard)
		r.Get("/users/me", h.me)
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	actor, _ := auth.Principal(r.Context())
	user, err := h.service.Me(r.Context(), actor)
	if err != nil {
		if errors.Is(err, identity.ErrAccessDenied) {
			writeError(w, http.StatusForbidden, "account_unavailable")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) onboard(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	var input identity.OnboardingInput
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	actor, _ := auth.Principal(r.Context())
	user, err := h.service.Onboard(r.Context(), actor, input)
	switch {
	case errors.Is(err, identity.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_onboarding")
	case errors.Is(err, identity.ErrProgramNotFound):
		writeError(w, http.StatusBadRequest, "invalid_study_program")
	case errors.Is(err, identity.ErrMembershipConflict):
		writeError(w, http.StatusConflict, "membership_conflict")
	case errors.Is(err, identity.ErrAccessDenied):
		writeError(w, http.StatusForbidden, "account_unavailable")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error")
	default:
		writeJSON(w, http.StatusOK, user)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
