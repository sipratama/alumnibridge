package profileshttp

import (
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

func RegisterPublishingRoutes(r chi.Router, s *profiles.Service, p *profiles.PublishingService, ids *identity.Service, v auth.Verifier) {
	h := &Handler{service: s, publication: p, identity: ids}
	r.Group(func(r chi.Router) {
		r.Use(noStore)
		r.Get("/profiles/{slug}", h.publicProfile)
		r.Get("/profiles/{slug}/photo", h.publicPhoto)
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(v))
		r.Get("/users/me/profile/privacy", h.privacy)
		r.Patch("/users/me/profile/privacy", h.updatePrivacy)
		r.Get("/users/me/profile/photo", h.ownerPhoto)
		r.Post("/users/me/profile/photo", h.uploadPhoto)
		r.Delete("/users/me/profile/photo", h.removePhoto)
	})
}
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
func (h *Handler) publicProfile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, err := h.publication.Public(r.Context(), slug)
	if err != nil {
		fail(w, err)
		return
	}
	if slug != p.Profile.Slug {
		w.Header().Set("Location", "/api/v1/profiles/"+p.Profile.Slug)
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}
	write(w, 200, p.Profile)
}
func (h *Handler) privacy(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err = h.service.Profile(r.Context(), u); err != nil {
		fail(w, err)
		return
	}
	p, err := h.publication.Privacy(r.Context(), u)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, p)
}
func (h *Handler) updatePrivacy(w http.ResponseWriter, r *http.Request) {
	var p profiles.PrivacyPatch
	if err := decode(r, &p); err != nil {
		fail(w, mapDecodeError(err))
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err = h.service.Profile(r.Context(), u); err != nil {
		fail(w, err)
		return
	}
	out, err := h.publication.UpdatePrivacy(r.Context(), u, p)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, out)
}
func (h *Handler) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		fail(w, profiles.ErrPhotoFormat)
		return
	}
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err = h.service.Profile(r.Context(), u); err != nil {
		fail(w, err)
		return
	}
	if err = h.publication.UploadPhoto(r.Context(), u, r.Body, kind); err != nil {
		fail(w, err)
		return
	}
	write(w, 201, map[string]string{"photo_url": "/api/v1/users/me/profile/photo"})
}
func (h *Handler) removePhoto(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	if err = h.publication.RemovePhoto(r.Context(), u); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func photoResponse(w http.ResponseWriter, p profiles.Photo, body io.ReadCloser) {
	defer body.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", p.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(p.Size, 10))
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(200)
	_, _ = io.Copy(w, io.LimitReader(body, p.Size))
}
func (h *Handler) ownerPhoto(w http.ResponseWriter, r *http.Request) {
	u, err := owner(h, r)
	if err != nil {
		fail(w, err)
		return
	}
	p, body, err := h.publication.OwnerPhoto(r.Context(), u)
	if err != nil {
		fail(w, err)
		return
	}
	photoResponse(w, p, body)
}
func (h *Handler) publicPhoto(w http.ResponseWriter, r *http.Request) {
	p, body, err := h.publication.PublicPhoto(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		fail(w, err)
		return
	}
	photoResponse(w, p, body)
}
