# Implementation plan

Each milestone should leave a runnable, reviewable slice. Write small changes, add tests for behavior and boundaries, and keep OpenAPI and setup docs aligned with what actually runs.

| Milestone | Deliverable | Completion check |
| --- | --- | --- |
| 0. Product and architecture foundation | PRD, architecture, conventions, proposed layout, open decisions | Documents agree on public/private fields and module boundaries; only docs/directories exist. |
| 1. Fullstack bootstrap | Go module, Chi server, health/readiness, React/Vite landing shell, Tailwind/shadcn setup, PostgreSQL Compose, example environment, tests | Go tests/build and web tests/build pass; health works and readiness reflects PostgreSQL; Compose startup is verified where Docker is available. |
| 2. Authentication and user onboarding | Dedicated Keycloak realm/database, SPA PKCE sign-in, API JWT validation, UUID provisioning, student/alumni onboarding, migration runner, OpenAPI contract | Auth checks reject invalid tokens; PostgreSQL integration covers duplicate provisioning and both roles; web and API build; local sign-in is verified. |
| 3. Professional profile | Profile schema, owner editing, experience, education, skills, photo, privacy and public profile | Owner-only mutation, slug collision, upload validation and public privacy tests pass; web flows are responsive and accessible. |
| 4. Alumni Directory & Discovery | Separate default-false discoverability consent, verified-only anonymous search, consented academic filters, bounded PostgreSQL pagination and React directory | Private/hidden/non-opted-in/unverified profiles and hidden fields never match; SQL/HTTP/UI/privacy/pagination tests and milestone review pass. |
| 5. Identity operations (design review) | Confirm deployment topology, lifecycle/deletion policy, admin role assignment, and account recovery | Maintainer approves the security decisions and ordered issues; later implementation and operational tests cover the approved behavior. Design approval alone is not production acceptance. |
| 6. Membership review (deferred) | Restricted alumni proof workflow and administrator decision | Only authorized reviewers can decide claims; evidence stays out of public responses and logs; loss of final verified membership invalidates directory consent atomically. |
| 7. Profile follow-up (deferred) | Address measured profile usability or performance gaps after release | Scope defined from real usage; avoid duplicating Milestone 3. |
| 8. Verification hardening (deferred) | Audit, retention, and appeal procedures for alumni claims | Review lifecycle and retention tests pass. |
| 9. Connections | Send, accept, decline, withdraw and list requests | Transition tests cover duplicates, self-requests, authorization, and concurrent requests. |
| 10. Release readiness | Accessibility review, rate limits, privacy/retention procedures, observability, backup/restore exercise, deployment docs | CI is green, threats and data flows reviewed, and release acceptance criteria in the PRD pass. |

## Roadmap alignment

The maintainer reprioritized Directory from the old Milestone 8 to **Milestone 4** on 2026-10-24. The old Milestones 4–7 move to 5–8; Connections remains 9 and release readiness remains 10. Deferred identity, membership and verification work is preserved, not silently treated as implemented. Historical milestone reports retain their original scope and numbering.

## Current and next task

Milestones 1–3 and the Milestone 4 Directory are merged into the development baseline. Milestone 3 phases covered design, owner operations, public/privacy/photo backend and the [professional profile frontend](phase-d-professional-profile-frontend.md). Its [final review](reviews/milestone-3-final-review.md) retains production conditions and acceptance follow-ups. [PR #22](https://github.com/sipratama/alumnibridge/pull/22) merged the Issue #11 optimistic concurrency implementation into `master` at `dc9851f` on 2026-10-25. The older [development concurrency ADR](decisions/0001-last-writer-wins-development.md) is historical; [ADR 0003](decisions/0003-atomic-optimistic-concurrency-proposal.md), the [remediation review](issue-11-remediation-review.md) and [rollout runbook](issue-11-rollout-rollback.md) describe the implemented boundary and remaining production work. [Issue #11](https://github.com/sipratama/alumnibridge/issues/11) remains open for production acceptance; merging the PR does not establish deployment or release approval.

Milestone 4 Phase E integrated acceptance is **complete locally with conditions**, and the Directory work was merged through PR #21 at `8424ce5`. Phase D implements `/alumni`, typed public search, filter URLs, cursor navigation, publication-aware cards and `/settings/privacy` discovery controls; see [frontend evidence](milestone-4-phase-d-frontend.md). [Phase E acceptance](milestone-4-phase-e-acceptance.md) records disposable Keycloak browser journeys, privacy and abuse checks, accessibility automation and remaining platform/CI conditions. See the [directory plan](milestone-4-directory-plan.md), [design](milestone-4-directory-design.md), [Phase B review](reviews/milestone-4-phase-b-review.md) and [discoverability ADR](decisions/0002-directory-discoverability.md).

Milestone 5 has entered **design review only** on a branch from the merged `master`. The [identity operations plan and draft issues](milestone-5-identity-operations-plan.md) inventory the current Keycloak/OIDC, provisioning, membership, slug, photo and consent behavior; [ADR 0004](decisions/0004-identity-operations-boundaries-proposal.md) proposes authority and lifecycle boundaries. Neither document approves a production Keycloak topology, administrator grants, account deletion, recovery, audit retention or a verification workflow. The maintainer must resolve the listed decisions before implementation. Issues #2, #5, #8, #10, #17 and #19 retain their separate follow-ups; no issue status changes are implied.

| Phase | Deliverable | Issues |
| --- | --- | --- |
| A | Repository/tracking alignment, product policy, API/database/query design | #14; input to #15–#19 |
| B | Owner consent, privacy-aware backend search, initial bounded cursor and OpenAPI | #14, #15; baseline #16/#18 |
| C | Pagination edge cases, measured query performance, abuse/privacy hardening | #16, #18 |
| D | React directory, filters, cards, pagination and owner discovery controls | #17 |
| E | E2E/platform/accessibility/security review, documentation and approved PR preparation | #19; final acceptance #14–#18 |

GitHub Actions CI was added through PR #12. The Issue #11 [remediation review](issue-11-remediation-review.md) records exact-SHA CI and later local evidence; it does not establish a production deployment. Required branch protection remains #10 follow-up. Browser CI and sqlc generation policy remain separate follow-up decisions. M5 design artifacts may be submitted as a draft PR for maintainer review; that review does not authorize implementation, merge, deployment, issue creation or issue closure.

Before public launch, review the production storage and moderation conditions in [phase-c-profile-privacy.md](phase-c-profile-privacy.md). Alumni proof/retention and OIDC deployment topology remain open before their respective release work.

Directory development may have empty inventory before deferred verification operations are available. Use isolated synthetic verified fixtures for tests, never automatic promotion of real pending alumni.
