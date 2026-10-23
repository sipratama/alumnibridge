package postgres

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
	identitypg "github.com/sipratama/alumnibridge/apps/api/internal/identity/postgres"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
	profileshttp "github.com/sipratama/alumnibridge/apps/api/internal/profiles/http"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles/objectstorage"
)

type publicationVerifier struct{ subject string }

func (v publicationVerifier) Verify(_ context.Context, token string) (identity.Principal, error) {
	if token != "valid" {
		return identity.Principal{}, errors.New("invalid token")
	}
	return identity.Principal{Subject: v.subject}, nil
}
func TestPublicProfilesAndMinIO(t *testing.T) {
	dbURL, endpoint := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_OBJECT_STORAGE_ENDPOINT")
	if dbURL == "" || endpoint == "" {
		t.Skip("TEST_DATABASE_URL and TEST_OBJECT_STORAGE_ENDPOINT required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bucket := "alumnibridge-test-" + uuid.NewString()
	access, secret := os.Getenv("TEST_OBJECT_STORAGE_ACCESS_KEY"), os.Getenv("TEST_OBJECT_STORAGE_SECRET_KEY")
	storage, err := objectstorage.New(ctx, endpoint, access, secret, bucket, false)
	if err != nil {
		t.Fatal(err)
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		if err := client.RemoveBucket(ctx, bucket); err != nil {
			t.Error("bucket cleanup", err)
		}
	}()
	ids := identity.NewService(identitypg.New(db))
	subject := uuid.NewString()
	u, err := ids.Me(ctx, identity.Principal{Subject: subject, Email: "secret@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID)
	store := New(db)
	svc := profiles.New(store)
	pub := profiles.NewPublishing(store, storage)
	p, err := svc.Profile(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	name, bio := "Public Member", "Private biography"
	p, err = svc.UpdateProfile(ctx, u.ID, profiles.ProfilePatch{DisplayName: &name, Bio: &bio})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		profileshttp.RegisterRoutes(r, svc, ids, publicationVerifier{subject})
		profileshttp.RegisterPublishingRoutes(r, svc, pub, ids, publicationVerifier{subject})
	})
	call := func(method, path string, body []byte, kind, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", kind)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	call("GET", "/profiles/"+p.Slug, nil, "", "", 404)
	call("PATCH", "/users/me/profile/privacy", []byte(`{"visibility":"public"}`), "application/json", "", 401)
	for _, bad := range []string{`{"visibility":"public","moderation_status":"visible"}`, `{"visibility":"public","verified_alumni":true}`, `{"visibility":"PUBLIC"}`, `{}`} {
		call("PATCH", "/users/me/profile/privacy", []byte(bad), "application/json", "valid", 400)
	}
	call("PATCH", "/users/me/profile/privacy", []byte(`{"visibility":"public"}`), "application/json", "valid", 200)
	_, err = db.Exec(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year) SELECT $1,id,'alumni','pending',2020 FROM study_programs LIMIT 1`, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	w := call("GET", "/profiles/"+p.Slug, nil, "", "", 200)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("public cache can retain private data")
	}
	for _, secret := range []string{u.ID.String(), subject, "secret@example.invalid", "Private biography", "oidc_subject", "user_id", "moderation_status"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("public leak %q", secret)
		}
	}
	var response profiles.PublicProfile
	json.Unmarshal(w.Body.Bytes(), &response)
	if response.VerifiedAlumni {
		t.Fatal("pending alumni got verified badge")
	}
	db.Exec(ctx, `UPDATE memberships SET status='verified' WHERE user_id=$1`, u.ID)
	w = call("GET", "/profiles/"+p.Slug, nil, "", "", 200)
	json.Unmarshal(w.Body.Bytes(), &response)
	if !response.VerifiedAlumni {
		t.Fatal("trusted verified membership not reflected")
	}
	// Each publication flag is applied by the server projection.
	for _, field := range []string{"bio", "location", "current_company", "current_job_title", "experiences", "educations", "skills"} {
		for _, enabled := range []bool{true, false} {
			payload, _ := json.Marshal(map[string]any{"visibility": "public", "show_" + field: enabled})
			call("PATCH", "/users/me/profile/privacy", payload, "application/json", "valid", 200)
			out := call("GET", "/profiles/"+p.Slug, nil, "", "", 200)
			var fields map[string]json.RawMessage
			json.Unmarshal(out.Body.Bytes(), &fields)
			_, exists := fields[field]
			if exists != enabled {
				t.Fatalf("publication flag %s=%v not enforced", field, enabled)
			}
		}
	}
	// Enabling optional fields requires explicit owner flags.
	call("PATCH", "/users/me/profile/privacy", []byte(`{"visibility":"public","show_bio":true,"show_photo":true}`), "application/json", "valid", 200)
	w = call("GET", "/profiles/"+p.Slug, nil, "", "", 200)
	if !strings.Contains(w.Body.String(), bio) {
		t.Fatal("opted-in biography missing")
	}
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	call("POST", "/users/me/profile/photo", img.Bytes(), "image/png", "", 401)
	call("POST", "/users/me/profile/photo", []byte("<svg/>"), "image/svg+xml", "valid", 415)
	call("POST", "/users/me/profile/photo", []byte("bad PNG"), "image/png", "valid", 400)
	call("POST", "/users/me/profile/photo", make([]byte, profiles.MaxPhotoBytes+1), "image/png", "valid", 413)
	call("POST", "/users/me/profile/photo", img.Bytes(), "image/png", "valid", 201)
	photo, err := store.OwnerPhoto(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	// MinIO bucket is private even when the corresponding profile is public.
	remote, err := http.Get("http://" + endpoint + "/" + bucket + "/" + photo.Key)
	if err != nil {
		t.Fatal(err)
	}
	remote.Body.Close()
	if remote.StatusCode != 403 {
		t.Fatalf("anonymous bucket access %d", remote.StatusCode)
	}
	call("GET", "/profiles/"+p.Slug+"/photo", nil, "", "", 200)
	// WebP fixture exercises the real decoder and PNG normalization.
	webp, _ := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if _, _, err := profiles.NormalizePhoto(webp, "image/webp"); err != nil {
		t.Fatal("WebP input", err)
	}
	call("POST", "/users/me/profile/photo", webp, "image/webp", "valid", 201)
	if w := call("GET", "/users/me/profile/photo", nil, "", "valid", 200); w.Header().Get("Content-Type") != "image/png" {
		t.Fatal("WebP not normalized to PNG")
	}
	// Replacements schedule old objects for durable cleanup.
	call("POST", "/users/me/profile/photo", img.Bytes(), "image/png", "valid", 201)
	if err = pub.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if body, err := storage.Get(ctx, photo.Key); err == nil {
		body.Close()
		t.Fatal("replaced photo not collected")
	}
	// Historical redirects happen only after privacy and moderation checks.
	old := p.Slug
	next := "public-" + uuid.NewString()
	p, err = svc.UpdateProfile(ctx, u.ID, profiles.ProfilePatch{Slug: &next})
	if err != nil {
		t.Fatal(err)
	}
	w = call("GET", "/profiles/"+old, nil, "", "", 307)
	if w.Header().Get("Location") != "/api/v1/profiles/"+next {
		t.Fatal("wrong owner redirect")
	}
	db.Exec(ctx, `UPDATE profiles SET moderation_status='hidden' WHERE user_id=$1`, u.ID)
	call("GET", "/profiles/"+old, nil, "", "", 404)
	call("GET", "/profiles/"+next+"/photo", nil, "", "", 404)
	call("PATCH", "/users/me/profile/privacy", []byte(`{"visibility":"public"}`), "application/json", "valid", 200)
	call("GET", "/profiles/"+next, nil, "", "", 404)
	db.Exec(ctx, `UPDATE profiles SET moderation_status='visible' WHERE user_id=$1`, u.ID)
	call("PATCH", "/users/me/profile/privacy", []byte(`{"visibility":"private"}`), "application/json", "valid", 200)
	call("GET", "/profiles/"+old, nil, "", "", 404)
	call("GET", "/profiles/"+next+"/photo", nil, "", "", 404)
	call("GET", "/users/me/profile/photo", nil, "", "valid", 200)
	if err := storage.Put(ctx, "../traversal", img.Bytes(), "image/png"); err == nil {
		t.Fatal("traversal accepted")
	}
	// Cross-owner activation cannot attach another user's staged object.
	other := uuid.New()
	db.Exec(ctx, `INSERT INTO users(id,oidc_subject) VALUES($1,$2)`, other, other.String())
	defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, other)
	staged := profiles.Photo{Key: "photos/" + uuid.NewString(), ContentType: "image/png", Size: int64(img.Len())}
	if err = store.StagePhoto(ctx, u.ID, staged); err != nil {
		t.Fatal(err)
	}
	if err = storage.Put(ctx, staged.Key, img.Bytes(), "image/png"); err != nil {
		t.Fatal(err)
	}
	if err = store.ActivatePhoto(ctx, other, staged.Key); err != profiles.ErrNotFound {
		t.Fatal("cross-owner activation", err)
	}
	// Failed deletes retain cleanup intent; a retry succeeds.
	call("DELETE", "/users/me/profile/photo", nil, "", "valid", 204)
	if err = store.CleanupPhotos(ctx, func(context.Context, string) error { return errors.New("storage offline") }); err == nil {
		t.Fatal("failure not surfaced")
	}
	db.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()-interval '1 second' WHERE owner_id=$1`, u.ID)
	if err = pub.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow(ctx, `SELECT count(*) FROM profile_photo_objects WHERE owner_id=$1`, u.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("orphan registry entries: %d", count)
	}
	call("GET", "/users/me/profile/photo", nil, "", "valid", 404)
	if _, err = db.Exec(ctx, `UPDATE profiles SET photo_size_bytes=2 WHERE user_id=$1`, u.ID); err == nil {
		t.Fatal("partial photo metadata accepted")
	}
	if _, err = db.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	call("GET", "/profiles/"+old, nil, "", "", 404)
	call("GET", "/profiles/"+next, nil, "", "", 404)

}
