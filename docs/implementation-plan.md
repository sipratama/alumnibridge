# Implementation plan

Each milestone should leave a runnable, reviewable slice. Write small changes, add tests for behavior and boundaries, and keep OpenAPI and setup docs aligned with what actually runs. Do not start feature implementation during this documentation milestone.

| Milestone | Deliverable | Completion check |
| --- | --- | --- |
| 0. Product and architecture foundation | PRD, architecture, conventions, proposed layout, open decisions | Documents agree on public/private fields and module boundaries; only docs/directories exist. |
| 1. API shell | Go module, Chi server, environment validation, health endpoints, graceful shutdown | `go test ./...` passes; invalid configuration fails startup; health response is contract-tested. |
| 2. Web shell | Vite React TypeScript app, Tailwind/shadcn setup, public landing route, typecheck/build scripts | npm install from lockfile, typecheck, and production build pass; no member features exist. |
| 3. Local runtime and CI | Compose for API, web, PostgreSQL, and Keycloak; example environment; GitHub Actions smoke checks | Fresh local startup and clean CI run pass using documented commands. |
| 4. Data and API contract | Migration runner, first institution/member identity schema, sqlc config, `/api/v1` OpenAPI workflow | Migrations apply to empty PostgreSQL and repeat safely; generated queries build; contract check passes. |
| 5. OIDC boundary | Keycloak realm/client configuration, browser sign-in, API token validation | Invalid issuer/audience/expired tokens fail; browser sign-in and sign-out work locally. |
| 6. Registration | Member provisioning, student/alumni applicant enrollment and status | Duplicate enrollment and actor spoofing fail; registration path passes integration tests. |
| 7. Profiles | Profile editing, experience, education, skills, and visibility choices | Owner-only mutation tests pass; private and public DTO tests cover every field. |
| 8. Verification | Claim review, role checks, restricted evidence handling, audit history | Only authorized reviewers can decide claims; evidence stays out of public responses and logs. |
| 9. Directory | Verified-only public listings and filtered, paginated search | Unverified/private profiles never appear; filters and pagination pass SQL/HTTP integration tests. |
| 10. Connections | Send, accept, decline, withdraw and list requests | Transition tests cover duplicates, self-requests, authorization, and concurrent requests. |
| 11. Release readiness | Accessibility review, rate limits, privacy/retention procedures, observability, backup/restore exercise, deployment docs | CI is green, threats and data flows reviewed, and release acceptance criteria in the PRD pass. |

## Immediate next task: milestone 1

Create the smallest runnable Go API shell: configuration, Chi routing, a health endpoint, graceful shutdown, and Go standard tests. Record exact Go version and setup commands in the README. The web shell, Compose, and CI follow in milestones 2 and 3.

Before milestone 4 or 5, resolve the public field policy, alumni proof/retention workflow, and OIDC deployment topology listed in [architecture.md](architecture.md). Their answers affect schema, authorization, and API contracts. Other product decisions can be resolved when their milestone starts.
