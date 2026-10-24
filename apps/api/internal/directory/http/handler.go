package directoryhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

type Handler struct{ service *directory.Service }

func RegisterRoutes(r chi.Router, service *directory.Service) {
	h := &Handler{service: service}
	r.Route("/directory", func(r chi.Router) {
		r.Use(noStore)
		r.Get("/", h.search)
	})
}
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
func parse(raw string) (directory.Spec, string, error) {
	s := directory.Spec{Limit: 20}
	if len(raw) > 4096 {
		return s, "", directory.ErrInvalid
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return s, "", directory.ErrInvalid
	}
	for key, values := range q {
		switch key {
		case "query", "institution", "study_program", "graduation_year", "company", "job_title", "skills", "limit", "cursor":
		default:
			return s, "", directory.ErrInvalid
		}
		if key != "skills" && len(values) != 1 {
			return s, "", directory.ErrInvalid
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return s, "", directory.ErrInvalid
			}
		}
	}
	s.Query = q.Get("query")
	s.Institution = q.Get("institution")
	s.StudyProgram = q.Get("study_program")
	s.Company = q.Get("company")
	s.JobTitle = q.Get("job_title")
	s.Skills = q["skills"]
	if value, ok := q["limit"]; ok {
		s.Limit, err = strconv.Atoi(value[0])
		if err != nil {
			return s, "", directory.ErrInvalid
		}
	}
	if value, ok := q["graduation_year"]; ok {
		s.GraduationYear, err = strconv.Atoi(value[0])
		if err != nil || s.GraduationYear == 0 {
			return s, "", directory.ErrInvalid
		}
	}
	s, err = directory.Normalize(s)
	return s, q.Get("cursor"), err
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	spec, token, err := parse(r.URL.RawQuery)
	var out directory.Page
	if err == nil {
		out, err = h.service.Search(r.Context(), spec, token)
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		status, code := 500, "internal_error"
		if errors.Is(err, directory.ErrInvalid) {
			status, code = 400, "invalid_input"
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status, code = 503, "directory_unavailable"
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}
