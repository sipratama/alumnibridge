package postgres

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

type stagedPhotoStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
	started chan struct{}
	release chan struct{}
}

func (s *stagedPhotoStorage) Put(_ context.Context, key string, body []byte, _ string) error {
	if s.started != nil {
		close(s.started)
		<-s.release
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = bytes.Clone(body)
	return nil
}
func (s *stagedPhotoStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.objects[key]
	if !ok {
		return nil, errors.New("missing object")
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}
func (s *stagedPhotoStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func TestConditionalPhotoUploadDeleteRace(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	dbA, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	owner := uuid.New()
	if _, err := dbA.Exec(ctx, `INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,'Photo Race')`, owner, "photo-race-"+owner.String()); err != nil {
		t.Fatal(err)
	}
	defer dbA.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
	storeA, storeB := New(dbA), New(dbB)
	p, err := storeA.Profile(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	storage := &stagedPhotoStorage{objects: make(map[string][]byte)}
	serviceA := profiles.NewPublishing(storeA, storage)
	serviceB := profiles.NewPublishing(storeB, storage)
	revision, err := serviceA.UploadPhotoConditional(ctx, owner, bytes.NewReader(imageBody.Bytes()), "image/png", p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	original, err := storeA.OwnerPhoto(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	// The upload has completed normalization and object Put but cannot activate
	// until this test releases it. DELETE commits on a separate DB connection.
	storage.started = make(chan struct{})
	storage.release = make(chan struct{})
	uploadResult := make(chan error, 1)
	go func() {
		_, uploadErr := serviceA.UploadPhotoConditional(ctx, owner, bytes.NewReader(imageBody.Bytes()), "image/png", revision)
		uploadResult <- uploadErr
	}()
	<-storage.started
	removedRevision, err := serviceB.RemovePhotoConditional(ctx, owner, revision)
	if err != nil || removedRevision <= revision {
		t.Fatalf("delete did not advance content revision: %d %v", removedRevision, err)
	}
	close(storage.release)
	if err := <-uploadResult; !errors.Is(err, profiles.ErrStale) {
		t.Fatalf("stale upload result: %v", err)
	}
	var active, garbage int
	if err := dbA.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state='active'),count(*) FILTER (WHERE state='garbage')
		FROM profile_photo_objects WHERE owner_id=$1`, owner).Scan(&active, &garbage); err != nil {
		t.Fatal(err)
	}
	if active != 0 || garbage != 2 {
		t.Fatalf("stale upload side effects: active=%d garbage=%d", active, garbage)
	}
	var key *string
	var current int64
	if err := dbA.QueryRow(ctx, `SELECT photo_object_key,content_revision FROM profiles WHERE user_id=$1`, owner).Scan(&key, &current); err != nil {
		t.Fatal(err)
	}
	if key != nil || current != removedRevision {
		t.Fatalf("stale upload replaced deleted pointer: %v %d", key, current)
	}
	if err := serviceA.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	storage.mu.Lock()
	remaining := len(storage.objects)
	storage.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("rejected or removed photo object retained: %d", remaining)
	}
	if _, err := storage.Get(ctx, original.Key); err == nil {
		t.Fatal("deleted photo still readable")
	}
	if err := dbA.QueryRow(ctx, `SELECT count(*) FROM profile_photo_objects WHERE owner_id=$1`, owner).Scan(&active); err != nil || active != 0 {
		t.Fatalf("cleanup registry: %d %v", active, err)
	}

	// Reverse commit order: an old DELETE cannot remove a newly activated photo.
	storage.started, storage.release = nil, nil
	newRevision, err := serviceA.UploadPhotoConditional(ctx, owner, bytes.NewReader(imageBody.Bytes()), "image/png", current)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := serviceB.RemovePhotoConditional(ctx, owner, current); !errors.Is(err, profiles.ErrStale) {
		t.Fatalf("stale delete result: %v", err)
	}
	if err := dbA.QueryRow(ctx, `SELECT photo_object_key,content_revision FROM profiles WHERE user_id=$1`, owner).Scan(&key, &current); err != nil || key == nil || current != newRevision {
		t.Fatalf("stale delete changed pointer: %v %d %v", key, current, err)
	}
	other := uuid.New()
	if _, err := dbA.Exec(ctx, `INSERT INTO users(id,oidc_subject) VALUES($1,$2)`, other, "foreign-photo-"+other.String()); err != nil {
		t.Fatal(err)
	}
	defer dbA.Exec(ctx, `DELETE FROM users WHERE id=$1`, other)
	if _, err := storeA.Profile(ctx, other); err != nil {
		t.Fatal(err)
	}
	staged := profiles.Photo{Key: "photos/" + uuid.NewString(), ContentType: "image/png", Size: 2}
	if err := storeA.StagePhoto(ctx, owner, staged); err != nil {
		t.Fatal(err)
	}
	if _, err := storeB.ActivatePhotoConditional(ctx, other, staged.Key, 1); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatalf("foreign activation: %v", err)
	}
	if err := storeB.DiscardStagedPhoto(ctx, other, staged.Key); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := dbA.QueryRow(ctx, `SELECT state FROM profile_photo_objects WHERE object_key=$1`, staged.Key).Scan(&state); err != nil || state != "staging" {
		t.Fatalf("foreign cleanup touched owner object: %s %v", state, err)
	}
}
