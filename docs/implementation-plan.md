# Implementation plan

Each milestone should leave a runnable, reviewable slice. Write small changes, add tests for behavior and boundaries, and keep OpenAPI and setup docs aligned with what actually runs.

| Milestone | Deliverable | Completion check |
| --- | --- | --- |
| 0. Product and architecture foundation | PRD, architecture, conventions, proposed layout, open decisions | Documents agree on public/private fields and module boundaries; only docs/directories exist. |
| 1. Fullstack bootstrap | Go module, Chi server, health/readiness, React/Vite landing shell, Tailwind/shadcn setup, PostgreSQL Compose, example environment, tests | Go tests/build and web tests/build pass; health works and readiness reflects PostgreSQL; Compose startup is verified where Docker is available. |
| 2. Authentication and user onboarding | Dedicated Keycloak realm/database, SPA PKCE sign-in, API JWT validation, UUID provisioning, student/alumni onboarding, migration runner, OpenAPI contract | Auth checks reject invalid tokens; PostgreSQL integration covers duplicate provisioning and both roles; web and API build; local sign-in is verified. |
| 3. Professional profile | Profile schema, owner editing, experience, education, skills, photo, privacy and public profile | Owner-only mutation, slug collision, upload validation and public privacy tests pass; web flows are responsive and accessible. |
| 4. Identity operations | Confirm deployment topology, lifecycle/deletion policy, admin role assignment, and account recovery | Security decisions documented; operational tests cover deployment configuration. |
| 5. Membership review | Restricted alumni proof workflow and administrator decision | Only authorized reviewers can decide claims; evidence stays out of public responses and logs. |
| 6. Profile follow-up | Address measured profile usability or performance gaps after release | Scope defined from real usage; avoid duplicating Milestone 3. |
| 7. Verification hardening | Audit, retention, and appeal procedures for alumni claims | Review lifecycle and retention tests pass. |
| 8. Directory | Verified-only public listings and filtered, paginated search | Unverified/private profiles never appear; filters and pagination pass SQL/HTTP integration tests. |
| 9. Connections | Send, accept, decline, withdraw and list requests | Transition tests cover duplicates, self-requests, authorization, and concurrent requests. |
| 10. Release readiness | Accessibility review, rate limits, privacy/retention procedures, observability, backup/restore exercise, deployment docs | CI is green, threats and data flows reviewed, and release acceptance criteria in the PRD pass. |

## Current and next task

Milestones 1 and 2, plus authentication verification/hardening, are complete. Milestone 3 is Professional Profile. Phase A covered GitHub project setup, branch creation, domain/API design and database schema. Phase B implements owner-only profile, experience, education, and skills. Phase C implements public profile reads, photo storage, privacy and moderation boundaries. Phase D frontend work requires separate approval. Runtime/CI and sqlc generation policy remain follow-up work outside this milestone.

Before public launch, review the production storage and moderation conditions in [phase-c-profile-privacy.md](phase-c-profile-privacy.md). Alumni proof/retention and OIDC deployment topology remain open before their respective release work.
