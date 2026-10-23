# Implementation plan

Each milestone should leave a runnable, reviewable slice. Write small changes, add tests for behavior and boundaries, and keep OpenAPI and setup docs aligned with what actually runs.

| Milestone | Deliverable | Completion check |
| --- | --- | --- |
| 0. Product and architecture foundation | PRD, architecture, conventions, proposed layout, open decisions | Documents agree on public/private fields and module boundaries; only docs/directories exist. |
| 1. Fullstack bootstrap | Go module, Chi server, health/readiness, React/Vite landing shell, Tailwind/shadcn setup, PostgreSQL Compose, example environment, tests | Go tests/build and web tests/build pass; health works and readiness reflects PostgreSQL; Compose startup is verified where Docker is available. |
| 2. Local runtime and CI | Add API/web containers, Keycloak local configuration, and GitHub Actions smoke checks | Fresh full-stack Compose startup and clean CI run pass using documented commands. |
| 3. Data and API contract | Migration runner, first institution/member identity schema, sqlc config, `/api/v1` OpenAPI workflow | Migrations apply to empty PostgreSQL and repeat safely; generated queries build; contract check passes. |
| 4. OIDC boundary | Keycloak realm/client configuration, browser sign-in, API token validation | Invalid issuer/audience/expired tokens fail; browser sign-in and sign-out work locally. |
| 5. Registration | Member provisioning, student/alumni applicant enrollment and status | Duplicate enrollment and actor spoofing fail; registration path passes integration tests. |
| 6. Profiles | Profile editing, experience, education, skills, and visibility choices | Owner-only mutation tests pass; private and public DTO tests cover every field. |
| 7. Verification | Claim review, role checks, restricted evidence handling, audit history | Only authorized reviewers can decide claims; evidence stays out of public responses and logs. |
| 8. Directory | Verified-only public listings and filtered, paginated search | Unverified/private profiles never appear; filters and pagination pass SQL/HTTP integration tests. |
| 9. Connections | Send, accept, decline, withdraw and list requests | Transition tests cover duplicates, self-requests, authorization, and concurrent requests. |
| 10. Release readiness | Accessibility review, rate limits, privacy/retention procedures, observability, backup/restore exercise, deployment docs | CI is green, threats and data flows reviewed, and release acceptance criteria in the PRD pass. |

## Immediate next task: milestone 2

Complete a reproducible full-stack local runtime and CI. The bootstrap already supplies a native Go API, Vite web shell, and PostgreSQL Compose service; add containerized API/web services, Keycloak development configuration, and CI checks without starting authentication behavior.

Before milestone 3 or 4, resolve the public field policy, alumni proof/retention workflow, and OIDC deployment topology listed in [architecture.md](architecture.md). Their answers affect schema, authorization, and API contracts. Other product decisions can be resolved when their milestone starts.
