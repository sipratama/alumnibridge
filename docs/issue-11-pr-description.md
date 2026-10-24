# Proposed update for draft PR #22

## Summary

Issue #11 replaces last-writer-wins owner mutations with atomic optimistic concurrency where a mutation replaces existing state. Strong owner ETags and mandatory `If-Match` now protect profile content, shared privacy and Directory discovery consent, existing experience/education items, and the profile photo pointer. The PR remains draft; Issue #11 remains open and public production launch remains blocked pending the gates below.

## Resource boundaries and behavior

- Profile PATCH and photo upload/delete share the profile-content validator. A stale photo action returns 412 without changing the pointer. An uploaded object whose activation is rejected remains in the durable cleanup registry and is collected safely.
- Privacy and discovery share one validator. PRIVATE withdrawal and verified-membership/moderation changes invalidate older consent drafts in the same database transaction. An old PUBLIC or Directory opt-in cannot resurrect a newer withdrawal.
- Existing experience/education PATCH/DELETE use item-specific validators. POST creates a distinct new item and uses the owner lock to enforce the 100-item cap under concurrent adds; a collection validator would reject independent appends without preventing a replacement loss.
- Skills are an owner-scoped set. Add is idempotent for a normalized skill; delete targets one exact owner link. The owner lock serializes add/delete and the link ID prevents an old delete from removing a later re-add. There is no whole-set replacement API, so no collection validator was added.
- React editors keep their original draft and validator on 412, show the latest saved values, and require the owner to select edits to reapply before a second Save. History DELETE and photo actions require explicit review of the newer version. Background refetch failures preserve open drafts. PRIVATE-to-PUBLIC still requires fresh explicit consent.

## Schema and API

Migrations 000007 and 000008 are additive to the immutable 000001–000006 history. 000007 adds positive content, shared privacy and item revisions with trigger advancement. 000008 invalidates shared consent revisions for relevant membership lifecycle changes, including simultaneous final revocations. No 000009 migration is needed for the chosen collection and photo boundaries. OpenAPI documents required photo `If-Match`, new ETags and 400/401/404/409/412/428 behavior. The dedicated PostgreSQL workflow applies all current migrations; its step names no longer imply a 000007 ceiling.

## Evidence

- PostgreSQL 17 two-connection tests: photo upload versus delete in both commit orders; stale staging cleanup and owner isolation; independent history appends; serialized 100-item capacity; duplicate skill add; delete/re-add with distinct link IDs. Existing tests cover signed JWT authorization, stale privacy/discovery/item writes, membership revocation and rollback, public privacy projection and MinIO cleanup.
- Disposable PostgreSQL/MinIO: migrations 000001–000008 applied, 000008/000007 down then up; `go test -count=1 ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...` passed locally.
- Frontend: 70 tests across nine files, lint, typecheck and production build passed locally. Conflict tests cover profile, privacy, discovery, experience/education, stale DELETE, draft recovery and failed background refetch. An untouched discovery form refreshes eligibility after publication while an edited draft keeps its original validator.
- OpenAPI 3.1 semantic/reference validation and strict duplicate-key YAML parse passed (19 paths). Compose config and `git diff --check` passed locally.
- Chrome/Playwright with two independent browser sessions against disposable Keycloak, API, web, PostgreSQL and MinIO passed PRIVATE withdrawal (200/412 with preserved draft), Directory consent withdrawal (200/412 with preserved academic draft), and stale experience DELETE (412 comparison, explicit adoption, then 204). No persistent development identity or volume was used.
- The main four-job CI and dedicated PostgreSQL concurrency CI passed at baseline `9aeba6ff05843bdc4415a7ceeb82cb8069975996`. **Those runs do not test the final local remediation SHA.** Run both workflows on the pushed final SHA before approval.

## Review and release gates

- [ ] Security reviewer accepts the resource boundaries, photo staging cleanup, lock order and public/private projection.
- [ ] Maintainer reviews the two-session Keycloak browser evidence on the final local application build and decides whether an additional CI browser gate is required.
- [ ] Main four-job CI and dedicated PostgreSQL concurrency CI pass on the exact pushed remediation SHA.
- [ ] Maintainer reviews 400/401/404/409/412/428 contracts and rollback procedures before removing draft status.
- [ ] Production gates are decided separately: Issue #11 acceptance, ingress/proxy abuse controls, deployment/rollback planning, legal consent review and other operational gates in the milestone acceptance report. This PR does not claim production readiness.

## Rollback

The conditional-write API can be rolled back only with writes quiesced and an operator-reviewed compatibility plan. Downgrading 000008 restores the older membership trigger and can reintroduce stale shared ETags and simultaneous final-revocation risk. Downgrading 000007 removes validators. Reconcile consent/ETags and retained photo staging records before reopening writes; never apply the disposable down/up procedure directly to a public deployment.

## Linked work

Issue #11 is the security production blocker. Milestone 4 issues #14–#19 remain separately tracked; this PR does not close them automatically.
