package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput       = errors.New("invalid onboarding input")
	ErrProgramNotFound    = errors.New("study program not found")
	ErrMembershipConflict = errors.New("membership already exists with different details")
)

type Principal struct {
	Subject string
	Email   string
	Roles   map[string]bool
}

type Membership struct {
	ID             uuid.UUID `json:"id"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	Institution    string    `json:"institution"`
	StudyProgram   string    `json:"study_program"`
	GraduationYear *int      `json:"graduation_year,omitempty"`
}

type User struct {
	ID          uuid.UUID    `json:"id"`
	Email       string       `json:"email,omitempty"`
	DisplayName string       `json:"display_name,omitempty"`
	Memberships []Membership `json:"memberships"`
}

type OnboardingInput struct {
	Role           string `json:"role"`
	DisplayName    string `json:"display_name"`
	Institution    string `json:"institution"`
	StudyProgram   string `json:"study_program"`
	GraduationYear *int   `json:"graduation_year,omitempty"`
}

type Store interface {
	Provision(context.Context, string, string) (User, error)
	Onboard(context.Context, uuid.UUID, OnboardingInput) (User, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) Me(ctx context.Context, actor Principal) (User, error) {
	if actor.Subject == "" {
		return User{}, ErrInvalidInput
	}
	return s.store.Provision(ctx, actor.Subject, actor.Email)
}

func (s *Service) Onboard(ctx context.Context, actor Principal, input OnboardingInput) (User, error) {
	if err := validate(input, s.now().UTC()); err != nil {
		return User{}, err
	}
	user, err := s.Me(ctx, actor)
	if err != nil {
		return User{}, err
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	return s.store.Onboard(ctx, user.ID, input)
}

func validate(in OnboardingInput, now time.Time) error {
	name := strings.TrimSpace(in.DisplayName)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 100 {
		return ErrInvalidInput
	}
	if in.Role != "student" && in.Role != "alumni" {
		return ErrInvalidInput
	}
	if !validSlug(in.Institution) || !validSlug(in.StudyProgram) {
		return ErrInvalidInput
	}
	if in.Role == "student" && in.GraduationYear != nil {
		return ErrInvalidInput
	}
	if in.Role == "alumni" && (in.GraduationYear == nil || *in.GraduationYear < 1950 || *in.GraduationYear > now.Year()+1) {
		return ErrInvalidInput
	}
	return nil
}

func validSlug(s string) bool {
	if len(s) < 2 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}
