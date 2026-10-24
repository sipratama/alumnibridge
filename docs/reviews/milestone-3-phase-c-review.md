# Milestone 3 Phase C review and verification

Review date: 2026-10-24 (Asia/Jakarta).
Reviewed commit: `d10db40610e87bc518a4ac65e87bc259cdff7f18`.
Branch: `feature/milestone-3-professional-profile`.
Preflight: clean tree; origin fetched; branch ten commits ahead of origin/master, no divergence. Phase B remediation is present. AGENTS, architecture, profile design, Phase B report, Phase C design, migrations 000001–000005, OpenAPI and GitHub Issues #2/#6/#7 were reviewed.

## Verdict

**READY WITH CONDITIONS** for Phase D development after user review. Application findings have been fixed and local verification passed. This is not production approval: community MinIO is archived, Windows/Podman were not available here, and production storage, abuse limits and moderation operations need separate review. No provider was replaced, no migration or persistent volume was reset, and no Phase D work was started.

## Findings and targeted remediation

Line references below identify the reviewed SHA unless explicitly labeled as remediated code.

### H1 — High: upload admission does not bound body buffering (fixed)

Evidence: `apps/api/internal/profiles/http/public.go:97` reads up to 5 MiB before `apps/api/internal/profiles/photo.go:59` acquires a four-slot semaphore. A request waiting for an image-processing slot already retains its entire body. At `photo.go:42`, normalized output also accumulates in an unbounded buffer before the final size check.

Impact: an authenticated user can multiply memory consumption with concurrent requests despite the documented limit. Public registration makes this a low-privilege availability risk. No destructive memory-exhaustion load test was run against the development server; the code order and reader-boundary regression tests establish the issue.

Reproduction: block all processing slots, then send many valid authenticated 5 MiB bodies. The reviewed handler buffers every body before it can wait for admission. A large normalized PNG can also allocate past the output limit before rejection.

Remediation: `UploadPhoto` now accepts `io.Reader`, rejects unsupported MIME before reading, acquires a processing slot before buffering, and reads at most `MaxPhotoBytes+1`. Normalization uses a bounded writer that exposes no bypassing `WriteByte`/`WriteString` methods. Input, decoding, normalized output and storage upload share the same slot. Existing byte, pixel, dimension and request time limits remain.

Regression evidence: `TestUploadAdmissionAndStreamLimit` asserts an unadmitted canceled request reads zero bytes, an unknown-length reader is consumed only through limit+1, unsupported types read nothing, and rejected uploads never stage data. `TestPhotoEncodingBufferLimit` prevents encoder output growth beyond the limit. `TestTruncatedPhoto` covers malformed/truncated JPEG and PNG alongside existing format/dimension tests. Remediated code: `profiles/photo.go:22`, `:76`, `:89`; HTTP adapter passes the request stream directly.

### M1 — Medium: downloaded MinIO source archive has no artifact checksum (fixed)

Evidence: `infrastructure/minio/Dockerfile:5` pins a Git commit in its download URL but does not check the downloaded archive bytes before compiling them at line 6.

Impact: artifact integrity relies entirely on download transport and the hosting service; the build does not fail if different bytes are served at the pinned URL. This is build hardening, not evidence of a compromised archive.

Reproduction: substitute different archive bytes at the download step; the reviewed Dockerfile has no explicit expected-byte verification.

Remediation: the existing source commit and release are unchanged. The Dockerfile verifies SHA-256 `45521908307306e925c98d629e1c17d78c8b72b6ee242b1bfb1409f7d8ee5841` before extraction. It uses ordinary `sha256sum` rather than requiring a newer Docker-specific ADD flag.

Regression evidence: downloaded the pinned archive independently, checked its digest, then rebuilt the MinIO image. The build emitted `/tmp/minio.tar.gz: OK` and completed. Base-image tags, Alpine packages and build timestamps remain mutable; this does not establish bit-for-bit reproducibility or make the archived server production-supported.

### L1 — Low: cleanup suppresses retry persistence errors (fixed)

Evidence: `apps/api/internal/profiles/postgres/public.go:154` ignores the retry timestamp update error and `:155` ignores its commit error after object deletion fails.

Impact: the job remains recoverable, but the returned error hides a simultaneous database scheduling failure, reducing diagnosis quality. This is not evidence of lost cleanup intent.

Reproduction: cause storage Delete to fail and install a disposable-database trigger that rejects the retry timestamp update. The reviewed implementation returns only the storage error.

Remediation: explicitly roll back failed retry scheduling and join database/update/commit errors with the storage error. HTTP responses and worker logs still use safe generic messages.

Regression evidence: `TestCleanupRetryPersistenceFailure` injects that SQL trigger and asserts both causes are retained and the durable job still exists with its prior eligible timestamp. The normal failed-delete/retry test remains successful.

### L2 — Low: Phase C integration coverage uses a verifier stub and misses lifecycle interleavings (resolved)

Evidence: reviewed `apps/api/internal/profiles/postgres/public_integration_test.go:29` accepts the literal token `valid`. Phase B tests verify signatures independently, but Phase C does not itself cover real signed-token actor switching, concurrent replacements, or an actual API process restart.

Impact: a verification gap, not a demonstrated authorization vulnerability.

Remediation and regression evidence: Phase C integration now uses RSA-signed JWTs, a test JWKS endpoint and the real JWT verifier. It tests valid/expired/forged tokens, actor switching, forged owner hints, private cross-user photo reads/deletes, public/private/hidden states, field omission, verification/moderation rejection, temporary public bucket policy rejection, concurrent replacements, stale cleanup, transaction rollback, restart via a new service instance, account deletion and idempotent cleanup. Actual Chrome/runtime tests additionally restarted the API process with a pending photo job and observed recovery by the new worker.

Application totals: zero Critical; one High, one Medium and two Low findings, all remediated. The separate production-only storage risk below remains open by explicit scope.

## Security and architecture assessment

- Owner operations use verified subjects resolved to internal UUIDs; no body or query-supplied owner ID determines the actor. Cross-user attempts cannot change another profile, privacy settings or photo. Owner photo routes expose only the caller's photo. No anonymous listing/internal-ID lookup route exists.
- Public SQL filters both `visibility='public'` and `moderation_status='visible'` and constructs an allowlisted projection. The service rechecks those states. Disabled fields are omitted, not sent as null. Public experience/education IDs, Keycloak subjects, email, membership identifiers, object keys and internal metadata are absent.
- `verified_alumni` is an EXISTS query over trusted verified alumni memberships. Students/pending applicants do not gain it through profile input. Owner APIs reject verification and moderation fields.
- Historical slugs resolve through immutable owner reservations. Redirects are relative, current-slug requests do not redirect, and old private/hidden/tombstone slugs return the same 404 without Location. `no-store` prevents permanent redirect/photo caching. Bytes already read or an in-flight authorized request cannot be retroactively revoked.
- JPEG/PNG/WebP are fully decoded with dimension/pixel limits and normalized. Metadata and appended payloads are not served. SVG, MIME mismatches, truncation, traversal and arbitrary object key input are rejected. Binary responses use the recorded image MIME, `inline`, `nosniff` and `no-store`.
- A photo object is registered before Put, activated only for its staged owner, and replaced atomically under row locks. Cleanup locks candidate objects and rechecks live references. Failed upload/activation preserves the previous photo and durable staging. Deletion clears API access first, retaining retry intent. Concurrent replacement tests leave exactly one live object reference, and expired active objects are not collected while referenced.
- Moderation remains a trusted operator action, separate from owner consent and verification. Owners cannot restore a hidden public profile by publishing again. Future moderator APIs must authorize the actor and record private audit history; no dashboard/reporting workflow was added.
- Bounded queries, parameterized values, explicit resource cleanup, request deadlines and safe generic API failures were retained. No BOLA/IDOR, open redirect or public DTO leak was found. Global ingress quotas, slug-change throttling and vocabulary abuse controls remain release hardening.

## Database and API review

No migration was added or edited. Migration 000005 retains its complete photo-metadata CHECK, private moderation default, object registry owner FK, profile-object FK, unique active-object index and cleanup indexes. Existing tombstone constraints and the 000004 rollback guard remain intact.

Disposable PostgreSQL databases exercised the complete 000001–000005 apply/rollback/reapply chain. A separate disposable database explicitly exercised both refusal guards: rollback 000005 rejected a pending photo registry entry, and rollback 000004 rejected a deleted-account slug tombstone. These expected errors confirmed that rollback cannot silently discard cleanup intent or reservations. SQL triggers used for fault injection were confined to disposable test databases; no production/development schema trigger was changed.

The transaction-failure test rejects the registry activation update after the profile reference has already changed inside the transaction. The entire transaction rolls back, the prior photo remains live and the newly uploaded staging object can be collected later. Cleanup tests also reject timestamp persistence and confirm durable intent remains.

API routes/statuses are unchanged. OpenAPI now clarifies streamed-body admission; it continues to document 400/401/404/413/415/500-class failures and storage 503 behavior. No 403 response is added to owner-only routes: unauthorized tokens are 401; absent/foreign resources are 404. PostgreSQL errors, storage endpoints and credentials are not serialized. A dedicated full OpenAPI validator was not installed; YAML parsing and contract inspection were executed.

## Storage production assessment (separate from local readiness)

**High production risk, unchanged:** the [community MinIO repository is archived](https://github.com/minio/minio). Its pinned 2025 release receives no assurance of future community security maintenance. There is no demonstrated new exploit in this review, but it should not become a production deployment merely because local tests pass. Provider replacement was explicitly prohibited and was not attempted.

Local controls reviewed: dedicated project service/network/volume, loopback-only host ports 19000/19001, UID 10001, private bucket, fail-closed policy check, readiness healthcheck and environment-based example credentials. The local API uses root MinIO example credentials and plaintext container-network traffic for development. Production must use TLS, dedicated least-privilege identities and managed secret delivery. The adapter currently uses static V4 credentials, so workload identity/temporary credential support needs assessment before production.

Portability assessment (design inference, not a provider integration test): Put/Get/Delete/Head and the domain `PhotoStorage` interface are S3-oriented, so another provider can be substituted without changing the profile domain. Bootstrap is less portable: `objectstorage/minio.go:28–46` auto-creates a bucket and requires an empty bucket policy. [AWS S3](https://docs.aws.amazon.com/AmazonS3/latest/userguide/access-control-block-public-access.html) is a candidate to evaluate with a precreated private bucket, Block Public Access, least-privilege permissions and production credential handling. [Cloudflare R2's compatibility table](https://developers.cloudflare.com/r2/api/s3/api/) documents object operations but marks GetBucketPolicy unsupported; the current bootstrap would fail there. R2 would need an approved provider-specific privacy/bootstrap check, not a silent fallback that skips privacy verification. Neither alternative was configured or tested, and no external provider account was accessed.

Backup/recovery requirements: stop API writes and the cleanup worker for a consistent PostgreSQL/bucket snapshot; protect configuration/secrets separately; restore the matching pair before starting cleanup; verify every active object reference and then resume retries. Restoring unrelated snapshots can leave missing active objects or untracked newer objects. Do not prune unknown objects automatically after a mismatched restore. A disaster recovery restore exercise remains unexecuted and is a production gate.

## Verification evidence

All final checks below were executed during this review, not inferred from earlier reports:

| Command / check | Result |
| --- | --- |
| `go test ./...` | Passed with PostgreSQL and MinIO integration variables enabled |
| `go test -race ./...` | Passed with the same integration services; gcc/musl installed in the disposable Go container |
| `go vet ./...` | Passed |
| `go build ./cmd/api` | Passed |
| `npm run test` | Passed, 8 tests |
| `npm run build` | Passed |
| `npm run lint` | Passed, no warnings |
| Full migration apply/rollback/reapply | Passed in isolated temporary database |
| Rollback refusal guards for pending photos and deleted-account tombstones | Passed in a separate isolated database; both unsafe rollbacks were rejected |
| MinIO/private bucket/object lifecycle tests | Passed, including policy rejection, rollback/retry and live-reference protection |
| `docker compose ... config --quiet` | Passed as the available Compose configuration validator |
| `docker compose ... build minio` | Passed, source SHA-256 verified during build |
| `git diff --check` | Passed |

The Go suites run in `golang:1.27.3-alpine` with `TEST_DATABASE_URL` pointing to a temporary database and `TEST_OBJECT_STORAGE_ENDPOINT/ACCESS_KEY/SECRET_KEY` pointing to the dedicated local MinIO instance. Tests create and remove unique temporary buckets. The temporary database is removed after testing. See the environment instructions in `docs/phase-c-profile-privacy.md` for reruns.

An initial working-tree run overlapped an upload interface edit and vet observed inconsistent source signatures. That result was discarded. The exact reviewed SHA was then tested from a separate `git archive` checkout, and the final modified tree was tested after edits completed. Both exact-baseline and final checks passed.

### Runtime and real browser evidence

The remediated API and checksum-verified MinIO image were rebuilt using AlumniBridge Compose services with existing volumes retained. Final HTTP checks returned `/health` 200, `/ready` 200, an absent public profile 404, and unauthenticated owner privacy/photo endpoints 401. Owner photo upload/delete and visibility changes were checked. Phase B signed-token CRUD/concurrency tests remained passing. The persistent development database remained at migration version 5 with `dirty=false`; the photo registry contained zero objects after synthetic-account cleanup.

Playwright launched local Google Chrome against real Keycloak/web/API services. Disposable Student and Alumni accounts completed registration, onboarding, account/dashboard/settings/onboarding refresh, logout protection, and login restoration. Actual Keycloak bearer tokens exercised profile update, private 404, photo upload, public consent, anonymous profile/photo reads, unverified badges, switching back to private and photo deletion. For the Student account, the harness confirmed a durable pending photo job, restarted the actual API process, and observed that the new worker removed the job. All final flows passed. Synthetic Keycloak/application users were removed; their tombstones intentionally remain.

### Windows and Podman status

Host: macOS (`Darwin`), Docker Desktop. `podman` is not installed; Windows execution and Podman Compose configuration/build/runtime checks were **not executed**. Docker Compose validation and local source builds passed; portable Containerfile syntax does not prove Windows/Podman compatibility.

Before Windows approval: validate with `podman compose ... config`, build/start only this project, run the temporary database/MinIO integration suite using the appropriate reachable host endpoint, and repeat registration/onboarding/refresh/logout plus photo/privacy checks. Do not reset shared volumes to work around migration guards.

## GitHub acceptance status and next actions

| Issue | Status |
| --- | --- |
| #2 | Backend slug uniqueness/history/redirect and public projection pass. Offensive-slug moderation vocabulary still needs product agreement; frontend work is Phase D. |
| #6 | Local backend photo acceptance criteria pass, including signed-JWT ownership, real object storage, limits and failure recovery. Production provider/security and restore validation remain conditions. |
| #7 | Backend consent, per-field privacy, anonymous access and moderation separation pass. Owner/public UI is Phase D; production moderation authorization/audit is future work. |

All issues remain open. No comments, push, merge, PR or provider migration was performed. Review these local fixes before authorizing Phase D; separately approve a supported production storage plan, least-privilege credentials, abuse controls, moderation procedure and backup/restore exercise.

## Files changed

- `apps/api/internal/profiles/http/public.go`: pass the authenticated request stream to upload admission.
- `apps/api/internal/profiles/photo.go`: acquire admission before reading and bound normalized output.
- `apps/api/internal/profiles/photo_test.go`: streamed-input, admission, output-limit and truncated-image regressions.
- `apps/api/internal/profiles/postgres/public.go`: retain retry persistence errors.
- `apps/api/internal/profiles/postgres/public_integration_test.go`: signed JWTs, actor isolation and storage lifecycle regressions.
- `apps/api/openapi/openapi.yaml`: document streamed upload admission.
- `infrastructure/minio/Dockerfile`: verify the pinned source archive checksum.
- `docs/phase-c-profile-privacy.md`: describe upload bounds and checksum verification; link this review.
- `docs/reviews/milestone-3-phase-c-review.md`: review evidence and remaining conditions.

Changes remain uncommitted for user review. HEAD is still the reviewed commit; no commit, push, merge or PR was created during this review.
