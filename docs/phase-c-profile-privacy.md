# Milestone 3 Phase C: publication, privacy and photos

Date: 2026-10-24 (Asia/Jakarta). Base: `e6397e5` (committed Phase B review remediation). Branch: `feature/milestone-3-professional-profile`. Phase D is not implemented.

## Publication policy and moderation

All profiles start private and all optional publication flags start false. An owner must explicitly PATCH `visibility: public` to publish. Students and pending alumni may publish, as may verified alumni. Public visibility does not grant directory eligibility or verified status. `verified_alumni` is true only when trusted `memberships` contains an alumni row with status `verified`; there is no profile input for verification.

Display name and headline form the basic public card after consent. Bio, location, current company/title, experience, education, skills and photo each require separate owner publication flags. Education publishing never reads private academic membership identifiers or graduation years. Experience and education public objects have no internal IDs. Neither email, Keycloak subjects, owner/resource UUIDs, object keys, moderation fields, timestamps, evidence nor audit data appear in public DTOs.

`moderation_status` is independent from `visibility`, with states `visible` (default) and `hidden`. Public reads require both public consent and visible moderation. Owners cannot write moderation state through either profile or privacy routes; unknown fields are rejected. Hidden owners retain private editing/photo access, but publishing again cannot override a moderation hide.

For this phase, only a trusted database operator can hide/restore a profile. Use a parameterized update of `profiles.moderation_status` by internal UUID in a transaction. Future moderator use cases must authorize the moderator server-side, record reason/actor/time privately, and review user-provided content before restoring visibility. No admin API, dashboard or reporting flow is introduced. Slug moderation vocabulary and production abuse/rate limits remain release decisions.

## API and privacy boundaries

- `GET /api/v1/profiles/{slug}` is anonymous and returns the explicit `PublicProfile` allowlist.
- `GET/PATCH /api/v1/users/me/profile/privacy` requires the current verified actor. PATCH requires an explicit lowercase `public` or `private`; omitted flags retain their values, and null/unknown fields fail.
- `GET/POST/DELETE /api/v1/users/me/profile/photo` is owner-only.
- `GET /api/v1/profiles/{slug}/photo` checks current public consent, moderation visibility and `show_photo` before accessing storage.

The public PostgreSQL query filters consent and moderation and projects only opted-in fields. The publication service checks both states again. Old slugs join through the immutable owner reservation and redirect with 307 to the current relative profile API URL only after visibility checks. Missing, private, hidden and deleted-owner tombstone slugs all return the same 404 without a redirect. Old photo slugs resolve through the same owner mapping. No internal-ID lookup or arbitrary object-key route is exposed.

All public profile/photo responses use `Cache-Control: no-store`; photo responses also use `nosniff` and the server-controlled image MIME type. Owner auth middleware already disables caching. This allows privacy/moderation changes to affect subsequent requests immediately; bytes already downloaded or an in-flight checked request cannot be revoked.

## Storage and upload lifecycle

`PhotoStorage` is a small application port; the MinIO SDK lives only in an edge adapter. SDK and WebP decoder versions are pinned in Go modules. Compose creates an AlumniBridge-only MinIO service and named volume. Loopback host ports 19000 and 19001 avoid sharing work-related containers. The private bucket has no bucket policy; startup refuses any preconfigured bucket policy rather than changing it silently. Use identity policies for authenticated access. Credentials are environment variables with local example placeholders only. Production must use dedicated least-privilege credentials and reviewed S3 deployment settings.

Official registry images were unavailable during implementation, so `infrastructure/minio/Dockerfile` builds the published release `RELEASE.2025-10-15T17-29-55Z` from immutable source commit `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`. The downloaded source archive is checked against SHA-256 `45521908307306e925c98d629e1c17d78c8b72b6ee242b1bfb1409f7d8ee5841` before extraction. The upstream community repository is archived; this is a local development choice, not a production storage recommendation. Primary sources: [server source](https://github.com/minio/minio), [SDK source](https://github.com/minio/minio-go).

Uploads accept raw JPEG, PNG or WebP bytes with matching MIME, up to 5 MiB. The decoder checks dimensions (maximum 4096 per side and eight million pixels) before full decoding. Pixels are re-encoded: JPEG to JPEG, PNG/WebP to PNG. This removes metadata and appended payloads. Normalized output is also bounded to 5 MiB. SVG, malformed data and MIME/content mismatches fail. At most four uploads may buffer, decode, normalize and upload concurrently per process. Admission is acquired before reading the bounded request stream; encoding stops at the output limit. Object keys are random server UUIDs under `photos/`; the adapter rejects traversal and other key shapes. Multipart uploads are disabled in the SDK to avoid abandoned multipart state.

There is no PostgreSQL/object-store distributed transaction. A durable `profile_photo_objects` row is committed before uploading. It starts `staging` with a 15-minute expiry. After Put succeeds, activation locks that object row and the owner profile, swaps complete metadata atomically, marks the new object active, and queues the previous object as garbage. Only the staged object's owner can activate it. An upload or activation failure leaves cleanup intent, preserving the previous profile photo. A process crash in either gap also leaves a tracked object.

DELETE clears photo metadata immediately and queues the object for deletion. A worker runs each minute, handles at most 20 unreferenced objects, locks them, rechecks references and deletes through the storage port. Failed deletes retain durable intent and retry after a minute. Abandoned staging and deleted-account active objects are eligible after expiry. Replacement keys are never reused. A unique profile-object index and FK prevent cleanup from deleting a still-referenced object. Storage outages may delay physical deletion but do not restore API access to removed/private photos. No user-provided object key or storage URL is exposed.

The API initializes its private bucket at startup with a bounded timeout. Storage failure at startup prevents startup; subsequent failures affect photo operations with safe 503 responses while identity/profile operations continue. The worker logs a generic retry message without keys or personal data.

## Migration

`000005_public_profiles_photos` adds moderation state and the durable object registry, fixes the SQL NULL loophole in the photo metadata CHECK, adds referential integrity and cleanup indexes, and preserves existing profiles/photo metadata. It does not alter migrations 000001–000004 or slug tombstones. Unexpected incomplete preexisting metadata causes migration failure for operator review rather than silent deletion.

Rollback removes the new schema but preserves profile photo metadata and bucket contents. It refuses to drop the registry while unreferenced objects still need cleanup. After cleanup, active photo metadata can repopulate the registry on reapplication. The existing 000004 tombstone rollback guard remains intact. Never reset volumes to bypass these guards.

## Verification record

Executed against temporary PostgreSQL databases and temporary private buckets on the dedicated local MinIO instance:

- Full migration chain 000001–000005 apply, rollback and reapply.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./cmd/api`, with PostgreSQL and MinIO integration variables enabled.
- Real MinIO Put/Get/Delete, anonymous bucket access rejected with 403, replacement cleanup, traversal rejection and retry after simulated storage deletion failure.
- Public/private/hidden/missing and historical/tombstone HTTP responses; per-field publication matrix; pending alumni without a verified badge; trusted verified membership badge; verification/moderation input rejection; unauthorized privacy/photo writes; cross-owner activation rejection.
- PNG/JPEG full decoding/re-encoding, actual WebP decoding and upload normalization, MIME mismatch, SVG, malformed data, dimensions and oversized payload rejection.
- Put/activation failure unit tests verify durable staging precedes storage and failed operations do not activate new metadata. An actual staged uploaded object with a failed owner activation is collected after expiry.
- Existing Phase B signed-JWT HTTP/SQL and concurrency tests run alongside the new tests. Phase C handler integration uses a verifier fixture; cryptographic signature/expiry validation remains exercised by the unchanged real-verifier tests.
- Frontend tests (8), build and lint pass without warnings. OpenAPI YAML parses successfully. `git diff --check` passes.

Current-turn runtime evidence: rebuilt the API and ran migration 000005 on the existing development database without deleting volumes. Health and readiness returned 200, missing/hidden public profile reads 404, unauthenticated privacy/photo reads 401, and oversized skill search limits 400. Local MinIO is healthy and the private runtime bucket was initialized successfully.

Actual browser evidence: Playwright launched local Google Chrome on this Mac against the running Keycloak, web and API. Disposable Student and Alumni accounts completed registration, onboarding, refresh on account/dashboard/settings/onboarding, logout protection, and login restoration. Using their actual observed Keycloak bearer tokens, API requests also exercised profile update, private 404, upload, public consent, anonymous public profile/photo reads, pending alumni without verified badge, returning private and photo removal. All final flows passed. Disposable Keycloak and application users were removed. A malformed hardcoded PNG in the initial browser harness was correctly rejected with 400; the final harness generated a valid PNG through Chrome canvas and passed without an application change.

The new migration rollback guard was also tested explicitly with a staged object: it refused rollback with `photo cleanup must complete before rollback`. All guard and integration databases were removed; temporary test buckets were removed after each run. The normal cleanup worker retains responsibility for deleted runtime test photos.

No Phase D UI was built. Browser coverage is Chrome on this Mac only; repeat the documented sign-in/onboarding/refresh/logout checklist on Windows Podman before deploying there.

## Readiness conditions

Verdict: **READY FOR PHASE C REVIEW**. All requested local checks passed; production readiness is not implied. Production storage maintenance, moderation workflow/audit, content policy and abuse controls are operational follow-ups. Field visibility and photo endpoints are backend-only; no Phase D profile UI or public page has been built. Shared work-related resources and all persistent volumes are preserved. GitHub Issues #2, #6 and #7 remain open; commits reference them, with no push, PR or merge.

For integration reruns, create an isolated PostgreSQL database and apply `infrastructure/migrations/*.up.sql` in order. Set `TEST_DATABASE_URL` to that database and `TEST_OBJECT_STORAGE_ENDPOINT`, `TEST_OBJECT_STORAGE_ACCESS_KEY`, and `TEST_OBJECT_STORAGE_SECRET_KEY` to the dedicated local MinIO instance. Native tests use endpoint `localhost:19000`; Docker test containers on this Mac use `host.docker.internal:19000`. Tests create unique temporary buckets and delete them afterward. Run the Go commands above, then remove only the temporary database. Do not point this suite at production data: slug tests intentionally retain permanent tombstones. Runtime cleanup was checked after the browser run: zero registry objects remained.

The Phase C review adds signed-JWT publication integration tests, concurrent replacement/fault-injection checks and actual process-restart cleanup verification. See [the review report](reviews/milestone-3-phase-c-review.md) for the final conditions and Windows status.
