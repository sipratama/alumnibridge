package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/auth"
	"image"
	"image/png"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

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

func publicationJWT(t *testing.T, ctx context.Context) (auth.Verifier, func(string, time.Time) string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "publication", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	}))
	t.Cleanup(jwks.Close)
	verifier, err := auth.NewJWTVerifier(ctx, jwks.URL, "https://publication.example", "alumnibridge-api", "alumnibridge-web")
	if err != nil {
		t.Fatal(err)
	}
	return verifier, func(subject string, expiry time.Time) string {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": subject, "iss": "https://publication.example", "aud": "alumnibridge-api", "typ": "Bearer", "azp": "alumnibridge-web", "exp": expiry.Unix()})
		token.Header["kid"] = "publication"
		signed, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
}
func TestPublicProfilesAndMinIO(t *testing.T) {
	dbURL, endpoint := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_OBJECT_STORAGE_ENDPOINT")
	if dbURL == "" || endpoint == "" {
		t.Skip("TEST_DATABASE_URL and TEST_OBJECT_STORAGE_ENDPOINT required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	verifier, sign := publicationJWT(t, ctx)
	ownerToken := sign(subject, time.Now().Add(time.Hour))
	otherSubject := uuid.NewString()
	otherToken := sign(otherSubject, time.Now().Add(time.Hour))
	otherUser, err := ids.Me(ctx, identity.Principal{Subject: otherSubject})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, otherUser.ID)
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
		profileshttp.RegisterRoutes(r, svc, ids, verifier)
		profileshttp.RegisterPublishingRoutes(r, svc, pub, ids, verifier)
	})
	call := func(method, path string, body []byte, kind, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", kind)
		if token == "valid" {
			token = ownerToken
		}
		if token == "other" {
			token = otherToken
		}
		if token == "expired" {
			token = sign(subject, time.Now().Add(-time.Hour))
		}
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
	call("GET", "/users/me/profile/photo", nil, "", "expired", 401)
	call("GET", "/users/me/profile/photo", nil, "", "forged.jwt.token", 401)
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
	// Forged owner hints cannot read, replace or remove another user's photo.
	call("GET", "/profiles/"+p.Slug+"/photo", nil, "", "other", 200)
	call("PATCH", "/users/me/profile/privacy", []byte(`{"visibility":"private","user_id":"`+u.ID.String()+`"}`), "application/json", "other", 400)
	call("POST", "/users/me/profile/photo?user_id="+u.ID.String(), img.Bytes(), "image/png", "other", 201)
	otherPhoto, err := store.OwnerPhoto(ctx, otherUser.ID)
	if err != nil || otherPhoto.Key == photo.Key {
		t.Fatal("cross-user replacement", err)
	}
	call("DELETE", "/users/me/profile/photo?user_id="+u.ID.String(), nil, "", "other", 204)
	still, err := store.OwnerPhoto(ctx, u.ID)
	if err != nil || still.Key != photo.Key {
		t.Fatal("cross-user removal", err)
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
	photoRead := call("GET", "/profiles/"+p.Slug+"/photo", nil, "", "", 200)
	for key, want := range map[string]string{"Content-Type": "image/png", "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Content-Disposition": "inline"} {
		if photoRead.Header().Get(key) != want {
			t.Fatalf("photo header %s", key)
		}
	}
	// WebP fixture exercises the real decoder and PNG normalization.
	webp, _ := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if _, _, err := profiles.NormalizePhoto(webp, "image/webp"); err != nil {
		t.Fatal("WebP input", err)
	}
	call("POST", "/users/me/profile/photo", webp, "image/webp", "valid", 201)
	if w := call("GET", "/users/me/profile/photo", nil, "", "valid", 200); w.Header().Get("Content-Type") != "image/png" {
		t.Fatal("WebP not normalized to PNG")
	}
	// Startup refuses a bucket policy that would expose private objects.
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` + bucket + `/*"]}]}`
	if err = client.SetBucketPolicy(ctx, bucket, policy); err != nil {
		t.Fatal(err)
	}
	if _, err = objectstorage.New(ctx, endpoint, access, secret, bucket, false); err == nil {
		t.Fatal("public bucket accepted")
	}
	if err = client.SetBucketPolicy(ctx, bucket, ""); err != nil {
		t.Fatal(err)
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
	call("GET", "/users/me/profile/photo", nil, "", "other", 404)
	call("GET", "/profiles/"+next+"/photo", nil, "", "other", 404)
	if err := storage.Put(ctx, "../traversal", img.Bytes(), "image/png"); err == nil {
		t.Fatal("traversal accepted")
	}
	// Cross-owner activation cannot attach another user's staged object.
	other := otherUser.ID
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
	// Concurrent replacements leave one live reference; stale cleanup must not delete it.
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures <- pub.UploadPhoto(ctx, u.ID, bytes.NewReader(img.Bytes()), "image/png")
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, err := store.OwnerPhoto(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()-interval '1 second' WHERE owner_id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err = pub.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	stream, err := storage.Get(ctx, current.Key)
	if err != nil {
		t.Fatal("active object deleted", err)
	}
	stream.Close()
	// A transaction failure after changing the profile must roll back its new reference.
	if _, err = db.Exec(ctx, `CREATE FUNCTION review_reject_photo_activation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='active' THEN RAISE EXCEPTION 'synthetic activation failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `CREATE TRIGGER review_reject_photo_activation BEFORE UPDATE ON profile_photo_objects FOR EACH ROW EXECUTE FUNCTION review_reject_photo_activation()`); err != nil {
		t.Fatal(err)
	}
	call("POST", "/users/me/profile/photo", img.Bytes(), "image/png", "valid", 500)
	if _, err = db.Exec(ctx, `DROP TRIGGER review_reject_photo_activation ON profile_photo_objects; DROP FUNCTION review_reject_photo_activation()`); err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.OwnerPhoto(ctx, u.ID)
	if err != nil || unchanged.Key != current.Key {
		t.Fatal("failed transaction replaced current photo", err)
	}
	if _, err = db.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()-interval '1 second' WHERE owner_id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err = pub.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	// A new service instance resumes durable jobs after a simulated restart.
	if err = pub.RemovePhoto(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	restarted := profiles.NewPublishing(New(db), storage)
	if err = restarted.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Cleanup(ctx); err != nil {
		t.Fatal("non-idempotent cleanup", err)
	}
	if err = pub.UploadPhoto(ctx, u.ID, bytes.NewReader(img.Bytes()), "image/png"); err != nil {
		t.Fatal(err)
	}
	deletedPhoto, err := store.OwnerPhoto(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	call("GET", "/profiles/"+old, nil, "", "", 404)
	call("GET", "/profiles/"+next, nil, "", "", 404)

	if _, err = db.Exec(ctx, `UPDATE profile_photo_objects SET expires_at=now()-interval '1 second' WHERE object_key=$1`, deletedPhoto.Key); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if stream, err := storage.Get(ctx, deletedPhoto.Key); err == nil {
		stream.Close()
		t.Fatal("deleted-account photo was not cleaned")
	}

}

func TestCleanupRetryPersistenceFailure(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := "photos/" + uuid.NewString()
	if _, err = db.Exec(ctx, `INSERT INTO profile_photo_objects(object_key,state,content_type,size_bytes,expires_at) VALUES($1,'garbage','image/png',10,now()-interval '1 second')`, key); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM profile_photo_objects WHERE object_key=$1`, key)
	if _, err = db.Exec(ctx, `CREATE FUNCTION review_reject_cleanup_retry() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic retry schedule failure'; END $$; CREATE TRIGGER review_reject_cleanup_retry BEFORE UPDATE ON profile_photo_objects FOR EACH ROW EXECUTE FUNCTION review_reject_cleanup_retry()`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DROP TRIGGER review_reject_cleanup_retry ON profile_photo_objects; DROP FUNCTION review_reject_cleanup_retry()`)
	removeErr := errors.New("synthetic object delete failure")
	err = New(db).CleanupPhotos(ctx, func(context.Context, string) error { return removeErr })
	if !errors.Is(err, removeErr) || !strings.Contains(err.Error(), "synthetic retry schedule failure") {
		t.Fatalf("retry persistence error lost: %v", err)
	}
	var retained bool
	if err = db.QueryRow(ctx, `SELECT expires_at<now() FROM profile_photo_objects WHERE object_key=$1`, key).Scan(&retained); err != nil || !retained {
		t.Fatal("failed retry persistence lost durable job", err)
	}
}
