# Milestone 3 final review and release readiness

Date: 2026-10-24 (Asia/Jakarta).
Reviewed SHA: `f5e11e8512fa6454dc8b7b6cdb2e5d77456ff87b`.
Branch: `feature/milestone-3-professional-profile`.

## Verdict and scope

**READY TO MERGE WITH CONDITIONS as a development baseline. Production: NOT READY.**

No unresolved Critical or High application vulnerability was identified within this review. A confirmed keyboard-focus defect was fixed locally, browser acceptance coverage was extended, and verification documentation was corrected. Atomic same-field conflict detection and automated CI remain conditions requiring maintainer acceptance or follow-up before treating this as an unrestricted release. A development merge does not resolve the separate production gates below.

Before merge:

1. Review and commit the local remediation/report; the remote reviewed SHA does not yet contain these final-review changes.
2. Explicitly accept documented last-writer behavior for this development baseline, or approve a separate concurrency design before merge if conflict prevention is mandatory now. Do not advertise atomic optimistic concurrency.
3. Require the executed checks below as manual PR evidence while CI is absent, or add the proposed minimal CI before merging.

Windows/Podman and actual screen-reader execution remain unverified. They are compatibility/accessibility sign-off tasks before claiming that support or public launch; this review provides exact Windows instructions and a manual accessibility checklist. No production storage migration, destructive database operation, new versioning mechanism or Milestone 4 feature was introduced.

## Repository and review evidence

- Preflight was clean on the expected branch and SHA. `git fetch origin` succeeded. `origin/feature/milestone-3-professional-profile` contained the same full SHA; ahead/behind was `0/0`. `git merge-base --is-ancestor f5e11e8 origin/feature/milestone-3-professional-profile` succeeded.
- Reviewed `git diff origin/master...HEAD`: 77 files, including profile adapters/services, migrations 000002–000005, OpenAPI, frontend/auth integration, storage, tests and documentation. Phase C remediation is in ancestor `6608405`; it was not missing or uncommitted.
- Read AGENTS, [architecture](../architecture.md), [PRD](../prd.md), [implementation plan](../implementation-plan.md), [profile design](../professional-profile-design.md), [Phase B review](milestone-3-phase-b-review.md), [Phase C review](milestone-3-phase-c-review.md), [Phase C decisions](../phase-c-profile-privacy.md) and [Phase D implementation](../phase-d-professional-profile-frontend.md).
- Read current GitHub Issues #2–#8 and their acceptance criteria. All were open. There is no `.github/workflows` directory at this SHA; `gh run list --branch feature/milestone-3-professional-profile --limit 5` returned an empty list.
- This report covers the reviewed commit plus the focused local remediation below. HEAD remains the reviewed SHA, with uncommitted final-review changes. No push, merge, PR, issue closure or GitHub comment was performed.

## Findings and remediation

Line references identify the reviewed SHA unless marked as remediated. Prior Phase B/C findings are historical and are not counted again as newly discovered application vulnerabilities.

| ID | Severity | Evidence | Impact and disposition |
| --- | --- | --- | --- |
| M1 | Medium | `apps/api/internal/profiles/postgres/store.go:114`, `:132`, `:165`, `:212`; `postgres/public.go:59`; frontend `ProfileEditor.tsx:36–46`, `PrivacyPage.tsx:51–62` | Database writes are atomic, but same-field profile/privacy updates and replacement history PATCH do not have an atomic expected-version predicate. Separate frontend GET/PATCH checks cannot close that race. **Open, documented development condition**; no versioning contract was introduced without approval. |
| M2 | Medium | Reviewed `apps/web/src/features/profile/components.tsx:166–173` restores focus only if the invoking element still exists. | Deleting a row removes that trigger, leaving keyboard focus on BODY. **Fixed** by retaining the containing section/photo editor and focusing its surviving action when the original trigger disappears. |
| M3 | Medium | No `.github/workflows` at the reviewed SHA; no branch Actions runs returned. | Regressions, skipped integration tests and migrations have no automatic PR gate. **Open process condition**; minimal CI is proposed below, not silently declared green. |
| L1 | Low | Reviewed `README.md:5`, `:64`, `:74–77`, `:109` describes profile UI as future work, names an absent registration button, and only demonstrates identity integration against the development URL. | Stale setup/verification guidance can mislead reviewers and does not safely exercise the profile fault-injection suite. **Fixed:** accurate UI entry points, explicit isolated-test requirement and a complete project-scoped Windows guide. |
| L2 | Low | Reviewed `apps/web/e2e/phase-d.cjs:225–229`, `:392–398` exercises upload/removal, without actually replacing the image or restarting the API. | Coverage gap, not evidence of broken replacement/recovery. **Resolved:** real UI replacement compares controlled bytes; an opt-in actual API restart verifies durable cleanup, and delete checks its focus destination. |

Application/process totals: **0 Critical, 0 High, 3 Medium (1 fixed, 2 documented conditions), 2 Low (both resolved)**. A separate inherited High production storage-maintenance risk remains open below.

### M1: concurrency assessment and compatibility proposal

Reproduction executed with an actual browser-acquired Keycloak JWT and a disposable Student: concurrent PATCH requests supplied different headlines to the same owner profile. Both returned 200; the final profile retained one draft. The synthetic original headline was restored afterward. Existing PostgreSQL integration tests also re-ran concurrent different-field PATCH and same-slug collisions: unrelated fields both survived, and exactly one user reserved the colliding slug.

Profile initialization, slug reservation/update, collection limits and photo activation use transactions/locks. Experience/education updates are single owner-scoped SQL statements; privacy PATCH is one owner-scoped statement. This prevents partial writes, broken ownership or invalid cross-resource references. It does **not** mean a stale same-field edit is rejected. Experience/education PATCH intentionally replaces editable fields and can overwrite an earlier edit of an omitted optional value. Concurrent photo replacements intentionally select one final live photo and durably collect superseded objects.

The frontend preserves drafts, displays real slug 409s and known preflight conflicts, and submits only dirty profile fields. History editors submit complete replacement payloads. Privacy preflight prevents known stale publication choices from being restored, but another change between that GET and PATCH can still be overwritten. A stale same-owner public choice can win over a concurrent withdrawal; this is not a cross-user authorization bypass, and should be addressed before relying on multi-session revocation guarantees in production.

An atomic safeguard is justified before public multi-session use, particularly for privacy/history. Proposed follow-up: resource-specific server validators and conditional writes checked under the same transaction/row lock, with a frontend reload/merge choice. Optional ETag/If-Match support can preserve current unconditional clients, but it protects only clients that send it. Enforcing preconditions for every write would add 412/428 behavior and require coordinated client/OpenAPI changes; a new revision column would also need an approved reversible migration. No such change was made in this review. Approval must settle the required compatibility policy first.

### M2: focus regression and fix

Reproduce: focus Delete experience, confirm removal, then inspect `document.activeElement`. On the reviewed build, actual Chrome reported BODY for both Student and Alumni because the old trigger was disconnected. Edit cancellation/Escape did return to its surviving trigger.

A new `components.test.tsx` regression failed on the reviewed code, then passed after remediation. Remediated `components.tsx:167–179` retains the nearest section or photo-editor region and chooses its enabled button/link only when the original trigger no longer exists. A disconnected route region is not focused. The real browser rerun asserts that successful experience deletion returns focus to Add experience; cancellation and Escape remain available when a mutation is not pending. Pending mutation cancellation remains intentionally disabled to avoid presenting a canceled write as rolled back.

## Architecture, API and security assessment

- The modular monolith is intact: one Go API, application/domain services and small consumer ports, HTTP/pgx/object-store adapters, constructor wiring, PostgreSQL transactions, and external Keycloak identity. Profile services do not import Chi, pgx or Keycloak. No messaging infrastructure, directory or unrelated module was scaffolded.
- Verified OIDC subjects resolve to internal user UUIDs. Owner/resource IDs from bodies or query hints do not grant access. Owner SQL scopes updates/deletes by both resource and actor where applicable. Signed-JWT integration tests re-ran invalid/expired tokens, cross-user experience/education/skill mutations, forged owner hints and photo read/removal cases.
- JSON input is bounded, object-only and rejects unknown/non-nullable null fields. Profile PATCH preserves omission versus explicit empty strings. History PATCH replacement semantics, nullable dates and collection limits match the current OpenAPI/client. Skills use `query`/`limit`, not the earlier issue sketch's `q`/`cursor`; privacy uses `/profile/privacy`, not `/profile/visibility`. These decisions are already documented and should be reconciled in issue wording before closure.
- SQL values are parameterized. The only interpolated collection identifier comes from an explicit three-table whitelist. Lists/search are bounded and deterministically ordered. Errors reaching API consumers are generic; no SQL error, storage endpoint, credentials or token was observed in responses/browser logs. HTTP logs contain method/path/status/timing/request ID, not request bodies or Authorization headers.
- Public access uses an explicit allowlist and SQL filtering for public consent plus visible moderation. Hidden optional fields are absent. Email, UUIDs, OIDC subjects, academic membership identifiers, evidence, reviewer notes and internal object keys are not public. Verification derives only from trusted verified alumni membership. Pending Alumni and Students remained unverified.
- New profiles are private with all optional flags false. Publishing requires an explicit visibility request; the UI requires an unchecked consent selection when transitioning from saved Private. Moderator hiding and membership verification are independent of owner consent and cannot be assigned through owner inputs.
- Historical slugs remain owned/reserved and deletion leaves permanent tombstones. Only a currently public, visible owner redirects; private/hidden/tombstone URLs return the same 404 without Location. Redirects are relative, canonical navigation is local, and the browser tests found no loop.
- Public/profile/photo requests and HTTP responses use no-store where relevant; images use controlled MIME, inline disposition and nosniff. Public TanStack queries are separate from owner data, have no retention after unmount, revalidate on entry/focus and poll while visible. The UI removes old content during revalidation/denial and preserves geometry with a neutral placeholder. Mutations discard public caches in the same client. Other tabs/downloaded bytes cannot be recalled; the next check denies withdrawn access. This is not a promise of remote memory erasure or instantaneous cross-browser revocation.
- No API route/DTO or migration was changed by the final remediation. Explicit frontend required/optional discoverability and full assistive-technology behavior remain items for manual sign-off; automated testing is not a WCAG certification.

## Photo lifecycle and storage

The four-slot admission precedes body buffering. Input and normalized output are capped at 5 MiB; supported JPEG/PNG/WebP content is decoded, dimension/pixel checked, and re-encoded to strip metadata/trailing content. Limits are 4096 pixels per side and eight million pixels. Decoder memory, encoder buffers and byte-buffer capacity mean this is not a 5 MiB total-RSS guarantee; production memory/ingress budgets still require load measurement. Safe opaque UUID keys are generated server-side and validated by the adapter. Bucket policy checks fail closed; no anonymous direct object access is allowed.

A durable registry precedes upload. Activation, reference replacement and garbage scheduling are atomic. Cleanup rechecks live references under the object lock, bounds work per sweep, retains retry intent and safely handles failed activation/deletion. Storage errors remain generic. Integration exercised normalization/format/size/traversal checks, cross-user access, rollback injection, concurrent replacement, live-reference protection, deleted-account cleanup and retry persistence. Real browser replacement changed the controlled image bytes, and the actual API process was restarted with pending synthetic jobs; its new worker collected them.

Final cleanup confirmed zero synthetic application accounts and zero photo registry objects. The isolated review database and unique storage test buckets were removed. Existing development migration state remained 5, `dirty=false`. Persistent volumes, unrelated resources and the production provider were not reset or replaced.

## Authentication and runtime verification

Read-only inspection of the running Keycloak client confirmed public client `alumnibridge-web`, standard flow enabled, S256 PKCE, sole redirect `http://localhost:5173/`, web origin `http://localhost:5173`, implicit flow disabled and direct grants disabled. A direct authorization request using `/dashboard` as redirect_uri returned 400 with a redirect_uri error, confirming the allowlist was not widened. The frontend still uses the fixed callback and safe session destination state; it does not reconstruct callback URIs from protected paths. Adapter state/nonce handling and API signature/issuer/audience/expiry/type/authorized-party validation remain intact.

Actual Chrome covered Student and pending Alumni registration/onboarding, profile edits, experience/education CRUD, skill creation/reuse/removal, photo upload/replacement/removal, unchecked explicit consent, anonymous public reads, omitted About and private identifiers, no false verified badge, historical slugs, Public-to-Private denial, protected-route refresh, logout protection and interactive login restoring the intended profile. No page error, redirect_uri failure, redirect loop or token leakage was observed. Existing onboarding remains reachable after completion for affiliation management; it is not a mandatory redirect loop.

401 is correct for absent/invalid/expired authentication. Cross-owner resources use non-disclosing 404, not 403. The existing role middleware's 403 path passed its unit tests; Milestone 3 has no administrator HTTP endpoint that should be invented merely to produce a runtime 403.

## Executed checks

All results in this table were executed during this final review. Earlier Phase B/C/D reports are historical evidence only.

| Check | Result |
| --- | --- |
| `go test ./...` | Passed with PostgreSQL and MinIO integration variables enabled |
| `go test -race ./...` | Passed with the same integration services in Linux/arm64 Go container and C toolchain |
| `go vet ./...` | Passed |
| `go build ./cmd/api` | Passed |
| Full migration 000001–000005 apply/rollback/reapply | Passed in isolated `alumnibridge_m3_final_review`; removed afterward |
| PostgreSQL / MinIO integration | Passed; real storage, RSA-signed JWTs, ownership/privacy/concurrency/failure/retry checks enabled |
| `npm run test` | Passed: 29 tests in six files, including new focus regression |
| `npm run build` | Passed, TypeScript and production assets; no bundle-size warning |
| `npm run lint` | Passed, zero warnings |
| OpenAPI syntax and repository documentation links | YAML parsed with Ruby (17 paths); all relative Markdown links resolve, no developer-specific absolute links found. No dedicated response-schema validator was run |
| Docker Compose config validation | Passed using `docker compose -p alumnibridge --env-file .env.example config --quiet` |
| Real Chrome acceptance | Passed both roles, including actual replacement, delete focus and intended destination restoration |
| Actual API restart with pending photo cleanup | Passed; new worker removed the scoped synthetic jobs |
| Concurrent same-field signed-token PATCH probe | Both returned 200 and one draft survived, establishing M1 rather than claiming conflict detection |
| `/health`, `/ready`, MinIO `/minio/health/ready` | 200; API readiness currently checks PostgreSQL, not every external dependency |
| Keycloak invalid redirect allowlist check | Rejected with 400 |
| `git diff --check` | Passed |
| GitHub Actions | No workflows/runs available; **not reported as passed** |
| Windows / Podman / real screen reader | Not executed; host/tooling limitations described below |

Go ran in `golang:1.27.3-alpine` with gcc/musl, using a temporary PostgreSQL database and `host.docker.internal:19000` for the dedicated MinIO instance. No Go source changed after that successful suite. Frontend checks were rerun after the focus correction. Services used the existing AlumniBridge Docker project; only web was rebuilt and API restarted. The application dependency/lockfiles were unchanged.

Browser commands used Playwright 1.64.1 outside the repo and installed local Google Chrome. The committed harness now reproduces the acceptance checks and opt-in restart test:

```sh
# Local example values only, from the repository root with the stack running.
PLAYWRIGHT_MODULE=/tmp/alumnibridge-browser-review/node_modules/playwright \
CHROME_EXECUTABLE='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
KEYCLOAK_ADMIN_USERNAME=localadmin KEYCLOAK_ADMIN_PASSWORD=local_admin_only \
VERIFY_API_RESTART=true node apps/web/e2e/phase-d.cjs
```

The final run additionally used a temporary wrapper with axe-core Playwright 4.10.2 and the signed-token concurrency probe. No audit/tool dependency was added to the executable app. Baseline runs exposed the focus defect; the new regression was observed failing before correction. A first modal keyboard assertion incorrectly treated browser-chrome/BODY focus as an inert page control; it was corrected to check that no outside page control receives focus. That interrupted development run was not counted as a pass.

## Accessibility findings and limitations

Axe checked WCAG 2 A/AA and 2.1 A/AA rule tags in six actual browser states: invalid editor, owner desktop/mobile, privacy, public desktop/mobile. It reported **zero violations** in those scans. Five page states had no incomplete checks; the invalid editor had one incomplete contrast rule affecting two nodes (slug help and About textarea) due to overlap/occlusion detection. Those two results remain **incomplete**, not passed. No actual screen-reader speech/braille interaction was executed: the available automation is headless Chrome/DOM/axe, not a connected VoiceOver/NVDA session.

The keyboard review covered native dialog focus containment relative to inert page controls, Escape, cancellation, surviving-trigger restoration and the corrected deletion fallback. Responsive checks covered 1440px desktop, 390px mobile and owner reflow at 320 CSS pixels without horizontal overflow. Labels, error descriptions, status/alert regions, skip links, image alternatives and semantic landmarks were inspected in source and browser. Recent success feedback is announced; polling hides stale content without repeating a loading live announcement. Real delayed polling retained the mobile reading position within two pixels.

The following heuristic skill audit is evidence-scoped, not a compliance score:

| Dimension | Score / 4 | Evidence / limitation |
| --- | --- | --- |
| Accessibility | 3 | Automated states and keyboard checks pass after fix; actual AT and two incomplete contrast nodes remain |
| Performance | 3 | Lazy route/form bundles, bounded API collections and canceled photo reads; production load not benchmarked |
| Responsive design | 3 | Tested 1440/390 and 320 reflow; physical touch and text zoom still need manual checks |
| Theming | 1 | Coherent existing palette, predominantly hardcoded CSS; dark mode/token redesign is not required for this milestone |
| Implementation integrity | 4 | Existing identity preserved, no invented public fields, detector returned `[]` |
| Total | 14 / 20 | Good within the inspected scope; no WCAG compliance claim |

### Manual accessibility checklist

- Use NVDA with Chrome on Windows and VoiceOver with Safari/Chrome on macOS. Read member navigation, page headings, visibility state and verification labels in order.
- Open each editor from the keyboard; ensure its name and fields are announced, Tab/Shift+Tab cannot focus inert page controls, Escape/Cancel return to the invoker, and deleting an entry returns to the section action. Confirm pending-write behavior is understandable.
- Submit empty/invalid required fields, invalid slug and reversed dates; verify first-error focus, associated messages, required-field discoverability and preservation of drafts after a server error.
- Check unchecked publication consent and every optional field control. Confirm saving Private announces success and public refresh never announces or reveals an old profile after denial.
- Inspect the slug help/About textarea contrast and clipping in the invalid editor, including scrolling its modal; do not mark the axe incomplete checks as passed without examining them.
- Test 200% text zoom and 400% browser zoom/reflow, visible focus, high-contrast/forced-colors mode, mobile touch targets and reduced-motion preference. Existing 36–40px controls should be checked for usable spacing; no unsupported WCAG failure is inferred solely from being smaller than a 44px recommendation.
- Verify image/fallback alternatives, live success/error announcements, and that routine public polling preserves reading position without noisy status repetition.

## Windows and Podman

Host is macOS (`Darwin`), using Docker Desktop. `podman` is absent; no Windows/Podman execution passed. Existing native PowerShell setup and the Windows file-source migration path were inspected, not executed on that OS. The reviewed MinIO Dockerfile uses ordinary ADD/RUN/checksum commands, but source inspection and Docker Compose validation do not establish Podman build compatibility.

Exact project-only PowerShell steps are in [Windows verification](../windows-verification.md): machine inspection, Compose validation, required services, non-destructive migration startup, dependency health, isolated migration/integration/race checks, frontend checks, real Chrome acceptance and safe scoped cleanup. Do not modify shared machines/volumes or broaden Keycloak redirects to make a failing test pass.

## CI assessment and minimal recommendation

There is no automatic backend/frontend gate in this branch, and no Actions run was available to validate. Proposed minimal follow-up, with read-only repository permissions and pinned action revisions:

1. On pull requests, use the Go version from `apps/api/go.mod`; run format verification, `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./cmd/api`.
2. Start disposable PostgreSQL and the dedicated private object store. Apply/rollback/reapply migrations in an empty temporary database, then explicitly set all `TEST_DATABASE_URL`/storage variables before Go tests. A green unit job that silently skips integration is insufficient. Keep fault-injection triggers out of a shared database.
3. Use Node 24, `npm ci`, `npm run test`, `npm run build`, `npm run lint`; fail on lint warnings when appropriate for the chosen linter invocation. Validate Compose configuration and whitespace.
4. Add a separately scoped browser job against disposable Keycloak/API/web services once runtime cost is accepted. Use synthetic users, pinned test tooling and cleanup in `finally`; no production secrets/provider account is required.
5. Mark the actual jobs required after their first verified runs. This proposal was not implemented or claimed green during this final review.

## Production readiness: separate assessment

**NOT READY FOR PRODUCTION.** The [community MinIO repository](https://github.com/minio/minio) is archived/read-only; its maintenance status was verified during this review. `infrastructure/minio/Dockerfile:5–6` pins the source revision and verifies its archive digest, but mutable base tags/packages and the archived server still require an approved production storage/security plan. No new exploit was demonstrated, and no provider was replaced automatically.

The `PhotoStorage` port and object operations are S3-oriented. Provider portability is a design assessment, not an executed external-provider test. The adapter also creates/checks buckets and requires an empty bucket policy (`objectstorage/minio.go:28–46`). [AWS S3](https://docs.aws.amazon.com/AmazonS3/latest/userguide/access-control-block-public-access.html) is a candidate with a precreated private bucket, Block Public Access, least-privilege credentials and reviewed identity handling. [Cloudflare R2](https://developers.cloudflare.com/r2/api/s3/api/) currently lists GetBucketPolicy as unsupported, so the existing bootstrap cannot be assumed to work there; an approved provider-specific privacy check is required, not a silent bypass. Neither was configured or tested.

Other launch gates: TLS and exact production OIDC origins; dedicated non-root storage credentials/managed secrets rather than local example values/static root keys; network isolation, ingress quotas/rate limits and measured memory budgets; atomic concurrency policy for privacy/history; approved offensive-slug/content moderation policy with authorized audit procedures; recovery/retention/consent wording; dependency/security maintenance and a consistent PostgreSQL-plus-object-store backup/restore exercise. API `/ready` currently covers PostgreSQL only; storage health/retry alerts and cleanup backlog monitoring need operational review. Windows/AT sign-off remains separate from Linux-container correctness.

## GitHub Issues #2–#8

| Issue | Acceptance assessment | Executed evidence / remaining work | Suggested closure status |
| --- | --- | --- | --- |
| #2 Profile/slugs | Owner profile, completeness, collisions, permanent reservations and historical public navigation satisfied; inappropriate-slug vocabulary still unspecified | Go domain/SQL/concurrency tests, signed-token owner tests and real browser rename/history. M1 same-field behavior documented | Keep open until the vocabulary criterion is resolved or explicitly split into launch follow-up |
| #3 Experience | CRUD, ownership, dates, limits, deterministic ordering and frontend integration satisfied | PostgreSQL/signed-JWT tests; real Student/Alumni add/edit/delete/current toggle. Replacement updates retain M1 limitation | Eligible for closure after merge if documented concurrency contract is accepted |
| #4 Education | CRUD and optional dates, independent membership verification, ownership satisfied | Go/SQL/date tests and browser add/edit/delete; pending Alumni remain pending | Eligible after merge with the same concurrency condition |
| #5 Skills | Normalization/reuse, bounded search, duplicate prevention and owner-only removal satisfied | SQL/HTTP tests and browser shared skill reuse/removal. Implemented contract is `query`/`limit`, without cursor | Eligible after merge once issue sketch is reconciled with approved OpenAPI |
| #6 Photo | Content/size/dimension/key safety, controlled delivery, replacement, deletion and durable recovery satisfied for local development | MinIO integration/failure/concurrency tests; actual UI byte-changing replacement; actual API restart cleanup | Eligible after development merge; track archived-provider/production credentials/recovery as explicit release follow-up |
| #7 Privacy | Private default, explicit consent, field omission, role-derived badge, hidden/private denial and safe slug history satisfied | SQL privacy matrix, signed-token tests and anonymous browser transitions. Cross-session atomic revocation remains M1 | Eligible after merge only with documented M1 acceptance/follow-up; production stronger guarantees remain gated |
| #8 Frontend/integration | All implemented profile flows verified on local Chrome/macOS with Linux backend containers; focus issue remediated | 29 frontend tests, backend suite, actual Student/Alumni browser acceptance and six axe states. Windows/real AT and CI absent | Keep open pending final user acceptance or explicit separation of those verification tasks |

All seven issues remain open. Suggested closure is advice for the maintainer, not an action performed here.

## Files changed during final review

- `apps/web/src/features/profile/components.tsx`: deletion focus fallback.
- `apps/web/src/features/profile/components.test.tsx`: failing-before/passing-after focus regression.
- `apps/web/e2e/phase-d.cjs`: real byte-changing replacement, delete-focus assertion, opt-in real API restart recovery.
- `README.md`: accurate shipped UI/registration copy and isolated test guidance.
- `docs/windows-verification.md`: exact PowerShell steps and safe disposable resource scope.
- This report, plus cross-references in milestone documentation.

Backend code, applied migrations, OpenAPI routes/DTOs, dependencies/lockfiles and Keycloak configuration were not changed. Documentation screenshot links were checked as repository-relative; prior [Phase D captures](../screenshots/phase-d/README.md) remain historical synthetic UI evidence. Final review additionally inspected fresh browser captures outside the repository; no developer-specific screenshot link is required.

## Recommended pull request

Title: **feat(profile): complete Milestone 3 professional profiles**

Suggested description:

```markdown
## Scope
- Complete owner profile, experience, education, skills, photos and privacy UI/APIs.
- Add consented anonymous profiles, safe historical slugs/tombstones and trusted verification badges.
- Include durable photo cleanup and final-review keyboard focus remediation.

## Verification
- Go tests/race/vet/build passed with PostgreSQL and MinIO integration enabled.
- Migration apply/rollback/reapply passed in an isolated database.
- Frontend: 29 tests, build and lint passed.
- Real Chrome Student/Alumni acceptance passed, including photo replacement,
  privacy withdrawal, original destination restoration and API restart cleanup.
- Automated axe scans: no detected violations; one incomplete contrast rule
  in the invalid editor remains for manual inspection. No WCAG claim.

## Conditions
- Accept documented same-field last-writer behavior for this development baseline
  or approve atomic preconditions before merge.
- Use attached manual check evidence while automated CI is absent.
- Windows/Podman and actual screen-reader verification remain unexecuted.
- Production storage, secrets/TLS, abuse controls, moderation and recovery are gated.

Refs #2, #3, #4, #5, #6, #7, #8.
See docs/reviews/milestone-3-final-review.md.
```

No PR was created. Review stops here; the maintainer decides whether to approve the stated development merge conditions.
