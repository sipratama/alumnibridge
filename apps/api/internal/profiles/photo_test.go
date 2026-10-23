package profiles

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func testImage(t *testing.T, kind string) []byte {
	t.Helper()
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var err error
	if kind == "image/jpeg" {
		err = jpeg.Encode(&b, im, nil)
	} else {
		err = png.Encode(&b, im)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestPhotoValidation(t *testing.T) {
	for _, kind := range []string{"image/png", "image/jpeg"} {
		data := append(testImage(t, kind), []byte("private metadata")...)
		out, outKind, err := NormalizePhoto(data, kind)
		if err != nil || outKind != kind || bytes.Contains(out, []byte("private metadata")) {
			t.Fatalf("normalize %s: %v", kind, err)
		}
	}
	for _, c := range []struct {
		data []byte
		kind string
		want error
	}{{[]byte("<svg/>"), "image/svg+xml", ErrPhotoFormat}, {testImage(t, "image/png"), "image/jpeg", ErrInvalid}, {[]byte("not an image"), "image/png", ErrInvalid}, {make([]byte, MaxPhotoBytes+1), "image/png", ErrPhotoTooLarge}} {
		if _, _, err := NormalizePhoto(c.data, c.kind); err != c.want {
			t.Fatalf("got %v want %v", err, c.want)
		}
	}
	var huge bytes.Buffer
	png.Encode(&huge, image.NewGray(image.Rect(0, 0, 4097, 1)))
	if _, _, err := NormalizePhoto(huge.Bytes(), "image/png"); err != ErrInvalid {
		t.Fatal("dimension cap not enforced")
	}
}

type failurePhotoStore struct {
	PublicationStore
	staged         bool
	activated      bool
	failActivation bool
}

func (s *failurePhotoStore) StagePhoto(_ context.Context, _ uuid.UUID, p Photo) error {
	if !strings.HasPrefix(p.Key, "photos/") {
		return ErrInvalid
	}
	s.staged = true
	return nil
}
func (s *failurePhotoStore) ActivatePhoto(_ context.Context, _ uuid.UUID, _ string) error {
	if s.failActivation {
		return errors.New("db failure")
	}
	s.activated = true
	return nil
}

type failureStorage struct{ putError bool }

func (s failureStorage) Put(context.Context, string, []byte, string) error {
	if s.putError {
		return errors.New("storage failure")
	}
	return nil
}
func (s failureStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("unavailable")
}
func (s failureStorage) Delete(context.Context, string) error { return errors.New("unavailable") }
func TestPhotoFailureStaysTracked(t *testing.T) {
	for _, putFail := range []bool{false, true} {
		store := &failurePhotoStore{failActivation: !putFail}
		s := NewPublishing(store, failureStorage{putError: putFail})
		err := s.UploadPhoto(context.Background(), uuid.New(), testImage(t, "image/png"), "image/png")
		if err == nil || !store.staged || store.activated {
			t.Fatalf("lost tracking or activated failure: %v", err)
		}
	}
}
