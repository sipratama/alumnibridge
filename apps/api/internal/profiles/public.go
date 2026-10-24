package profiles

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// PublicProfile is the allowlist for anonymous responses. It contains no internal IDs.
type PublicProfile struct {
	Slug            string              `json:"slug"`
	DisplayName     string              `json:"display_name"`
	Headline        string              `json:"headline"`
	VerifiedAlumni  bool                `json:"verified_alumni"`
	Bio             *string             `json:"bio,omitempty"`
	Location        *string             `json:"location,omitempty"`
	CurrentCompany  *string             `json:"current_company,omitempty"`
	CurrentJobTitle *string             `json:"current_job_title,omitempty"`
	Experiences     *[]PublicExperience `json:"experiences,omitempty"`
	Educations      *[]PublicEducation  `json:"educations,omitempty"`
	Skills          *[]string           `json:"skills,omitempty"`
	PhotoURL        *string             `json:"photo_url,omitempty"`
}
type PublicExperience struct {
	Company        string  `json:"company"`
	JobTitle       string  `json:"job_title"`
	EmploymentType string  `json:"employment_type"`
	Location       string  `json:"location"`
	StartDate      string  `json:"start_date"`
	EndDate        *string `json:"end_date,omitempty"`
	IsCurrent      bool    `json:"is_current"`
	Description    string  `json:"description"`
}
type PublicEducation struct {
	Institution  string  `json:"institution"`
	Degree       string  `json:"degree"`
	FieldOfStudy string  `json:"field_of_study"`
	StartDate    *string `json:"start_date,omitempty"`
	EndDate      *string `json:"end_date,omitempty"`
	Description  string  `json:"description"`
}
type Privacy struct {
	Visibility          string `json:"visibility"`
	ShowBio             bool   `json:"show_bio"`
	ShowLocation        bool   `json:"show_location"`
	ShowCurrentCompany  bool   `json:"show_current_company"`
	ShowCurrentJobTitle bool   `json:"show_current_job_title"`
	ShowExperiences     bool   `json:"show_experiences"`
	ShowEducations      bool   `json:"show_educations"`
	ShowSkills          bool   `json:"show_skills"`
	ShowPhoto           bool   `json:"show_photo"`
	Revision            int64  `json:"-"` // owner ETag only; never in JSON
}

// Visibility is mandatory even for a partial update of optional publication flags.
type PrivacyPatch struct {
	Visibility          string `json:"visibility"`
	ShowBio             *bool  `json:"show_bio"`
	ShowLocation        *bool  `json:"show_location"`
	ShowCurrentCompany  *bool  `json:"show_current_company"`
	ShowCurrentJobTitle *bool  `json:"show_current_job_title"`
	ShowExperiences     *bool  `json:"show_experiences"`
	ShowEducations      *bool  `json:"show_educations"`
	ShowSkills          *bool  `json:"show_skills"`
	ShowPhoto           *bool  `json:"show_photo"`
}

// ConditionalPrivacyStore is the opt-in atomic write contract while legacy
// clients migrate to mandatory If-Match. The HTTP route uses this only when
// an expected revision is supplied; old clients remain a production gate.
type ConditionalPrivacyStore interface {
	UpdatePrivacyConditional(context.Context, uuid.UUID, PrivacyPatch, int64) (Privacy, error)
}
type Photo struct {
	Key         string
	ContentType string
	Size        int64
}
type Publication struct {
	Profile    PublicProfile
	Visibility string
	Moderation string
	Photo      *Photo
}
type PublicationStore interface {
	Public(context.Context, string) (Publication, error)
	Privacy(context.Context, uuid.UUID) (Privacy, error)
	UpdatePrivacy(context.Context, uuid.UUID, PrivacyPatch) (Privacy, error)
	OwnerPhoto(context.Context, uuid.UUID) (Photo, error)
	StagePhoto(context.Context, uuid.UUID, Photo) error
	ActivatePhoto(context.Context, uuid.UUID, string) error
	RemovePhoto(context.Context, uuid.UUID) error
	CleanupPhotos(context.Context, func(context.Context, string) error) error
}

// Photo pointer writes use the profile-content validator. The object registry
// is separate so a rejected activation can be collected without touching the
// currently published pointer.
type ConditionalPhotoStore interface {
	ActivatePhotoConditional(context.Context, uuid.UUID, string, int64) (int64, error)
	RemovePhotoConditional(context.Context, uuid.UUID, int64) (int64, error)
	DiscardStagedPhoto(context.Context, uuid.UUID, string) error
}
type PhotoStorage interface {
	Put(context.Context, string, []byte, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type PublishingService struct {
	store      PublicationStore
	storage    PhotoStorage
	photoSlots chan struct{}
}

func NewPublishing(store PublicationStore, storage PhotoStorage) *PublishingService {
	return &PublishingService{store: store, storage: storage, photoSlots: make(chan struct{}, 4)}
}
func (s *PublishingService) Public(ctx context.Context, slug string) (Publication, error) {
	if !ValidSlug(slug) {
		return Publication{}, ErrNotFound
	}
	p, err := s.store.Public(ctx, slug)
	if err != nil {
		return Publication{}, err
	}
	if !PubliclyVisible(p.Visibility, p.Moderation) {
		return Publication{}, ErrNotFound
	}
	return p, nil
}
func (s *PublishingService) Privacy(ctx context.Context, id uuid.UUID) (Privacy, error) {
	return s.store.Privacy(ctx, id)
}
func (s *PublishingService) UpdatePrivacyConditional(ctx context.Context, id uuid.UUID, p PrivacyPatch, expected int64) (Privacy, error) {
	if p.Visibility != "public" && p.Visibility != "private" || expected < 1 {
		return Privacy{}, ErrInvalid
	}
	conditional, ok := s.store.(ConditionalPrivacyStore)
	if !ok {
		return Privacy{}, ErrInvalid
	}
	return conditional.UpdatePrivacyConditional(ctx, id, p, expected)
}
func (s *PublishingService) UpdatePrivacy(ctx context.Context, id uuid.UUID, p PrivacyPatch) (Privacy, error) {
	if p.Visibility != "public" && p.Visibility != "private" {
		return Privacy{}, ErrInvalid
	}
	return s.store.UpdatePrivacy(ctx, id, p)
}
