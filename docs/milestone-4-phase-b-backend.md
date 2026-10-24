# Milestone 4 Phase B — Privacy-safe directory backend

Date: 2026-10-24 (Asia/Jakarta).
Branch: `feature/milestone-4-alumni-directory`.
Implementation base: `f1a7193` (approved Phase A; master base `1537c88`).
Status: **Reviewed with conditions for Phase C; user approval required before further implementation**. See the [independent Phase B review](reviews/milestone-4-phase-b-review.md). No Phase C/D/E implementation or production approval.

## Scope and architecture

Implemented the [approved directory design](milestone-4-directory-design.md) and [ADR 0002](decisions/0002-directory-discoverability.md): consent schema, authenticated owner discovery settings, anonymous search, bounded filtering, initial cursor pagination, OpenAPI and tests. No React directory pages, verification workflow, external search service or additional runtime infrastructure.

- `internal/directory`: validated search specification, public allowlist entry/page, application service and consumer repository interface. No HTTP/pgx/Keycloak imports. The cursor codec uses Go standard-library AES-256-GCM, SHA-256 and cryptographic randomness.
- `internal/directory/postgres`: one parameterized page statement, with publication predicates before matching and bounded public projection. No per-result profile reads or full history aggregation.
- `internal/directory/http`: strict query parsing, anonymous route, safe JSON errors and cache headers. Context cancellation propagates through the service into pgx; the existing API request deadline remains 10 seconds.
- `internal/profiles/discovery.go`, `profiles/postgres/discovery.go`, `profiles/http/discovery.go`: owner publication choices, small consumer store, transaction and authenticated adapter. Existing identity resolution supplies the actor UUID. `PubliclyVisible` is shared with direct publication; directory adds verified membership and consent requirements.
- `cmd/api`: explicit constructor/route wiring. Authentication architecture, direct public profiles and photos remain compatible.

## Migration 000006

[Up](../infrastructure/migrations/000006_directory_discovery.up.sql) and [down](../infrastructure/migrations/000006_directory_discovery.down.sql) are additive/reversible; migrations 000001–000005 are unchanged.

- `profiles.directory_discoverable BOOLEAN NOT NULL DEFAULT false` and nullable `directory_consented_at TIMESTAMPTZ`.
- CHECK requires PUBLIC plus a timestamp when discoverable, and null timestamp when disabled. Existing PUBLIC profiles remain opted out; no backfill derives consent from visibility.
- `profile_directory_affiliations`: membership PK/FK with cascade deletion, three false-default publication flags, UTC timestamp, and CHECK preventing program/year consent without institution consent. Owner identity is derived through the membership FK, not duplicated in a second owner column.
- Partial C-collation profile slug index matches the eligible page predicate/order. Partial verified membership index supports the per-owner trusted membership lookup. No speculative trigram/full-text or company/title indexes; query-plan/performance work remains Phase C.
- Narrow lifecycle triggers provide atomic privacy and membership withdrawal, described below. No evidence, Keycloak identifiers or public search shadow tables are added.

Rollback removes only directory schema/safeguards/indexes and loses the new consent settings. Downgrade must be paired with an older application that has no directory; do not run the new binary without its schema. Existing permanent slug reservations/tombstones and photo registry are preserved. Reapplying starts discovery disabled again, requiring fresh opt-in.

## API contracts

The executable contract is [OpenAPI 0.4.0](../apps/api/openapi/openapi.yaml).

| Route | Access | Behavior |
| --- | --- | --- |
| `GET /api/v1/directory` | Anonymous, identical projection when a bearer header is supplied | Bounded public search; no Keycloak redirect or login prerequisite |
| `GET /api/v1/users/me/profile/discovery` | Verified JWT, current owner only | Current stored consent/timestamp, eligibility, effective discoverability and verified membership choices with academic flags |
| `PATCH /api/v1/users/me/profile/discovery` | Verified JWT, current owner only | Atomic consent/affiliation replacement with current eligibility checks |

Owner GET initializes a missing profile as PRIVATE with false consent through the existing idempotent profile service. `eligible` means PUBLIC/VISIBLE with trusted verified alumni membership, independently of opt-in; `effective_discoverable` additionally requires stored consent. Internal membership UUIDs appear only in the authenticated owner's choices.

PATCH accepts either/both `directory_discoverable` and `affiliations`. Omitted fields stay unchanged; `affiliations: []` removes academic consent and a supplied array replaces the complete set. Entries require membership ID and all three explicit booleans. Maximum 20 affiliations and 16 KiB JSON body. Reject nulls, unknown/nested unknown fields, duplicate object keys/references, invalid UUIDs, missing booleans and contradictory opt-out/publication choices. Program/year requires institution consent. Never accept actor, verification or moderation fields.

Errors use `{"error":"code"}`: 400 invalid payload/query/cursor, 401 invalid/expired/wrong-audience/missing JWT, non-disclosing 404 foreign/nonexistent membership, 409 ineligible owner/membership or academic publication without discovery, 413 oversized body, 415 wrong media, generic 500 unexpected failure. Directory cancellation/deadline failure maps to safe 503. No 429 is advertised because rate limiting is not implemented. All three new routes set `Cache-Control: no-store` and `X-Content-Type-Options: nosniff` on success and errors, including authentication failures.

## Consent transactions and lock ordering

Discovery PATCH validates at both service and persistence boundaries. Within one transaction it:

1. Locks the owner's existing memberships in UUID order.
2. Locks the owner profile and reads its current PUBLIC/moderation/consent state.
3. Checks current trusted verification and all supplied owner-scoped membership references before replacement.
4. Updates consent/timestamp and replaces academic choices; reads the resulting owner settings before commit.

A foreign/missing reference yields 404 without distinguishing existence. Owned non-verified references yield 409. Failed validation, SQL error or cancellation rolls back the whole change. Opt-out is allowed when no longer eligible. Publication is never enabled implicitly. Concurrent replacements remain bounded to 20 because each request replaces the whole set while holding the profile lock.

The existing Milestone 3 privacy UPDATE acquires the profile row lock. The new `profiles_directory_withdrawal` BEFORE trigger clears discovery, timestamp and all academic consent within that **same SQL statement transaction** whenever visibility is PRIVATE or discovery is disabled. It applies to existing API requests and trusted direct SQL writes, without a second HTTP call or committed transaction. Returning to PUBLIC does not restore discovery. Hidden moderation suppresses reads but retains prior consent; a new opt-in against HIDDEN is rejected.

### Why membership locks precede profile locks

This deliberately refines Phase A's preliminary profile-first order. A membership DELETE already locks its membership tuple; FK cascades can then lock affiliation children. The membership BEFORE trigger locks the owner profile before child deletion. Discovery follows **membership → profile → affiliation** to avoid reversing that lifecycle order. Privacy writes use **profile → affiliation** and do not lock memberships. The review also changed profile editing to lock **user → profile**, consistent with onboarding/account deletion, after reproducing an opposite-order deadlock. The reads used to find owner affiliations do not take membership row locks.

Future bulk verification writers must lock all affected memberships in UUID order before profiles, and use deterministic owner ordering for cross-owner work. Arbitrary privileged bulk SQL can still encounter a database-detected deadlock and must retry the entire aborted transaction; PostgreSQL must never be configured to ignore trigger failures. This is not a reason to weaken withdrawal safeguards.

## Verification revocation and fresh consent

There is no administrative verification service yet. A small `memberships_directory_revocation` BEFORE UPDATE/DELETE trigger therefore enforces withdrawal at the persistence lifecycle boundary, including trusted SQL operators and future services:

- No-op membership updates preserve consent.
- Changed status/role/owner/program/graduation details or deletion remove that membership's academic consent before the change. A later verified state never restores those flags.
- Losing/deleting the **last** verified alumni membership clears stored discovery/timestamp and all academic choices in the same transaction. Reverification cannot reactivate the old listing.
- If another verified alumni membership remains, generic discovery can continue, but the changed affiliation needs fresh academic consent.
- Editing affiliation details while retaining verified status invalidates the changed academic consent without unnecessarily withdrawing generic verified-alumni discovery.
- Outside account-deletion cascades (where the parent user and profile are being removed), the trigger writes the profile timestamp as a serialization point before checking for another verified membership excluding the old row. A lock alone is insufficient for an obsolete REPEATABLE READ snapshot: the profile write forces SQLSTATE 40001 rather than allowing both old snapshots to retain consent. Lifecycle callers must retry the entire transaction with a fresh snapshot. Tests reproduce the lock-only failure and verify rejection plus safe retry. Two memberships revoked concurrently cannot each leave stale discovery assuming the other will remain verified.

Queries still independently require current verification, PUBLIC/VISIBLE, discovery and field consent. A stale consent row on a non-verified membership cannot affect matching or projection. Account deletion retains existing tombstones and object cleanup semantics.

These safeguards are narrowly scoped database functions/triggers, not a new authentication/verification architecture. No real user was promoted. All verified fixtures were created only in the disposable review database.

## Search and privacy rules

- `query`: 2–100-character display-name literal prefix; `company`/`job_title`: 2–160-character published-field literal prefixes. Normalize with Go Unicode simple lowercase and Unicode whitespace collapse on both query and stored values; the SQL adapter uses the same Go-derived translation map under C collation, avoiding libc/locale drift. No accent stripping or expanding folds. Escape LIKE percent, underscore and backslash. No regex/fuzzy/wildcard language.
- `institution`/`study_program`: exact lowercase public reference slugs (1–64); program requires institution. `graduation_year`: 1950–2100. Combined academic filters must match the same consented verified membership. Pending affiliations never borrow verification from another membership.
- Repeated `skills`: up to five exact normalized unique names, all must match and `show_skills` must be enabled. Controls, invalid text and equivalent duplicates are rejected.
- All filters combine with AND. Reject unknown parameters, duplicate scalar keys, explicit empty values, malformed integers, invalid UTF-8/control text and raw query strings over 4096 bytes.
- Default page size 20, maximum 50; empty filters permit bounded browse. Unknown valid references or no matches return `{"items":[],"next_cursor":null}`.
- SQL gates company/title/skills predicates with their publication flags. The academic EXISTS joins verified membership and its consent flags. The limited page projection returns only public core, optional published company/title/skills/photo URL and consented affiliation fields. At most 20 affiliations and 100 skills per entry.
- No internal UUID, email, OIDC subject, evidence, private field, moderation metadata, object key, completeness, total count, facet or private-value autocomplete appears in results. Historical slugs never add rows/matches; only the canonical slug is listed.

## Cursor format and configuration

`DIRECTORY_CURSOR_KEY` is required at native API startup: strict canonical standard-base64 encoding of exactly 32 random bytes. Invalid or missing material fails configuration loading without printing the value. Generate with `openssl rand -base64 32`, or use the Windows generator in [README](../README.md). Persist the value through an approved secret mechanism if cursors should survive restarts. Compose and `.env.example` have an explicitly **development-only** example fallback; CI uses separate all-zero test material only in isolated fixtures.

AES-256-GCM encrypts/authenticates a JSON payload with version, 15-minute expiration, SHA-256 fingerprint of normalized filters/page size/fixed ordering contract, and last returned public canonical slug. Random nonces and a directory-specific associated-data string are used. No internal IDs, raw filter values or private attributes enter the payload. URL-safe unpadded base64 tokens have a 2048-byte maximum; canonical decoding, authentication, version, expiry, fingerprint and slug are validated. All invalid/tampered/expired/filter-mismatched tokens produce the same 400 response. Rotation to a different key invalidates existing cursors; clients should restart browsing.

SQL uses keyset `slug COLLATE "C" > anchor`, fixed C ordering and limit + 1. The extra row determines whether a cursor is returned; the anchor is the last returned row. Deleted/hidden anchors still function as lexical boundaries without an existence lookup. Static data pages are deterministic. Live inserts/renames can cause skips/repeats across requests; no cross-request database snapshot is promised.

Each new statement checks consent at its PostgreSQL snapshot. Requests started after a committed withdrawal exclude it. Already delivered data and a SELECT whose snapshot precedes withdrawal cannot be recalled. Application logs record path/outcome/request ID rather than raw query/cursor/key data.

## Executed verification

Backend ran in Go 1.27.3 Linux/arm64 Docker with GCC/musl for race detection, not a host Go installation. PostgreSQL 17 and the existing pinned MinIO development image ran as **new disposable test containers** on a dedicated test network, with tmpfs data and no shared volumes. `TEST_DATABASE_URL` pointed only to disposable `alumnibridge_m4b_test` / `alumnibridge_m4b_final_test`; storage variables pointed only to the disposable MinIO instance. Frontend checks ran on macOS with Node 22.23.3; CI retains Node 24.

| Check | Result |
| --- | --- |
| `go test -count=1 ./...` | Passed with PostgreSQL and MinIO integrations enabled |
| `go test -race -count=1 ./...` | Passed with those integrations enabled |
| `go vet ./...` | Passed |
| `go build ./cmd/api` | Passed |
| `npm run test` | Passed: 29 tests, six files |
| `npm run lint` / `npm run build` | Passed |
| Migrations 000001–000006 up/down/up | Passed on an empty disposable database |
| Existing PUBLIC fixture + 000006 up/down/reapply | Passed; no implicit discovery, retained tombstone/photo registry |
| 000004/000005 unsafe rollback guards | Rejected tombstone/unfinished-cleanup rollback as expected, inside disposable transactions |
| OpenAPI | Ruby/Psych YAML parse and 103 internal references validated; not a full external OpenAPI semantic validator |
| Compose | `docker compose --env-file .env.example config --quiet` passed |
| Git | `git diff --check` passed; old migration files unchanged |

`directory/postgres/store_integration_test.go` exercises real pgx, generated RSA/JWKS JWT verification, middleware/identity/service/HTTP adapters and SQL. Cases include anonymous/authenticated equivalence; invalid/expired/wrong-audience JWT; student/pending/rejected/no-consent/private/hidden exclusion; stale persisted consent exclusion; foreign/missing membership 404; pending affiliation 409; strict payloads/limits; SQL constraints; injected post-write SQL failure with full rollback and generic errors; same-membership academic AND; hidden company/title/skills/year/institution matching; literal wildcard behavior; exact paired hidden-value response equivalence; multi-page/changed-filter/limit/hidden-deleted-anchor behavior; membership revocation/reverification and simultaneous last-membership loss; repeated privacy/discovery and verification/discovery races; canonical history/tombstones and cancellation. Unit tests additionally cover cursor tampering/expiry/key mismatch, payload allowlist, normalization and strict query/JSON parsing.

A targeted old-snapshot regression initially failed against a lock-only revocation trigger, then passed after adding the profile write serialization point. Full regression then caught an account-deletion FK failure from that write; restricting it to an existing parent user preserved cascade deletion. An initial development run caught a reserved SQL alias (`year`); it was corrected to `grad_year` before final verification. The first disposable MinIO tmpfs had root ownership and failed startup; recreating only that test container with UID/GID 10001 fixed it. Neither failed run is counted as a pass. No live development service/schema was upgraded or reset. No application browser E2E, Windows/Podman or screen-reader test was run in Phase B.

## Reproducing isolated tests

Use a fresh disposable PostgreSQL database, never the active application database. The profile and directory suites install temporary failure-injection triggers. Apply all up migrations, all down migrations in reverse order, then all up migrations before fixtures. Existing tombstone/photo guards intentionally prevent unsafe rollback after fixtures.

After preparing isolated services and applying migrations, set these **test-only** variables for the Go process:

```sh
export TEST_DATABASE_URL='postgres://TEST_USER:TEST_PASSWORD@TEST_HOST:5432/DISPOSABLE_TEST_DB?sslmode=disable'
export TEST_OBJECT_STORAGE_ENDPOINT='TEST_MINIO_HOST:9000'
export TEST_OBJECT_STORAGE_ACCESS_KEY='TEST_ONLY_ACCESS'
export TEST_OBJECT_STORAGE_SECRET_KEY='TEST_ONLY_SECRET'
cd apps/api
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./cmd/api
```

The tests generate their own cursor material and signed JWTs; no live Keycloak credentials or real accounts are needed. Remove only the disposable database/containers/network created for this run. [Windows verification](windows-verification.md) supplies project-scoped PowerShell instructions, reviewed but unexecuted on Windows.

The existing [CI workflow](../.github/workflows/ci.yml) already applies every migration and executes all Go packages with explicit disposable PostgreSQL/MinIO variables. It will discover the new unit/integration tests automatically; the new CI-only cursor key documents mandatory configuration. Local CI-equivalent commands passed. The implementation was subsequently pushed at `3d28c8b`; its exact-SHA workflow_dispatch [run 40066120046](https://github.com/sipratama/alumnibridge/actions/runs/40066120046) passed during review. Additional review remediation remains local and has not run in GitHub Actions. Master branch-protection follow-up #10 remains separate.

## Changed file inventory

- New migration pair: `infrastructure/migrations/000006_directory_discovery.{up,down}.sql`.
- New directory module: `apps/api/internal/directory/directory.go`, `cursor.go`, `directory_test.go`; `http/handler.go`, `http/handler_test.go`; `postgres/store.go`, `postgres/store_integration_test.go`.
- New owner consent: `apps/api/internal/profiles/discovery.go`, `discovery_test.go`, `http/discovery.go`, `http/discovery_test.go`, `postgres/discovery.go`.
- Updated API wiring/policy: `apps/api/cmd/api/main.go`, `apps/api/internal/profiles/http/handler.go`, `apps/api/internal/profiles/public.go`.
- Updated configuration/contracts: `apps/api/internal/config/config.go`, `config_test.go`, `apps/api/openapi/openapi.yaml`, `.env.example`, `compose.yaml`, `.github/workflows/ci.yml`.
- Documentation: this report, `README.md`, `docs/architecture.md`, `docs/prd.md`, `docs/implementation-plan.md`, `docs/milestone-4-directory-design.md`, `docs/milestone-4-directory-plan.md`, `docs/decisions/0002-directory-discoverability.md`, `docs/windows-verification.md`.

## Limitations, issue progress and next review

- [#14](https://github.com/sipratama/alumnibridge/issues/14): approved policy, schema and executable OpenAPI covered; owner UI consent wording is later work.
- [#15](https://github.com/sipratama/alumnibridge/issues/15): backend search/consent foundation implemented and tested; awaits Phase B review.
- [#16](https://github.com/sipratama/alumnibridge/issues/16): initial bounded C-collation keyset/cursor tests implemented. Representative EXPLAIN/load budgets, optimization and broader pagination hardening remain Phase C.
- [#18](https://github.com/sipratama/alumnibridge/issues/18): SQL/HTTP privacy and signed-JWT baseline implemented. Rate limits, global query admission, trusted-proxy/ingress policy, resource measurement and final abuse review remain Phase C. There is no rate limiter or anti-scraping guarantee now.
- Owner GET currently lists the owner's verified memberships; publication is capped to 20 selections, but large private membership-choice catalogs may need pagination as the community model grows. Do not use this endpoint as a public membership list.
- [#11](https://github.com/sipratama/alumnibridge/issues/11) remains OPEN and a production gate. Locks/transactions prevent partial withdrawal or opt-in against an already PRIVATE/HIDDEN profile; they do not reject a stale same-owner update after a newer choice. A stale existing PUBLIC privacy request may still restore visibility under [ADR 0001](decisions/0001-last-writer-wins-development.md). Returning to PUBLIC alone cannot restore directory consent, but a later eligible opt-in can win. No ETag/revision/optimistic concurrency redesign is claimed.
- No verification administrator workflow exists; real directory inventory can be empty. No real pending account should be promoted to make a demo nonempty.
- Directory UI is Phase D and browser search E2E is Phase D/E. Production storage/MinIO maintenance, moderation, consent/legal/retention, TLS/OIDC topology, operational rate controls and backup/restore remain launch gates.

All related M4 issues remain open; no GitHub tracking/comment changes, push, PR or merge is authorized by completing this phase. Stop here for user review before Phase C.
