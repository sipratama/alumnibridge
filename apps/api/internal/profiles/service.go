package profiles

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid profile input")
var ErrNotFound = errors.New("profile resource not found")
var ErrConflict = errors.New("profile conflict")
var ErrStale = errors.New("stale resource revision")
var ErrAccountUnavailable = errors.New("account unavailable")

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var reserved = map[string]bool{"api": true, "admin": true, "auth": true, "in": true, "me": true, "settings": true, "dashboard": true, "login": true, "logout": true, "register": true, "onboarding": true, "profile": true, "profiles": true}

type Profile struct {
	DisplayName     string `json:"display_name"`
	Slug            string `json:"slug"`
	Headline        string `json:"headline"`
	Bio             string `json:"bio"`
	Location        string `json:"location"`
	CurrentCompany  string `json:"current_company"`
	CurrentJobTitle string `json:"current_job_title"`
	Completeness    int    `json:"completeness"`
	Revision        int64  `json:"-"`
}
type ProfilePatch struct {
	DisplayName     *string `json:"display_name"`
	Slug            *string `json:"slug"`
	Headline        *string `json:"headline"`
	Bio             *string `json:"bio"`
	Location        *string `json:"location"`
	CurrentCompany  *string `json:"current_company"`
	CurrentJobTitle *string `json:"current_job_title"`
}
type Experience struct {
	Revision       int64     `json:"revision"`
	ID             uuid.UUID `json:"id"`
	Company        string    `json:"company"`
	JobTitle       string    `json:"job_title"`
	EmploymentType string    `json:"employment_type"`
	Location       string    `json:"location"`
	StartDate      string    `json:"start_date"`
	EndDate        *string   `json:"end_date,omitempty"`
	IsCurrent      bool      `json:"is_current"`
	Description    string    `json:"description"`
}
type Education struct {
	Revision     int64     `json:"revision"`
	ID           uuid.UUID `json:"id"`
	Institution  string    `json:"institution"`
	Degree       string    `json:"degree"`
	FieldOfStudy string    `json:"field_of_study"`
	StartDate    *string   `json:"start_date,omitempty"`
	EndDate      *string   `json:"end_date,omitempty"`
	Description  string    `json:"description"`
}
type Skill struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Store interface {
	Profile(context.Context, uuid.UUID) (Profile, error)
	UpdateProfile(context.Context, uuid.UUID, func(Profile) (Profile, error)) (Profile, error)
	Counts(context.Context, uuid.UUID) ([3]int, error)
	Experiences(context.Context, uuid.UUID) ([]Experience, error)
	SaveExperience(context.Context, uuid.UUID, Experience, bool) (Experience, error)
	DeleteExperience(context.Context, uuid.UUID, uuid.UUID) error
	Educations(context.Context, uuid.UUID) ([]Education, error)
	SaveEducation(context.Context, uuid.UUID, Education, bool) (Education, error)
	DeleteEducation(context.Context, uuid.UUID, uuid.UUID) error
	SearchSkills(context.Context, string, int) ([]Skill, error)
	Skills(context.Context, uuid.UUID) ([]Skill, error)
	AddSkill(context.Context, uuid.UUID, string, string) (Skill, error)
	DeleteSkill(context.Context, uuid.UUID, uuid.UUID) error
}

// RevisionStore implements atomic conditional writes across all editable owner resources.
type RevisionStore interface {
	UpdateProfileRevision(context.Context, uuid.UUID, int64, func(Profile) (Profile, error)) (Profile, error)
	SaveExperienceRevision(context.Context, uuid.UUID, Experience, int64) (Experience, error)
	SaveEducationRevision(context.Context, uuid.UUID, Education, int64) (Education, error)
	DeleteExperienceRevision(context.Context, uuid.UUID, uuid.UUID, int64) error
	DeleteEducationRevision(context.Context, uuid.UUID, uuid.UUID, int64) error
}
type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }
func trim(s string) string     { return strings.Join(strings.Fields(s), " ") }
func valid(s string, min, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) >= min && utf8.RuneCountInString(s) <= max
}
func NormalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
func ValidSlug(s string) bool {
	return len(s) >= 3 && len(s) <= 64 && slugPattern.MatchString(s) && !reserved[s]
}
func (s *Service) Profile(ctx context.Context, id uuid.UUID) (Profile, error) {
	p, err := s.store.Profile(ctx, id)
	if err != nil {
		return p, err
	}
	c, err := s.store.Counts(ctx, id)
	if err != nil {
		return p, err
	}
	p.Completeness = Completeness(p, c)
	return p, nil
}
func Completeness(p Profile, c [3]int) int {
	n := 0
	for _, v := range []string{p.DisplayName, p.Headline, p.Bio, p.Location, p.CurrentCompany, p.CurrentJobTitle} {
		if v != "" {
			n++
		}
	}
	for _, v := range c {
		if v > 0 {
			n++
		}
	}
	return n * 100 / 9
}
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, in ProfilePatch) (Profile, error) {
	if _, err := s.store.Profile(ctx, id); err != nil {
		return Profile{}, err
	}
	_, err := s.store.UpdateProfile(ctx, id, func(p Profile) (Profile, error) { return ApplyProfilePatch(p, in) })
	if err != nil {
		return Profile{}, err
	}
	return s.Profile(ctx, id)
}

func (s *Service) UpdateProfileRevision(ctx context.Context, id uuid.UUID, in ProfilePatch, revision int64) (Profile, error) {
	if revision < 1 {
		return Profile{}, ErrInvalid
	}
	store, ok := s.store.(RevisionStore)
	if !ok {
		return Profile{}, ErrInvalid
	}
	if _, err := store.UpdateProfileRevision(ctx, id, revision, func(p Profile) (Profile, error) { return ApplyProfilePatch(p, in) }); err != nil {
		return Profile{}, err
	}
	return s.Profile(ctx, id)
}
func (s *Service) SaveExperienceRevision(ctx context.Context, id uuid.UUID, e Experience, revision int64) (Experience, error) {
	if revision < 1 {
		return e, ErrInvalid
	}
	e, err := NormalizeExperience(e)
	if err != nil {
		return e, err
	}
	st, ok := s.store.(RevisionStore)
	if !ok {
		return e, ErrInvalid
	}
	return st.SaveExperienceRevision(ctx, id, e, revision)
}
func (s *Service) SaveEducationRevision(ctx context.Context, id uuid.UUID, e Education, revision int64) (Education, error) {
	if revision < 1 {
		return e, ErrInvalid
	}
	e, err := NormalizeEducation(e)
	if err != nil {
		return e, err
	}
	st, ok := s.store.(RevisionStore)
	if !ok {
		return e, ErrInvalid
	}
	return st.SaveEducationRevision(ctx, id, e, revision)
}
func (s *Service) DeleteExperienceRevision(ctx context.Context, id, item uuid.UUID, revision int64) error {
	if revision < 1 {
		return ErrInvalid
	}
	st, ok := s.store.(RevisionStore)
	if !ok {
		return ErrInvalid
	}
	return st.DeleteExperienceRevision(ctx, id, item, revision)
}
func (s *Service) DeleteEducationRevision(ctx context.Context, id, item uuid.UUID, revision int64) error {
	if revision < 1 {
		return ErrInvalid
	}
	st, ok := s.store.(RevisionStore)
	if !ok {
		return ErrInvalid
	}
	return st.DeleteEducationRevision(ctx, id, item, revision)
}

// ApplyProfilePatch runs against the locked current profile to avoid lost updates.
func ApplyProfilePatch(p Profile, in ProfilePatch) (Profile, error) {
	if in.DisplayName != nil {
		p.DisplayName = trim(*in.DisplayName)
	}
	if in.Slug != nil {
		p.Slug = NormalizeSlug(*in.Slug)
	}
	if in.Headline != nil {
		p.Headline = trim(*in.Headline)
	}
	if in.Bio != nil {
		p.Bio = strings.TrimSpace(*in.Bio)
	}
	if in.Location != nil {
		p.Location = trim(*in.Location)
	}
	if in.CurrentCompany != nil {
		p.CurrentCompany = trim(*in.CurrentCompany)
	}
	if in.CurrentJobTitle != nil {
		p.CurrentJobTitle = trim(*in.CurrentJobTitle)
	}
	if !valid(p.DisplayName, 2, 100) || !ValidSlug(p.Slug) || !valid(p.Headline, 0, 160) || !valid(p.Bio, 0, 5000) || !valid(p.Location, 0, 120) || !valid(p.CurrentCompany, 0, 160) || !valid(p.CurrentJobTitle, 0, 160) {
		return Profile{}, ErrInvalid
	}
	return p, nil
}

func date(v string) (time.Time, error) { return time.Parse("2006-01-02", v) }
func validDates(start *string, end *string, required bool, current bool) bool {
	if required && start == nil {
		return false
	}
	var a, b time.Time
	var err error
	if start != nil {
		a, err = date(*start)
		if err != nil {
			return false
		}
	}
	if end != nil {
		b, err = date(*end)
		if err != nil {
			return false
		}
	}
	return !(current && end != nil) && !(start != nil && end != nil && b.Before(a))
}
func NormalizeExperience(e Experience) (Experience, error) {
	e.Company = trim(e.Company)
	e.JobTitle = trim(e.JobTitle)
	e.EmploymentType = trim(e.EmploymentType)
	e.Location = trim(e.Location)
	e.Description = strings.TrimSpace(e.Description)
	if !valid(e.Company, 1, 160) || !valid(e.JobTitle, 1, 160) || !valid(e.EmploymentType, 0, 80) || !valid(e.Location, 0, 120) || !valid(e.Description, 0, 5000) || !validDates(&e.StartDate, e.EndDate, true, e.IsCurrent) {
		return e, ErrInvalid
	}
	return e, nil
}
func NormalizeEducation(e Education) (Education, error) {
	e.Institution = trim(e.Institution)
	e.Degree = trim(e.Degree)
	e.FieldOfStudy = trim(e.FieldOfStudy)
	e.Description = strings.TrimSpace(e.Description)
	if !valid(e.Institution, 1, 200) || !valid(e.Degree, 0, 160) || !valid(e.FieldOfStudy, 0, 160) || !valid(e.Description, 0, 5000) || !validDates(e.StartDate, e.EndDate, false, false) {
		return e, ErrInvalid
	}
	return e, nil
}
func NormalizeSkill(name string) (string, string, error) {
	for _, r := range name {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return "", "", ErrInvalid
		}
	}
	name = trim(name)
	if !valid(name, 1, 80) {
		return "", "", ErrInvalid
	}
	normalized := strings.ToLower(name)
	if normalized == "" || !valid(normalized, 1, 80) {
		return "", "", ErrInvalid
	}
	return name, normalized, nil
}
func (s *Service) Experiences(ctx context.Context, id uuid.UUID) ([]Experience, error) {
	return s.store.Experiences(ctx, id)
}
func (s *Service) SaveExperience(ctx context.Context, id uuid.UUID, e Experience, update bool) (Experience, error) {
	e, err := NormalizeExperience(e)
	if err != nil {
		return e, err
	}
	return s.store.SaveExperience(ctx, id, e, update)
}
func (s *Service) DeleteExperience(ctx context.Context, id, item uuid.UUID) error {
	return s.store.DeleteExperience(ctx, id, item)
}
func (s *Service) Educations(ctx context.Context, id uuid.UUID) ([]Education, error) {
	return s.store.Educations(ctx, id)
}
func (s *Service) SaveEducation(ctx context.Context, id uuid.UUID, e Education, update bool) (Education, error) {
	e, err := NormalizeEducation(e)
	if err != nil {
		return e, err
	}
	return s.store.SaveEducation(ctx, id, e, update)
}
func (s *Service) DeleteEducation(ctx context.Context, id, item uuid.UUID) error {
	return s.store.DeleteEducation(ctx, id, item)
}
func (s *Service) SearchSkills(ctx context.Context, q string, limit int) ([]Skill, error) {
	q = trim(q)
	if !valid(q, 1, 80) || limit < 1 || limit > 50 {
		return nil, ErrInvalid
	}
	return s.store.SearchSkills(ctx, q, limit)
}
func (s *Service) Skills(ctx context.Context, id uuid.UUID) ([]Skill, error) {
	return s.store.Skills(ctx, id)
}
func (s *Service) AddSkill(ctx context.Context, id uuid.UUID, name string) (Skill, error) {
	name, key, err := NormalizeSkill(name)
	if err != nil {
		return Skill{}, err
	}
	return s.store.AddSkill(ctx, id, name, key)
}
func (s *Service) DeleteSkill(ctx context.Context, id, item uuid.UUID) error {
	return s.store.DeleteSkill(ctx, id, item)
}
