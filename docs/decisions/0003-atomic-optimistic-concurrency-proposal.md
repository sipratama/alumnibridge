# Issue #11 — Atomic Optimistic Concurrency: Design Proposal

**Status:** Proposed for maintainer review — **NOT approved or implemented**  
**Date:** 2026-10-24  
**Base:** `master` at `8424ce5628bfcce1bca4371e9548e9f713bb7540`  
**Tracking:** [#11](https://github.com/sipratama/alumnibridge/issues/11)  
**Scope:** Owner profile, publication/privacy, discovery consent, experience/education, and related owner-editable collections. Do not start Milestone 5 implementation until this design receives a decision.

## 1. Problem and current code evidence

The development-only [ADR 0001](decisions/0001-last-writer-wins-development.md) accepts last-writer-wins; it **does not** accept public production deployment. [Milestone 3 final review](reviews/milestone-3-final-review.md) reproduced two same-field signed-JWT PATCH requests returning 200; Phase E reproduced simultaneous discovery PATCH requests returning 200. A stale publication request may restore PUBLIC after another session has saved PRIVATE.

Inspected current paths on `master`:

| Surface | Endpoint / current data writer | Risk |
| --- | --- | --- |
| Professional profile | `PATCH /api/v1/users/me/profile`; `apps/api/internal/profiles/postgres/store.go:UpdateProfile` locks user/profile and updates fields | Atomic serialization but no expected revision, so stale same-field writes succeed |
| Publication/field flags | `PATCH /api/v1/users/me/profile/privacy`; `postgres/public.go:UpdatePrivacy` | Stale PUBLIC or field-enabled patch may override privacy withdrawal |
| Directory and membership-scoped academic consent | `PATCH /api/v1/users/me/profile/discovery`; `postgres/discovery.go:UpdateDiscovery` locks memberships/profile | Stale discovery opt-in may revive consent after a newer opt-out without an expected-version validator; eligibility/visibility checks remain necessary |
| Experience and education | `PATCH /api/v1/users/me/experiences/{id}` / `educations/{id}`; `postgres/store.go:SaveExperience/SaveEducation` | Replacement edits can overwrite newer same-item updates |
| Experience/education deletion, skill and photo operations | Existing owner mutation APIs | Need explicit scope/validator policy and race/ABA review; do not silently exclude deletions or replacement semantics |
| React privacy/discovery editors | `PrivacyPage.tsx` and `DiscoveryPanel.tsx` read-before-write | A preflight GET is not an atomic concurrency guard |

Server-side authorization, privacy/eligibility checks, transactional revocation triggers, and public field allowlists **must stay intact**. Versioning supplements, never replaces, them. Previously delivered public content cannot be recalled.

## 2. Design goals and invariants

1. Reject writes that were based on a stale owner-resource representation **within the database write transaction**; return an actionable client error instead of silently winning.
2. **Critical privacy invariant:** after a newer PRIVATE/withdrawal write commits, a stale PUBLIC/discovery opt-in request based on a preceding revision cannot restore visibility or discoverability. Fresh *intentional* re-publication may be allowed with a newly read validator and existing explicit consent.
3. Preserve valid independent edits and support idempotent reads; avoid unrelated global contention.
4. Protect owner-scoped IDs; missing/foreign child resources remain 404, not version-oracle leakage.
5. Keep approved migrations `000001`–`000006` immutable; preserve zero data loss and tested reversible migrations.
6. Invalidate relevant React caches after successful writes; preserve unsaved drafts and explicit publication consent on `412`.
7. Document compatibility and migration rollout before deciding if preconditions are mandatory.

## 3. Proposed contract (decision requested)

**Preferred:** strong, opaque, resource-specific ETags, backed by positive monotonically increasing `BIGINT` revisions; require `If-Match` on protected PATCH/DELETE once clients are migrated. Do **not** derive ETags from `updated_at`, timestamps, the current body JSON, or client-supplied sequence values.

- Owner GET returns `ETag: "ab-<resource-code>-<opaque-revision>"` as an illustration only; final syntax must be validated to prevent cross-resource confusion and avoid exposing internal identifiers. The exact revision may be base-10 internally but externally ETags are opaque.
- Owner PATCH/DELETE sends `If-Match` with the exact ETag from the latest matching owner GET.
- Success returns the **new** ETag on applicable response, including 204 delete semantics where useful; GET afterward must reflect the committed state.
- Missing precondition: `428 Precondition Required` **after mandatory rollout**. Stale/incorrect validator: `412 Precondition Failed` with safe `{"error":"stale_revision"}`; malformed/unsupported conditional syntax: 400. Do not collapse existing domain `409` (slug conflict, membership/eligibility invalidation, capacity limits) into 412.
- No `If-Match: *` bypass in production-required mode. Define a bounded, strict ETag parser; reject ambiguous multiple validators/weak `W/` tags unless explicitly designed and tested.
- `ETag` on an authenticated owner representation does not grant access; always enforce JWT and owner checks. Set `Cache-Control: no-store` for owner privacy/discovery responses. Never include internal membership IDs in public ETags.
- `GET /api/v1/directory`, public `/in/:slug`, and public photo URLs do **not** become conditional *write* endpoints.

**Contract choice to approve:** A. mandatory 428 immediately on all supported owner mutation routes, or B. phased compatibility rollout (report/optional validation first, deploy clients, then require 428). **Recommendation:** B for active development clients, with a firm production gate requiring enforcement; **never** accept an optional validator in production while claiming stale-write protection.

## 4. Resource revision boundaries (must be decided explicitly)

**Recommended initial boundaries:**

- **Publication/privacy + discovery:** use one **shared owner privacy-consent revision** on `profiles`, incrementing on every privacy/discovery mutation, including trigger-driven PRIVATE withdrawals, membership verification loss, moderation suppression, and academic consent replacement. This shared revision is important: separate privacy/discovery counters can allow cross-surface stale opt-ins. Evaluate existing lock order and database triggers carefully, including membership-before-profile locking.
- **Profile professional fields:** a distinct `profiles` content revision (or equivalently a clearly documented shared profile revision if simpler). Define `display_name` in `users` as part of the same transactional revision; GET owner profile must return a coherent representation.
- **Experience/education:** per-row revisions for individual PATCH/DELETE. For list/replacement-style operations, define a list-level revision or another strategy that detects concurrent add/delete/reorder and prevents lost updates/ABA. Owner reads must supply the relevant child validators.
- **Skills/photo:** separately assess whether owner mutation semantics are purely append/delete, replace, or visibility-sensitive; design revisions accordingly. Photo publication **flags** belong to privacy-consent revision even if photo file data has separate mutation semantics.

**Important:** a single revision is insufficient unless every writer that changes protected state advances it, including current SQL writers, lifecycle handlers, relevant database triggers, and administrative/moderation paths. If using triggers for revision increments, avoid double increments/recursion and verify transactional rollback semantics.

## 5. Atomic PostgreSQL mechanism (illustrative, NOT final SQL)

All write requests must resolve the authenticated owner, parse the exact expected revision and perform the revision predicate inside the same transaction as all side effects. Row lock + compare + increment is equivalent when implemented correctly.

Illustrative conditional update:

```sql
UPDATE profiles
SET visibility = $new_visibility,
    privacy_revision = privacy_revision + 1,
    updated_at = now()
WHERE user_id = $owner_id
  AND privacy_revision = $expected_revision
RETURNING privacy_revision;
```

If zero rows update, distinguish stale revision vs missing owner **without leaking cross-owner resource existence**. Do not modify academic consent, photo pointers, slug reservations, audit data, or publication triggers on failure; roll back the complete transaction. For composite changes, lock rows in the established global order, compare revision before child mutations, commit once, and return a validator corresponding to committed state.

If a stale PUBLIC write races a PRIVATE write, whichever commits second MUST reject whenever based on the earlier privacy revision. A brand-new consentful re-publication after a fresh GET is separate behavior. Verify read/commit chronology explicitly with two real PostgreSQL connections; `READ COMMITTED` and `REPEATABLE READ` behavior, serialization errors and deadlocks need tests.

**Strong warning:** `UpdateDiscovery` currently locks memberships ordered by UUID then profile; triggers on membership status changes can update the profile. Keep consistent lock order or provide a demonstrated deadlock-free redesign. Avoid standalone optimistic locks in frontend only.

## 6. Frontend conflict UX

- Owner GET hooks retain the correct resource ETag alongside response data. Confirm CORS exposes ETag when API/frontend are cross-origin.
- Each editor snapshots the validator **when its draft is created**; never fetch the latest ETag moments before saving and silently apply an older draft with a newer token.
- On `412`, retain draft and pending publication consent, clearly explain what changed, provide **Reload latest**, **Review differences**, and an explicit **Apply my edits to latest** workflow that rebuilds a patch from reviewed differences and fresh validation.
- No automatic blind retries on `412` or `428`; show a useful error. After session expiry hide protected data and require reauthentication.
- Clear owner/public/directory caches after successful consent/publication writes as today; never seed public cache with owner DTO.
- Profile, history, privacy and discovery screens should share a consistent error contract without conflating `409` domain conflict and `412` stale representation.

## 7. Compatibility, migration, rollback

- Add new migration `000007_*.up.sql` / `.down.sql` **only after design approval**. Backfill revisions safely for existing rows and ensure positive non-null default; account for original schema and trigger dependencies.
- Do not rewrite migrations `000001`–`000006`.
- Rollback must refuse unsafe downgrade if the target state would silently re-enable last-writer-wins in a production mode, or require explicit operator-controlled rollback procedure. Define expected version counter handling for deleted/recreated child IDs and already deleted records.
- Versioned API changes require typed frontend transport (headers + response ETag), OpenAPI 3.1 docs, separate 412/428/400 examples, and clear client compatibility policy.
- Consider other callers (scripts, Postman, legacy clients) and startup/readiness rollout order. Record a cutover plan that makes the required precondition enforced before public launch.

## 8. Required verification after approval

1. HTTP handler tests: missing, malformed, wrong-resource, stale, matched, weak and wildcard If-Match, status 400/412/428; existing 401/403/404/409 maintained.
2. Actual PostgreSQL integration races (two connections, deterministic barrier): same-field conflict; stale PUBLIC vs PRIVATE; stale discovery opt-in vs opt-out; profile rename and slug collision; two child-item updates; delete/update race; membership revocation/verification loss; moderation transition.
3. Property/invariant checks: consent withdrawal atomic; no private data or directory result resurrected; aborted/stale write leaves all tables, slug history, photo refs, skills and academic consent unchanged.
4. Frontend tests: ETag capture and headers, stale draft preserved, explicit manual conflict recovery, tab/session race, cache invalidation; browser with disposable Keycloak for privacy and discovery if practical.
5. Normal and race Go tests; vet, build; frontend test/lint/build; PostgreSQL 17/MinIO integration; migration up/down/up and refusal guards; OpenAPI semantic validation; CI at final SHA.
6. Regression across M1–M4 and account onboarding/deletion, with real signed JWT owner authorization. No degradation of public search or 15-second Directory polling.

## 9. Design decisions awaiting approval

| Decision | Recommendation | Rationale |
| --- | --- | --- |
| Resource validators | Strong opaque ETag / If-Match | HTTP-native conditional writes |
| Privacy/discovery coupling | Shared owner privacy-consent revision | Prevent cross-endpoint privacy races |
| Profile data revision | Separate professional-profile revision | Reduce unrelated conflicts |
| Experience/education revisions | Per-row plus policy for list replacements | Prevent same-row overwrite and collection ABA |
| Skill/photo mutators | Explicitly include in inventory before implementation | Avoid leaving stale-write holes |
| Enforce 428 | Phased client cutover, mandatory before public launch | Compatibility without false production guarantees |
| Stale status | 412; existing 409 reserved for domain conflicts | Precise protocol semantics |
| Client recovery | Preserve draft, explicit reload/merge | Avoid silent overwrites |
| Migration | New reversible 000007 after approval | Existing migrations immutable |
| Production gate | #11 remains OPEN until concurrency acceptance and CI pass | Verified remediation, not design acceptance alone |

## 10. Stop conditions and review gate

This document is **design-only**: no migrations, API routes, frontend behavior, CI workflows, or development volumes were changed. Before implementation, the maintainer must approve revision boundaries, ETag syntax, 428 rollout, and handling of skills/photo/history deletion. Once approved, implement in focused backend/migration/frontend phases with explicit review checkpoints. Do **not** automatically open a PR, merge, deploy, or close #11.
