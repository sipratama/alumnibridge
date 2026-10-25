# Issue #11 rollout and rollback runbook

**Status:** Review procedure, not deployment authorization. Keep PR #22 draft and Issue #11 open until the maintainer accepts the release gates. This runbook assumes PostgreSQL 17, the current single API process image, React static assets, Keycloak, and MinIO-compatible photo storage. Assign an operator and a rollback decision maker before a production window.

## Boundary and compatibility

- Migration 000007 adds profile-content, shared privacy/discovery, and per-history-item revisions. Its triggers advance them in the same transaction as changed rows. Migration 000008 replaces the older membership lifecycle trigger and adds verified-insert invalidation; it must follow 000007.
- The new API **requires** `If-Match` on profile/privacy/discovery PATCH, photo POST/DELETE, and existing experience/education PATCH/DELETE. Missing preconditions return 428; stale ones return 412. The React build captures owner GET ETags and shows an explicit conflict review. Skill set operations and history POST retain their documented owner-serialized semantics.
- An old browser tab or third-party client without `If-Match` cannot write through the new API and must refresh/upgrade. Do not temporarily make the precondition optional to preserve compatibility. A rollback to the old API with writes open restores last-writer-wins even if 000007/000008 remain installed.
- `compose.yaml` starts the up-only migrator before the API. **Do not use a routine Compose restart as a production migration plan while an old API replica can still write.** Hold all writes at the ingress or take the old API offline first. Maintain public read behavior according to the incident plan, but stop owner mutations, moderation/membership SQL writers, onboarding writes, account deletion, and photo cleanup jobs while taking the consistent backup and changing schema.

## Before the window

1. Confirm exact application/React image digests and source SHA, both exact-SHA workflows, independent security approval, migration checksum review, and the maintenance communication for old clients. Practice this runbook and restore in an isolated environment.
2. Record the live migration version, PostgreSQL health/replication lag, MinIO bucket health, photo cleanup warnings, and the read-only queries below. Investigate any `schema_migrations.dirty=true`, invalid consent row, or growing overdue photo backlog before proceeding.
3. Block writes at ingress and drain in-flight API requests. Stop or pause every direct SQL membership/moderation writer and the API photo cleanup worker. Confirm no write transactions remain (`pg_stat_activity`) and that all API replicas are fenced. Keep the write gate closed until post-deploy checks finish.
4. Take a timestamped PostgreSQL consistent backup (`pg_dump -Fc` or the approved physical/PITR snapshot) and a matching MinIO bucket snapshot/versioned restore point **after** writes are quiescent. Store backup credentials and artifacts outside the repository. Verify both backup manifests and perform a restore rehearsal; a database-only restore can leave the photo registry and object storage out of step.

Read-only checks (run with an approved read-only database role; record counts, not owner identifiers):

```sql
SELECT version, dirty FROM schema_migrations;
SELECT count(*) AS invalid_private_consent FROM profiles
 WHERE visibility='private' AND (directory_discoverable OR directory_consented_at IS NOT NULL);
SELECT count(*) AS consent_without_verified_alumni FROM profiles p
 WHERE p.directory_discoverable AND NOT EXISTS (
   SELECT 1 FROM memberships m WHERE m.user_id=p.user_id
     AND m.role='alumni' AND m.status='verified');
SELECT count(*) AS invalid_academic_consent FROM profile_directory_affiliations a
 JOIN memberships m ON m.id=a.membership_id
 WHERE m.role<>'alumni' OR m.status<>'verified';
SELECT state, count(*) AS items, max(now()-created_at) AS oldest_age,
       count(*) FILTER (WHERE expires_at<=now()) AS due_items
 FROM profile_photo_objects o
 WHERE state IN ('staging','garbage') OR
       (state='active' AND NOT EXISTS
         (SELECT 1 FROM profiles p WHERE p.photo_object_key=o.object_key))
 GROUP BY state ORDER BY state;
```

## Deploy with writes fenced

1. Apply reviewed 000007 **up**, then 000008 **up**, once, using the approved migration runner. On a fresh deployment the up-only runner may apply earlier versions first; never reapply already recorded versions. Confirm `schema_migrations` reports version 8 with `dirty=false` and inspect the installed trigger definitions. Stop on a dirty migration; do not force a version or open writes.
2. Deploy the new API image against schema 8 while writes remain fenced. Confirm startup, Keycloak verification, PostgreSQL and MinIO connectivity, and no cleanup-worker warnings. Deploy the matching React static build and invalidate old cached HTML/assets according to the web release procedure. Old open tabs remain possible and are safely rejected with 428 until refreshed.
3. Use a dedicated staging/canary owner, never production personal data without authorization, to verify GET ETags, matching write success, stale 412, missing 428, malformed 400, owner 401/404, domain 409, and PRIVATE/public Directory denial. Check the server still derives the owner from the verified token. Verify the public projection contains no private fields.
4. Re-run the read-only queries. Expect zero invalid consent counts, positive `content_revision`/`privacy_revision` and item revisions, and no active profile pointer lacking an active registry row:

```sql
SELECT count(*) AS invalid_revisions FROM profiles
 WHERE content_revision<1 OR privacy_revision<1;
SELECT count(*) AS invalid_item_revisions FROM (
 SELECT revision FROM profile_experiences UNION ALL SELECT revision FROM profile_educations
) items WHERE revision<1;
SELECT count(*) AS invalid_photo_pointer FROM profiles p
 LEFT JOIN profile_photo_objects o ON o.object_key=p.photo_object_key
 WHERE p.photo_object_key IS NOT NULL AND
       (o.object_key IS NULL OR o.state<>'active');
```

5. Reopen writes only after the new API and React are healthy on all intended replicas, old writers are fenced or upgraded, schema version/checks pass, the backup is verified, and the on-call operator explicitly accepts the cutover. Watch 428/412/5xx rates, membership/consent transitions, and photo backlog during the release window. A high 428 rate signals old clients that need a reload or upgrade; it is not a reason to disable the precondition.

## Photo backlog and safe cleanup retry

The API worker calls `PublishingService.Cleanup` once per minute with a 20-second context, processing at most 20 eligible registry rows per tick. A staged upload normally expires after 15 minutes; garbage rows are due immediately; an active object detached by account deletion is retained until its existing expiration. A failed `DiscardStagedPhoto` state transition is deliberately swallowed by the request path, leaving the durable staging row for expiry cleanup. The worker logs `photo cleanup retry pending` on failure but does not log object keys.

- Record the backlog query above before and after rollout. Alert/investigate if due-item count rises across several worker ticks, the oldest due row remains after the expected batch-drain time, or cleanup warnings persist. Check PostgreSQL connectivity/locks and MinIO availability; inspect aggregate counts and ages without exporting object keys or owner data into tickets.
- Once the underlying fault is repaired, allow the existing worker to retry on its next minute tick. If it is stopped, restart or roll **one** healthy API worker replica with the same DB/object-storage configuration, wait for a tick, and repeat the read-only backlog check. Large backlogs need multiple ticks at 20 items each. A second worker is safe because cleanup uses row locks with `SKIP LOCKED`, but avoid unnecessary concurrent manual jobs.
- There is no supported one-shot production cleanup CLI. Do not directly delete MinIO objects, remove registry rows, or shorten the 15-minute staging period to accelerate an incident. The worker rechecks that no current profile points at an object after taking the registry lock. If repeated retries fail, keep the job durable and escalate with the backup/restore plan.

## Failed rollout and rollback decision

1. **Keep writes fenced.** Prefer a forward fix of the new API/React while retaining schema 8 and its strong trigger behavior. If only React is faulty, roll back static assets while owner writes stay blocked until a compatible ETag client is restored.
2. If an application rollback is necessary, keep schema 8 installed. The pre-Issue-11 API accepts writes without mandatory `If-Match`; therefore its write routes must remain disabled until a compatible fix is deployed. Do not reopen writes solely because old endpoints respond successfully.
3. Do **not** run 000008 down in production as a routine rollback. It restores the weaker 000006 membership trigger, which can leave old privacy/discovery ETags valid after a membership changes and can retain consent during simultaneous final revocations. Do **not** run 000007 down: it drops resource revision columns/triggers, removes the atomic precondition backing, and breaks the new API. The CI disposable down/up sequence proves reversibility of DDL in isolation; it is **not** a production procedure.
4. If schema downgrade or point-in-time restore becomes unavoidable, obtain a separate incident decision with writes quiesced. Restore a consistent PostgreSQL and MinIO pair, reconcile public consent/ETags and detached photo jobs, re-run the privacy/photo invariants, and get a security reviewer to approve the exact recovery plan before any write gate opens.

## Account deletion versus photo activation

Photo activation locks its staging object row before the owner profile. Account deletion locks the user, cascades to the profile, then `ON DELETE SET NULL` needs the photo registry row. A two-connection PostgreSQL test reproduces both delete-before-activation and the resulting lock inversion (`40P01` deadlock). PostgreSQL aborts one complete transaction. The losing upload leaves a durable detached/staged object for expiry cleanup; the winner never leaves a live profile pointer after deletion. The test verifies the worker removes the object and registry row.

For a trusted account-deletion job, retry the **entire aborted transaction** on SQLSTATE `40P01` or `40001` with bounded exponential backoff and jitter (for example, at most three attempts over a few seconds), then surface an operational failure. Never retry only the final SQL statement inside the aborted transaction. The current photo HTTP handler surfaces an unclassified deadlock as 500; this is a transient operation failure, not permission to replay the upload. A photo client must not replay the old `If-Match` or reuse a staged object automatically: re-read its owner profile, stop on 404, and require a fresh owner action with the new validator if the account still exists. An API error during the deadlock is not evidence that object cleanup is complete; check the durable backlog as above.
