# Milestone 4 Phase C — Final review and verification

Date: 2026-10-24 (Asia/Jakarta).
Verdict: **READY FOR PHASE D WITH CONDITIONS** for the development baseline. Production readiness is not established.

## Reviewed revision and final repository state

- Branch: `feature/milestone-4-alumni-directory`.
- Reviewed and final Git HEAD: **`51be6d0b164f3345df6f3bd1e928ca701f5409cf`**.
- Preflight tree was clean; `git fetch origin` confirmed the same feature branch SHA remotely. Reviewed the complete Phase C diff against `830f463`, AGENTS, architecture, Phase C evidence, query/consent design and acceptance criteria for Issues #16/#18.
- No old migration, dependency, frontend implementation or unrelated resource change was found in the Phase C diff. No migrations are added or edited by this review.
- This review leaves four intentional **uncommitted** files: the OpenAPI correction, two verification test files and this report. No new commit/push/PR/merge/issue modification was performed. Therefore the Git HEAD remains the requested SHA; the working tree differs from that committed revision.
- Exact-SHA CI below verifies the committed `51be6d0` implementation. It does **not** verify the uncommitted OpenAPI correction or added review tests; those were verified locally. After maintainer approval, commit/push the review changes and obtain CI for the new SHA before milestone merge. Do not attribute this CI run to a future commit.

## Exact-SHA GitHub Actions

No run existed for `51be6d0` at preflight. Dispatched the existing workflow on the existing feature branch with:

```sh
gh workflow run ci.yml --repo sipratama/alumnibridge --ref feature/milestone-4-alumni-directory
```

[Run 40079447618](https://github.com/sipratama/alumnibridge/actions/runs/40079447618) completed **successfully**, with exact head SHA `51be6d0b164f3345df6f3bd1e928ca701f5409cf`:

| Job | Result |
| --- | --- |
| Backend / test, race, vet, build | PASS |
| Frontend / test, lint, build | PASS |
| Integration / PostgreSQL, MinIO, migrations | PASS |
| Repository / whitespace | PASS |

Inspected integration job logs: migration lifecycle and `go test -count=1 ./...` completed with directory and profiles PostgreSQL packages executing successfully. The old `830f463` CI run is not used as evidence for this SHA.

## Confirmed findings

| ID | Severity | Evidence and impact | Remediation |
| --- | --- | --- | --- |
| L1 | Low | At reviewed HEAD, `apps/api/openapi/openapi.yaml:302`/`:303` and `:328`/`:329` declare search-only 408/429 responses on authenticated owner discovery GET/PATCH. `apps/api/internal/profiles/http/discovery.go:17` registers only cache/auth middleware; `directory/http/handler.go:30` scopes search controls to `/directory`. The contract incorrectly suggests owner consent uses the search bucket/cancellation codes. | Removed those four owner response references; preserved directory responses. Reproduced contract mismatch with parsed OpenAPI and a real API test: after public search reached 429, signed owner GET and PATCH still returned 200. Semantic validation and owner/search contract assertions passed after correction. |

**Critical: 0; High: 0; Medium: 0; Low: 1 (fixed in working tree).** No production Go implementation change was necessary. The additional tests close verification gaps without changing behavior or SQL architecture.

Existing deployment limitations are separated below from new Phase C defects. The private owner catalog and Issue #11 remain documented limitations, not silently claimed as remediated.

## Security and reliability assessment

### Rate arithmetic, bounded state and identity

Reviewed `apps/api/internal/directory/http/controls.go:51` and configuration bounds at `apps/api/internal/config/config.go:67`:

- One token is exactly 60,000,000,000 integer credits. With the largest valid burst (1,000), capacity is 60,000,000,000,000, safely within int64. Refill compares elapsed time with time-to-cap **before multiplication**; the multiplication branch cannot exceed the remaining capacity. No floating-point precision dependency remains.
- `Retry-After` uses ceiling division of remaining credits by credits/second. For configured rate 1–60,000/minute, the denominator and numerator fit int64 and the result is 1–60 seconds. Slow refill at 59/60 seconds, fractional refill and simultaneous burst tests pass.
- The finite map is bounded by validated 1–100,000 clients (default 10,000). Lazy bounded sweeps expire inactive entries, and full tables fail closed; active client replacement does not grant fresh bursts. TTL validation requires a complete burst refill before possible state expiry. Map allocation remains bounded even though Go may retain deleted map capacity.
- `controls.go:41` derives keys from parsed direct peers, drops ports/zones and unmaps IPv4-mapped IPv6. Equivalent IPv6 spellings share a key. Neither forwarding header is read. Authentication gives no exemption.
- Reverse proxies intentionally share their API-facing peer bucket. A real socket test with forged `Forwarded`/`X-Forwarded-For` still produced exactly 10 successes and 4 rate rejections from 14 rapid requests. Shared deployment must enforce true-client policy at a trusted ingress; there is no configured trusted-forwarding bypass in the API.

### Admission, cancellation and cleanup

`directory/http/handler.go:101` scopes the search context; admission at `:129` has a non-blocking rejection branch, with deferred release at `:130`. There is no semaphore waiter queue. The limiter mutex is released before parsing/cursor work/SQL; its finite sweep is a bounded critical section, not a claim of protection against unlimited HTTP connections.

The new [admission regression](../../apps/api/internal/directory/http/final_review_test.go) sets capacity to one and follows each 400/408/500/503 repository failure and invalid cursor with a successful request. It also cancels a request **after repository entry** and verifies a later request is admitted. This passes with the race detector. Existing timeout/saturation/other-route tests also pass.

`directory/postgres/store.go:97` begins a read-only transaction; `:106` sets only a transaction-local statement timeout. Success commits at `:130`; error cleanup at `:102` uses a separate one-second rollback context. Real PostgreSQL tests verify server-side statement cancellation, active-query client cancellation, exhausted pool waits, no remaining active search, restored timeout and subsequent connection usability. No global/session database setting is changed. Cleanup may add bounded time beyond the execution deadline.

`cmd/api/main.go:46` copies primary pool configuration, caps the separate search pool at the admission limit and sets its minimum connections to zero. Owner/readiness connections remain in the primary pool. In the disposable runtime fixture primary max was 4 and search max was 8: total configured application allowance 12, plus diagnostics. Production must budget **primary max + search max + operational connections** against server capacity. Separate pools do not isolate database CPU/I/O.

Re-ran the existing real-PostgreSQL 25k/100-peer burst: **8 HTTP 200, 92 HTTP 503**, longest search handler 24.096 ms; readiness returned 200 in 0.278 ms after search pool activity began. This is a single in-process burst observation, not a sustained-load SLO or network benchmark.

### Responses, projection and logging

Verified distinct safe 400, 408, 429, 500 and 503 handling, retry hints and no-store/nosniff on successful/error search responses. Invalid/expired/tampered cursor errors remain generic. Owner requests still require verified JWT identity and internal UUID ownership; public search does not start OIDC login.

Same-statement eligibility and publication predicates remain intact: PUBLIC/VISIBLE/opt-in/trusted verified alumni; profession/skills flags; academic filters from one consented verified membership; explicit minimal DTOs; no totals/facets/private identifiers. Signed-JWT, cross-owner, paired hidden-field, private/hidden exclusion, verification revocation/reverification and atomic consent withdrawal tests pass. These logical checks do not establish constant-time SQL or erase prior delivered public data.

New search events contain bounded outcome/status/duration/capacity/correlation fields, not filters, cursor, token, peer address or academic values. Existing HTTP logging uses URL path without RawQuery. Healthy real-process log checks excluded a unique search term, the generated bearer token, direct loopback peer and forged forwarding addresses. Existing JWKS-library transport diagnostics can contain the **configured infrastructure endpoint URL**, observed when the disposable mock was initially started too late. That pre-existing diagnostic is not a directory source-IP/search-value event; no blanket claim that every dependency log omits all network addresses is made. Initialize dependencies before runtime probes and apply deployment log-redaction policy where infrastructure URLs must also be suppressed.

## PostgreSQL verification and performance

Inspected the original [165-plan evidence](../performance/milestone-4-phase-c-explain.jsonl) and independently reran comparisons with [a new differential harness](../../apps/api/internal/directory/postgres/final_review_integration_test.go).

The frozen Phase B SQL was compared with the actual SQL constant from commit `830f463` and matches. New verification compared **complete ordered public JSON**, not just cardinalities:

- 1k, 10k and 25k synthetic profiles, in rolled-back transactions on a separate database.
- Fourteen cases: browse, max page, Indonesian/Unicode names, no-match, company, low-selectivity company/title, hidden company, academic combination, skills, combined filters, 75%-range cursor and the final 20 source slugs.
- Statistics targets 100 and 1,000, each followed by ANALYZE of users/profiles/memberships/skills/consents.
- **84 full-result pairs identical; 168 before/after EXPLAIN plans.**
- At 25k, five additional cases ran as actual bound prepared queries with both `force_generic_plan` and `force_custom_plan`: **10 more full-result pairs identical**. All 94 pairs passed. EXPLAIN alone is not evidence of cached statement latency, so cached executions were timed separately.

### Fresh EXPLAIN observations (ms, target 100)

| Query | 1k before / after | 10k before / after | 25k before / after |
| --- | --- | --- | --- |
| browse | 1.991 / 2.183 | 0.532 / 0.555 | 0.556 / 0.622 |
| name | 59.132 / 5.041 | 623.943 / 4.169 | 1647.175 / 3.644 |
| unicode_name | 60.798 / 4.744 | 622.177 / 3.105 | 1654.871 / 3.802 |
| name_no_match | 57.658 / 6.238 | 622.967 / 66.318 | 1643.125 / 180.813 |
| company_low_selectivity | 7.598 / 3.710 | 3.061 / 2.378 | 2.910 / 2.376 |
| title_low_selectivity | 6.025 / 2.257 | 2.391 / 0.895 | 2.362 / 0.916 |
| late | 1.379 / 1.357 | 0.513 / 0.517 | 0.457 / 0.532 |
| tail | 0.406 / 0.392 | 0.132 / 0.144 | 0.166 / 0.174 |

The final name case remained approximately 3.6–3.7 ms at target 1,000, with no-match around 182 ms. At 25k, actual cached generic/custom executions were respectively: name 4.52/4.46 ms; no-match 179.33/178.93 ms; low-selectivity company 2.97/3.09 ms; title 1.35/1.55 ms; late page 0.75/1.15 ms. These timings include driver/result handling and are not directly interchangeable with EXPLAIN execution time.

The eligible ordered subquery at `directory/postgres/store.go:36` retains the PostgreSQL 17 `OFFSET 0` boundary before normalization, using the partial C slug index and bounded lateral user/verified-membership probes. The expensive users normalization scan in Phase B is avoided. Publication CASE guards remain in profession matching, and shared academic predicates require one verified consented membership. Existing indexes are used; no speculative index, new migration, materialized search projection or SQL replacement was introduced.

The ASCII normalization branch is equivalent to the ASCII subset of the Go Unicode mapping. Non-ASCII values retain the complete mapping. Existing whitespace/control/NBSP/NEL, new Unicode letter, circled capital, Greek, Turkish, sharp-S, accent distinction and literal wildcard/backslash regressions pass. Canonical slugs are unique and ordered in explicit C collation; static pages remain deterministic.

These are warm local PostgreSQL 17.11 Alpine/musl ARM64 observations on synthetic data, not distribution-wide guarantees. The fixture uses the current single academic program/year and limited membership fan-out. No-match/hidden-only scans still grow with eligible population; changing PostgreSQL versions, distributions, collation/Unicode tables, server load or statistics can alter plans. Re-run fixtures on upgrades and before production sizing. Results after statistics changes and forced cache modes were stable here; no optimizer GUC is changed in application code.

Live pagination still has no cross-request snapshot: inserts before an anchor may be missed; renames can repeat/skip a person; private/hidden/deleted anchors remain lexical boundaries. New statements recheck privacy; already-running snapshots and delivered data cannot be recalled. Issue #11 remains the independent stale-write production gate.

## Runtime and regression execution

Used macOS Docker with new `ab-m4c-review-net`, `ab-m4c-review-pg`, `ab-m4c-review-minio` and `ab-m4c-review-go` only. PostgreSQL/object data are disposable tmpfs; no development database, Keycloak account or persistent volume was reset or modified. Go 1.27.3 and gcc/musl in Linux/arm64; native frontend Node 22.23.3. CI independently ran its configured Linux environment and Node 24.

Started the built API against disposable PostgreSQL/MinIO and a temporary RSA JWKS server. The private key remained in memory; a test bearer token stayed in a temporary mode-0600 file and was not printed. Actual HTTP checks:

- `/health` and `/ready`: 200.
- Owner discovery without bearer: 401.
- Generated RSA-signed bearer with correct issuer/audience/azp: owner discovery 200.
- Fourteen direct-peer public searches with forged proxy headers: ten 200 then four 429; no-store/nosniff verified.
- Owner discovery GET/PATCH after public rate exhaustion: 200; readiness still 200.
- Every new numeric environment key set to invalid zero: binary rejected startup with exit 1 and a safe key-specific error. Unit tests additionally cover invalid ranges and timeout/TTL dependencies.
- Distinct primary and `alumnibridge-directory` connection application names were observed; no primary-pool label contamination.

| Check executed in this review | Result |
| --- | --- |
| `go test -count=1 ./...` with isolated PostgreSQL/MinIO | PASS, including new always-on admission test |
| `go test -race -count=1 ./...` with same integration environment | PASS |
| `go vet ./...` | PASS |
| `go build ./cmd/api` | PASS; runtime also executed from a built binary |
| Frontend `npm run test` | PASS, 29 tests / 6 files |
| Frontend `npm run lint` | PASS |
| Frontend `npm run build` | PASS |
| PostgreSQL/MinIO + signed-JWT/consent/privacy/Unicode/live cursor suites | PASS |
| Server statement timeout, active cancellation, pool cleanup/exhaustion | PASS |
| Rate refill/bounds/expiry/concurrency/IP/proxy and admission failures | PASS |
| Isolated migrations 000001–000006 apply/down/reapply | PASS |
| Original tombstone/photo rollback refusal guards | PASS on another disposable database; fixture transactions rolled back |
| Optional full SQL differential / stats / cached-plan assessment | PASS, 94 equal public-result pairs and 168 plans |
| Optional 25k / 100-peer real database burst | PASS |
| Actual API startup, readiness and numeric startup rejections | PASS |
| OpenAPI semantic validation + corrected owner/search response checks | PASS, Swagger Parser 13.1.0 / OpenAPI 3.1 / 19 paths |
| `docker compose config --quiet` | PASS for current configuration; development stack not recreated |
| `git diff --check` | PASS |
| Exact-SHA CI | PASS at 51be6d0, all four jobs |
| Browser E2E / Windows + Podman / assistive technology | NOT RUN; no claim of verification |

Raw local logs are retained under `/tmp/alumnibridge-m4c-final-review-evidence`. Temporary test services and their network are removed after verification; development services remain running. The validators/helpers add no application dependency.

### Reproduce the additional differential test

From `apps/api`, set `TEST_DATABASE_URL` to a **fresh empty migrated disposable PostgreSQL 17 database**, separate from concurrent integration fixtures:

```sh
DIRECTORY_FINAL_REVIEW=1 go test -count=1 \
  -run '^TestFinalReviewSearchEquivalenceAndPlans$' -v ./internal/directory/postgres
```

Fixture writes roll back. The heavy opt-in harness is skipped during ordinary suites; the new permit-release test is always on. A future CI opt-in execution must use its own empty database, not the database being mutated concurrently by other integration packages. Existing tests still use the CI PostgreSQL/MinIO environment normally. No GitHub run for uncommitted review additions is claimed.

## Files changed by this review

1. `apps/api/openapi/openapi.yaml`: remove owner search-only response declarations.
2. `apps/api/internal/directory/http/final_review_test.go`: release-after-failure and active-cancellation verification.
3. `apps/api/internal/directory/postgres/final_review_integration_test.go`: full DTO differential, statistics changes and actual cached-plan execution verification.
4. `docs/reviews/milestone-4-phase-c-review.md`: this report.

All are intentionally uncommitted for maintainer review. No existing runtime Go source or migration is changed.

## Issue recommendations and Phase D conditions

| Issue | Acceptance recommendation |
| --- | --- |
| #16 Pagination/performance | Backend acceptance criteria are satisfied for the development baseline: deterministic static pages, bounded invalid inputs, per-page privacy, integration boundaries and measured limitations. Recommend accepting the Phase C implementation. Keep issue OPEN for maintainer closure/whole-milestone reconciliation |
| #18 Abuse/privacy | SQL/HTTP privacy matrix, sensitive-data exclusion, signed JWT and development abuse controls are verified. Final Phase C SHA CI passes. Keep OPEN: real Directory UI/navigation/browser/cache regressions and CI on the eventual milestone PR still belong to D/E; distributed ingress enforcement remains a production gate |

Conditions:

1. Maintainer reviews the four local review changes; commit/push only with authorization and rerun CI for the resulting SHA before merge. This report does not equate current working-tree content with the successful committed CI revision.
2. Phase D must preserve the approved anonymous DTO, consent defaults, server-side privacy and live-cursor limitations; handle 429 Retry-After and distinct 503 failures without stale public result display. This is a requirement for later implementation, not work started here.
3. Carry existing deployment gates: shared ingress/proxy rate policy, finite connection budgeting, representative larger/cold/sustained load evidence, private owner catalog assessment before multi-institution expansion, #11 stale-write protection, trusted verification/moderation, production OIDC/TLS/secret/storage/backup/consent policy. The archived community MinIO deployment remains a production risk.
4. Browser, Windows/Podman and assistive-technology checks remain explicitly unverified and belong to their planned phase. This backend review is not production or WCAG approval.

**No unresolved Critical/High/Medium Phase C defect blocks development of Phase D once the maintainer approves this review.** Stop here: no Phase D implementation, issue closure, PR, merge or push has been performed.
