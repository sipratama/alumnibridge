package profiles

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"time"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const MaxPhotoBytes = 5 * 1024 * 1024

var ErrPhotoTooLarge = errors.New("photo exceeds limit")
var ErrPhotoFormat = errors.New("unsupported photo format")
var ErrStorage = errors.New("photo storage unavailable")

type photoBuffer struct{ buffer bytes.Buffer }

func (b *photoBuffer) Len() int      { return b.buffer.Len() }
func (b *photoBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *photoBuffer) Write(p []byte) (int, error) {
	if len(p) > MaxPhotoBytes-b.Len() {
		return 0, ErrPhotoTooLarge
	}
	return b.buffer.Write(p)
}
func photoFormat(declared string) string {
	return map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/webp": "webp"}[declared]
}

// NormalizePhoto decodes the entire image and re-encodes pixels, discarding metadata and trailing payloads.
func NormalizePhoto(data []byte, declared string) ([]byte, string, error) {
	if len(data) > MaxPhotoBytes {
		return nil, "", ErrPhotoTooLarge
	}
	expected := photoFormat(declared)
	if expected == "" {
		return nil, "", ErrPhotoFormat
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expected {
		return nil, "", ErrInvalid
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 8_000_000 {
		return nil, "", ErrInvalid
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != expected {
		return nil, "", ErrInvalid
	}
	var out photoBuffer
	kind := "image/png"
	if format == "jpeg" {
		kind = "image/jpeg"
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, img)
	}
	if errors.Is(err, ErrPhotoTooLarge) {
		return nil, "", ErrPhotoTooLarge
	}
	if err != nil {
		return nil, "", ErrInvalid
	}
	if out.Len() > MaxPhotoBytes {
		return nil, "", ErrPhotoTooLarge
	}
	return out.Bytes(), kind, nil
}
func (s *PublishingService) UploadPhoto(ctx context.Context, id uuid.UUID, input io.Reader, declared string) error {
	_, err := s.uploadPhoto(ctx, id, input, declared, 0)
	return err
}
func (s *PublishingService) UploadPhotoConditional(ctx context.Context, id uuid.UUID, input io.Reader, declared string, expected int64) (int64, error) {
	if expected < 1 {
		return 0, ErrInvalid
	}
	if _, ok := s.store.(ConditionalPhotoStore); !ok {
		return 0, ErrInvalid
	}
	return s.uploadPhoto(ctx, id, input, declared, expected)
}
func (s *PublishingService) uploadPhoto(ctx context.Context, id uuid.UUID, input io.Reader, declared string, expected int64) (int64, error) {
	if photoFormat(declared) == "" {
		return 0, ErrPhotoFormat
	}
	select {
	case s.photoSlots <- struct{}{}:
		defer func() { <-s.photoSlots }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxPhotoBytes+1))
	if err != nil {
		return 0, ErrInvalid
	}
	if len(data) > MaxPhotoBytes {
		return 0, ErrPhotoTooLarge
	}
	body, kind, err := NormalizePhoto(data, declared)
	if err != nil {
		return 0, err
	}
	p := Photo{Key: "photos/" + uuid.NewString(), ContentType: kind, Size: int64(len(body))}
	// Registration commits before Put: failed uploads and failed activations remain discoverable by cleanup.
	if err = s.store.StagePhoto(ctx, id, p); err != nil {
		return 0, err
	}
	if err = s.storage.Put(ctx, p.Key, body, p.ContentType); err != nil {
		s.discardStagedPhoto(ctx, id, p.Key)
		return 0, ErrStorage
	}
	if expected == 0 {
		err = s.store.ActivatePhoto(ctx, id, p.Key)
		if err != nil {
			s.discardStagedPhoto(ctx, id, p.Key)
		}
		return 0, err
	}
	revision, err := s.store.(ConditionalPhotoStore).ActivatePhotoConditional(ctx, id, p.Key, expected)
	if err != nil {
		s.discardStagedPhoto(ctx, id, p.Key)
	}
	return revision, err
}
func (s *PublishingService) discardStagedPhoto(ctx context.Context, id uuid.UUID, key string) {
	conditional, ok := s.store.(ConditionalPhotoStore)
	if !ok {
		return
	}
	// A disconnected request must still persist cleanup intent. The existing
	// expiration and retry worker cover a transient database/storage outage.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = conditional.DiscardStagedPhoto(cleanup, id, key)
}
func (s *PublishingService) RemovePhoto(ctx context.Context, id uuid.UUID) error {
	return s.store.RemovePhoto(ctx, id)
}
func (s *PublishingService) RemovePhotoConditional(ctx context.Context, id uuid.UUID, expected int64) (int64, error) {
	if expected < 1 {
		return 0, ErrInvalid
	}
	conditional, ok := s.store.(ConditionalPhotoStore)
	if !ok {
		return 0, ErrInvalid
	}
	return conditional.RemovePhotoConditional(ctx, id, expected)
}
func (s *PublishingService) OwnerPhoto(ctx context.Context, id uuid.UUID) (Photo, io.ReadCloser, error) {
	p, err := s.store.OwnerPhoto(ctx, id)
	if err != nil {
		return p, nil, err
	}
	body, err := s.storage.Get(ctx, p.Key)
	if err != nil {
		return Photo{}, nil, ErrStorage
	}
	return p, body, nil
}
func (s *PublishingService) PublicPhoto(ctx context.Context, slug string) (Photo, io.ReadCloser, error) {
	p, err := s.Public(ctx, slug)
	if err != nil {
		return Photo{}, nil, err
	}
	if p.Photo == nil {
		return Photo{}, nil, ErrNotFound
	}
	body, err := s.storage.Get(ctx, p.Photo.Key)
	if err != nil {
		return Photo{}, nil, ErrStorage
	}
	return *p.Photo, body, nil
}
func (s *PublishingService) Cleanup(ctx context.Context) error {
	return s.store.CleanupPhotos(ctx, s.storage.Delete)
}
