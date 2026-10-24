# Issue #11 — Implementation phases and release gates

**Status:** Implementation and local security verification in progress. Mandatory conditional writes are implemented for the profile, privacy/discovery, and existing experience/education items. The remaining mutation boundaries and release conditions are recorded in [the remediation review](issue-11-remediation-review.md). Issue #11 remains open and production is blocked.
**Approved design:** [ADR 0003 proposal](decisions/0003-atomic-optimistic-concurrency-proposal.md) (maintainer approved 2026-10-24).
**Branch:** `security/issue-11-optimistic-concurrency` from design commit `cba2ca0`.

## Phase A — Migration and PostgreSQL revision groundwork

Committed `000007_resource_revisions.up/down.sql`:

- Add positive `BIGINT` counters `profiles.content_revision`, `profiles.privacy_revision`, and per-row `profile_experiences.revision` and `profile_educations.revision` initialized to 1.
- Profile trigger bumps professional-field revision on field changes, and the **shared privacy/discovery revision** when publication, moderation, discovery consent or visibility changes.
- Existing `withdraw_directory_consent()` trigger should remain effective; test PRIVATE withdrawal and counter bump as one atomic write.
- Per-item triggers bump revisions for updated history rows.
- Add `revision_migration_integration_test.go` covering counter boundaries, no-op, PRIVATE withdrawal, per-item updates and rejected SQL rollback.

**Implemented after Phase A:** consent-only affiliation updates bump `privacy_revision` inside the `UpdateDiscovery` transaction. Migration 000008 invalidates the shared revision for membership lifecycle changes and serializes simultaneous last-membership revocations. Onboarding display-name changes invalidate profile content revisions. The remaining collection/photo boundaries are classified in the remediation review.

**Phase A acceptance:** run migration up/down/up against disposable PostgreSQL 17; run all integration tests with schema 000007, isolation/trigger ordering checks and two-connection races. Nothing is reported PASS until actually executed.

## Phase B — Backend atomic guards and HTTP contract

- Introduce an internal revision/conditional-write abstraction with per-resource strong ETag mapping and strict `If-Match` parser.
- Track source ETag in owner GET/patch responses; no public identifiers or validators exposed to anonymous Directory.
- Use owner-scoped, atomic compare-and-update / row-lock-and-compare inside complete transactions. Ensure the same privacy revision guards BOTH privacy PATCH and discovery PATCH; stale condition returns HTTP 412 with stable `stale_revision`.
- Define 428 missing-precondition enforcement/cutover, 400 malformed, existing 409 domain conflict, 401 auth, 404 owner/child not found.
- Do not perform any side effects (slug reservation, academic consent delete/insert, photo references) before validating the expected revision; roll back on failure.
- Protect experience/education updates and deletes with per-row revision. Decide list-level policy for adds/deletes, skills, photo and account lifecycle; document all remaining holes.
- Preserve membership->profile->affiliation lock discipline. Add deterministic two-connection race tests.

**Phase B acceptance:** no stale same-field write returns 200; no stale publication request restores PRIVATE; no stale consent restores revoked directory listing. Signed-JWT authorization unchanged.

## Phase C — React integration

- Capture ETag with each original owner GET result; update shared API client to read `ETag` (CORS `Access-Control-Expose-Headers` as required).
- Send draft's original validator, not a just-refetched validator, with PATCH.
- On 412 retain drafts and any required publication confirmation; show server state, compare choices and allow **explicit** retry only after reviewing latest values.
- Apply to Profile, Privacy, Discovery, Experience, Education and classified skills/photo mutations. Invalidate owner/public/Directory caches on success as before.
- 428 never triggers a blind automatic retry; require reload/clear explanation.

**Phase C acceptance:** deterministic component tests and disposable Keycloak browser runs for two-session race, opt-out/revocation, stale retry and session expiry.

## Phase D — Validation, compatibility and release review

- Test all migration sequences and rollback guards in disposable PostgreSQL 17 with MinIO.
- Validate OpenAPI; update frontend typed contracts.
- Go test/race/vet/build, frontend test/lint/build, integration, `git diff --check`, CI at exact SHA.
- Verify existing Milestones 1–4 privacy/photo/directory/pagination/15-second polling.
- Confirm phased 428 cutover is explicitly safe. **Do not present optional If-Match as production-safe.**
- Review Issue #11 against all acceptance criteria; no PR/merge/issue closure without maintainer authorization.

## Current gates

1. Complete the focused mutation and React conflict audit in the [remediation review](issue-11-remediation-review.md), including the explicitly unguarded collection/photo routes.
2. Validate the final committed SHA in GitHub CI after maintainer approval to push. Local test results must not be attributed to that future SHA.
3. Keep Issue #11 open and public production launch blocked pending formal review and maintainer release decisions.
