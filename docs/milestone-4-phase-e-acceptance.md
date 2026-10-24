# Milestone 4 Phase E — Integrated acceptance

Review date: 2026-10-24 (Asia/Jakarta). Branch: `feature/milestone-4-alumni-directory`. The Phase E browser and application review tested `e1a432efc2fb7508762c1b6d05686780b5af1174`; the fetched remote feature branch matched that SHA at the time. The Phase E documentation and screenshots were subsequently committed at `5bb849e43dee3dcaff73121e0238769541167070`. A local revalidation implementation commit, `3d09a2731ac30507b4ef74e1a3b3c3eea63ea7ee`, and this report amendment follow it. The final remediation HEAD is the commit containing this amended report; its exact SHA is recorded in the final delivery report because a commit cannot contain its own SHA. No code, schema, dependency, or OpenAPI correction was needed during the original Phase E execution.

## Recommendation

**READY WITH CONDITIONS for Milestone 4 PR review.** The development baseline passed the integrated checks below, including four-job CI at the original application SHA and later at the Phase E documentation SHA. Merge approval still requires maintainer review and CI on the final remediation SHA after it is pushed, plus PR CI. Windows/Podman and a real screen-reader session remain unverified; do not claim those platforms or WCAG compliance. Production approval is separate and blocked by Issue #11 and the operational gates listed below.

No new Critical or High defect was found. The existing last-writer-wins privacy/consent behavior was confirmed as Issue #11's High production risk; it was not changed in this phase. No Milestone 5 work was started.

## Stack and isolation

The macOS/OrbStack Docker test stack used project `abm4e` with its own PostgreSQL 17 and Keycloak PostgreSQL databases, Keycloak 26.7.0, project MinIO, current Go API and compiled React frontend. Browser origins were `http://localhost:15173` for web and `http://localhost:28085` for Keycloak; API was on `18081`. The realm was imported from a temporary copy with exactly one callback `http://localhost:15173/`, the corresponding web origin, public client, standard code flow and S256 PKCE. An additional temporary API process on `18082` tested search admission separately. The test databases were `alumnibridge`, `abm4e_test` and `abm4e_migration_test` inside the `abm4e` PostgreSQL container. All ports, containers, network and volumes were distinct from the running `alumnibridge` development project. Synthetic test identities, verification changes, photos and SQL fixtures existed only in the disposable stack.

The running disposable Keycloak Admin API returned the sole redirect URI `http://localhost:15173/`, web origin `http://localhost:15173`, standard flow enabled, implicit/direct grants disabled and PKCE `S256`. A deliberately invalid `/evil` callback returned HTTP 400 with `Invalid parameter: redirect_uri`. Browser authorization requests contained that exact root callback, a state value, a nonce and `code_challenge_method=S256`; normal Keycloak callbacks succeeded. The browser used the unmodified `check-sso` adapter. No credentials, access tokens or private keys were stored in the repository. After testing, only `abm4e` containers, network, disposable volumes and images were removed; the six pre-existing `alumnibridge` services and their persistent volumes remained running.

## Browser identity and consent evidence

Headless Chrome on macOS registered disposable Student and Alumni accounts through Keycloak, completed both onboarding forms, reached `/dashboard`, and stayed authenticated after page reload. The Alumni membership began `pending`. A third account registered and onboarded through the browser; only a direct fixture update in the disposable PostgreSQL database marked its membership `verified`. Direct navigation to `/settings/privacy` restored that destination after Keycloak login. Signing out and then visiting the protected route showed the sign-in state. Revoking the disposable Keycloak session through the Admin API and refreshing the protected page also showed the sign-in state. An already issued access token may remain valid until expiry; immediate backend token revocation is not claimed. Real browser waiting through access-token expiry was not executed; signed expired-JWT rejection is covered by Go tests.

The verified owner completed this browser sequence, with SQL and anonymous API checks after each material transition:

| Action | Observed result |
| --- | --- |
| Explicitly publish profile | `visibility=public`, discovery still false; absent from Directory |
| Opt in | Anonymous Directory includes its canonical slug |
| Enable institution, program and year | Returned and matched only when all respective flags were consented |
| Withdraw year | Year filter stopped matching |
| Withdraw institution | Dependent program/year choices cleared; institution filter stopped matching |
| Opt out | Listing disappeared; timestamp became null and affiliation consent was deleted |
| Opt in again, then make PRIVATE | Listing disappeared; direct public profile and photo access returned 404 where applicable; discovery was cleared in the same persisted transition |
| Return to PUBLIC | Directory stayed off until a new explicit opt-in |

Two independent Keycloak browser sessions exercised an edit conflict: session B kept its checked draft, session A saved opt-in, and B received the existing preflight conflict message without overwriting A. Removing a membership before a saved draft was submitted also produced a conflict and preserved the draft. This is a client-side check; Issue #11's atomic stale-write rejection remains absent. Two simultaneous valid discovery PATCH requests both returned 200 and the final value reflected the last write, confirming the documented development behavior. With a Directory page open, revoking the last verified membership removed the old card during a delayed focus refetch and kept it excluded after completion. Reverification did not restore consent. PUBLIC-to-PRIVATE during an open Directory page also masked the old card, cleared consent and made the direct profile return 404. A further browser run injected PATCH 400, 401, 404, 409, 503 and network failures: each preserved the unsaved draft and showed its corresponding error. A real successful PATCH followed by an intercepted GET failure retained the saved result and success message while separately reporting the refresh failure.

The actual Keycloak Student access token returned owner discovery settings with no verified choices. Student opt-in returned 409, an attempt to update another owner's membership returned 404, and missing/invalid bearer tokens returned 401. These browser/API observations complement the generated RSA-signed JWT tests in Go.

## Anonymous Directory and privacy matrix

Disposable records represented Student, pending and rejected Alumni, verified without opt-in, opted-in verified, private verified, moderator-hidden verified, and a verified account with an additional pending academic membership. Anonymous and signed-in browser sessions used the same public search endpoint; the signed-in Directory request sent no bearer token. The Student and pending/rejected profiles remained directly shareable while PUBLIC, without a verified badge or Directory inclusion. The verified account without consent remained directly shareable and correctly showed its server-derived badge, while remaining absent from Directory. PRIVATE, HIDDEN, non-opted-in and unverified records did not appear. HIDDEN and PRIVATE direct profile reads returned 404.

Real API and browser tests covered name/company/job-title prefixes, same-membership institution/program/year, two exact skills including `Design, research`, Unicode/whitespace normalization, unknown references, duplicate/invalid filters, canonical `/in/:slug` navigation, shareable URL/back/forward behavior, and 20+2 cursor pages with Next/Previous. With both later-page profiles withdrawn, the browser showed its changed-page state and returned safely to the first page. A pending affiliation on a verified account did not match academic filters. A published profile with hidden company/title did not match those filters. Public Directory JSON contained no email, internal/user/membership IDs, OIDC subject, evidence or moderation fields. A historical slug redirected to the new canonical profile; Directory emitted only the canonical slug. Both old and new direct links returned 404 after PRIVATE withdrawal. A deleted-account tombstone returned 404 and could not be reassigned. Tampered/invalid cursor handling and expired cursor behavior also have existing Go regression coverage; an actual 15-minute browser expiry was not waited out.

An authenticated browser uploaded a synthetic 400×400 PNG. After explicit photo publication, the anonymous photo endpoint returned 200; withdrawing the photo field returned 404. The Directory's fallback without a public photo was observed. The existing MinIO integration suite covers replacement, deletion and cleanup transitions; this Phase E browser run did not repeat every photo mutation.

Screenshots contain only synthetic public fixtures: [desktop search](screenshots/milestone-4-phase-e/directory-desktop.png) and [mobile search](screenshots/milestone-4-phase-e/directory-mobile.png). Test scripts and credentials were kept under `/tmp/ab-m4e`, outside version control.

## Abuse, accessibility and platforms

Twenty-five rapid anonymous searches with different `X-Forwarded-For` values yielded ten 200 responses and fifteen 429 responses, with `Retry-After: 1`; the direct peer remained one rate-limit identity. This is a per-process token bucket. A separate API process with search concurrency one returned 503 `directory_overloaded` for the second concurrent request and 503 `directory_database_timeout` for the blocked first request under a disposable database lock; `/health` stayed 200. A canceled browser-style fetch raised `AbortError`, and a subsequent search returned 200. The dedicated search connection budget and statement timeout are also covered by Go integration tests. This is a smoke test, not production throughput proof. Proxy ingress must enforce a trusted source-IP policy and aggregate/distributed budgets before a public launch.

Real Chrome focus after a search and keyboard pagination moved to the results heading. The first Tab reached the skip link. Enter opened and closed mobile filter disclosure with `aria-expanded` updated. Axe scanned the real Directory and authenticated settings at 320, 390, 768 and 1440 px: zero reported violations and no horizontal overflow at each width. A delayed focus refetch hid old cards until current results arrived. An interactive VoiceOver session was unavailable in this headless execution environment. Screen-reader announcements, full tab order, confirmation focus restoration and WCAG conformance remain **NOT VERIFIED**. A manual reviewer should use VoiceOver with Safari/Chrome, navigate every filter, pagination control, consent checkbox and confirmation dialog by keyboard, and check spoken loading/error/success updates and focus after close/cancel.

The host was macOS (`Darwin`) with Docker/OrbStack; `podman` and Windows PowerShell were unavailable. [Windows verification steps](windows-verification.md) were reviewed and now include anonymous Directory and `/alumni` smoke commands. **Windows/Podman execution is NOT VERIFIED.** A Windows reviewer must run those commands on the intended workstation before claiming platform support. No existing development container, image, network, database, volume, Keycloak account or membership was changed.

## Regression and CI

The table below records the original Phase E execution at application SHA `e1a432e`; the later local remediation checks are recorded in the addendum.

| Check | Actual result |
| --- | --- |
| `go test -count=1 ./...` | PASS against isolated PostgreSQL 17 and MinIO |
| `go test -race -count=1 ./...` | PASS |
| `go vet ./...`; `go build ./cmd/api` equivalent output to container `/tmp` | PASS |
| `npm run test`; `npm run lint`; `npm run build` | PASS; 54 tests in 8 files, no lint warnings |
| Migrations 000001–000006 apply/down/reapply | PASS in separate empty test database |
| Slug tombstone and orphan-photo rollback refusal guards | PASS, with expected SQL exceptions |
| OpenAPI 3.1 semantic validation; Compose configuration; internal Markdown links; `git diff --check` | PASS |
| Runtime `/health`, `/ready`, anonymous Directory, unauthenticated owner endpoint | 200, 200, 200, 401 respectively |

The first isolated Go test attempt found no tables because the separate `abm4e_test` database had not yet received migrations; it was migrated through 000006, then the complete normal/race/vet/build sequence passed. A transient Alpine compiler package download was retried successfully. Neither failure occurred in the final test run.

The earlier [run 40090480917](https://github.com/sipratama/alumnibridge/actions/runs/40090480917) tested `2ba71cc0`. After a safe fetch confirmed the remote feature branch was at `e1a432efc2fb7508762c1b6d05686780b5af1174`, Phase E dispatched [run 40099487422](https://github.com/sipratama/alumnibridge/actions/runs/40099487422). It completed **success at that exact application SHA**; it did not test the later documentation/screenshots. Those artifacts were committed in `5bb849e43dee3dcaff73121e0238769541167070`, and [run 40101484894](https://github.com/sipratama/alumnibridge/actions/runs/40101484894) completed **success at that exact documentation SHA** with Backend, Frontend, PostgreSQL/MinIO Integration, and Repository jobs passing. Neither run covers the later local remediation commits. Verify CI on the final remediation SHA and the milestone PR before merge. No push, PR or issue mutation was performed in the original Phase E review.

## Final local remediation addendum

Implementation commit `3d09a2731ac30507b4ef74e1a3b3c3eea63ea7ee` adds the approved 15-second visible-page recheck through TanStack Query's existing interval. It pauses while hidden or while a request is in flight, stops after an error, and suppresses automatic focus retries after 429/503. A manual retry or a new filter/page request remains available. The active cursor and committed filters stay in the Query key; obsolete requests are canceled when that key changes. During any recheck, the page masks previously downloaded cards until fresh results arrive. Query data remains memory-only with `gcTime: 0`; the transport uses `cache: no-store`.

At the default direct-peer budget of 60 requests/minute with a burst of 10, one visible tab uses approximately four periodic requests/minute before navigation or focus requests. Fifteen visible tabs sharing a direct peer can consume the steady budget; synchronized tabs can exhaust the burst sooner. Behind a shared proxy, unrelated users may share one bucket. This remediation leaves the approved cadence and server limits unchanged, stops automatic retries after errors, and retains manual recovery. Before shared/public deployment, enforce a trusted client-aware ingress budget and assess tab concurrency; if the approved budget is insufficient, review a coordinated client/backoff or ingress policy with measured traffic instead of silently raising server limits. This is a release limitation, not a production throughput claim.

Deterministic frontend tests cover cadence, hidden suppression, stale-card masking, 429/503 halt and manual recovery, filter/cursor retention, obsolete-fetch abort, invalid URLs, cleanup, and a single interval. The local frontend suite passed with 61 tests in 9 files; lint and build passed. Go tests and race tests passed against a separate PostgreSQL 17/MinIO Compose project; vet and build passed. Migrations 000001–000006 applied, rolled back, and reapplied in that project's disposable database. OpenAPI 3.1 semantic validation (19 paths), Compose configuration, repository Markdown links, and `git diff --check` passed. The exact final remediation HEAD is recorded in the delivery report. The latest GitHub CI currently covers `5bb849e`, not this local remediation.

## Issue acceptance and remaining gates

| Issue | Implementation and evidence | Recommendation |
| --- | --- | --- |
| [#14](https://github.com/sipratama/alumnibridge/issues/14) policy | Approved ADR 0002, privacy matrix, OpenAPI and real anonymous projection/consent tests | Maintainer may close after reviewing final policy evidence |
| [#15](https://github.com/sipratama/alumnibridge/issues/15) search | SQL/HTTP tests plus browser filters, hidden-field and canonical-link checks | Close after final PR CI/review |
| [#16](https://github.com/sipratama/alumnibridge/issues/16) pagination/performance | Static 20+2 browser pages, cursor adversarial Go tests, Phase C EXPLAIN evidence and current bounded runtime smoke | Close for development baseline after final PR CI; retain production load measurement separately |
| [#17](https://github.com/sipratama/alumnibridge/issues/17) UI | Student/pending login/onboarding, public browse, owner settings, responsive axe/keyboard checks and revalidation | Conditional closure after maintainer accessibility review; real screen reader remains unverified |
| [#18](https://github.com/sipratama/alumnibridge/issues/18) abuse/privacy | Signed-JWT tests, actual Keycloak ownership checks, privacy matrix, 429/503/cancellation smoke | Close for development baseline after final PR CI; retain ingress/distributed protection as production gate |
| [#19](https://github.com/sipratama/alumnibridge/issues/19) E2E/docs | This report, browser integration, regression and updated Windows steps | Keep open until final PR CI and Windows/Podman evidence or explicit maintainer scope decision |

[#11](https://github.com/sipratama/alumnibridge/issues/11) remains OPEN as a separate **High production risk and blocker**. The frontend preflight GET in `apps/web/src/features/directory/DiscoveryPanel.tsx:55` cannot prevent a privacy or consent write racing before PATCH. The owner-scoped SQL update in `apps/api/internal/profiles/postgres/discovery.go:98` has no expected-version predicate; the two simultaneous real-token PATCH requests both returned 200. Design atomic expected-version rejection and conflict UX with compatibility review before production use. No substantial API/schema redesign was performed in this acceptance phase.

Development baseline: accepted by this review. Milestone merge: ready for PR review with exact-SHA CI and maintainer sign-off conditions. Production: **not ready**. Additional production gates include trusted verification/moderation operations, ingress-aware abuse controls, realistic scale/load measurement, production OIDC/TLS/secrets, consent/legal review, backup/restore, and assessment of the archived MinIO community implementation and S3-compatible replacement portability. Already delivered public data cannot be recalled from visitors. No issue was closed, no PR was opened, and no merge or push was performed.
