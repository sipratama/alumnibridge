package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	user                    User
	provisions, onboardings int
}

func (s *memoryStore) Provision(_ context.Context, _, _ string) (User, error) {
	s.provisions++
	return s.user, nil
}
func (s *memoryStore) Onboard(_ context.Context, _ uuid.UUID, _ OnboardingInput) (User, error) {
	s.onboardings++
	return s.user, nil
}

func TestOnboardingValidation(t *testing.T) {
	store := &memoryStore{user: User{ID: uuid.New()}}
	service := NewService(store)
	service.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	base := OnboardingInput{Role: "student", DisplayName: "Ada Lovelace", Institution: "uhamka", StudyProgram: "mathematics-education"}
	for _, mutate := range []func(*OnboardingInput){
		func(v *OnboardingInput) { v.Role = "admin" },
		func(v *OnboardingInput) { v.DisplayName = " " },
		func(v *OnboardingInput) { v.Institution = "../uhamka" },
		func(v *OnboardingInput) { n := 2020; v.GraduationYear = &n },
		func(v *OnboardingInput) { v.Role = "alumni" },
		func(v *OnboardingInput) { v.Role = "alumni"; n := 1949; v.GraduationYear = &n },
	} {
		input := base
		mutate(&input)
		if _, err := service.Onboard(context.Background(), Principal{Subject: "s"}, input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	}
	if store.provisions != 0 || store.onboardings != 0 {
		t.Fatal("invalid input touched store")
	}
	if _, err := service.Onboard(context.Background(), Principal{Subject: "s"}, base); err != nil {
		t.Fatal(err)
	}
	alumni := base
	alumni.Role = "alumni"
	year := 2020
	alumni.GraduationYear = &year
	if _, err := service.Onboard(context.Background(), Principal{Subject: "s"}, alumni); err != nil {
		t.Fatal(err)
	}
	if store.onboardings != 2 {
		t.Fatalf("got %d valid onboardings", store.onboardings)
	}
}
