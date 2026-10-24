# Milestone 4 Phase B review and remediation

Review date: 2026-10-24 (Asia/Jakarta).
Reviewed implementation: `3d28c8bdce9c539e16dc9bf3349bec1e8f6c0dec`.
Branch: `feature/milestone-4-alumni-directory`.
Verdict: **READY FOR PHASE C WITH CONDITIONS** after user review. Phase C was not started.

## Scope and preflight

The working tree was clean at review start. Origin was fetched; the feature branch matched its remote at the reviewed SHA, with 5 commits ahead / 0 behind `origin/master` (`1537c88`). Phase A and the three Phase B commits were present. Reviewed AGENTS, approved directory design/ADR 0002, development concurrency ADR 0001, Phase B evidence, migrations, API contracts/configuration/wiring, and Issues #14/#15/#16/#18/#11. No unrelated changes were overwritten.

Reviewed directory domain/service/cursor/HTTP/pgx code, profile discovery service/HTTP/pgx, migration 000006 up/down, existing profile/identity lock paths and associated tests. Runtime infrastructure remained untouched except new disposable audit resources. No schema remediation was needed: migrations 000001–000006 are unchanged.

## Findings by severity

No Critical or High vulnerability was confirmed. Two Medium findings were remediated. One Low contract defect was fixed; two Low scale/readiness findings are explicitly deferred to Phase C assessment rather than introducing broad hardening here. Existing #11 remains a separate accepted development limitation and production gate.

### M1 — Medium: Go and PostgreSQL normalization disagree (fixed)

Evidence at reviewed SHA: `apps/api/internal/directory/directory.go:98` uses Go Unicode lowercase plus `strings.Fields`; `apps/api/internal/directory/postgres/store.go:29–31` uses locale-dependent PostgreSQL `lower`, `btrim` and POSIX whitespace regex.

Root cause: the two runtimes use different Unicode/case/whitespace tables. PostgreSQL 17.11 Alpine/musl with `en_US.utf8` leaves NBSP/narrow spaces and some uppercase characters unchanged. An exhaustive diagnostic over Go simple-uppercase mappings found 124 lowercase differences on this runtime. The database's Latin accented examples worked, but that did not establish contract equivalence.

Impact: valid public profile names could not be found even using their own text; company/title had the same mismatch. Onboarding trims only the ends of a display name, so non-ASCII spaces are reachable without a privileged database change. This is search correctness, not a privacy bypass.

Reproduction: `TestReviewLiteralPrefixNormalization` failed against the original SQL for NBSP, Unicode spaces, circled capital A and U+1C89, across name/company/title. It passed after correction. ASCII/Indonesian spacing, mixed case, accented Latin, Turkish I, sharp-S, Greek, literal percent/underscore/backslash and negative accent/expanding-fold cases also pass.

Fix: new `directory/postgres/normalization.go` derives a one-rune translation from the same Go `unicode.ToLower`/`unicode.IsSpace` tables used by query validation. SQL translates using bound parameters, collapses/trims literal ASCII spaces and compares under explicit C collation. Values, escaping, privacy predicates and page bounds remain parameterized/unchanged. No dependency, SQL function, stored normalized column or migration is introduced. Semantics are simple Unicode lowercase, not accent stripping, NFC normalization, fuzzy matching or multi-character folding (`Straße` does not equal `STRASSE`).

Cost: the shared translation parameters total 8,614 UTF-8 bytes on Go 1.27.3 and are constructed once per process. Prefix evaluation remains an expression scan; the small plan assessment below makes the Phase C performance cost explicit.

### M2 — Medium: profile edit reverses account-deletion lock order (fixed)

Evidence at reviewed SHA: `apps/api/internal/profiles/postgres/store.go:114` selects `FOR UPDATE OF p,u`, allowing the profile lock to be held while waiting for its user row. Account deletion holds the parent user row before cascading to children; onboarding already locks the user first.

Reproduction: `TestReviewProfileEditAndAccountDeletionLockOrder` holds a parent lock in a deletion transaction, starts the actual profile store update, waits until PostgreSQL reports the lock dependency, then deletes the account. The original code returned `SQLSTATE 40P01` for deletion while the edit succeeded. The controlled reproduction used a disposable-session deadlock timeout; the committed regression needs no privileged timeout setting.

Impact: concurrent offboarding/deletion can abort unnecessarily, presenting transient failures. PostgreSQL aborts the victim transaction, so this is an availability/correctness defect, not a demonstrated authorization bypass or partial deletion. This path existed before directory; it is relevant to the requested profile/discovery/lifecycle lock audit.

Fix: `profiles/postgres/store.go:113–119` explicitly locks the user first, then only the profile. A deletion that already owns the parent can finish; the waiting edit returns the existing non-disclosing not-found error. Adds one parent-lock round trip per profile update. No generic retry loop, lifecycle redesign or schema change.

Regression: deterministic deletion/edit test passes. Twelve rounds of actual onboarding/profile/discovery store calls also pass concurrently; existing privacy/discovery and membership-revocation/discovery races continue to pass.

### L1 — Low: parseable YAML was not a valid OpenAPI response (fixed)

Evidence: `apps/api/openapi/openapi.yaml:227` had an unquoted description containing commas inside a flow mapping. YAML interpreted the following phrases as additional response-object properties.

Reproduction: pinned `@apidevtools/swagger-parser` 13.1.0 rejected the photo POST 400 response with unevaluated-property errors. Quoting the description made the complete OpenAPI 3.1 document pass semantic/reference validation (19 paths). No runtime status/body changes. Earlier YAML parsing alone did not catch this existing documentation defect.

### L2 — Low: private owner membership choices have no page cap (documented)

Evidence: `apps/api/internal/profiles/postgres/discovery.go:16–22` aggregates all verified memberships for the actor. The 20-affiliation limit applies to publication selections, not the private choices list.

Measured: `TestReviewOwnerMembershipCatalog` creates 301 verified memberships in isolated fixtures; the response is 75,672 JSON bytes, ordered deterministically by institution/program C slugs, with publication flags false. It is one query, not N+1, and no other user's choices are exposed.

Pagination is not necessary for the current single-program catalog and Phase B development baseline: callers cannot mint trusted verified memberships through onboarding, and anonymous requests cannot reach this response. It becomes necessary to reconsider a bounded authenticated selector before large catalog/membership deployments. Adding pagination now would change the approved owner response/selection UX. Owner resolution also calls identity.Me, which already loads the actor's all-memberships projection, so paging this new list alone would not bound the entire path. Do not silently truncate consent choices. Phase C should specify a compatible bound/picker contract if measured inventory warrants it.

### L3 — Low: expression scans and operational budgets remain Phase C work (documented)

The query is result-bounded but not work-bounded by row count. Name-prefix search can scan users; consent aggregates may use sequential scans on small tables. The API has a 10-second request context and pgx cancellation, but the isolated PostgreSQL `statement_timeout` is disabled; there is no dedicated search statement timeout, rate limiter or global search admission policy. No 429 is advertised.

The assessment below demonstrates this limitation; it does not justify adding speculative indexes or implementing the entire Phase C roadmap in a review. Logical privacy tests do not prove constant-time SQL or resistance to statistical timing analysis. In the observed name plan, normalization occurs in a users scan before the eligibility join; no private profile is returned, but this is an explicit timing/performance review item for Phase C's eligible search projection/plan design.

## Consent, authorization and privacy analysis

- Existing and new profiles default discovery false with null timestamp. A preexisting PUBLIC fixture stayed undiscoverable when migration 000006 was applied.
- Owner routes require the real JWT verifier and actor-to-UUID resolution. Body IDs cannot select an owner; membership references are checked with actor ownership and verified alumni status. Foreign/nonexistent references share 404. No owner-supplied moderation or verification marker is accepted.
- PUBLIC/VISIBLE/trusted-verified prerequisites are read under the consent transaction's locks. Owner opt-in does not change visibility. Explicit opt-out and the existing PRIVATE update clear directory consent, timestamp and academic flags in one committed transaction. SQL failure injection demonstrates complete rollback.
- Directory SQL rechecks all eligibility conditions and published-field predicates. Pending/student/rejected/no-consent/private/hidden states are excluded. Company/title CASE expressions, skill flag plus exact membership links, and consented verified academic EXISTS prevent hidden fields contributing logical matches. Multiple academic filters use the same membership row.
- Tests compare two actual profiles with identical public core except canonical slug and different private values; their public projections agree after removing the public identity key. Changing one profile's private company/title/year leaves the entire anonymous response byte-for-byte unchanged. Hidden-filter probes return no results. Existing signed-token/privacy tests remain active.
- Public DTOs exclude email, OIDC subject, internal user/membership IDs, evidence, moderation state, timestamps, photo object keys and counts/facets. No new autocomplete/export endpoint exists. The existing shared skill vocabulary is not a disclosure of a private user's skill links.
- Historical slugs do not create directory rows. Existing public historical redirects, tombstone non-reassignment, private-photo delivery, replacement and durable cleanup regressions pass. Account deletion remains supported after the lock-order fix.
- New routes support both slash variants without redirecting to Keycloak. Strict key/duplicate/length/UTF-8/control validation remains intact. JSON unknown fields, duplicate keys, null/missing flag distinctions, 20-selection and 16 KiB bounds are tested. Safe 400/401/404/409/413/415/500/503 handling and no-store/nosniff are covered; extra handler regressions verify generic canceled/deadline/SQL-error responses. An in-flight request blocked on an exhausted one-connection PostgreSQL pool also returns safe 503 when its context expires; this is not a benchmark of database statement cancellation.

## Transactions, SQLSTATE 40001 and retries

Migration 000006's membership BEFORE trigger removes changed membership academic consent, touches the owner profile as a serialization point, and clears discovery after last-verified loss. A user already being cascade-deleted is excluded from the initial profile touch. No trigger body or order was changed in this review.

At READ COMMITTED, profile serialization plus fresh trigger statements handles concurrent different-membership revocations: the last completed revocation observes that no other verified membership remains. Reverification cannot resurrect removed academic consent or withdrawn discovery.

At REPEATABLE READ/SERIALIZABLE, a membership change with a snapshot older than another profile write may raise 40001, including two different memberships for the same owner. This is intentional fail-closed behavior, not the previous deadlock. `TestMembershipRevocationWithOldSnapshot` confirms the second command aborts and an explicit whole-transaction retry from fresh state withdraws consent; later verification does not restore it. The error is not swallowed by the trigger/store. An unexpected persistence failure on existing HTTP paths remains a generic 500, never raw SQLSTATE or details.

The current discovery store uses db.Begin with the database default isolation, which was READ COMMITTED in the tested environment; it does not contain a membership status writer. Future membership lifecycle writers should own bounded, cancellation-aware whole-transaction retries for 40001 and carefully selected 40P01 cases, re-read authorization/current membership state, and preserve atomic revocation. Retrying only the failing statement of an aborted transaction is incorrect. Arbitrary multi-owner/bulk privileged SQL still needs deterministic membership/owner ordering and full rollback/retry; no universal deadlock-freedom claim is made.

Do not add automatic blind retries to owner publication choices here. An old opt-in remains an old user intention and could undo a later withdrawal if replayed after eligibility changes. Semantic 409s must not be retried as transient failures. Expected-state validators/UI draft resolution are the separate [#11](https://github.com/sipratama/alumnibridge/issues/11) production gate under [ADR 0001](../decisions/0001-last-writer-wins-development.md). Locks/SQL transactions are not optimistic concurrency.

## Cursor security results

AES-256-GCM is retained. Configuration strictly requires canonical base64 of 32 bytes, with generic configuration errors. Nonces come from crypto/rand. Purpose binding, authenticated decryption, payload version, expiry/future-bound checks, normalized-filter hash, page size, canonical public slug and maximum 2048-byte canonical URL-safe token all remain enforced.

Additional tests cover authenticated invalid version/expiry/future time/slug/filter payloads, random/truncated/tampered tokens, nonce/token uniqueness, skill-filter ordering/binding, restart with the same key and rotation to another key. Same-key restart preserves a still-valid cursor; rotation rejects old tokens. The payload has only version/expiry/fingerprint/public anchor; no internal identity/private field. Application logs record route path rather than query/cursor/key values.

Static canonical-slug order and limit + 1 traversal pass. Hidden/deleted anchors remain lexical boundaries; later queries exclude withdrawn profiles. Live inserts/renames can still skip/repeat across requests, and data already delivered or read in an older in-flight snapshot cannot be recalled. No cross-request snapshot or perfect revocation is claimed.

## Small SQL plan assessment

Ran actual remediated SQL with EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) on 1,000 disposable synthetic accounts with mixed PUBLIC/PRIVATE/HIDDEN/consent and published-field states. Each case fetched 21 rows (20 plus lookahead). Setup and data were rolled back afterward. These are single local observations, not throughput benchmarks or production SLOs.

| Filter | Execution time | Observations |
| --- | --- | --- |
| Empty browse | 1.298 ms | Partial directory slug index and verified-membership lookup used |
| Name prefix | 56.712 ms | Users sequential scan plus normalization; primary Phase C measurement target |
| Published company prefix | 2.426 ms | Eligible slug index; field predicate retained |
| Institution/year | 1.658 ms | Verified/reference indexes; planner also scanned small membership/consent/reference tables |
| Exact published skill | 1.506 ms | Normalized vocabulary and profile-skill indexes used; users scan also chosen |

Public aggregate loops are SQL correlated subplans on the limited page, not application N+1 network queries; maximum 20 affiliations and 100 skills per entry. No unnecessary index was added. Phase C must evaluate representative larger/selective/skewed fixtures and privacy-safe normalized indexing, rather than assuming a generic lower()/ILIKE index matches the corrected normalization expression.

## Executed verification and environment

Host: macOS. Backend: Go 1.27.3 Linux/arm64 container with GCC/musl race support. Database: PostgreSQL 17.11 Alpine/musl, `en_US.utf8`. Storage: existing pinned development MinIO image in a new isolated container. New audit containers used tmpfs data, a dedicated network and no existing development volumes. Frontend commands executed natively on macOS with Node 22.23.3. This is not native-macOS Go or Windows/Podman verification.

| Check | Outcome |
| --- | --- |
| Baseline Go test/race/vet/build | Passed against disposable PostgreSQL/MinIO before remediation |
| New normalization regression before fix | Failed on NBSP/Unicode-space/case-table examples, establishing M1 |
| Controlled deletion/edit regression before fix | Reproduced 40P01, establishing M2 |
| Remediated `go test -count=1 ./...` | Passed, including SQL/signed-JWT/privacy/serialization/MinIO integrations |
| Remediated `go test -race -count=1 ./...` | Passed with the same integration variables enabled |
| `go vet ./...`, `go build ./cmd/api` | Passed |
| `npm run test` | Passed: 29 tests in six files |
| `npm run lint`, `npm run build` | Passed |
| Migrations 000001–000006 up/down/up | Passed on an empty disposable database |
| Existing PUBLIC + 000006 down/reapply | Default remains false; tombstones/photo registry retained |
| 000004/000005 rollback refusal guards | Refused unsafe tombstone/cleanup rollback as expected |
| OpenAPI | Full semantic/reference validation passed with swagger-parser 13.1.0 after L1 fix; 19 paths |
| Markdown links / `git diff --check` | Validated before local commit |

The PostgreSQL tests require explicit TEST_DATABASE_URL, and storage tests require explicit TEST_OBJECT_STORAGE_*; these were set to the new audit services. No integration test used active application data. The baseline CI and local commands run all packages, so skipping integration in the unit-only backend job is covered by the separately enabled integration job. Initial infrastructure probes before PostgreSQL readiness, and an unsupported lc_ctype configuration lookup, were corrected; they are not counted as successful tests. The intentional failing regression and pre-fix OpenAPI validation are likewise not passes.

No application browser interactions or screen-reader/Windows tests were executed. Browser Directory flows remain Phase D/E work; no Directory UI was added. No real account was promoted, no persistent schema reset or volume deletion was performed, and no Phase C implementation began.

## Exact-SHA CI evidence

The reviewed SHA was already pushed, but had zero checks/runs because `.github/workflows/ci.yml` automatically triggers on master pushes and master-targeting PRs, not ordinary feature pushes. Using its authorized workflow_dispatch entry, this review started [run 40066120046](https://github.com/sipratama/alumnibridge/actions/runs/40066120046) on the existing feature branch.

GitHub confirms head SHA **3d28c8bdce9c539e16dc9bf3349bec1e8f6c0dec**, status completed, conclusion **success**, with all four jobs successful: Backend / test, race, vet, build; Frontend / test, lint, build; Integration / PostgreSQL, MinIO, migrations; Repository / whitespace. The integration job supplies an isolated PostgreSQL service, test-only MinIO container and explicit database/storage variables, applies/down/reapplies every migration and runs `go test -count=1 ./...`.

This is evidence for the original reviewed implementation, not the additional unpushed remediation commits. Local remediated checks passed; exact-remediation GitHub CI must run after the maintainer authorizes pushing or creating a PR. No old-master result is substituted for that missing evidence. No workflow/branch protection configuration was changed. A minimal follow-up is a pinned semantic OpenAPI validation check in CI with reviewed tooling/lockfile, because L1 passed the earlier YAML-only checks.

## Changes, remaining conditions and issue status

Changed code: directory SQL plus new normalization adapter; profile update parent-first locking; cursor and safe-handler regression tests; new PostgreSQL review regression suite. Changed contract/docs: quoted photo response description, explicit normalization semantics, implementation/design notes and this report. No dependencies or migrations changed in the repository.

- **#14:** approved privacy/consent policy remains implemented and independently covered. The normalization wording is clarified without expanding product scope. Leave OPEN for milestone acceptance.
- **#15:** backend foundation and targeted remediation pass local review/tests. Leave OPEN pending maintainer review and CI on the remediation SHA.
- **#16/#18:** baseline cursor/privacy/concurrency checks pass. Statement timeout/admission/rate controls, larger plan measurements, normalization/index strategy and large private catalog bounds remain explicit Phase C priorities.
- **#11:** remains OPEN, unchanged and production-blocking. No version/ETag or stale-intent redesign was implemented.

Conditions for proceeding: review the focused fixes and obtain user approval before starting Phase C. Before merging the remediations, run CI for their exact pushed SHA. Production remains unapproved due to #11 and the previously documented storage, moderation/verification, privacy/legal/retention, operations and recovery gates. No issues were closed or commented on; no PR, merge or push occurred during this review.
