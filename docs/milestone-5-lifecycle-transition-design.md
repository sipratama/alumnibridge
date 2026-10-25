# Milestone 5 — lifecycle transition and publication withdrawal design

**Status:** Proposal for review; documentation only. Based on `master` after [PR #23](https://github.com/sipratama/alumnibridge/pull/23). The access check and schema merged through [PR #24](https://github.com/sipratama/alumnibridge/pull/24) are the development baseline, not a production lifecycle writer. This document approves no retention, deletion, admin, recovery, issuer migration or deployment policy.

## Goal and boundaries

A trusted application transition must deny future owner operations and withdraw public disclosure together when an account begins deletion. The locally verified token alone cannot revoke a still-valid token; the merged access check denies protected requests after an access-state commit. The current public profile, photo and directory routes do not consult `users.access_status`; changing status alone does not hide anything publicly. Existing requests may pass middleware before a transition commits. These gaps require transaction and query review before enabling a writer.

Keycloak disablement and object deletion happen outside PostgreSQL; neither can join its transaction. A durable operation record and idempotent worker are needed for the later deletion workflow. No code or migration in this design branch implements these steps.

## Proposed transaction contract for a future trusted writer

1. Authenticate the operator or owner through an **approved** authorization path. Reauthentication, who may request deletion, hold periods and emergency authority are policy gates. A normal bearer token or client-supplied role must never grant direct SQL write authority.
2. Begin a PostgreSQL transaction. Take the advisory lock on `(issuer, sub)` introduced in PR #24 **before** locking the target `users` row, matching `Provision`'s order; then lock the user before profile and dependent records. Insert the subject block **before** removing the user row. Keep the lock expression in one shared implementation; changing the issuer binding requires a separate migration. Review interactions with photo and membership locks before choosing final SQL.
3. In that transaction, set the approved non-active access state and make the profile private, clear directory discoverability and timestamp, remove academic affiliation disclosure, and increment the privacy/content revisions affected by these changes. Preserve the existing trigger semantics and photo registry cleanup intent. Do not delete a referenced photo object in this transaction.
4. Persist a unique, durable operation ID, target identity, authorized reason/reference, step state and retry metadata using a reviewed restricted schema. Commit the access denial and public withdrawal together. Only after commit may a worker disable/revoke Keycloak sessions and perform erasure. Failures leave the application blocked and the operation pending for retry and alerting.
5. For `40P01` and `40001`, retry the **whole** database transaction with bounded backoff and a stable operation ID. Never replay a stale client ETag, restore consent automatically, or retry only the failed SQL statement. A partial external step must be idempotently reconciled.

**Lock and race review required:** map actual profile/privacy, discovery, membership triggers, photo activation and onboarding lock order against `users → profile → affiliations` and `photo staging object → profile`. An advisory lock and user lock at the lifecycle writer do not by themselves stop an owner request that already passed middleware and may mutate only a profile row. Either make all relevant owner mutations serialize against the user state in the same database transaction, or prove an equivalent conditional write that checks the current state at commit. Test both transaction orders. A public read that began before the withdrawal commit may finish with the old snapshot; document the effective revocation boundary and any cache behavior rather than promising impossible retroactive cancellation.

**Development follow-up:** The [lifecycle race hardening review](milestone-5-lifecycle-race-hardening.md) maps the merged lock graph and adds active-user row guards to application owner writes. Photo activation now takes user before staging/profile. Direct trusted membership SQL still begins with the membership row and is outside that application guard. A future writer must resolve this order and require full-transaction retry before deletion is enabled; the synthetic transition in the tests is not a production writer or policy approval.

## Proposed state and publication matrix

| State | Protected owner request after access check | Public profile/photo/directory | Allowed transition |
| --- | --- | --- | --- |
| New subject with no block | Existing enrollment behavior in PR #24 | No published profile | Enrollment only |
| Active | Allowed subject to owner authorization | Existing explicit visibility and consent rules | Suspension or deletion request through trusted writer |
| Suspended | Denied by PR #24 | **Policy open:** whether every suspension withdraws publication and how reinstatement obtains fresh consent | Reinstatement only through separately approved procedure |
| Deleting | Denied | Must be withdrawn in the same PostgreSQL commit as status change | Retry external steps; no automatic reactivation |
| Deleted row with issuer/subject block | Denied, including reprovisioning | Publication must already have been withdrawn in the transition transaction; a block alone does not hide public data | Retention/purge only under approved policy |

For deletion, public withdrawal is mandatory in the proposed contract. For suspension, decide whether the state means security freeze, user pause, or another reason before implementing a writer. Restoration must not silently republish, reinstate directory consent or reuse an old revision. The duration and form of an issuer/subject block, literal slug reservation and backups remain policy decisions.

## Tests and rollout gate for the writer

- Two-connection PostgreSQL tests: owner PATCH versus transition in both orders; provisioning versus block insertion in both orders; onboarding, membership-trigger updates and photo activation versus withdrawal/deletion. Verify no post-transition owner mutation, public disclosure, lost cleanup intent or orphan active account. Exercise deadlock/serialization retries and failure exhaustion.
- Signed JWT HTTP tests: old token denied on every protected group after commit; anonymous profile/photo/directory return the approved private result; database lookup failure denies owner routes. Verify revision/ETag invalidation and explicit consent on any permitted reinstatement.
- Fault injection after each durable step: PostgreSQL commit, Keycloak disable/session revoke, object cleanup and account erasure. Repeated worker runs must be safe. No personal data, proof, tokens, passwords or object keys in ordinary logs.
- Rehearse application compatibility and rollback with mixed old/new binaries. The old API ignores new access state, so production rollout needs a write fence or other proven isolation until all writers and readers honor it. Preserve schema and restrictions on rollback; a down migration that drops protection is not a production recovery plan.
- Record a sanitized release runbook, step backlog/age metrics, alert recipient, backup/restore exercise and independent review before enabling deletion.

## Policy decisions and order

| Area | Decision required before implementation | Safe work now |
| --- | --- | --- |
| Lifecycle/deletion | Self-service or operator-assisted request; proof and reauthentication; hold and legal hold; completion notice; literal slug and subject-block purpose, representation and retention; backup erasure | Review lock graph and write contract with disposable accounts. Do not add production writers or purge jobs. |
| Administrator authority | Scope of application grants; optional Keycloak role; first grant, expiry, emergency path and distinct approval or tested compensating control | Design restricted operator procedure and tests with synthetic accounts. No production privileged route. |
| Recovery | Same-subject factor reset; accepted proof for total loss; session cutoff; whether exceptional subject rebind is allowed; independent or compensating review | Rehearse ordinary same-subject recovery in disposable Keycloak. No email-based relink or rebind writer. |
| Audit | Custodian, readers, immutable/restricted storage, fields, duration, legal hold and purge | Define minimum events and redact test fixtures; do not invent retention periods. |
| Topology | Production operator/hosting, HTTPS issuer, backup custody, issuer migration and restore target | Provider-neutral rehearsal only; no production cutover. |

**Recommended sequence:** review this transaction/race design against the access baseline merged in PR #24; approve deletion and retention policy before implementing a writer; review admin and recovery decisions as separate issues; run a production-like rehearsal only after operator/topology decisions. Milestone 6 reviewer decisions must wait for a scoped, current application grant boundary. No existing issue is closed by this proposal.
