package profiles

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const MaxPhotoBytes = 5 * 1024 * 1024

var ErrPhotoTooLarge = errors.New("photo exceeds limit")
var ErrPhotoFormat = errors.New("unsupported photo format")
var ErrStorage = errors.New("photo storage unavailable")

// NormalizePhoto decodes the entire image and re-encodes pixels, discarding metadata and trailing payloads.
func NormalizePhoto(data []byte, declared string) ([]byte, string, error) {
	if len(data) > MaxPhotoBytes {
		return nil, "", ErrPhotoTooLarge
	}
	expected := map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/webp": "webp"}[declared]
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
	var out bytes.Buffer
	kind := "image/png"
	if format == "jpeg" {
		kind = "image/jpeg"
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, img)
	}
	if err != nil {
		return nil, "", ErrInvalid
	}
	if out.Len() > MaxPhotoBytes {
		return nil, "", ErrPhotoTooLarge
	}
	return out.Bytes(), kind, nil
}
func (s *PublishingService) UploadPhoto(ctx context.Context, id uuid.UUID, data []byte, declared string) error {
	select {
	case s.photoSlots <- struct{}{}:
		defer func() { <-s.photoSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	body, kind, err := NormalizePhoto(data, declared)
	if err != nil {
		return err
	}
	p := Photo{Key: "photos/" + uuid.NewString(), ContentType: kind, Size: int64(len(body))}
	// Registration commits before Put: failed uploads and failed activations remain discoverable by cleanup.
	if err = s.store.StagePhoto(ctx, id, p); err != nil {
		return err
	}
	if err = s.storage.Put(ctx, p.Key, body, p.ContentType); err != nil {
		return ErrStorage
	}
	return s.store.ActivatePhoto(ctx, id, p.Key)
}
func (s *PublishingService) RemovePhoto(ctx context.Context, id uuid.UUID) error {
	return s.store.RemovePhoto(ctx, id)
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
