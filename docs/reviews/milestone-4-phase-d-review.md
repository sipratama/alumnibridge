# Milestone 4 Phase D final review

Date: 2026-10-24. Branch: `feature/milestone-4-alumni-directory`.
Reviewed SHA: `2ba71cc0d07f18ca3b5b53b659aae6328cde3ac0`.
Remediation implementation HEAD: `3f320f1` (full hash available with `git rev-parse 3f320f1`). Final local HEAD is the documentation-only child carrying this report; its exact hash is recorded in the delivery response. No push, PR, merge, issue update or Phase E implementation was performed.

## Scope and preflight

The branch and expected SHA matched; the working tree was clean. Fetched origin safely and confirmed 0 ahead/behind before edits. Compared the branch with origin/master and reviewed Phase D's sources/tests, AGENTS.md, Directory design/plan, ADR 0002, Phase B/C contracts and review evidence, Phase D documentation and Issue #17 acceptance criteria. Existing backend migrations and API contracts were inspected; no backend code, migration or API semantics changed.

## Findings

No Critical or High findings. Five Medium findings and one Low finding were fixed.

| ID | Severity | Evidence at reviewed SHA | Impact and remediation |
| --- | --- | --- | --- |
| M1 | Medium | `apps/web/src/features/directory/DirectoryPage.tsx:85` only tested `isPending`; cards at line 89 remained rendered during background fetching | Previously delivered cards could appear current while renewed consent was being checked. Mask the entire result body whenever fetching; preserve the measured region height, keep a status message and expose errors/manual retry after completion. No downloaded content revocation claim. |
| M2 | Medium | `DirectoryPage.tsx:106` joined skills with commas; line 114 split all commas | A valid skill such as `Design, research` could not be submitted or restored intact. Use a labeled textarea with one exact skill per line, preserving commas and repeated URL parameters. No backend matching change. |
| M3 | Medium | `apps/web/src/features/directory/DiscoveryPanel.tsx:20` returned only LoadError for every query error | A transient background GET failure unmounted the form, discarding unsaved consent selections; a successful PATCH followed by failed GET also lost its feedback. Keep the form and saved response on transient refresh errors with an alert. Initial errors and 401 still hide the form. Pre-save owner GET and server validation remain mandatory. |
| M4 | Medium | `DirectoryPage.tsx:167` accessible name inserted the person's name between the visible words `View profile` | Axe label-content-name-mismatch; speech activation might not match the visible label. Accessible name now begins `View profile for ...`; regression assertion updated. |
| M5 | Medium | `.directory-page .section-kicker` inherited #198174; `.directory-page-note` used #63786f on #f5f8f6 | Axe measured 4.42:1 and 4.41:1 for small text, below 4.5:1. Apply darker colors scoped to Directory. Final axe scans report no contrast violation in tested content. |
| L1 | Low | `directory.css:2` header navigation and brand shared a non-wrapping row | At 320 px Sign in extended to x=322.86. Wrap the header navigation at widths up to 360 px. Four viewport checks with long names/affiliations now have no horizontal overflow. |

Remediation sources: `DirectoryPage.tsx:38` retains region height; the result condition masks fetching; `DiscoveryPanel.tsx:20` preserves transient-error drafts. CSS fixes are scoped to Directory. No unrelated style redesign or dependency was introduced.

## Revalidation, pagination and URLs

Controlled deferred-fetch tests load cards, trigger focus revalidation, assert the cards leave the DOM while pending, then complete with empty data, 429, 503 or a network rejection. All four outcomes hide the earlier cards and issue no automatic retries. A real Chrome check also confirms the delayed masking and empty completion. Filter changes cancel obsolete requests; the existing old/new response race regression passes.

Cursor values remain opaque and only in component memory. There is no offset, cursor URL serialization or persistent response storage. Filter/page-size changes reset history; previous pages refetch; expired and empty later pages offer first-page recovery. While a page request is pending, cards and pagination controls are masked, preventing repeated page navigation from the old result. Existing tests exercise next/previous, final-page disable, expiry, empty withdrawal pages, page-size reset and browser filter history. Pages remain live reads, with possible skips/repeats after renames, rather than a database snapshot.

## Consent and privacy assessment

PUBLIC and discovery remain separate; enabling discovery never publishes a private profile. Eligibility and verified membership choices come from the authenticated backend response. Academic choices are membership-specific; unchecking institution clears program/year. PATCH omits unchanged fields, supplies a full replacement only for edited affiliations, uses [] to clear and relies on atomic backend opt-out withdrawal. Owner IDs never enter public cards or URLs.

The pre-save GET detects changed saved consent or membership choices; the server rejects foreign, removed or ineligible memberships. A visibility/verification change after that GET remains subject to server transaction checks. This is not Issue #11 optimistic concurrency. Controls are disabled during saving. Failed writes preserve selections; new regressions preserve selections after a failed refresh and preserve a successful PATCH result when follow-up GET fails. Session-expiry errors are still displayed and owner data is hidden after a refresh returns 401. Public caches are removed after successful owner writes; logout's existing query-client clear remains unchanged.

## Executed verification

| Check | Actual result |
| --- | --- |
| `npm run test` | PASS, 54 tests across eight files, including seven added regressions |
| `npm run lint` | PASS, no warnings |
| `npm run build` | PASS |
| `go test -count=1 ./...` | PASS in Go 1.27.3 Linux/arm64 container with real disposable PostgreSQL/MinIO |
| `go test -race -count=1 ./...` | PASS with the same disposable services |
| `go vet ./...`, `go build ./cmd/api` | PASS |
| PostgreSQL/MinIO suites | PASS, including signed RSA JWT, privacy/consent, revocation, cursor, timeout/cancellation and photo tests; opt-in heavy performance/load harnesses were not requested or rerun |
| Migrations 000001–000006 | Up/down/reapply passed on a new tmpfs PostgreSQL database |
| M4/M5 rollback guards | Original down files refused a tombstone/orphan-photo fixture as expected; connection exit rolled each transaction back |
| OpenAPI semantic validation | PASS, Swagger Parser, OpenAPI 3.1.0 |
| Compose validation | PASS |
| `git diff --check` | PASS |

The temporary database contained no development data. No persistent development database, account, volume or running development service was modified during this review. Temporary API, PostgreSQL, MinIO, static server and network were removed after verification. Test tooling installed under /tmp did not change package dependencies or lockfiles.

## Exact-SHA CI

No run existed initially for the requested SHA. Dispatched the existing workflow on the feature branch and verified [run 40090480917](https://github.com/sipratama/alumnibridge/actions/runs/40090480917) tested **2ba71cc0d07f18ca3b5b53b659aae6328cde3ac0**. All four jobs completed successfully: Backend; Frontend; PostgreSQL/MinIO integration; Repository whitespace.

This run does not cover the remediation commit or report commit. They are local and unpushed. After authorized push, CI must run again on the final SHA before merge. The existing workflow already discovers all added frontend tests; no workflow/dependency change is needed.

## Browser and accessibility evidence

Executed headless Chrome on macOS against the current compiled React build and a separate temporary API/PostgreSQL/MinIO stack. Synthetic fixtures covered active Students, pending Alumni, verified Alumni without opt-in and explicitly opted-in verified Alumni, plus 21 synthetic listed records for pagination. Browser interactions passed listing eligibility, professional/academic/skill filters, canonical /in/:slug navigation, next/previous/final-page disable, filter back/forward and PUBLIC-to-PRIVATE exclusion. The latter also returned direct profile 404 and cleared stored discovery/timestamp/academic flags. No internal IDs were rendered by public DTO/cards. Opt-out, authorization, rate/admission error semantics are additionally covered by real backend HTTP/JWT tests and frontend error-state tests.

Browser harness limitation: requests were routed to the temporary frontend/API rather than the existing development stack. Initial Vite/Keycloak callback attempts failed because redirect handling fetched an older asset manifest from the existing web service. The final harness served the compiled build and disabled the identity adapter's automatic check-sso option **only in intercepted test JavaScript**. Application source/OIDC behavior was not modified. These are public UI/real search API checks, not authenticated Keycloak E2E or proof of unchanged SSO redirects. No real account was promoted.

Axe ran on Directory at 320, 390, 768 and 1440 px with long synthetic display names/affiliations. Initial scans reproduced the contrast and label findings above. Final scans reported zero violations and no horizontal overflow at all four widths. A deferred browser refetch masked cards and settled to empty data. Focus-after-search is covered by component tests; native disclosure labels, aria-expanded, form labels/status and visible focus remain intact. Screenshots/traces/scripts from this review are temporary evidence under `/tmp/ab-m4dr-browser`; earlier implementation screenshots remain historical. No real VoiceOver/NVDA session or Windows/Podman execution occurred. Automated checks do not establish WCAG compliance.

## Remaining conditions and Issue #17

Recommendation: accept #17's implementation, filtering/URL/card/pagination/component-test and privacy-revalidation portions after maintainer review. Keep the issue open until its Student/pending Alumni authenticated browser acceptance criterion is executed or explicitly reconciled. No issue was closed or commented on.

Phase E priorities, after approval:

1. Push only with authorization and verify all four CI jobs at the final SHA.
2. Use disposable Keycloak identities to register/onboard Student and pending Alumni; test login, logout, refresh, original destination and stable redirect URI. Confirm owner GET/PATCH, session expiry and protected-page behavior in a real authenticated browser.
3. Exercise verified synthetic owner consent UI through real Keycloak: opt-in/out, affiliation changes, PRIVATE withdrawal, revoked/disappearing memberships, concurrent sessions and follow-up failures. The present component/signed-JWT tests do not substitute for these browser flows.
4. Perform real assistive technology and Windows/PowerShell/Podman verification; assess mobile focus and native confirmation with the selected reader.
5. Retain Issue #11, ingress/proxy/distributed abuse controls, legal consent review and other recorded production gates. Frontend code cannot recall downloaded data or guarantee cross-device immediate revocation.

**Verdict: READY FOR PHASE E WITH CONDITIONS.** No confirmed Critical/High finding remains. Remediation is locally verified; final-SHA CI and authenticated/platform/assistive-technology evidence remain explicit conditions. Phase E has not started.
