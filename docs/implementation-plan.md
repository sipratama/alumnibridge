# Implementation plan

Each milestone should leave a runnable, reviewable slice. Write small changes, add tests for behavior and boundaries, and keep OpenAPI and setup docs aligned with what actually runs.

| Milestone | Deliverable | Completion check |
| --- | --- | --- |
| 0. Product and architecture foundation | PRD, architecture, conventions, proposed layout, open decisions | Documents agree on public/private fields and module boundaries; only docs/directories exist. |
| 1. Fullstack bootstrap | Go module, Chi server, health/readiness, React/Vite landing shell, Tailwind/shadcn setup, PostgreSQL Compose, example environment, tests | Go tests/build and web tests/build pass; health works and readiness reflects PostgreSQL; Compose startup is verified where Docker is available. |
| 2. Authentication and user onboarding | Dedicated Keycloak realm/database, SPA PKCE sign-in, API JWT validation, UUID provisioning, student/alumni onboarding, migration runner, OpenAPI contract | Auth checks reject invalid tokens; PostgreSQL integration covers duplicate provisioning and both roles; web and API build; local sign-in is verified. |
| 3. Professional profile | Profile schema, owner editing, experience, education, skills, photo, privacy and public profile | Owner-only mutation, slug collision, upload validation and public privacy tests pass; web flows are responsive and accessible. |
| 4. Alumni Directory & Discovery | Separate default-false discoverability consent, verified-only anonymous search, consented academic filters, bounded PostgreSQL pagination and React directory | Private/hidden/non-opted-in/unverified profiles and hidden fields never match; SQL/HTTP/UI/privacy/pagination tests and milestone review pass. |
| 5. Identity operations (deferred) | Confirm deployment topology, lifecycle/deletion policy, admin role assignment, and account recovery | Security decisions documented; operational tests cover deployment configuration. |
| 6. Membership review (deferred) | Restricted alumni proof workflow and administrator decision | Only authorized reviewers can decide claims; evidence stays out of public responses and logs; loss of final verified membership invalidates directory consent atomically. |
| 7. Profile follow-up (deferred) | Address measured profile usability or performance gaps after release | Scope defined from real usage; avoid duplicating Milestone 3. |
| 8. Verification hardening (deferred) | Audit, retention, and appeal procedures for alumni claims | Review lifecycle and retention tests pass. |
| 9. Connections | Send, accept, decline, withdraw and list requests | Transition tests cover duplicates, self-requests, authorization, and concurrent requests. |
| 10. Release readiness | Accessibility review, rate limits, privacy/retention procedures, observability, backup/restore exercise, deployment docs | CI is green, threats and data flows reviewed, and release acceptance criteria in the PRD pass. |

## Roadmap alignment

The maintainer reprioritized Directory from the old Milestone 8 to **Milestone 4** on 2026-10-24. The old Milestones 4–7 move to 5–8; Connections remains 9 and release readiness remains 10. Deferred identity, membership and verification work is preserved, not silently treated as implemented. Historical milestone reports retain their original scope and numbering.

## Current and next task

Milestones 1–3 are merged into the development baseline. Milestone 3 phases covered design, owner operations, public/privacy/photo backend and the [professional profile frontend](phase-d-professional-profile-frontend.md). Its [final review](reviews/milestone-3-final-review.md) and [development concurrency ADR](decisions/0001-last-writer-wins-development.md) retain production conditions and acceptance follow-ups. Same-field last-writer-wins behavior remains a production gate in [#11](https://github.com/sipratama/alumnibridge/issues/11), not an implemented optimistic concurrency mechanism.

Milestone 4 Phase E integrated acceptance is **complete locally with conditions**. Phase B remediation `830f463`, Phase C review `9c90cfe` and Phase D reviewed application commit `e1a432e` are on the feature branch. Phase D implements `/alumni`, typed public search, filter URLs, cursor navigation, publication-aware cards and `/settings/privacy` discovery controls; see [frontend evidence](milestone-4-phase-d-frontend.md). [Phase E acceptance](milestone-4-phase-e-acceptance.md) records disposable Keycloak browser journeys, privacy and abuse checks, accessibility automation and remaining platform/CI conditions. See the [directory plan](milestone-4-directory-plan.md), [design](milestone-4-directory-design.md), [Phase B review](reviews/milestone-4-phase-b-review.md) and [discoverability ADR](decisions/0002-directory-discoverability.md).

| Phase | Deliverable | Issues |
| --- | --- | --- |
| A | Repository/tracking alignment, product policy, API/database/query design | #14; input to #15–#19 |
| B | Owner consent, privacy-aware backend search, initial bounded cursor and OpenAPI | #14, #15; baseline #16/#18 |
| C | Pagination edge cases, measured query performance, abuse/privacy hardening | #16, #18 |
| D | React directory, filters, cards, pagination and owner discovery controls | #17 |
| E | E2E/platform/accessibility/security review, documentation and approved PR preparation | #19; final acceptance #14–#18 |

GitHub Actions CI is now implemented via merged PR #12; master `1537c88` passed backend, frontend, isolated PostgreSQL/MinIO/migration and whitespace jobs. Required branch protection is not enabled and remains #10 follow-up. Browser CI and sqlc generation policy remain separate follow-up decisions. No phase authorizes automatic pushes, PR creation, merges or issue closures.

Before public launch, review the production storage and moderation conditions in [phase-c-profile-privacy.md](phase-c-profile-privacy.md). Alumni proof/retention and OIDC deployment topology remain open before their respective release work.

Directory development may have empty inventory before deferred verification operations are available. Use isolated synthetic verified fixtures for tests, never automatic promotion of real pending alumni.
