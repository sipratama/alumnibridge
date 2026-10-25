# Milestone 5 access status baseline and disposable identity rehearsal

**Status:** Local implementation slice for review, based on `master` `dc9851f`; depends on [draft design PR #23](https://github.com/sipratama/alumnibridge/pull/23). No production issuer, hosting, retention, admin grant, account deletion or recovery decision is implied. This branch must be reviewed after the design policy is reconciled; do not merge it ahead of PR #23 without resolving documentation and issuer-binding conflicts.

## Protected route and provisioning inventory

The Go API registers four groups using `auth.Middleware`. Every route below verifies the bearer token and calls the current PostgreSQL access check **before** entering its handler. The middleware has no cached allow result. The same store rejects blocked subjects and non-active users during `Provision`, closing the gap between a successful middleware check and the subsequent upsert. `Onboard` checks the locked user row for `active` before changing membership. Future privileged routes must use this same check plus their separate grant check.

| Route group | Methods and paths | `identity.Me` path |
| --- | --- | --- |
| Identity | `GET /api/v1/auth/me`, `GET /api/v1/users/me` | Handler calls `Service.Me` directly. |
| Onboarding | `POST /api/v1/auth/onboarding` | `Service.Onboard` validates input, then calls `Service.Me`. |
| Owner profile | `GET/PATCH /api/v1/users/me/profile` | `profiles/http.owner` calls `Service.Me`. |
| Owner history | `GET/POST /api/v1/users/me/experiences`, `PATCH/DELETE /api/v1/users/me/experiences/{id}`; same four methods and paths for `educations` | Every handler uses `profiles/http.owner`. |
| Owner skills | `GET/POST /api/v1/users/me/skills`, `DELETE /api/v1/users/me/skills/{id}` | Every handler uses `profiles/http.owner`. |
| Owner privacy/photo | `GET/PATCH /api/v1/users/me/profile/privacy`; `GET/POST/DELETE /api/v1/users/me/profile/photo` | Every handler uses `profiles/http.owner`. |
| Owner discovery | `GET/PATCH /api/v1/users/me/profile/discovery` | Every handler uses `profiles/http.owner`. |

Public `/health`, `/ready`, `/api/v1/skills`, `/api/v1/profiles/{slug}`, its photo route, and `/api/v1/directory` do not consult owner access state. This slice does **not** withdraw a suspended or deleting account's public publication. Future deletion must atomically block protected access and withdraw public visibility/discovery; in-flight requests already past the check require transaction/lock review. No account status mutation API is included here.

## HTTP and identity contract

| Condition after token verification | Server result |
| --- | --- |
| Missing, malformed, expired, wrong issuer/audience/`azp`, or wrong signature | `401 {"error":"unauthorized"}`; no database status lookup before token validity. |
| New subject under the configured issuer, with no block | Allowed; current behavior provisions on the first identity or owner route that calls `Service.Me`. Enrollment never reads email as an identity key. |
| Existing `active` user | Allowed. The subject remains the existing `users.oidc_subject` key under this deployment's single verified issuer. |
| Existing `suspended`, `deleting` or `deleted` user, or matching `(issuer, sub)` block | `403 {"error":"account_unavailable"}` on every protected route, including `/auth/me` and onboarding; no status detail or owner data is returned. A still-valid old JWT is denied after the state/block commit. |
| PostgreSQL access lookup fails, or the verifier and store issuers differ | `503 {"error":"access_check_unavailable"}`; no handler or provisioning runs. A provisioning database failure after the check also fails closed, although the existing handler may report a generic 500. |

`Cache-Control: no-store` remains on middleware responses. The API trusts only a verified JWT subject; no client owner ID, role or email can override a denial. All current protected operations have 403 and 503 entries in [OpenAPI](../apps/api/openapi/openapi.yaml). The status check does not implement immediate invalidation of requests already in progress when a state transition commits; the future lifecycle writer must serialize that boundary with owner mutations and public withdrawal.

## Additive migration and compatibility

[Migration 000009](../infrastructure/migrations/000009_identity_access.up.sql) adds `users.access_status` with `active` default and a restricted-value CHECK, so existing users keep their current behavior. It also creates `identity_subject_blocks(issuer, subject)` with a composite primary key. No production block is inserted by this slice, no erasure or scheduled purge is implemented, and **no marker retention period is chosen**. The [down migration](../infrastructure/migrations/000009_identity_access.down.sql) refuses to drop restrictions if any user is non-active or any block exists; down/up testing belongs in a disposable database. An old API binary ignores the new status and could re-enable a suspended account, so production rollback would need a write fence and reviewed forward fix; a green down migration is not a production rollback plan.

`Provision` obtains an advisory transaction lock over the length-prefixed `(issuer, subject)` pair, checks the block, then upserts only an `active` existing row. Any future deletion marker writer must take the **same lock** before inserting a block or deleting the user; otherwise it can race first provisioning. The current schema still has a unique `users.oidc_subject` without an issuer column. This slice supports one configured issuer at a time; switching issuer with existing users is **not** authorized by adding the block table. A future issuer migration must bind existing rows explicitly, reject collisions and old tokens, and rehearse rollback. Do not infer an identity link from email or from an identical `sub` under another issuer.

## Ordered PRs after design review

1. **PR #23 design gate:** reconcile ADR 0004 and the maintainer decisions; keep it draft until accepted. This access slice is based on `master` to keep implementation review separate and must not be merged before the design dependency is resolved.
2. **Access baseline (this branch):** migration 000009, verified-token status check, guarded provisioning/onboarding, OpenAPI and focused signed-token/PostgreSQL tests. It can be tested without production hosting or a retention number because it creates no production deletion markers or status writer. Review the issuer compatibility and in-flight request limits explicitly.
3. **Issuer-bound lifecycle writer:** only after issuer migration, account-deletion and marker-retention policies are approved, add trusted state transitions, publication withdrawal, marker insertion and full-transaction retry under the documented lock order. Keep admin grants and recovery out of this PR.
4. **Operational topology:** choose a production operator, stable issuer/domain, TLS, backup custody and restore target; repeat the disposable drill in a production-like environment before any deployment. Admin bootstrap, deletion and recovery remain separate later PRs.

## Disposable Keycloak runbook and observed exercise

Use an isolated Compose project and ports, never the shared development volumes. The following rehearsal used `KEYCLOAK_PORT=28085 docker compose -p abm5topo -f compose.yaml up -d keycloak-db keycloak`, plus a second temporary Keycloak process on port 28086 against a restored database. All accounts, databases, containers, volumes and the temporary token file were removed afterward. The commands below are an outline; supply only disposable credentials through the local example environment.

Core commands for a fresh disposable run (execute from the repository root, with the repository's example development credentials available to the containers):

```sh
KEYCLOAK_PORT=28085 docker compose -p abm5topo -f compose.yaml up -d keycloak-db keycloak
docker exec abm5topo-keycloak-1 sh -c '/opt/keycloak/bin/kcadm.sh config credentials --server http://localhost:8080 --realm master --user "$KC_BOOTSTRAP_ADMIN_USERNAME" --password "$KC_BOOTSTRAP_ADMIN_PASSWORD"'
docker exec abm5topo-keycloak-1 /opt/keycloak/bin/kcadm.sh get clients -r alumnibridge -q clientId=alumnibridge-web
docker exec abm5topo-keycloak-1 /opt/keycloak/bin/kcadm.sh create users -r alumnibridge -s username=m5-restore-fixture -s enabled=true -s email=m5-restore@example.invalid
docker exec abm5topo-keycloak-db-1 pg_dump -U keycloak -Fc -f /tmp/m5-keycloak.dump keycloak
docker exec abm5topo-keycloak-db-1 createdb -U keycloak keycloak_restore
docker exec abm5topo-keycloak-db-1 pg_restore -U keycloak -d keycloak_restore /tmp/m5-keycloak.dump
```

Query discovery and JWKS at `http://localhost:28085/realms/alumnibridge/.well-known/openid-configuration` and the returned `jwks_uri`. For the restore comparison, start a **second disposable** Keycloak container against `jdbc:postgresql://keycloak-db:5432/keycloak_restore` on the isolated Compose network, with `KC_HOSTNAME=http://localhost:28086`, then compare the synthetic user's ID through the admin API. Keep the original process and database untouched. To rotate the disposable RS256 key, create a `rsa-generated` KeyProvider component with a higher `config.priority` through `kcadm.sh`; inspect `GET /admin/realms/alumnibridge/keys` and JWKS before and after. Token signature checking can use any standard RS256/JWK verifier; keep token bytes out of command output and repository files. Stop the second container and run `KEYCLOAK_PORT=28085 docker compose -p abm5topo -f compose.yaml down --volumes` only for this project after recording results. Do not apply these commands to a shared realm or database.

1. Read `/.well-known/openid-configuration`; assert `issuer` exactly equals `http://localhost:28085/realms/alumnibridge` and fetch its advertised JWKS. Inspect `alumnibridge-web` via `kcadm.sh`: public standard flow, PKCE S256, one callback `http://localhost:5173/` and one web origin `http://localhost:5173`.
2. Send authorization requests with S256 challenge and the exact callback, a path-suffixed callback and an unrelated origin. Expect login page only for the exact callback; reject the others before login. A real browser code exchange, refresh and logout are separate acceptance work.
3. In the disposable realm, create a synthetic user and obtain a token through Keycloak's built-in `admin-cli` client for the test realm. This token is not an AlumniBridge API token. Add a generated RS256 key provider with a higher priority. Confirm the active signing `kid` changes, both old and new public keys remain in JWKS, and old and new unexpired token signatures and issuer claims verify. Do not print or commit token bytes. Rehearse key retirement and API verifier refresh behavior before production; they were not part of this exercise.
4. Take a PostgreSQL logical dump after creating the synthetic user. Restore it into a new database and start a separate Keycloak process with that database. Confirm the restored user has the same Keycloak ID. If the restored hostname differs, the discovery issuer differs even when `sub` is identical; an application cutover must not auto-link those identities. A combined application-PostgreSQL/object-store restore and production backup custody are not exercised here.
5. Stop only the isolated containers and remove their project volume and temporary token file. Record commands, source revision, test results and gaps before any later deployment rehearsal.

Observed on 2026-10-25 (Asia/Jakarta), using Keycloak 26.7.0 and PostgreSQL 17 in disposable Docker containers:

| Check | Observed result | Limit |
| --- | --- | --- |
| Discovery and JWKS | Issuer was `http://localhost:28085/realms/alumnibridge`; advertised JWKS was reachable. | HTTP development mode, no TLS/proxy. |
| Client allowlist | Exact `http://localhost:5173/` returned a login page (200); `/evil` and `http://attacker.invalid/` returned 400. Admin client inspection showed public standard flow and PKCE S256. | No completed browser sign-in or logout. |
| Signing key rotation | A higher-priority RS256 provider changed the token signing `kid`; both pre- and post-rotation token signatures verified against retained JWKS keys and the same issuer. | No key retirement, JWKS outage or API refresh drill. |
| Keycloak identity restore | Synthetic user `m5-restore-fixture` retained the same Keycloak ID after `pg_dump`/`pg_restore` and startup against the restored database. Restore instance issuer changed to port 28086. | No application row reconciliation, paired object storage restore or production backup test. |

The separate PostgreSQL access test used an isolated PostgreSQL 17 container with migrations 000001–000009. It verified a signed JWT succeeds before state changes, then gets 403 on `/auth/me`, onboarding and a protected probe after `suspended`, `deleting` and `deleted` commits; a matching block prevents reprovision after deleting the user row; a closed pool yields 503. Migration 000009 refused down while restrictions existed, then completed down/up after cleanup. `go test -count=1 ./...` passed against the isolated database. These tests do not prove deletion orchestration, public publication withdrawal, production Keycloak topology or a retention schedule.

Follow-up access evidence uses the real `GET /api/v1/users/me/profile` route with a signed RS256 token and counting identity and profile stores. The route returns 200 while active, then 403 for each non-active state without calling the handler's first `identity.Me` operation or the profile service again; no bearer token returns 401, and an access-check database outage returns 503 without reaching either operation. A separate two-connection PostgreSQL test makes a synthetic block writer take the same advisory transaction lock as `Provision`, observes the provisioning connection waiting on a lock, commits the block, and confirms that both the waiting and a later provisioning attempt are denied with no active user row. This verifies the block-first interleaving only. A production lifecycle writer, including its account transition, public withdrawal, retry policy and marker retention, has not been implemented.
