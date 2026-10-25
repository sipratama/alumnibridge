# Milestone 5 — Identity operations design and issue drafts

**Status:** Design for maintainer review; no M5 implementation or production acceptance.
**Date:** 2026-10-25 (Asia/Jakarta).
**Baseline:** `master` `dc9851f05f0a5ca0db7bd724cd9b1c6f1d84f286` (merged PR #22). Issue #11 remains open for production acceptance. This plan does not close or subsume [#2](https://github.com/sipratama/alumnibridge/issues/2), [#5](https://github.com/sipratama/alumnibridge/issues/5), [#8](https://github.com/sipratama/alumnibridge/issues/8), [#10](https://github.com/sipratama/alumnibridge/issues/10), [#17](https://github.com/sipratama/alumnibridge/issues/17) or [#19](https://github.com/sipratama/alumnibridge/issues/19). The proposed authority and tradeoffs are in [ADR 0004](decisions/0004-identity-operations-boundaries-proposal.md).

## Current state versus M5 need

| Area | Observed mechanism | M5 gap or unproved assumption |
| --- | --- | --- |
| Identity and topology | Local Compose has separate Keycloak PostgreSQL, `start-dev`, imported realm, exact localhost web callback, public PKCE client and API audience. API verifies RS256/JWKS, issuer, audience, expiry, Bearer type and `azp`; subject is the stable database key, email optional and mutable. | No production domain/issuer, proxy, TLS, Keycloak upgrade, backup/restore, session or availability plan. Current `users.oidc_subject` lacks an issuer namespace; an issuer switch needs an explicit migration and collision/reprovision analysis. Realm import does not update an existing realm. Local sign-in is not production topology evidence. |
| Provisioning and onboarding | `/auth/me` and `/users/me` upsert by OIDC subject; onboarding locks `users`, creates student `active` or alumni `pending` membership and advances profile content revision on display-name change. | Unknown valid subjects are provisioned automatically, including through owner handlers that call `identity.Me`. No disabled/deleting/deleted access state or non-reprovision guard. Recovery/re-registration rules and a fail-closed status check on every protected route are absent. |
| Roles and verification | `alumnibridge-admin` is defined as a realm role and `RequireRole` middleware can check a token claim. Onboarding cannot grant it. PostgreSQL membership status owns alumni verification; verified-only directory policy is [ADR 0002](decisions/0002-directory-discoverability.md). | No grant/revoke workflow, current-state check, admin route, reviewer scope, evidence handling or decision audit. A role claim in an unexpired token may outlive Keycloak revocation. M6 owns claim review; M8 owns verification retention/appeals. |
| Deletion and consent | SQL FKs cascade user-owned memberships/profiles; migration 000004 preserves immutable literal slug reservations and removes their user FK on deletion. PRIVATE/consent withdrawal and final verification loss clear directory consent. Public reads require current PUBLIC/VISIBLE state; Issue #11 adds ETags and membership-trigger invalidation. | No account deletion API/job or user-visible process; direct SQL test deletion is not a lifecycle service. Need immediate publication denial, token/reprovision guard, legal retention and cross-system orchestration. `profile_slug_history` keeps literal slugs after owner deletion; name-bearing slugs may remain personal data even when `user_id` is NULL. Consent history is not a full audit. |
| Photos | Private object registry tracks staging/active/garbage; cleanup retries due objects each minute in batches of 20. `owner_id ON DELETE SET NULL` leaves detached objects eligible at expiry; a DB/MinIO distributed transaction does not exist. | No deletion completion SLA, production monitoring or restore drill. Account-delete/photo-activation deadlock is reproduced; trusted deletion must retry whole transactions. Physical deletion can lag withdrawal. |
| Recovery | Local realm allows password reset; SPA logs out through Keycloak and clears Query cache. | No tested email-delivery/factor-loss/helpdesk/rebind runbook, proof policy, session cutoff or recovery audit. Password-reset setting alone is not account recovery acceptance. |

Source boundaries: [architecture](architecture.md), [PRD](prd.md), [identity migration](../infrastructure/migrations/000001_identity.up.sql), [slug migration](../infrastructure/migrations/000004_slug_reservations.up.sql), [photo migration](../infrastructure/migrations/000005_public_profiles_photos.up.sql), [membership revision migration](../infrastructure/migrations/000008_membership_discovery_revisions.up.sql), [Keycloak realm](../infrastructure/keycloak/alumnibridge-realm.json), [Issue #11 remediation](issue-11-remediation-review.md) and [rollout runbook](issue-11-rollout-rollback.md). These are source review findings, not a live deployment audit.

## Decisions requiring maintainer approval

1. **Production Keycloak operator:** self-hosted dedicated service or supported managed offering; name the team responsible for patching, uptime, secrets, Keycloak database backups, restore and incident response. Approve public issuer/domain, web/API origins and logout/session policy, including the rule for future issuer migration and existing subject bindings. Local Compose remains development-only.
2. **Admin authority:** choose application grants as authoritative with optional mandatory Keycloak role, or an alternative that proves immediate revocation. Set who may request/approve/grant/revoke, required separation of duties, expiry and periodic review. Define two-operator first-grant bootstrap and time-limited, audited emergency access without circular authorization or self-approval. Define the future reviewer capability/scope separately from platform administration.
3. **Account deletion contract:** choose owner self-service versus operator-assisted requests, reauthentication/hold period, public withdrawal timing, retained categories and exact retention windows, legal hold procedure, backup erasure schedule and completion notice. Approve a marker keyed to the issuer/subject boundary and a retention rule covering old tokens, subject recreation and restored backups. Decide the purpose, retention and privacy treatment of literal slug reservations, which may contain names; `user_id=NULL` alone does not anonymize them. Any change to the current permanent non-reuse rule needs an explicit migration and policy decision.
4. **Recovery contract:** which Keycloak password/factor recovery channels are allowed, how identity is proven when all factors are lost, who approves an exceptional same-subject restore or subject rebind, and whether a subject rebind is allowed at all. Email alone is insufficient.
5. **Audit/retention:** log fields, separate access role, storage and retention, deletion exception handling, review cadence and alert ownership. Do not preempt M6 proof/appeal or M8 verification retention decisions.

Until these decisions are approved, no production topology, deletion or recovery implementation should be represented as accepted. Independent Issue #11 production gates, branch protection #10, and M4 platform follow-ups retain their own owners.

## Recommended implementation order after approval

Use small PRs with reviewed migration/API/OpenAPI changes only when a slice needs them. Keep the modular monolith and explicit PostgreSQL transactions. No Keycloak production configuration or account data belongs in this design branch.

1. Define topology and operational baseline: domains, TLS/proxy/issuer/JWKS, exact redirects, secrets, service ownership, backup/restore and session lifetimes. Verify in a disposable production-like environment; document drift checks for existing realms.
2. Add application lifecycle/access state and denial checks before any destructive action. Existing users start active; deleted subjects cannot be automatically reprovisioned. Add a single fail-closed application status check before provisioning and on every protected route, including `/auth/me` and onboarding; define enrollment for truly new subjects and denial when the status store is unavailable. Prove an old valid token cannot access protected routes after a state change. Establish audited first-grant bootstrap, current admin grant/revoke authority and emergency controls before privileged operations.
3. Add deletion workflow: request authorization and confirmation, immediate app access/publication withdrawal in one transaction, Keycloak disable/session revoke, idempotent data erasure and Keycloak removal, photo backlog completion tracking. Keep each external step retryable and observable; never claim one cross-system atomic commit. Decide name-bearing slug retention/non-reuse and issuer-bound subject markers before erasing owner rows.
4. Add recovery runbooks and only the approved application operation needed for exceptional rebind. Exercise normal reset and lost-factor paths with disposable accounts; require fresh consent after any identity ownership ambiguity.
5. Run operational acceptance: fault injection at each step, stale-token checks, concurrency/deadlock tests, consistent restore, operator drills and documentation. This is a separate production gate; a green unit suite or local Compose login does not establish it.

### Transaction and partial-failure rules

Deletion must lock the user before profile changes, revoke application access and PUBLIC/directory/photo publication together, and retain a durable operation record. The worker must resume from persisted step state after Keycloak or object-store failure. A Keycloak disable failure leaves the application blocked; an erasure failure leaves public access blocked and a retryable restricted record. A rollback of deletion must be an explicit recovery action, never an automatic republish or consent restoration. Deletion that reaches the user-row cascade must preserve slug tombstones and detached photo registry rows. Coordinate `memberships` row → `profiles` → affiliation trigger order with the parent-user cascade and photo staging-object → profile order. Retry complete `40P01`/`40001` transactions with bounded backoff; test the exact SQL lock graph before release. Do not retry client photo uploads using stale ETags. See [Issue #11's observed race](issue-11-rollout-rollback.md#account-deletion-versus-photo-activation).

### Threat model and operational test matrix

| Adversary or failure | Required evidence before release |
| --- | --- |
| Old signed token after disable, admin revoke or deletion | Real signed JWT accepted before and denied after current application state/grant change, including `/auth/me` reprovision attempt and every protected route; database outage fails closed. |
| Forged role, self-claim or email-based takeover | No client-supplied admin/verification authority; role/grant and recovery proof boundary tests; duplicate/reassigned email cannot seize subject. |
| Partial Keycloak, PostgreSQL or object-store outage | Fault injection after every workflow step; idempotent retry, no public profile/photo/directory leak, durable job state and actionable alerts. |
| Concurrent deletion, onboarding, membership review and photo activation | Two-connection PostgreSQL tests for lock ordering, deadlock/serialization retry, no orphan public consent or lost cleanup intent. |
| Admin operator compromise or mistaken grant | Two-person approval, first-grant bootstrap, timed emergency access, scoped grant, expiry/review, immediate revocation and immutable restricted audit tests. |
| Name-bearing slug or issuer switch after deletion | Reviewed retention/non-reuse mechanism and issuer/subject marker tests across token expiry, subject recreation and restored backups. |
| Inconsistent restore or indefinite retained data | Rehearsed paired database/object-store/Keycloak restore, orphan reconciliation, photo backlog drain, retention and backup-erasure drill. |

## Draft GitHub issues (do not create during design review)

The following titles and bodies are ready for review. Numbers will be assigned only when the maintainer approves creating them. Each issue is limited to M5; it does not close the listed existing issues.

### Draft 1 — M5: Define production Keycloak topology and identity runbook

**Depends on:** ADR 0004 topology decision; no code issue dependency.

**Body:** Specify the production service owner and hosting model, stable HTTPS issuer, exact web/API/Keycloak origins and redirect/logout URIs, reverse-proxy and JWKS reachability, secret delivery, admin-console restriction, client/realm drift control, session/token lifetime, Keycloak and database backups, upgrade and restore process. Keep `start-dev` and localhost import development-only. Include a sanitized deployment diagram and operator runbook; no production credentials.

**Acceptance:** Disposable production-like topology validates login, refresh, logout, key rotation/JWKS outage behavior, wrong issuer/audience/azp rejection and exact redirect allowlists. A restore drill preserves issuer/subject identity. Rollback keeps the old issuer/client usable or blocks writes; an issuer switch requires a reviewed binding migration and cannot silently reprovision owners. Record operator and platform gaps; do not call this production tested until an actual deployment rehearsal passes.

### Draft 2 — M5: Add current application access state and administrator grant controls

**Depends on:** Draft 1 for token/session policy; ADR 0004 admin authority and retention approval.

**Body:** Add reviewed additive schema and server checks for active/suspended/deleting/deleted identities and deleted-subject reprovision prevention. Implement only the approved operator grant/revoke path with narrow capabilities, two-operator first-grant bootstrap, independently reviewed emergency access, approval, expiry, reason and restricted audit. Apply access checks before provisioning to every protected route, including `/auth/me` and onboarding; deny when the status store is unavailable. Define reconciliation with the Keycloak realm role. Do not implement M6 membership decisions or expose a broad admin API.

**Acceptance:** A still-valid signed token is denied immediately after suspension/deletion; `/auth/me` cannot recreate a deleted subject. Self-grant, forged role and cross-user actions fail. Revocation takes effect without waiting for JWT expiry; grant expiry and dual-approval failure are tested. Migration up/backfill/down passes on disposable databases, with production rollback restrictions documented. Operational drill grants/revokes a disposable operator and reconciles Keycloak/application state.

### Draft 3 — M5: Implement account deletion orchestration and consent withdrawal

**Depends on:** Draft 2; ADR 0004 deletion, retention and legal-hold decisions. Coordinate with Issue #11's release gate without changing its status.

**Body:** Add the approved owner/operator request flow, durable step state, immediate access and public publication withdrawal, Keycloak disable/session revoke, data erasure, the approved slug non-reuse/retention mechanism and photo cleanup completion tracking. Document retry, partial failure and completion semantics. Update OpenAPI and UI only for approved routes. Keep restricted verification evidence policy in M6/M8.

**Acceptance:** Public profile/photo/directory and protected routes deny as soon as deletion begins. Repeated requests and worker restarts are idempotent. Name-bearing slug and issuer/subject marker retention follow the approved policy; no deletion report calls raw retained slugs anonymous. Two-connection tests cover photo activation, membership revocation and onboarding races; `40P01`/`40001` retries restart the whole transaction. Keycloak and MinIO outages leave a safe retryable state; photo registry drains without deleting referenced objects. Deletion preserves the approved non-reuse policy and never restores consent. Test retention and legal-hold outcomes approved by the maintainer; rehearse rollback/restore without automatic republishing.

### Draft 4 — M5: Document and exercise account recovery and lost-access procedure

**Depends on:** Drafts 1–2; Draft 3 if approved recovery can intersect deletion; ADR 0004 recovery proof decision.

**Body:** Document ordinary Keycloak password/factor reset, account lockout and lost-all-factors escalation. Implement only approved same-subject restore or exceptional audited rebind logic. Freeze disputed identity and admin grants while checking proof; reject email-only matching. Specify support authority, communication and appeal boundary without implementing M6 verification review.

**Acceptance:** Disposable ordinary reset retains `sub` and member UUID; old sessions are invalidated per approved policy. Lost-factor drill shows independent approval and audit. Reassigned email, attacker-controlled profile data and old-subject token cannot claim the account. Rebind, if allowed, preserves one owner binding and requires publication/consent review. Fault injection cannot leave two active subjects mapped to one member. Runbook includes denied and unresolved cases.

### Draft 5 — M5: Perform identity operations release rehearsal and security review

**Depends on:** Drafts 1–4 and approved operational retention/backup policy; independent of closure of #2, #5, #8, #10, #11, #17 and #19.

**Body:** Run the M5 operational test matrix in a disposable production-like environment, verify exact-SHA CI, restore and rollback plans, review audit access/retention, and collect redacted evidence. Record remaining Issue #11 and other release gates separately. Do not deploy or close existing issues as part of this issue.

**Acceptance:** Signed-token revocation, admin grant/revoke, deletion/retry/photo cleanup, ordinary and exceptional recovery, Keycloak outage, paired backup/restore and issuer/redirect checks have dated evidence and named reviewers. Any untested production assumption stays explicitly open. Security review approves the scope and records a no-go if a required decision or drill is missing.

## Design review gate

Approve or revise the five decisions above and ADR 0004, then review issue boundaries and order. Stop here before application code, migrations, Keycloak production changes, GitHub issue creation, push, PR, merge, deploy or issue closure.
