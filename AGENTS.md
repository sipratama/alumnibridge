# AlumniBridge engineering conventions

These rules apply to contributors and coding agents throughout this repository. User instructions for a specific task take precedence.

## Scope and language

- Keep the product requirements in `docs/prd.md`, architecture decisions in `docs/architecture.md`, and staged work in `docs/implementation-plan.md` current when behavior changes.
- Write all code, documentation, comments, commit messages, and API descriptions in English. User-facing localization may be added deliberately later.
- Implement only the requested milestone. Do not scaffold unused modules, tables, or abstractions.

## Architecture

- Use a modular monolith. A single Go API process owns the business modules and PostgreSQL database; Keycloak provides identity, not alumni verification.
- Keep domain rules and application services independent of HTTP, pgx/sqlc, and Keycloak packages. Put adapters at the edge and wire dependencies through constructors.
- Define small interfaces at the consumer that needs them. Prefer concrete types until a port has a real second boundary or test need. Avoid generic repositories and framework-style service layers.
- Keep inter-module calls explicit. Use a transaction for changes that must be atomic; do not introduce distributed messaging without a demonstrated need.
- Use idiomatic Go, `context.Context` for request-scoped work, explicit errors, and standard `testing`. Do not panic for expected errors.

## Data, API, and security

- Use UUID primary keys and UTC timestamps. Make SQL migrations versioned, reviewed, and reversible where practical. Keep handwritten SQL as the source of truth and generated sqlc code inside storage adapters.
- Validate and authorize every protected operation on the server. Derive the actor from a verified OIDC token, never from client-submitted IDs or roles. Treat alumni verification as application state.
- Make public response DTOs explicit and minimal. Never expose verification evidence, academic proof, reviewer notes, private contact data, secrets, or raw database models in public APIs or logs.
- Parameterize SQL, bound pagination, validate input at API boundaries, and keep OpenAPI synchronized with implemented routes.
- Use environment-based configuration; commit example values only. Never commit credentials, tokens, production personal data, or real verification documents.

## Frontend and quality

- Use React + TypeScript with feature-focused folders. Keep network access in a typed API layer; use TanStack Query for server state and React Hook Form + Zod for forms when those features arrive.
- Ensure public pages work without authentication; make protected actions visibly require sign-in. Server validation remains authoritative.
- Format changed files, run relevant tests and static checks, and report what passed or could not run. Add tests for business rules, authorization, privacy boundaries, SQL behavior, and significant state transitions; avoid tests that only copy implementation details.
- Preserve accessibility, responsive layouts, and clear error states. Do not add a dependency without a concrete use case and a pinned version/lockfile in executable projects.

## Working process

- Check the repository state before edits and keep changes within the task scope. Do not overwrite unrelated work.
- Explain significant design decisions and unresolved risks in the relevant document. Ask for confirmation when a product or security decision cannot be inferred safely; continue independent work while waiting.
- Keep bootstrap changes free of authentication and business features until their planned milestones.
