# ADR 0001 — Temporary last-writer-wins semantics for development

Status: **Accepted for the development baseline only** (2026-10-24). Production use remains blocked pending atomic concurrency controls.

## Context

Milestone 3 introduced owner profile, experience/education, and privacy updates. The [final review](../reviews/milestone-3-final-review.md) reproduced simultaneous writes to the same profile field: both requests returned 200 and one value was lost. The privacy update path also permits a stale public choice to race a withdrawal of publication consent.

Current database statements enforce ownership and atomicity. Different-field profile patches survive concurrent changes, and slug reservations are collision-safe. These safeguards **do not** detect stale writes to the same field, and client-side preflight reads are not atomic with the subsequent update.

## Decision

For **local development and feature work only**, allow the existing **last-writer-wins** semantics for simultaneous same-field edits. Do not describe this as optimistic concurrency or as a cross-session privacy-revocation guarantee. Preserve all owner-authorization, privacy projection, moderation, and safe-response safeguards already implemented.

This temporary decision **does not authorize public production launch**. The privacy revocation race is higher priority than purely cosmetic editor collisions.

## Follow-up

Tracked by [optimistic concurrency issue #11](https://github.com/sipratama/alumnibridge/issues/11).

Before public multi-session use:
1. Specify how concurrent edits of profile, privacy, experience and education should conflict.
2. Introduce server-issued resource revisions/ETags or equivalent expected-state preconditions.
3. Check the revision atomically within the SQL transaction/UPDATE, never using only a frontend GET followed by PATCH.
4. Decide whether preconditions are mandatory (e.g. HTTP 428) or optional for compatibility; document 409/412 and draft-preservation behavior.
5. Ensure a stale PUBLIC request cannot silently undo a newer transition to PRIVATE.
6. Add concurrent signed-JWT integration tests, OpenAPI contracts and UI conflict-resolution tests; use a new migration if required.

## Scope and risks

This does not change deployed code or database schemas. It records the known behavior, the limited acceptance for the development baseline, and the production gate. Do not infer that photos, slug reservation, or authorization have this same defect merely because their updates are concurrent.
