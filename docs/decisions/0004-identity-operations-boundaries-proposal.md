# ADR 0004 — Identity operations authority and lifecycle (proposal)

Date: 2026-10-25 (Asia/Jakarta).
Status: **Proposed; maintainer approval required before implementation.**
Scope: Milestone 5 design. This ADR changes no runtime behavior. See the [M5 plan](../milestone-5-identity-operations-plan.md).

## Context

The development stack imports a Keycloak realm into `start-dev`; the browser uses Authorization Code with S256 PKCE, and the API validates signed access tokens locally. The verified OIDC `sub` maps to `users.oidc_subject`; email is mutable. Membership status and directory consent live in PostgreSQL. `alumnibridge-admin` exists as an unassigned realm role, but there is no administrator operation or grant ledger. Keycloak reset-password is enabled in the local realm; account recovery has not been exercised as an operational procedure. There is no application account-deletion route or lifecycle job. SQL cascades, slug tombstones and photo cleanup cover only parts of deletion.

Offline JWT verification means Keycloak role removal, user disablement and session revocation alone do not immediately invalidate an already issued access token. `GET /auth/me` currently provisions an unknown subject, so erasing the application row while a token remains valid could recreate it. A public profile may also remain visible if the Keycloak account is disabled while the application row is left intact.

## Proposed decision

1. **Authority.** Keycloak owns credentials, authenticator factors, sessions and the immutable OIDC subject. The application owns member UUIDs, student/alumni claims, verification, profile publication and deletion state. Email never links or restores accounts. Identity recovery must preserve the original subject where possible; subject replacement needs an exceptional, audited binding procedure, not automatic email matching.
2. **Deployment.** Use a dedicated production Keycloak realm and database, served through a stable HTTPS issuer behind a configured reverse proxy. Keep the web, API and Keycloak origins and exact callback/logout URI explicit. The API may fetch JWKS over a private route only if tokens retain the same public issuer. Use production Keycloak mode, managed secrets and restricted administration. Do not promote Compose `start-dev`, localhost redirects or the realm import file as a production configuration.
3. **Administrator access.** Treat a narrowly scoped application grant as the current authorization authority for future privileged API calls, checked on every call after token verification. Require the reserved Keycloak role as an additional condition only if the maintainer accepts the resulting two-system provisioning burden. Grant and revoke through a separate trusted operator path with two-person approval, a recorded reason, expiry/review date and an independent audit record. No self-grant, onboarding grant, email-domain grant or automatic alumni verification. A revoked application grant denies a still-valid token immediately; Keycloak role/session revocation is also required. Membership reviewers in M6 need a separate capability and potentially institution scope; the broad realm role must not silently authorize claim decisions.
4. **Lifecycle.** Introduce a durable application access state before exposing deletion. Start deletion by atomically denying protected operations and removing public visibility/discovery/academic consent; then disable the Keycloak account and revoke sessions; only then erase application personal data and request Keycloak removal. Track each external step durably with idempotent retries and restricted, privacy-safe audit. Preserve permanent anonymous slug reservations. Keep a non-reprovision marker for a deleted subject for an approved period or by another proven mechanism; never rely on email or the absence of a user row. Physical photo deletion remains asynchronous through the durable registry and must be monitored to completion. No transaction is claimed across PostgreSQL, Keycloak and object storage.
5. **Recovery.** Ordinary password/factor recovery stays in Keycloak and returns to the same subject. Lost access to all factors or a lost Keycloak user record requires documented identity proof and independent operator approval before any subject rebind. Freeze access and privileged grants during investigation. Recovery must never infer ownership from an email address, profile content or self-declared membership. A rebind, if permitted, is an exceptional atomic application change with full audit and publication review; old tokens and subject must remain denied.
6. **Audit and retention.** Record actor, action, target pseudonymous identifier, UTC time, result, correlation ID and approval reference for admin grants/revocations, recovery and deletion. Keep credentials, tokens, proof, reviewer notes, object keys and private contact data out of ordinary logs. Retention durations, legal holds, audit access and erasure exceptions require maintainer/legal approval before implementation. M5 must not invent the M6/M8 verification evidence or appeal policy.

## Alternatives considered

| Option | Assessment |
| --- | --- |
| Reuse local Compose `start-dev` in production | Rejected: development defaults, localhost issuer and imported client settings are not production topology or operational evidence. |
| Self-host dedicated Keycloak versus supported managed hosting | Either can satisfy the proposed boundaries. Choose by operator ownership, upgrade/backup/restore capacity, availability and data residency; product approval is required. |
| Keycloak realm role alone authorizes administrators | Simpler, but locally verified JWT roles remain effective until token expiry. It cannot provide immediate revocation without introspection or a separate current-state check. |
| Application grant alone authorizes administrators | Immediate revocation and narrow capabilities, but Keycloak role becomes informational and operator processes still need strong controls. |
| Require both current application grant and realm role | Defense in depth, with two-system partial failures and a reconciliation burden. Proposed only after explicit approval of the operator workflow. |
| Delete Keycloak identity first | Rejected: public application data can remain published if the database step fails. |
| Delete application user row first | Rejected: a still-valid token can reprovision the same subject; external cleanup may fail. |
| Restore account by verified email | Rejected: email is mutable and may be reassigned or compromised. |

## Consequences and migration

Implementation would need reviewed additive schema for access state, non-reprovision protection, operation progress and a least-privilege grant/audit model. The exact schema and retention design follow approval; do not modify migrations 000001–000008. Existing accounts need an explicit active-state backfill, and existing Keycloak administrators must be inventoried before any role/grant cutover. Restrict direct SQL writers and ensure new privileged routes check current grant state. Existing client sessions and token lifetimes need a documented compatibility window.

For deletion, take the `users` row lock before profile/cascade work, consistent with onboarding and profile writers. Membership writers use membership row → profile → affiliation order; deletion's FK cascade and photo activation (`profile_photo_objects` staging row → profile) can deadlock. A trusted lifecycle worker must retry the **whole** PostgreSQL transaction on `40P01` or `40001` with bounded backoff and jitter, then keep the operation pending and alert. Do not replay an old photo `If-Match` or retry only one statement. The [Issue #11 runbook](../issue-11-rollout-rollback.md) documents the reproduced deletion/photo lock inversion. Recovery rebinds must analyze the same user/profile/membership lock graph before SQL is approved.

Rollback after a deletion or subject rebind cannot restore erased credentials, consent or personal data by reversing a migration. Prefer a forward fix while access remains blocked. Schema down is only for disposable databases unless a reviewed data reconciliation and a consistent PostgreSQL/object-store/Keycloak backup exist. A restored profile must not automatically republish, reuse consent or expose a historical slug. Keycloak and application backups must be tested together; independent point-in-time restores can produce orphan subjects or public data.

## Threats and verification obligations

| Threat | Proposed boundary and test |
| --- | --- |
| Stolen or stale token after disable, deletion or admin revoke | Current application access/grant check on every protected/privileged request; signed-token tests before and after state changes, including unexpired old token. |
| Account takeover through email reuse or support recovery | No email-based linking; proof, independent approval, old-subject denial and audit; adversarial recovery drill. |
| Self-granted administrator/reviewer rights | Operator-only grant path, separation of duties, no client role input; forged-token/role and cross-scope tests. |
| Public data persists after external outage | Application transaction withdraws publication before Keycloak step; public profile, photo and directory queries deny during every lifecycle state; fault injection after each step. |
| Lost photo cleanup intent or unsafe rollback | Durable registry and backlog checks; MinIO outage/restart/delete race tests; migration refusal guards. |
| Sensitive operational logs or indefinite retention | Field allowlist, access controls, retention/purge test and backup erasure procedure approved before release. |

## Review gate

The maintainer must approve the production hosting/issuer ownership, administrator authority and revocation semantics, deletion eligibility and retention, recovery proof and rebind policy, and audit retention before implementation. This proposal is not evidence that production deployment, recovery, deletion or verification operations work. Issue #11 and the existing follow-up issues remain separate release gates.
