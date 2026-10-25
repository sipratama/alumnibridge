# ADR 0005 — M5 identity lifecycle policy decision (proposal)

Date: 2026-10-25 (Asia/Jakarta).  
Status: **Proposed; no production policy approved.**  
Owner of decision: project maintainer.  
Scope: account access, deletion, retained identity markers and slugs, operator authority, audit, and recovery. This document changes no runtime behavior. It complements [ADR 0004](0004-identity-operations-boundaries-proposal.md), the [lifecycle transaction design](../milestone-5-lifecycle-transition-design.md), and the [membership boundary review](../milestone-5-membership-lifecycle-boundary.md).

## Existing boundary and decision rule

Migrations 000009 and 000010, current protected-route checks, owner-write guards, and membership trigger are merged development safeguards. No production status writer, deletion request/worker, grant service, audit ledger, or recovery rebind exists. The user key is still unique by `sub` alone. The issuer/subject block has no expiry rule. Literal slug reservations can contain names. A synthetic withdrawal in integration tests is not a production operation.

A decision is **approved** only when the maintainer records a selected option, exact parameters, date, and rationale in the decision record below. Merging this proposal does not select the proposed defaults. Never fill an undecided duration with a code default or silently interpret "until further notice" as permanent retention. Before activating production identity operations, review applicable privacy, contractual, and records obligations with an appropriate adviser; this ADR does not assert a legal retention period.

## Proposed policy contract for review

### 1. Request, withdrawal, and erasure

Recommend allowing an authenticated owner to request deletion with a fresh authentication step; an operator-assisted request is exceptional and requires separately documented proof and reason. A request gets a stable operation ID and an idempotent response. The first committed PostgreSQL transition sets access to `deleting` and withdraws public profile, photo and Directory disclosure, academic affiliations, and directory consent in the same transaction. This is the effective application denial boundary for new reads/requests; an anonymous read that began before commit can complete from its earlier snapshot. Do not claim immediate invalidation of an already served response.

After commit, a restricted worker disables the Keycloak account/revokes sessions, removes eligible personal data and objects, and records completion or pending retry. Do not claim a cross-system atomic transaction. On external failure, the application remains blocked/private, the operation remains pending, and a responsible operator receives an alert. Retry is idempotent; `40P01`/`40001` restarts the whole database transaction. Completion means all approved steps and cleanup checks are satisfied, not merely a deleted user row. An operator can pause erasure for a documented hold without republishing or restoring consent.

Decide before writing the route or worker: who can request, the fresh-authentication window, whether there is a cancellation/hold interval, who can place/release a hold, how disputes are handled, and what completion notice is sent. A blocked or disputed request must remain private pending review. A cancellation or recovery path requires an explicit new consent/publication action; it must not replay historical consent.

### 2. Data categories and retention

Use an inventory with one rule per category. A category's clock must identify its start event (request, withdrawal commit, erasure completion, or last audit event), primary storage, backups, permitted readers, hold exception, and purge evidence. Do not implement a timed purge until the table is complete and approved.

| Category | Proposed handling | Parameter requiring explicit approval |
| --- | --- | --- |
| Profile, membership, contact, verification material | Erase or deidentify after the approved hold/cancellation window, subject to a documented exception; never leave public disclosure active during a hold. M6/M8 proof retention is a separate decision. | Exact categories, clock, erasure deadline, exception authority. |
| `identity_subject_blocks` keyed by issuer and subject | Keep a restricted non-reprovision marker through the modeled risk of still-valid tokens, Keycloak subject recreation, and restores. No arbitrary TTL may be inferred from token expiry alone. | Representation, retention end condition or duration, review cadence, backup treatment, purge test. |
| Literal `profile_slug_history` reservations | Treat name-bearing slugs as potentially identifying. Preserve non-reuse during migration; evaluate a restricted one-way lookup or another design against collision and old-link behavior before changing representation. | Whether names may remain literal, visibility/readers, non-reuse horizon, retention, link behavior. |
| Photo objects and registry | Withdraw URLs first, then delete eligible objects asynchronously; retain restricted cleanup state until verified complete. | Retry window, orphan sweep, backup/object-version erasure schedule. |
| Keycloak account/session data | Disable/revoke after withdrawal; remove only at the approved step, reconciling restored backups and old subject bindings. | Hold/erasure timing, session policy, recovery handling. |
| Operation and security audit | Keep a restricted, tamper-evident record with minimal identifiers; separate operational diagnostics from retained audit. | Exact fields, retention clock/duration, reader/custodian, hold and purge. |
| PostgreSQL, object-store and Keycloak backups | Reconcile restores without republishing or reprovisioning; apply documented expiry and restore quarantine. A backup may outlive live-row erasure and requires an explicit treatment rule. | Backup lifetime, access, restore checks, erasure-on-restore procedure. |

If an approved marker retention ends while an issuer can recreate the same subject or an old backup can restore a credential, the design must prove an alternative anti-reprovision fence before purging the marker. A complete erasure claim must enumerate retained exceptions and backup delay accurately. No personal data, object key, credential, token, or verification proof belongs in ordinary logs.

### 3. Sole operator, privileged grants, and audit

The maintainer operates the early project alone. Recommend current, capability-scoped application grants as the authority on every privileged request, with an issuer/subject binding, reason, expiry, and immediate revocation. An optional Keycloak role is an additional condition only after its reconciliation path is approved. The M6 membership reviewer capability must be distinct from identity platform administration, with institution scope if that workflow needs it.

For disposable development, a manual one-time first-grant drill with synthetic identities may be recorded by the same operator; its audit is evidence of action, not independent approval. A production first grant, renewal, emergency elevation, and sensitive deletion/rebind need a named second approver **or** a specifically documented compensating control with external review, time limit, notification and retrospective check. Do not manufacture a second operator in code or treat self-approval as two approvals. Until that control is approved and tested, privileged production routes remain disabled.

Audit the actor, capability/action, pseudonymous target reference, UTC time, result, correlation ID, reason and approval reference. Restrict audit write and read paths, detect alteration, and define alert ownership. Do not place proof, free-text sensitive case details, tokens or private contact data in ordinary application logs. Exact audit retention and access review cadence remain undecided.

### 4. Recovery and issuer changes

Prefer Keycloak factor/password recovery that preserves the same issuer and subject; require the approved proof channel and session cutoff. Freeze disputed access and grants while investigating. Reassignment of an email address, matching profile fields, or claimed alumni status cannot establish ownership. A lost Keycloak record or subject change is an exceptional rebind requiring a separate policy choice, proof, authorization, old-subject fence, atomic binding change, and consent/publication review. The default pending that decision is **no production rebind**.

A production issuer change cannot reuse the current sub-only schema without a reviewed identity migration. A paired PostgreSQL/Keycloak/object-store restore must validate issuer and subject mapping, blocks, private publication, operation backlog, and orphan objects before traffic resumes.

## Explicit decision record

Fill each row with **selected option, exact parameter, rationale, decision date, and approver**; keep `Open` until that information exists. An acceptance can be split by topic, but no topic grants authority to implement another.

| ID | Choice requiring maintainer decision | Recommended starting point | Status |
| --- | --- | --- | --- |
| D1 | Owner versus assisted deletion, fresh authentication, cancellation/hold interval and completion notice | Owner request with fresh authentication; assisted exception with proof | Open |
| D2 | Erasure inventory, exceptions and timing for personal/profile/photo/Keycloak data | Withdraw immediately at transition commit; erase asynchronously with durable state | Open |
| D3 | Issuer/subject block representation, exact retention/end condition and restore proof | Restricted issuer/subject fence; no purge until risk and restore evidence is reviewed | Open |
| D4 | Literal slug reservation treatment, non-reuse horizon and old-link behavior | Preserve existing reservations while designing a privacy-aware representation | Open |
| D5 | Audit fields, custody, access, retention duration, hold and purge | Minimal restricted append-only events; independent later review | Open |
| D6 | PostgreSQL/Keycloak/object backup lifetimes and erasure-on-restore procedure | Paired restore with quarantine and reconciliation | Open |
| D7 | Production first grant, emergency access, independent approver or compensating control | Scoped expiring grant with external review of a sole-operator control | Open |
| D8 | Same-subject recovery proof, session cutoff, and whether exceptional rebind exists | Same-subject reset only; no rebind pending proof policy | Open |
| D9 | Hosting, stable issuer/domain, incident and backup operator | Dedicated production topology; provider unresolved | Open |
| D10 | Security hold/dispute authority, appeal, and records exception procedure | Restrict access and publication while held; define release authority | Open |

For each approved row, append a dated entry below rather than replacing the historical proposal. Record an owner for the operational procedure and a test that demonstrates it. A legal or platform dependency that remains unknown must stay `Open`.

### Approval entries

_None yet._

## Implementation gate after decisions

1. Approve D1–D6 and D10 before implementing a deletion request, worker, marker purge or slug migration. Keep any M6/M8 verification-proof schedule separate.
2. Approve D7 and D9, and test first-grant/emergency controls, before enabling production privileged identity operations. M6 reviewer grant design can proceed as a bounded proposal, but no reviewer action may depend on a self-issued production grant.
3. Approve D8 before enabling production recovery beyond supported same-subject Keycloak flows. A subject rebind also requires an issuer-aware binding migration and lock/race review.
4. Test the exact approved transaction and rollback/restore behavior with disposable accounts, including stale JWTs, both membership lock orders, photo cleanup, partial Keycloak/object failures, whole-transaction retries, backup restore and expiry/purge boundaries. Perform a separate production readiness review; green CI alone is insufficient.

This ADR neither authorizes deployment nor changes the status of Issue #11 or existing M5/M6/M8 follow-ups.
