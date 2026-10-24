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
		err := s.UploadPhoto(context.Background(), uuid.New(), bytes.NewReader(testImage(t, "image/png")), "image/png")
		if err == nil || !store.staged || store.activated {
			t.Fatalf("lost tracking or activated failure: %v", err)
		}
	}
}

type observedReader struct {
	reads     int
	remaining int64
}

func (r *observedReader) Read(p []byte) (int, error) {
	r.reads++
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	clear(p[:n])
	r.remaining -= int64(n)
	return n, nil
}
func TestUploadAdmissionAndStreamLimit(t *testing.T) {
	store := &failurePhotoStore{}
	s := NewPublishing(store, failureStorage{})
	for i := 0; i < cap(s.photoSlots); i++ {
		s.photoSlots <- struct{}{}
	}
	reader := &observedReader{remaining: MaxPhotoBytes + 1024}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.UploadPhoto(ctx, uuid.New(), reader, "image/png"); err != context.Canceled || reader.reads != 0 {
		t.Fatalf("body read before admission: %v, %d reads", err, reader.reads)
	}
	for i := 0; i < cap(s.photoSlots); i++ {
		<-s.photoSlots
	}
	before := reader.remaining
	if err := s.UploadPhoto(context.Background(), uuid.New(), reader, "image/png"); err != ErrPhotoTooLarge || before-reader.remaining != MaxPhotoBytes+1 || store.staged {
		t.Fatalf("stream bound: %v remaining %d staged %v", err, reader.remaining, store.staged)
	}
	reader = &observedReader{remaining: MaxPhotoBytes}
	if err := s.UploadPhoto(context.Background(), uuid.New(), reader, "image/svg+xml"); err != ErrPhotoFormat || reader.reads != 0 {
		t.Fatalf("unsupported type read a body: %v", err)
	}
}
func TestPhotoEncodingBufferLimit(t *testing.T) {
	var out photoBuffer
	if _, err := out.Write(make([]byte, MaxPhotoBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte{1}); err != ErrPhotoTooLarge || out.Len() != MaxPhotoBytes {
		t.Fatal("output limit did not stop encoder allocation")
	}
}
func TestTruncatedPhoto(t *testing.T) {
	for _, kind := range []string{"image/jpeg", "image/png"} {
		data := testImage(t, kind)
		if _, _, err := NormalizePhoto(data[:len(data)/2], kind); err != ErrInvalid {
			t.Fatalf("truncated %s: %v", kind, err)
		}
	}
}
