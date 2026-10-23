# Implementation plan

Each milestone should leave a runnable, reviewable slice. Write small changes, add tests for behavior and boundaries, and keep OpenAPI and setup docs aligned with what actually runs.

| Milestone | Deliverable | Completion check |
| --- | --- | --- |
| 0. Product and architecture foundation | PRD, architecture, conventions, proposed layout, open decisions | Documents agree on public/private fields and module boundaries; only docs/directories exist. |
| 1. Fullstack bootstrap | Go module, Chi server, health/readiness, React/Vite landing shell, Tailwind/shadcn setup, PostgreSQL Compose, example environment, tests | Go tests/build and web tests/build pass; health works and readiness reflects PostgreSQL; Compose startup is verified where Docker is available. |
| 2. Authentication and user onboarding | Dedicated Keycloak realm/database, SPA PKCE sign-in, API JWT validation, UUID provisioning, student/alumni onboarding, migration runner, OpenAPI contract | Auth checks reject invalid tokens; PostgreSQL integration covers duplicate provisioning and both roles; web and API build; local sign-in is verified. |
| 3. Local runtime and CI | Add API/web containers, sqlc generation policy and GitHub Actions smoke checks | Fresh full-stack Compose startup and clean CI run pass using documented commands. |
| 4. Identity operations | Confirm deployment topology, lifecycle/deletion policy, admin role assignment, and account recovery | Security decisions documented; operational tests cover deployment configuration. |
| 5. Membership review | Restricted alumni proof workflow and administrator decision | Only authorized reviewers can decide claims; evidence stays out of public responses and logs. |
| 6. Profiles | Profile editing, experience, education, skills, and visibility choices | Owner-only mutation tests pass; private and public DTO tests cover every field. |
| 7. Verification hardening | Audit, retention, and appeal procedures for alumni claims | Review lifecycle and retention tests pass. |
| 8. Directory | Verified-only public listings and filtered, paginated search | Unverified/private profiles never appear; filters and pagination pass SQL/HTTP integration tests. |
| 9. Connections | Send, accept, decline, withdraw and list requests | Transition tests cover duplicates, self-requests, authorization, and concurrent requests. |
| 10. Release readiness | Accessibility review, rate limits, privacy/retention procedures, observability, backup/restore exercise, deployment docs | CI is green, threats and data flows reviewed, and release acceptance criteria in the PRD pass. |

## Current and next task

Milestone 2 implements authentication and onboarding from the current project request. The next recommended slice is CI plus a pinned sqlc workflow and repeatable runtime checks, followed by the product decisions below before professional profiles, verification evidence, or a public directory.

Before adding professional profiles or the directory, resolve the public field policy, alumni proof/retention workflow, and OIDC deployment topology listed in [architecture.md](architecture.md). Their answers affect schema, authorization, and API contracts.
