# Milestone 4 Phase D — Directory frontend and discovery settings

Date: 2026-10-24. Branch: `feature/milestone-4-alumni-directory`. Base commit: `9c90cfec52dd37b7cf5bcaba08e5d08c1257540b`.

## Routes and components

| Route | Access | Purpose |
| --- | --- | --- |
| `/alumni` | Anonymous and authenticated | Public search, consented cards and cursor navigation |
| `/in/:slug` | Anonymous and authenticated | Existing canonical public profile projection |
| `/settings/privacy` | Authenticated owner | Existing visibility/field controls plus separate Directory and academic consent |

`DirectoryPage` uses the existing brand and teal/slate design. The header has public Home and sign-in links; the member rail and home page link to the Directory. The page has a compact mobile filter disclosure, semantic form labels, a results status, accessible error/empty/loading states and visible keyboard focus. Submitting a search or changing pages moves keyboard focus to the results heading. Result cards use only the public Directory DTO. Their verification label is displayed only when `verified_alumni` is true in the response. The card never renders owner/membership IDs, evidence, email or hidden field placeholders. Photo URLs are accepted only when they match the controlled `/api/v1/profiles/{canonical-slug}/photo` path; broken images use an initial avatar. Image alt text is empty because the adjacent display name identifies the person.

## Typed API and search behavior

`features/directory/api.ts` defines Directory entries/pages, discovery settings and academic consent. The public request sends no bearer token, uses `cache: no-store` and propagates TanStack Query's `AbortSignal`. Search runs only when the user submits the form. Editing a field alone does not issue a request or consume the shared source-IP budget. The committed filters appear in the URL; back/forward restores them. Invalid, duplicate or unsupported URL parameters show a correction state without sending a request.

Name, company and current job title are literal prefixes. The UI does not promise fuzzy, substring, accent-insensitive, wildcard or autocomplete search. `skills` are repeated URL keys and require all selected exact normalized names. A maximum of five skills, 20 default results and 50 maximum results are enforced in the client; the server remains authoritative. `study_program` is disabled until an institution reference slug is entered. No institution/program catalog endpoint exists, so the UI accepts validated reference slugs and explains how to obtain them from a published affiliation. Unknown but valid slugs simply return no results.

Pagination stores opaque `next_cursor` values only in component state, never in shareable URLs, analytics or persistent storage. Next and Previous each fetch a fresh server page. Changing a filter or page size discards cursor history. An expired/invalid cursor or a later page emptied by publication withdrawal offers a first-page reset. Live pages can skip or repeat records when profiles change between requests; they are not a database snapshot.

Directory queries use `retry: false`, `staleTime: 0`, `gcTime: 0`, no placeholder data, refetch on mount/focus and manual retry after errors. Failed new filters never retain old cards. The API client parses bounded numeric `Retry-After` values and gives separate 429, overload, database timeout, invalid cursor and network guidance without exposing raw backend errors. The backend's rate limit is per direct TCP peer; local Nginx/Vite proxy users can share a budget. This UI does not claim to solve distributed abuse or ingress rate enforcement.

## Owner consent and privacy

The Discovery panel fetches owner settings with the existing verified OIDC token. The toggle is explicit and cannot turn on while the backend marks the owner ineligible or the profile is private. Making a profile PUBLIC remains a separate existing consent action. Only verified memberships from the owner response are offered for academic disclosure. Selecting study program or graduation year requires institution; unselecting institution clears both dependent controls.

PATCH sends only changed top-level fields. `affiliations` is sent as the complete selected set when those controls change; `[]` clears all selections. Opt-out sends `directory_discoverable: false` after native confirmation and relies on the backend's atomic academic-consent withdrawal. A fresh GET before PATCH detects changed saved settings in another session and displays a conflict without silently overwriting them; this is a best-effort UI guard, **not** optimistic concurrency or a replacement for Issue #11. The server remains authoritative for membership ownership, visibility, moderation and verification. Owner mutations clear Directory and public profile query caches. Logout already clears the entire TanStack Query client. Cross-tab changes are rechecked when the Directory gains focus, subject to normal browser behavior; previously downloaded data cannot be recalled.

## Verification

- Exact base SHA `9c90cfec` CI: [run 40083750206](https://github.com/sipratama/alumnibridge/actions/runs/40083750206), four jobs completed successfully. This verifies the base commit, not these local Phase D edits.
- Frontend local: `npm run test` — 47 tests across eight files passed after the final cases were added. `npm run lint` — clean. `npm run build` — passed. The two new Directory test files cover URL validation, anonymous transport, public fields, cursor transitions, page-size reset, empty later pages, stale request cancellation, error guidance and owner consent controls.
- Backend local: `go test -count=1 ./...`, `go vet ./...` and `go build ./cmd/api` passed in a disposable Go 1.27.3 container using separate PostgreSQL 17 and MinIO containers. Migrations 000001–000006 applied, rolled back and reapplied on the disposable database. The exact base SHA CI also ran the race detector and all four jobs.
- Browser with real backend: headless Chrome on macOS rendered the built `/alumni` page at 1440 px and 390 px. The browser routed `/api/v1/*` to a separate temporary API process and PostgreSQL database with synthetic verified, student, pending and unlisted fixtures. It confirmed only opted-in verified alumni appeared, tested real name/company/academic/skill filters, next/previous cursor pages, canonical public profile navigation, URL back/forward, mobile disclosure and no horizontal overflow. Changing the synthetic listed profile from PUBLIC to PRIVATE then removed its result, made its direct public profile return 404, and cleared discovery and academic consent in PostgreSQL. No browser page errors were observed. Separate intercepted-response browser checks exercised 429 and 503 guidance. Screenshots: [desktop](screenshots/milestone-4-phase-d/directory-desktop.png), [mobile](screenshots/milestone-4-phase-d/directory-mobile.png) and [mobile filters](screenshots/milestone-4-phase-d/directory-mobile-filters.png). These show disposable synthetic profiles.
- The development API initially returned HTTP 404 for `/api/v1/directory` because its persistent database was on migration version 5. After isolated tests, the existing version-controlled migration 000006 was applied non-destructively with the project's migration command and only the AlumniBridge API service was rebuilt. `schema_migrations` is now version 6, clean; the original one user and one profile remain, with zero discovery opt-ins. `/health`, `/ready` and anonymous `/api/v1/directory` now return 200; the last returns an empty page, as expected without a real verified opt-in. A final Chrome check against the ordinary local stack confirmed `/alumni` shows that empty state without an auth redirect or page error. The browser's verified fixture checks used the separate temporary API/database and Playwright routing, since no real account was promoted. Student/pending registration and onboarding, owner discovery PATCH through Keycloak, photo delivery, full consent withdrawal, and login/logout need isolated end-to-end or manual verification in Phase E. No actual screen-reader session or Windows/Podman run was performed.

## Remaining constraints

- The Directory's public reference slug inputs are functional but less discoverable than a catalog selector. A public catalog endpoint and controlled UI would require a separate approved contract.
- Server responses can reveal already delivered data to a visitor; publication withdrawal only controls future requests. Active views revalidate on focus; local owner mutations clear caches. Cross-device and cross-tab instantaneous revocation cannot be guaranteed by frontend code.
- Cursor expiry is 15 minutes; returning to an old page may require starting again.
- Authenticated browser E2E through Keycloak, assistive technology evaluation, Windows/Podman execution and final PR CI remain Phase E evidence.

## Issue progress and remaining browser checks

| Issue | Phase D status |
| --- | --- |
| #14 Directory consent policy | Approved backend policy is represented in owner controls and public copy; maintainer acceptance remains open. |
| #15 Directory search API | Existing contract is consumed without backend changes; anonymous real API browser filters passed in disposable environment. |
| #16 Pagination and performance | Existing Phase C cursor/backend implementation was exercised in browser; no new performance claim. |
| #17 React Directory UI | Route, search/filter state, cards, pagination, responsive layout, consent panel and component tests implemented; authenticated full E2E remains. |
| #18 Abuse and privacy | 429/503 UI guidance, no aggressive retries, private-field allowlist and privacy withdrawal browser regression covered; distributed ingress control remains. |
| #19 E2E and documentation | Phase D browser evidence and screenshots recorded; full milestone/Windows/accessibility/PR verification belongs to Phase E. |

For Phase E, use an isolated Keycloak/PostgreSQL/MinIO stack and disposable identities. Register a Student and pending Alumni, complete onboarding and confirm neither is listed. Create a trusted synthetic verified membership through test setup, publish its profile, opt into Directory, publish and withdraw each academic field, verify anonymous filters and cards, then opt out and make the profile PRIVATE. Confirm direct `/in/:slug`, browser refresh/back/forward, logout, rejected unauthorized owner calls and no redirect URI error. Test keyboard order, focus after filter submission and confirmation, and the chosen screen reader on desktop/mobile. Never promote an existing user's membership for this checklist.
