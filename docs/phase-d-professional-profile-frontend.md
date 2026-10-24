# Milestone 3 Phase D: professional profile frontend

Date: 2026-10-24 (Asia/Jakarta). Branch: `feature/milestone-3-professional-profile`.
Base commit: `6608405fba42ec4d482d98470bdd4d39974fc998` (`review phase c`).

## Preflight and scope

The working tree was clean. `git fetch origin` succeeded; HEAD and `origin/feature/milestone-3-professional-profile` both identified the base commit, with zero commits ahead/behind. The nine-file Phase C remediation was verified in that commit. The Phase C review's description of uncommitted changes records its earlier review state; the user subsequently committed and pushed those changes. AGENTS, profile/architecture/product documents, both review reports, OpenAPI and Issues #2–#8 were inspected before implementation.

Phase D integrates the approved owner and anonymous APIs. No backend source, migration, Keycloak configuration, production storage provider or persistent volume is changed. Directory, connections and later milestones remain outside this implementation.

## UI architecture and routes

`app/AppRoutes.tsx` owns route composition and lazy loading. Existing OIDC authentication and onboarding guards wrap the member shell. The established teal, slate and warm white identity, shadcn button primitive and Lucide icons are reused. The member rail becomes a four-item navigation row on mobile. Clear page headings, readable story sections and separate privacy controls support the primary task: build a professional story and deliberately decide what is shared.

The UI follows Operate mode within the existing visual system: the desktop member shell places a sticky rail beside the workspace, with profile story sections and a supporting aside. White panels, fine borders and restrained teal selection states organize the page; the inherited Inter/system font stack supplies headings, body text and controls. The aside moves below the story at 1120px, and the rail becomes a four-item row at 760px, where forms and supporting content stack. Flat CSS covers and initials provide the default profile identity; profile photos come from member uploads.

| Route | Access and purpose |
| --- | --- |
| `/dashboard` | Onboarded member home with links to real profile/account tasks |
| `/profile` | Onboarded member's profile, completeness, photo and career history |
| `/account` | Existing membership information, explicitly separate from professional education |
| `/settings/privacy` | Onboarded member's visibility, eight field choices and public link |
| `/settings` | Redirect to `/settings/privacy` |
| `/onboarding` | Existing authenticated onboarding flow |
| `/in/:slug` | Anonymous server-approved public profile, independent of owner caches |

`features/profile` owns typed API contracts, query/mutation helpers, server-aligned schemas, profile/editor/history/skills/photo/privacy/public components and scoped styles. `shared/api/client.ts` centralizes authenticated/anonymous HTTP behavior, no-store requests, typed errors and correct empty 204 responses. No additional application dependency was introduced. Route splitting keeps forms and their validation code out of the initial landing bundle.

The independently fetched public projection uses a narrower reading column with the same story sections and typography. During background revalidation, a neutral placeholder replaces the previous public content and retains its measured height to preserve scroll geometry. The story region exposes its busy state; initial loading has a separate status announcement. Privacy remains an explicit task on its own route, with visibility choices, optional fields, consent and the public link grouped together.

## Contracts and state

- TanStack Query keys include the authenticated actor UUID for owner data. No UUID is rendered. Mutations invalidate owner profile/collections and the identity display-name query; public queries are discarded rather than populated from owner data. Logout retains its existing complete query-cache clearing.
- `ProfileEditor` uses React Hook Form and Zod. Its baseline is captured at opening. Only dirty fields enter PATCH; empty strings explicitly clear optional text, and omitted fields are preserved. Reserved/invalid slugs and server 409s produce field feedback. Failed requests keep drafts and disable duplicate submissions while pending.
- Before profile PATCH, the editor re-reads current data and rejects differences in dirty fields. Before privacy PATCH, the form compares the explicit visibility and changed flags with its saved baseline. This avoids restoring a known outdated publication choice. **These checks are not atomic optimistic locking:** the backend has no ETag/version precondition, so a change between GET and PATCH can still win. The UI does not claim otherwise or redesign the API.
- Experience and education PATCH have replacement semantics: editors send all editable fields and convert blank nullable dates to null. Current employment clears/disables the end date. Calendar validity and date ordering are checked. Server ordering is preserved; collection limits are 100. Education never changes membership verification.
- Skills use a 300 ms debounce, public `GET /skills?query=...&limit=10`, existing normalized terms and the owner's POST/DELETE routes. Duplicate selected names are disabled case-insensitively. The vocabulary remains server-owned; no frontend registry exists.
- Photo files use raw bytes and their MIME, not multipart JSON. JPEG/PNG/WebP and the 5 MiB byte limit are checked locally. Preview URLs are revoked when replaced/unmounted. Authenticated photo GETs create temporary blob URLs; no storage keys, credentials or direct bucket URLs reach the UI. Dimension/content validation remains authoritative on the server. Pending, malformed, oversized, unauthorized and storage-failure states have recoverable feedback; deletion requires confirmation.
- Native modal dialogs provide protected focus, Escape dismissal and explicit focus restoration. Forms have associated labels/errors; async feedback uses status/alert semantics; visible focus and skip links support keyboard use. Desktop/mobile overflow and keyboard dialog behavior were exercised, but no full screen-reader certification is claimed.

## Privacy and anonymous behavior

Visibility is initially private, and all optional flags are initially false. Selecting Public from a saved Private state reveals an **unchecked** consent checkbox and cannot save until the owner selects it. There is no automatic opt-in. Display name/headline are explained as the basic public card. The actual `show_bio`, `show_location`, `show_current_company`, `show_current_job_title`, `show_experiences`, `show_educations`, `show_skills` and `show_photo` flags are used. PATCH always includes visibility and only changed flags. Users cannot edit moderation or verification.

The public view renders only `PublicProfile`, fetched anonymously. No owner response is used as a public preview. Hidden/omitted groups do not render; private, hidden and missing profiles share the non-disclosing unavailable state. Verification comes from the public server boolean or trusted authenticated memberships, never entered education or headline text. Pending Alumni are labeled pending and do not receive a verified badge. A private profile's public view is unavailable until publication; the UI does not simulate publication locally.

Fetch follows the backend's historical-slug redirect, then replaces the application URL with the returned canonical slug after validating its local slug shape. Photo delivery is constructed from the controlled API path, never an arbitrary profile URL. Queries have zero stale/cache retention after unmount, revalidate on mount/focus and poll every 15 seconds while visible. Public content is hidden during revalidation and on errors. Publication mutations clear public caches in the same client. A different browser can retain already downloaded content until its next request/poll; bytes already downloaded cannot be revoked. Returning to Private prevents subsequent anonymous API reads. Moderation can still make a consented public view unavailable.

There is no separate consent-revocation record/endpoint. Saving Private withdraws public access; optional choices remain saved for a future explicit opt-in. No stronger revocation behavior is invented.

A confirmed frontend integration defect was fixed: anonymous `check-sso` returns to the stable OIDC callback just as authenticated login does. Destination restoration now also restores safe anonymous deep links after that callback. It still consumes destinations only at `/`, rejects external/backslash destinations and leaves loading/error states alone. Authorization Code + S256 PKCE, state/nonce checks, explicit callback URI and token validation are unchanged.

## Verification and reproducibility

Backend commands executed with PostgreSQL and MinIO integration variables enabled in a disposable `golang:1.27.3-alpine` container:

```sh
go test ./...
go vet ./...
go build ./cmd/api
```

The database was created solely for this run, migrations 000001–000005 were applied, fully rolled back and reapplied, and integration suites executed against it plus unique temporary MinIO buckets. The temporary database/buckets were removed. Existing development tables/volumes were retained. No backend changes required a new migration.

Frontend and infrastructure commands:

```sh
cd apps/web
npm run test
npm run build
npm run lint
cd ../..
docker compose -p alumnibridge --env-file .env.example config --quiet
git diff --check
```

Final frontend verification passed: **28 tests across five files**, TypeScript/production build, and lint with **zero warnings**. The landing entry chunk is approximately 333 kB minified; validation and profile routes are loaded separately, with no bundle-size warning. The component suite covers empty/rendered profiles, codepoint text limits, dates, dirty PATCH clearing, slug conflicts, concurrent-edit detection, experience/education CRUD, shared skill search/duplicates, photo validation/failure, explicit consent, concurrent privacy conflicts, canonical navigation, public field omission, private-state cache rejection and authenticated/anonymous transport. Existing protected-route and authentication-destination regressions remain included.

### Browser harness

`apps/web/e2e/phase-d.cjs` drives real Chrome, Keycloak, frontend, Go API, PostgreSQL and dedicated MinIO. It registers unique synthetic Student and Alumni accounts, then deletes only those Keycloak/application accounts in `finally`; slug tombstones intentionally remain. The durable worker removes their queued photo objects. Credentials are supplied through environment variables and tokens are never printed. Run against the local development stack, not production.

The review reused Playwright 1.64.1 installed outside the repository; the app dependency graph/lockfile was unchanged. To reproduce without installing it in the application:

```sh
npm install --prefix /tmp/alumnibridge-browser-review --no-save --package-lock=false playwright@1.64.1
# Supply local admin credentials matching your .env; do not commit real values.
export KEYCLOAK_ADMIN_USERNAME=localadmin
export KEYCLOAK_ADMIN_PASSWORD=local_admin_only
export PLAYWRIGHT_MODULE=/tmp/alumnibridge-browser-review/node_modules/playwright
export CHROME_EXECUTABLE='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
node apps/web/e2e/phase-d.cjs
```

`WEB_URL`, `KEYCLOAK_URL`, `CONTAINER_CLI` and `SCREENSHOT_DIR` are optional overrides. Credentials shown above are the repository's local example values, never production credentials. On Windows, use equivalent PowerShell environment syntax and an available Chrome executable. Windows/Podman execution is not verified by this macOS run.

Browser assertions cover registration/onboarding; profile editing; experience/education add/edit/delete; current-date behavior; skill reuse/removal; photo preview/upload/delete; private default; explicit unchecked consent; anonymous allowed fields and omitted bio; pending Alumni badges; historical slug navigation; public-to-private denial; authenticated refresh; logout protection; login restoration; and keyboard Escape/focus restoration. Screenshots contain synthetic content, not real member data. They are evidence of UI rendering, not a claim of independent accessibility certification.

### Manual checks still recommended

1. Repeat the harness on Windows + Podman with the project-specific reachable API/storage endpoints.
2. Exercise NVDA/VoiceOver with form errors, modal focus, navigation and announcements.
3. Repeat photo replacement with real JPEG/WebP files and storage outages; backend regression tests cover those image/lifecycle boundaries, while browser automation uses a generated PNG.
4. Approve production consent wording, moderation operations, abuse controls and storage/backup recovery before public launch.

## Issue progress and limitations

| Issue | Phase D status |
| --- | --- |
| #2 | Profile editor, dirty PATCH, slug errors/history navigation and completeness integrated. Offensive-slug vocabulary remains a product release decision. |
| #3 | Experience list/add/edit/delete, current employment, date validation and ordering integrated. |
| #4 | Education list/add/edit/delete and optional dates integrated; verification stays separate. |
| #5 | Bounded debounced server search, shared-name reuse, duplicate prevention and removal integrated. The implemented query parameter is `query`; no unsupported cursor pagination is invented from the older issue sketch. |
| #6 | Controlled owner/public photo delivery, validation/preview/upload/replacement/removal UI integrated. Production storage risk from Phase C remains unchanged. |
| #7 | Explicit consent, eight field choices, public view/link, private denial and safe historical navigation integrated using the implemented `/profile/privacy` endpoint. |
| #8 | Component/API regressions and real Student/Alumni browser integration implemented. Windows/Podman and full assistive-technology review remain unexecuted. |

All issues remain open; no GitHub comments, push, merge or PR is performed. No production storage migration, directory, connections or later milestone is started.

## Final executed results and readiness

| Check | Result |
| --- | --- |
| `go test ./...` | Passed with PostgreSQL and MinIO integration enabled |
| `go vet ./...` | Passed |
| `go build ./cmd/api` | Passed |
| Migrations 000001–000005 apply/rollback/reapply | Passed in a disposable database; database removed afterward |
| `npm run test` | Passed: 28 tests across five files |
| `npm run build` | Passed, TypeScript plus production assets; no bundle-size warning |
| `npm run lint` | Passed, zero warnings |
| Docker Compose configuration validation | Passed |
| Real Chrome Student and pending Alumni journeys | Passed end to end using actual Keycloak and application services |
| Mobile reading during a delayed real public refresh | Passed for both roles: scroll position retained within two pixels, stale profile absent during validation |
| `/health`, `/ready` | Both returned 200 |
| Disposable-account/photo cleanup | Passed: zero synthetic application accounts and zero photo registry objects after the durable worker collected an expired orphan from an interrupted run |
| Persistent migration state | Version 5, `dirty=false`; no migration/source changes |
| `git diff --check` | Passed |
| Windows / Podman | Not available on this macOS/Docker host; not executed |

Early browser development runs found the anonymous callback destination defect and dialog focus restoration defect; both were fixed and covered by subsequent successful runs. Harness steps were also corrected to await dialog completion and SSO restoration before the next interaction/reload. The results above refer to the completed final run, not those interrupted runs. Chrome capture tiling artifacts were discarded; the retained screenshots were captured after layout settled using software rendering.

The independent UI finish review found a background-refresh geometry defect. That fix has a focused regression plus the real mobile scroll assertion described above. The reviewer confirmed that specific fix resolved (`ship`); the separate documentation pass reconciled architecture descriptions with the built code. The design detector ran once with no findings. This does not substitute for the outstanding screen-reader/Windows checks.

Retained screenshot evidence (all synthetic): [owner desktop](screenshots/phase-d/desktop.png), [owner mobile](screenshots/phase-d/mobile.png), [privacy](screenshots/phase-d/privacy-desktop.png), [public desktop](screenshots/phase-d/public-desktop.png), [public mobile](screenshots/phase-d/public-mobile.png). Their [provenance](screenshots/phase-d/README.md) identifies the capture method and synthetic data.

### Files changed

- `apps/web/src/features/profile/`: API contracts/hooks, server-aligned validation, shared controls, photo hook, owner/editor/history/skills/photo/privacy/public pages, scoped styles and regression tests.
- `apps/web/src/app/AppRoutes.tsx`, `main.tsx`: guarded member routes, anonymous public route and lazy loading.
- `apps/web/src/app/auth.tsx`, `authDestination.ts`, `authDestination.test.ts`: restore safe anonymous callback destinations and regression coverage.
- `apps/web/src/shared/api/client.ts`, `client.test.ts`: shared anonymous/authenticated transport, 204 handling, cancellation and token-failure regressions.
- `apps/web/src/features/account/AccountPage.tsx`, `App.tsx`: real member navigation and accurate profile availability copy.
- `apps/web/e2e/phase-d.cjs`: reproducible real-service browser verification and scoped account cleanup.
- `docs/prd.md`, `docs/architecture.md`, `docs/implementation-plan.md`: reflect the implemented Phase D behavior.
- This document and `docs/screenshots/phase-d/`: architecture, evidence, limitations and synthetic screenshots.
- `.gitignore`: exclude generated local `.impeccable/review/` captures; retained evidence lives in documentation.

**Verdict: READY FOR MILESTONE 3 FINAL REVIEW.** This is readiness for user review of the local implementation, not production deployment approval. Review the code and UI, then separately address the inherited production storage/privacy/abuse/backup gates and unavailable platform/accessibility checks. Work stops at Phase D; no push, merge, PR or issue closure is performed.
