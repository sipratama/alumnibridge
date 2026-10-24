package directory

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

var ErrInvalid = errors.New("invalid directory input")
var ErrDatabaseTimeout = errors.New("directory database timeout")

type Spec struct {
	Query          string   `json:"query"`
	Institution    string   `json:"institution"`
	StudyProgram   string   `json:"study_program"`
	GraduationYear int      `json:"graduation_year"`
	Company        string   `json:"company"`
	JobTitle       string   `json:"job_title"`
	Skills         []string `json:"skills"`
	Limit          int      `json:"limit"`
	After          string   `json:"-"`
}
type Reference struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}
type Affiliation struct {
	Institution    Reference  `json:"institution"`
	StudyProgram   *Reference `json:"study_program,omitempty"`
	GraduationYear *int       `json:"graduation_year,omitempty"`
}

// Entry is an explicit anonymous allowlist. Never add internal identity fields.
type Entry struct {
	Slug            string        `json:"slug"`
	DisplayName     string        `json:"display_name"`
	Headline        string        `json:"headline"`
	VerifiedAlumni  bool          `json:"verified_alumni"`
	CurrentCompany  *string       `json:"current_company,omitempty"`
	CurrentJobTitle *string       `json:"current_job_title,omitempty"`
	Skills          *[]string     `json:"skills,omitempty"`
	Affiliations    []Affiliation `json:"affiliations,omitempty"`
	PhotoURL        *string       `json:"photo_url,omitempty"`
}
type Page struct {
	Items      []Entry `json:"items"`
	NextCursor *string `json:"next_cursor"`
}
type Repository interface {
	Search(context.Context, Spec) ([]Entry, error)
}
type Service struct {
	repo   Repository
	cursor *Cursor
}

func New(repo Repository, cursor *Cursor) *Service { return &Service{repo: repo, cursor: cursor} }
func validText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func ReferenceSlug(s string) bool {
	if len(s) < 1 || len(s) > 64 || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func Normalize(s Spec) (Spec, error) {
	if s.Limit < 1 || s.Limit > 50 || (s.GraduationYear != 0 && (s.GraduationYear < 1950 || s.GraduationYear > 2100)) || len(s.Skills) > 5 {
		return s, ErrInvalid
	}
	for _, field := range []struct {
		p   *string
		max int
	}{{&s.Query, 100}, {&s.Company, 160}, {&s.JobTitle, 160}} {
		if *field.p == "" {
			continue
		}
		if !validText(*field.p) {
			return s, ErrInvalid
		}
		*field.p = strings.ToLower(strings.Join(strings.Fields(*field.p), " "))
		n := utf8.RuneCountInString(*field.p)
		if n < 2 || n > field.max {
			return s, ErrInvalid
		}
	}
	if s.Institution != "" && !ReferenceSlug(s.Institution) || s.StudyProgram != "" && (!ReferenceSlug(s.StudyProgram) || s.Institution == "") {
		return s, ErrInvalid
	}
	normalized := make([]string, 0, len(s.Skills))
	seen := map[string]bool{}
	for _, skill := range s.Skills {
		if !validText(skill) {
			return s, ErrInvalid
		}
		_, name, err := profiles.NormalizeSkill(skill)
		if err != nil || seen[name] {
			return s, ErrInvalid
		}
		normalized = append(normalized, name)
		seen[name] = true
	}
	sort.Strings(normalized)
	s.Skills = normalized
	if s.After != "" && !profiles.ValidSlug(s.After) {
		return s, ErrInvalid
	}
	return s, nil
}
func (s *Service) Search(ctx context.Context, spec Spec, token string) (Page, error) {
	spec, err := Normalize(spec)
	if err != nil {
		return Page{}, err
	}
	if token != "" {
		spec.After, err = s.cursor.Open(token, spec)
		if err != nil {
			return Page{}, ErrInvalid
		}
	}
	items, err := s.repo.Search(ctx, spec)
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: items}
	if out.Items == nil {
		out.Items = []Entry{}
	}
	if len(items) > spec.Limit {
		next, err := s.cursor.Seal(spec, items[spec.Limit-1].Slug)
		if err != nil {
			return Page{}, err
		}
		out.NextCursor = &next
		out.Items = items[:spec.Limit]
	}
	return out, nil
}
