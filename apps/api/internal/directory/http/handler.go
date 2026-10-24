package directoryhttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

type Handler struct {
	service *directory.Service
	limits  Limits
	limiter *limiter
	slots   chan struct{}
	logger  *slog.Logger
}

func RegisterRoutes(r chi.Router, service *directory.Service) {
	RegisterRoutesWithLimits(r, service, DefaultLimits(), slog.Default())
}
func RegisterRoutesWithLimits(r chi.Router, service *directory.Service, limits Limits, logger *slog.Logger) {
	h := &Handler{service: service, limits: limits, limiter: newLimiter(limits), slots: make(chan struct{}, limits.MaxConcurrent), logger: logger}
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
	start := time.Now()
	status, code := 200, "success"
	defer func() {
		h.logger.Info("directory search", "outcome", code, "status", status, "duration_ms", time.Since(start).Milliseconds(), "in_flight", len(h.slots), "capacity", cap(h.slots), "request_id", middleware.GetReqID(r.Context()))
	}()
	w.Header().Set("Content-Type", "application/json")
	fail := func(s int, c string) {
		status, code = s, c
		w.WriteHeader(s)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": c})
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.limits.RequestTimeout)
	defer cancel()
	if r.Context().Err() != nil {
		if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			fail(503, "directory_timeout")
		} else {
			fail(408, "directory_canceled")
		}
		return
	}
	ip, ok := clientIP(r.RemoteAddr)
	if !ok {
		fail(400, "invalid_client")
		return
	}
	if allowed, full, retry := h.limiter.allow(ip); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		if full {
			fail(503, "directory_overloaded")
		} else {
			fail(429, "directory_rate_limited")
		}
		return
	}
	spec, token, err := parse(r.URL.RawQuery)
	var out directory.Page
	if err == nil {
		select {
		case h.slots <- struct{}{}:
			defer func() { <-h.slots }()
		case <-ctx.Done():
			err = ctx.Err()
		default:
			w.Header().Set("Retry-After", "1")
			fail(503, "directory_overloaded")
			return
		}
		if err == nil {
			out, err = h.service.Search(ctx, spec, token)
		}
	}
	if err != nil {
		status, code = 500, "internal_error"
		if errors.Is(err, directory.ErrInvalid) {
			status, code = 400, "invalid_input"
		}
		if errors.Is(err, directory.ErrDatabaseTimeout) {
			status, code = 503, "directory_database_timeout"
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status, code = 503, "directory_timeout"
		}
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			status, code = 408, "directory_canceled"
		}
		fail(status, code)
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}
