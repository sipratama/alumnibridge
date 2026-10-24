package profileshttp

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

func RegisterDiscoveryRoutes(r chi.Router, service *profiles.Service, discovery *profiles.DiscoveryService, ids *identity.Service, verifier auth.Verifier) {
	h := &Handler{service: service, discovery: discovery, identity: ids}
	r.Route("/users/me/profile/discovery", func(r chi.Router) {
		r.Use(noStore, auth.Middleware(verifier))
		r.Get("/", h.getDiscovery)
		r.Patch("/", h.updateDiscovery)
	})
}
func (h *Handler) getDiscovery(w http.ResponseWriter, r *http.Request) {
	ownerID, err := owner(h, r)
	if err == nil {
		_, err = h.service.Profile(r.Context(), ownerID)
	}
	if err != nil {
		fail(w, err)
		return
	}
	out, err := h.discovery.Discovery(r.Context(), ownerID)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag",resourceETag("privacy",out.Revision))
 write(w, 200, out)
}
func (h *Handler) updateDiscovery(w http.ResponseWriter, r *http.Request) {
	patch, err := decodeDiscovery(r)
	if err != nil {
		fail(w, mapDecodeError(err))
		return
	}
	ownerID, err := owner(h, r)
	if err == nil {
		_, err = h.service.Profile(r.Context(), ownerID)
	}
	if err != nil {
		fail(w, err)
		return
	}
	expected, conditionalErr := requireRevision(r, "privacy")
 if conditionalErr != nil {fail(w, conditionalErr);return}
 out, err := h.discovery.UpdateConditional(r.Context(),ownerID,patch,expected)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag",resourceETag("privacy",out.Revision))
 write(w, 200, out)
}
func decodeDiscovery(r *http.Request) (profiles.DiscoveryPatch, error) {
	var out profiles.DiscoveryPatch
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return out, errMediaType
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	if err != nil {
		return out, err
	}
	if len(body) > 16384 {
		return out, errBodyLarge
	}
	// Reject nulls, duplicate object keys, non-object roots and trailing JSON.
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return out, profiles.ErrInvalid
	}
	tokens := json.NewDecoder(bytes.NewReader(body))
	if err = uniqueJSON(tokens); err != nil {
		return out, profiles.ErrInvalid
	}
	if _, err = tokens.Token(); err != io.EOF {
		return out, profiles.ErrInvalid
	}
	var in struct {
		DirectoryDiscoverable *bool `json:"directory_discoverable"`
		Affiliations          *[]struct {
			MembershipID       uuid.UUID `json:"membership_id"`
			ShowInstitution    *bool     `json:"show_institution"`
			ShowStudyProgram   *bool     `json:"show_study_program"`
			ShowGraduationYear *bool     `json:"show_graduation_year"`
		} `json:"affiliations"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&in); err != nil {
		return out, profiles.ErrInvalid
	}
	out.DirectoryDiscoverable = in.DirectoryDiscoverable
	if in.Affiliations != nil {
		affiliations := make([]profiles.AcademicConsent, 0, len(*in.Affiliations))
		for _, a := range *in.Affiliations {
			if a.ShowInstitution == nil || a.ShowStudyProgram == nil || a.ShowGraduationYear == nil {
				return out, profiles.ErrInvalid
			}
			affiliations = append(affiliations, profiles.AcademicConsent{MembershipID: a.MembershipID, ShowInstitution: *a.ShowInstitution, ShowStudyProgram: *a.ShowStudyProgram, ShowGraduationYear: *a.ShowGraduationYear})
		}
		out.Affiliations = &affiliations
	}
	return out, out.Validate()
}
func uniqueJSON(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return profiles.ErrInvalid
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return profiles.ErrInvalid
			}
			seen[name] = true
			if err = uniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return profiles.ErrInvalid
	}
	_, err = d.Token()
	return err
}
