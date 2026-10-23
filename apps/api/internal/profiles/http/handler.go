package profileshttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

type Handler struct {
	service  *profiles.Service
	identity *identity.Service
}

var errMediaType = errors.New("unsupported media type")

func RegisterRoutes(r chi.Router, s *profiles.Service, ids *identity.Service, v auth.Verifier) {
	h := &Handler{service: s, identity: ids}
	r.Get("/skills", h.searchSkills)
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(v))
		r.Get("/users/me/profile", h.profile)
		r.Patch("/users/me/profile", h.updateProfile)
		r.Get("/users/me/experiences", h.experiences)
		r.Post("/users/me/experiences", h.addExperience)
		r.Patch("/users/me/experiences/{id}", h.updateExperience)
		r.Delete("/users/me/experiences/{id}", h.deleteExperience)
		r.Get("/users/me/educations", h.educations)
		r.Post("/users/me/educations", h.addEducation)
		r.Patch("/users/me/educations/{id}", h.updateEducation)
		r.Delete("/users/me/educations/{id}", h.deleteEducation)
		r.Get("/users/me/skills", h.skills)
		r.Post("/users/me/skills", h.addSkill)
		r.Delete("/users/me/skills/{id}", h.deleteSkill)
	})
}
func owner(h *Handler, r *http.Request) (uuid.UUID, error) {
	actor, _ := auth.Principal(r.Context())
	u, err := h.identity.Me(r.Context(), actor)
	return u.ID, err
}
func decode(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errMediaType
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 16385))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("extra JSON")
	}
	return nil
}
func mapDecodeError(err error) error {
	if errors.Is(err, errMediaType) {
		return err
	}
	return profiles.ErrInvalid
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, errMediaType):
		status, code = http.StatusUnsupportedMediaType, "unsupported_media_type"
	case errors.Is(err, profiles.ErrInvalid):
		status, code = 400, "invalid_input"
	case errors.Is(err, profiles.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(err, profiles.ErrConflict):
		status, code = 409, "conflict"
	}
	write(w, status, map[string]string{"error": code})
}
func id(r *http.Request) (uuid.UUID, error) {
	v, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, profiles.ErrNotFound
	}
	return v, nil
}
func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	p, err := h.service.Profile(r.Context(), u)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, p)
}
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var in profiles.ProfilePatch
	if err := decode(r, &in); err != nil {
		fail(w, mapDecodeError(err))
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	p, err := h.service.UpdateProfile(r.Context(), u, in)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, p)
}
func (h *Handler) experiences(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	x, err := h.service.Experiences(r.Context(), u)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, x)
}
func (h *Handler) addExperience(w http.ResponseWriter, r *http.Request) {
	h.saveExperience(w, r, false)
}
func (h *Handler) updateExperience(w http.ResponseWriter, r *http.Request) {
	h.saveExperience(w, r, true)
}
func (h *Handler) saveExperience(w http.ResponseWriter, r *http.Request, update bool) {
	var e profiles.Experience
	if err := decode(r, &e); err != nil {
		fail(w, mapDecodeError(err))
		return
	}
	if update {
		var err error
		e.ID, err = id(r)
		if err != nil {
			fail(w, err)
			return
		}
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	e, err = h.service.SaveExperience(r.Context(), u, e, update)
	if err != nil {
		fail(w, err)
		return
	}
	if update {
		write(w, 200, e)
	} else {
		write(w, 201, e)
	}
}
func (h *Handler) deleteExperience(w http.ResponseWriter, r *http.Request) {
	item, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if err = h.service.DeleteExperience(r.Context(), u, item); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) educations(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	x, err := h.service.Educations(r.Context(), u)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, x)
}
func (h *Handler) addEducation(w http.ResponseWriter, r *http.Request) { h.saveEducation(w, r, false) }
func (h *Handler) updateEducation(w http.ResponseWriter, r *http.Request) {
	h.saveEducation(w, r, true)
}
func (h *Handler) saveEducation(w http.ResponseWriter, r *http.Request, update bool) {
	var e profiles.Education
	if err := decode(r, &e); err != nil {
		fail(w, mapDecodeError(err))
		return
	}
	if update {
		var err error
		e.ID, err = id(r)
		if err != nil {
			fail(w, err)
			return
		}
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	e, err = h.service.SaveEducation(r.Context(), u, e, update)
	if err != nil {
		fail(w, err)
		return
	}
	if update {
		write(w, 200, e)
	} else {
		write(w, 201, e)
	}
}
func (h *Handler) deleteEducation(w http.ResponseWriter, r *http.Request) {
	item, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if err = h.service.DeleteEducation(r.Context(), u, item); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) searchSkills(w http.ResponseWriter, r *http.Request) {
	n := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		n, err = strconv.Atoi(raw)
		if err != nil {
			fail(w, profiles.ErrInvalid)
			return
		}
	}
	x, err := h.service.SearchSkills(r.Context(), r.URL.Query().Get("query"), n)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, x)
}
func (h *Handler) skills(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	x, err := h.service.Skills(r.Context(), u)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, x)
}
func (h *Handler) addSkill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, mapDecodeError(err))
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	x, err := h.service.AddSkill(r.Context(), u, in.Name)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 201, x)
}
func (h *Handler) deleteSkill(w http.ResponseWriter, r *http.Request) {
	item, err := id(r)
	if err != nil {
		fail(w, err)
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if err = h.service.DeleteSkill(r.Context(), u, item); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
