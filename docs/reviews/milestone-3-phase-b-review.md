# Milestone 3 Phase B review

Review date: 2026-10-23 (Asia/Jakarta). Reviewed commit: `c3045482c87b4306144b59f543af88371ef2c910`.
Branch: `feature/milestone-3-professional-profile`; clean at preflight, five commits ahead of origin/master, no divergence.

## Stage 1 prioritized findings

- High: `000002_profiles.up.sql:31` cascades slug history deletion with user deletion. Previously used slugs can be reassigned; this violates the newly confirmed lifetime reservation policy.
- High: `profiles/service.go:140` reads and merges a complete profile before the persistence transaction. Concurrent PATCH requests can lose unrelated updates. `profiles/postgres/store.go:111` locks only after this stale merge.
- Medium: current slugs and historical reservations have independent uniqueness constraints, with no database guarantee that the current slug belongs to the same user.
- Medium: HTTP decoding uses a truncated reader and prefix MIME matching; `null` values silently act as omitted fields. Body limits and JSON shape need explicit handling.
- Medium: skill normalization uses a byte limit despite rune-based API limits, silently strips controls, and accepts invalid UTF-8/NUL in other text fields until PostgreSQL rejects it.
- Low: experience/education PATCH is documented replacement semantics rather than partial updates; this is intentional but requires explicit client awareness.
- Low: React lint warnings originate from form `watch` and a module mixing provider and hook exports.

Scope: complete Phase B diff, identity/auth wiring, migrations, HTTP contracts, frontend auth/onboarding regression checks, GitHub #2–#5. No public profile, visibility editor, photo, or new profile UI work.

Stage 2 results and final recommendation follow after verification.

## Stage 2 remediation and final assessment

Verdict: **READY WITH CONDITIONS**. All confirmed High findings and Phase B Medium findings were remediated. Review the new migration and preserve the documented replacement PATCH contract before building clients. Public projection policy, offensive-slug moderation policy, photo constraints/storage and production rate limits remain later decisions. No issue was closed and no GitHub comment, push, merge or PR was created.

| Severity | Finding / evidence | Disposition |
| --- | --- | --- |
| High | Account deletion releases historical slugs (`000002_profiles.up.sql:31`). | Fixed by `000004_slug_reservations.up.sql:2`: nullable owner with `ON DELETE SET NULL`, immutable reservations, no deletion/reassignment. Tests delete an owner and reject reuse. |
| High | Stale full-profile merge loses unrelated concurrent PATCH fields (reviewed `profiles/service.go:140`; `postgres/store.go:111`). | Fixed: service supplies a pure domain transformation, applied to the current row under `FOR UPDATE OF p,u` (`profiles/service.go:132`, `postgres/store.go:106`). Concurrent headline/location changes both survive. |
| Medium | Separate slug/history keys allow inconsistent owners through SQL. | Fixed: composite owner FK plus immutable reservation trigger (`000004_slug_reservations.up.sql:7`). Existing inconsistent records fail migration rather than changing ownership silently. |
| Medium | Truncation-based decode, MIME prefix checks, null/object ambiguity (`http/handler.go:52`). | Fixed: bounded full-body read, exact parsed MIME, JSON object required, non-nullable null rejected, request IDs rejected, 413/415 responses. Nullable education dates and experience end date remain clearable. |
| Medium | NUL/control/Unicode handling can produce database errors or inconsistent length rejection (`service.go:85`, `service.go:214`). | Fixed: validate UTF-8/NUL, reject control-bearing skill names, count normalized name runes instead of bytes. |
| Medium | API has no overall request deadline; body reads can linger (`cmd/api/main.go:54`). | Fixed: ten-second context deadline in `httpserver/server.go:20`; server read/write deadlines. Database work receives the canceled request context. |
| Medium | OpenAPI response `allOf` combines a closed input schema with an added ID, rejecting valid responses. | Fixed: explicit response properties for experience/education; nullable dates, body limit and media responses documented in `openapi/openapi.yaml`. |
| Low | Experience/education PATCH is replacement, not partial merge. | Retained documented contract: required fields must be supplied; omitted optional fields clear. Profile PATCH alone is partial. Clients must follow this contract. |
| Low | React warnings: `watch` and mixed exports. | Fixed with `useWatch`, separate `authContext.ts` and pure state initialization for maximum year. No rules disabled. Final lint has zero warnings. |
| Low | Deferred photo metadata CHECK uses SQL NULL semantics and may accept partial metadata. | Unreachable through Phase B APIs. Strengthen it in Phase C before enabling photo writes; no photo features were added here. |

No Critical findings. Two High, five Medium, and three Low findings; two Low items remain documented for client/Phase C work. No BOLA/IDOR or SQL injection was found in the implemented owner operations. SQL values are parameterized; the collection table identifier is selected only from a hardcoded whitelist. DTOs contain the owner's editable fields and resource IDs, not OIDC identifiers, email, academic proof, or audit metadata. Public skill search exposes shared skill names by existing contract, not member associations.

## Verification evidence

Baseline: `go test ./...`, `go vet ./...`, API build and frontend test/build passed at the reviewed SHA; baseline lint emitted the two reported warnings. Baseline database tests require an explicit database URL and are skipped by a plain Go run.

Final backend checks ran in `golang:1.27.3-alpine` against the isolated `alumnibridge_review_test` database, using `TEST_DATABASE_URL`; gcc and musl headers enabled race detection:

- `go test ./...`: passed, including identity and profile PostgreSQL integration tests.
- `go test -race ./...`: passed with those same integration tests enabled.
- `go vet ./...` and `go build ./cmd/api`: passed.
- Full migration chain 000001–000004 applied, rolled back, and reapplied in the temporary database. The 000004 down migration also explicitly refused an unsafe rollback after tombstones were created. Temporary database removed after testing.
- New `postgres/review_integration_test.go` uses generated RSA-signed JWTs and a test JWKS endpoint through the actual JWT verifier, middleware, identity provisioning, HTTP handlers and pgx adapter. Valid, invalid and expired tokens, authenticated CRUD, unknown owner input, null input, cross-owner item modifications, same-slug concurrency, unrelated concurrent profile patches, rapid slug changes, raw SQL ownership constraints, and deleted-account tombstones were exercised.
- Unit/handler regression tests cover body size, null rejection, reserved/Unicode/length slug boundaries, control characters, Unicode skill lengths, date rules and completeness.
- Frontend: `npm run test` passed (8 tests), `npm run build` passed, `npm run lint` passed with zero warnings.
- OpenAPI YAML parsed successfully; response schema review corrected the closed-schema composition defect. A dedicated OpenAPI validator was not available.
- `git diff --check`: passed.

Runtime rebuild used only AlumniBridge Compose services and retained all volumes. Migration status is version 4, dirty=false. HTTP smoke checks: `/health` 200, `/ready` 200, `/api/v1/auth/me` 401 without token, `/api/v1/users/me/profile` 401 without token, skill search with limit 51 returned 400. No unrelated Podman/Docker resources were touched.

### Actual browser interactions

Playwright launched local Google Chrome headlessly against the rebuilt local services. Both disposable Student and Alumni accounts completed registration, onboarding, account refresh, dashboard/settings aliases and their refreshes, onboarding refresh, logout and a protected-page revisit, and interactive login restoring the intended account page. No redirect_uri error or loop occurred. Both Keycloak and application test accounts were removed afterward; slug tombstones intentionally remain. These browser checks used local Mac Docker services, not the separate Windows Podman workstation. There is no profile UI in Phase B; profile APIs were tested through the signed-JWT HTTP integration tests instead.

The first browser harness expected an anonymous account revisit to stay at `/account`; it instead reached the public root after `check-sso`. The harness was corrected to assert that protected content is absent, sign-in is available, and login restores the stored destination. Both final flows passed. No application change was needed for this expectation.

Windows reproducibility: start the documented Podman services and Vite/API; register a Student and Alumni; complete onboarding; refresh `/account`, `/dashboard`, `/settings`, and `/onboarding`; sign out; revisit `/account` and confirm no account details; sign in again and confirm the destination restores. Ensure `VITE_OIDC_CALLBACK_URI` and Keycloak's exact allowlist both use `http://localhost:5173/`.

## GitHub acceptance classification

| Issue | Classification | Evidence / remaining work |
| --- | --- | --- |
| #2 Profile foundation and slugs | Partially completed; public reads require Phase C | Owner editing, completeness, uniqueness, concurrency and permanent reservations pass. The issue also mentions inappropriate-slug policy, which still needs an agreed moderation denylist. Public redirects are not exposed yet. |
| #3 Work experience | Completed for Phase B; public projection requires Phase C | CRUD, ownership, date consistency, deterministic order and collection cap verified. |
| #4 Education | Completed for Phase B; public projection requires Phase C | CRUD, external institutions, ownership and dates verified; membership tables are never mutated. Existing institution FK reuse remains optional and is not inferred from an ambiguous name. |
| #5 Skills | Completed for Phase B | Canonical lowercase/space normalization, duplicate reuse, owned links, bounded prefix search and limits verified. Final contract uses `query`/`limit`, without cursor pagination. |

## Remaining conditions and risks

- 000004 is a minimal reservation integrity migration; 000002/000003 were not edited. Review before shipping elsewhere. It may scan existing profiles/history and fails on conflicting ownership. Rollback refuses tombstones to avoid data loss; perform any future policy migration explicitly instead of deleting them.
- Public profile reads, redirects, privacy controls and photos remain unimplemented. Keep them disabled until Phase C tests every public field allowlist and strengthens photo metadata constraints.
- Operational abuse controls (including slug-change rate limits), global skill vocabulary growth and production HTTPS configuration need release hardening. Owner lists remain limited to 100 and prefix search to 50; no unbounded owner list is exposed.
- Generic persistence errors remain private 500 responses. No sensitive SQL errors are logged or returned. A production error observability policy remains operational follow-up.
- Browser evidence covers Chrome on this Mac only; Windows and other browsers require repeating the checklist. No frontend professional profile UI was introduced.

Changed files: profile service/store/HTTP adapters and regression tests; HTTP server/main timeout configuration; migration 000004 up/down; OpenAPI; auth provider/context and import sites/tests; onboarding form; professional profile design and this review report. The original reviewed commits were preserved. All review remediation remains local for user review.
