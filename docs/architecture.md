# Initial architecture

## Shape and boundaries

AlumniBridge starts as one Go API process, one React web application, one PostgreSQL database, and Keycloak as the identity provider. The API is a modular monolith: business modules share a deployment and database, but keep their own domain rules and ports. There are no internal HTTP calls or message brokers between modules.

```text
Browser (React/Vite)
  | public requests / authenticated requests
  v
Go API (Chi + OpenAPI)
  | HTTP handlers -> application services -> repository ports
  |                                |                |
  |                         domain rules      pgx/sqlc adapters
  |                                                 |
  +---- OIDC token verification                    PostgreSQL
              ^
              |
          Keycloak login
```

Planned modules:

| Module | Owns | Depends on |
| --- | --- | --- |
| `identity` | Application member identity, role/claim mapping, current-user context | OIDC verifier port |
| `institutions` | University and study-program reference data | Repository port |
| `profiles` | Professional profiles, experience, education, skills, publication policy | Member and institution identifiers |
| `verification` | Alumni claims, restricted evidence metadata, review decisions and audit trail | Member and institution identifiers |
| `connections` | Requests and accepted connection state | Member identifiers |
| `directory` | Public alumni projection and search policy | Read ports supplied by profiles and verification |

Start with packages only when a milestone needs them. A module can have `domain`, `app`, and `adapter` packages if this separation clarifies real dependencies; do not create empty layers. Domain and application code must not import Chi, pgx, sqlc output, Keycloak SDKs, or HTTP DTOs. HTTP and PostgreSQL adapters implement the inward-facing ports. Constructors wire concrete implementations in `cmd/api`.

## Identity and authorization

The proposed browser flow is OIDC Authorization Code with PKCE through a Keycloak public client. The web app holds no client secret. The API validates bearer token signature, issuer, audience, expiry, and relevant roles using OIDC discovery/JWKS, then maps the stable OIDC subject to an application member. Application roles and alumni verification state remain enforceable in the API. Every public route receives an explicit public response DTO; authenticated routes resolve the actor from verified token context.

The local implementation uses a dedicated `alumnibridge` Keycloak realm and `alumnibridge-web` public client on port 18085. `keycloak-js` uses Authorization Code Flow with S256 PKCE and keeps tokens in memory. The API checks the RS256 signature against the realm JWKS, issuer, `alumnibridge-api` audience, expiry, `Bearer` token type, and `azp=alumnibridge-web`. The verified subject is mapped to a UUID in `users`; the email is an optional mutable attribute, never the identity key. Keycloak realm role `alumnibridge-admin` is reserved for future administrator routes; onboarding cannot assign it. Configure exact production redirect URIs, origins, and HTTPS before deployment. [Keycloak's JavaScript adapter guide](https://www.keycloak.org/securing-apps/javascript-adapter) documents the public-client and PKCE setup.

Current identity tables are `users`, `institutions`, `study_programs`, and `memberships`. One user may have student and alumni memberships. Student membership is active at onboarding; alumni membership remains pending until a later review workflow. There are no academic identifiers or verification documents in this schema. `GET /api/v1/auth/me` and `GET /api/v1/users/me` provision the user idempotently; `POST /api/v1/auth/onboarding` is idempotent for matching membership details and rejects conflicting graduation years. The API's response model is private to the authenticated user. The migration runner applies versioned SQL from `infrastructure/migrations`.

## Data and search

Milestone 3 profile schema and API boundaries are recorded in [professional-profile-design.md](professional-profile-design.md). Phase B adds owner-only profile, experience, education, and skill operations with domain validation, an inward-facing store interface, pgx adapter, and HTTP adapter. Public projections and photo storage remain later phases.

- PostgreSQL is the system of record. UUID primary keys and `timestamptz` UTC instants are the baseline. Each module owns its schema changes, even though all tables live in one database.
- Handwritten, versioned SQL migrations are authoritative. `sqlc` generates query code for `pgx/v5`; generated code stays in adapters and is committed only after the generation policy is agreed. [sqlc's guide](https://docs.sqlc.dev/en/latest/howto/generate.html) supports this combination.
- Directory queries join or project only verified and discoverable alumni, use parameterized SQL, deterministic ordering, and bounded pagination. Start with indexed PostgreSQL filtering; assess full-text/trigram search only if real query needs justify it.
- Keep restricted verification data separate from public profile read models. Define public response fields explicitly; never serialize database rows directly.
- Cross-module changes that must succeed together use one database transaction coordinated by an application service. Avoid a generic repository or event bus until a concrete use case needs one.

## API and web client

- Version public HTTP routes under `/api/v1`. Keep OpenAPI as the contract for request/response formats, errors, and auth requirements. Select a spec-first or code-first generation workflow in the API milestone; do not maintain two competing definitions.
- Use Chi for routing and middleware, explicit validation and error mapping, request IDs, structured logging with redaction, and graceful shutdown.
- React + TypeScript + Vite is the web runtime. Tailwind CSS and shadcn/ui provide UI primitives; TanStack Query handles server state; React Hook Form + Zod handle form state and client-side validation. The API remains the authority for validation and permissions.
- Keep API access behind one typed client module. Do not fetch protected resources before authentication state is known. Public pages must work for logged-out visitors.

## Configuration, operations, and tests

- Environment variables configure API address, database URL, OIDC issuer/audience, CORS origins, and log level. A checked-in example file documents names, never credentials. Fail startup on invalid required values.
- Podman Compose is the verified Windows local runtime for application PostgreSQL, Keycloak PostgreSQL, and Keycloak. API and Vite run natively for development. Production deployment topology is not assumed. Health endpoints distinguish process liveness from PostgreSQL readiness.
- Go standard `testing` covers domain/application behavior; PostgreSQL integration tests exercise SQL and migrations; HTTP contract tests cover public field allowlists and authorization. Frontend checks include typecheck, lint, build, and focused behavior tests when implemented.
- GitHub Actions should run format/lint checks, Go tests, web checks, migration/sqlc generation checks, and secret-safe build verification. Add checks with the milestones that introduce their code.

## Proposed repository layout

```text
alumnibridge/
  AGENTS.md
  README.md
  compose.yaml                     # future local development services
  .github/workflows/               # future CI
  apps/
    api/
      cmd/api/                      # composition root and server entry point
      internal/
        identity/                   # modules created as needed
        institutions/
        profiles/
        verification/
        connections/
        directory/
        platform/                   # shared config, HTTP, and DB wiring only
      openapi/                      # contract
      sqlc.yaml
      go.mod
    web/
      src/
        app/                        # providers and route setup
        features/                   # directory, profiles, verification, connections
        shared/                     # API client and reusable UI
      package.json
  infrastructure/
    migrations/                     # versioned SQL
    keycloak/                       # local realm configuration
  docs/
    prd.md
    architecture.md
    implementation-plan.md
```

The API and web shells, identity module, Keycloak realm, PostgreSQL migrations, and OpenAPI contract exist. Local macOS Docker Compose builds the Go API and React web app, runs migrations as a one-shot service, and serves the web app through Nginx with an `/api/` proxy. The API fetches JWKS from Keycloak on the Compose network while validating the browser-visible localhost issuer. The web app uses one exact OIDC callback URI for login, registration, session restoration, and logout. It stores a same-origin destination in session storage while Keycloak handles the callback at the root URL. This keeps Keycloak's redirect allowlist limited to one URI; realm import does not change clients already stored in Keycloak. Profile, directory, verification, and connection paths describe future milestones.

## Planned dependencies

| Area | Initial choice | Reason / boundary |
| --- | --- | --- |
| Go HTTP | `github.com/go-chi/chi/v5` | Small router; handlers stay adapters. |
| PostgreSQL | `github.com/jackc/pgx/v5`, `sqlc` CLI | Explicit SQL and typed query generation. |
| Migrations | `golang-migrate/migrate/v4` CLI | Versioned SQL migrations with local and CI execution. |
| OIDC | Keycloak, `keycloak-js`, a maintained Go JWT/OIDC verifier selected at implementation | Provider-managed login; API validates tokens itself. |
| Web | React, TypeScript, Vite, Tailwind CSS, shadcn/ui | Requested UI stack. |
| Web data/forms | `@tanstack/react-query`, React Hook Form, Zod | Server state and form validation. |
| Tooling | npm lockfile, Go modules, GitHub Actions, Docker Compose | Reproducible builds and local environment. |

Pin compatible versions and commit lockfiles when executable projects are added. Do not add a dependency solely because it appears in this table. [golang-migrate's CLI](https://github.com/golang-migrate/migrate/blob/master/cmd/migrate/README.md) supports versioned migrations; [shadcn/ui's Vite guide](https://ui.shadcn.com/docs/installation/vite) covers its Vite setup.

## Technical risks and decisions to confirm

1. **Public data policy:** Directory value depends on alumni visibility, while names, education, and workplaces can be sensitive. Confirm the exact public field allowlist and opt-in behavior before schema/API work.
2. **Verification workflow:** Evidence type, storage location, malware scanning if uploads are allowed, retention, appeal process, and reviewer access are undefined. Do not build evidence uploads before these are specified.
3. **Authentication topology:** Local development uses Vite's API proxy and separate Keycloak origin. Confirm production web/API/Keycloak domains, session/logout behavior, and administrator role provisioning before deployment. Local redirect URI and web origin are exact for port 5173.
4. **Identity lifecycle:** Define what happens on email change, account deletion, re-registration, and a member moving from student to alumnus. Keycloak subject should be the stable link, not email.
5. **Multi-institution rules:** Define whether one member may affiliate with several programs and whether reviewers are scoped by institution. The initial model should avoid hardcoded UHAMKA IDs or names.
6. **Search and scale:** Directory performance, case-insensitive company/profession matching, and skill vocabulary need real examples. Start with PostgreSQL indexes and measure before adding a search service.
7. **Operational setup:** Podman Compose works on the Windows workstation. The Go migration runner needs no external migration CLI. Identity queries currently use parameterized pgx in the storage adapter; adopt sqlc generation when the query set grows and a pinned generator is available, keeping migrations and handwritten SQL authoritative.

The Phase B review adds permanent anonymous slug tombstones after account deletion, a database FK binding current slug ownership, and profile PATCH transformations under row locks. Migration 000004 rollback refuses to lose tombstones. See [review results](reviews/milestone-3-phase-b-review.md).
