# Milestone 4 Phase C — Performance, pagination and abuse hardening

Date: 2026-10-24 (Asia/Jakarta). Verdict: **READY FOR PHASE C REVIEW**.

## Scope, provenance and repository safety

- Branch: `feature/milestone-4-alumni-directory`.
- Approved baseline: `830f46313e4c6747c41e33ab846e7b36a2b401a7`.
- Implementation commits: `852bc0f` (SQL, timeouts, pagination and EXPLAIN) and `507f747` (admission, rate limiting, configuration, observability and regression tests). This report describes implementation HEAD `507f747`; its own documentation commit follows it.
- Preflight confirmed a clean tree, fetched origin, verified the approved SHA locally/remotely, inspected the master comparison, AGENTS, architecture, Phase B review, directory design/plan, ADRs 0001/0002, migration 000006 and Issues #15/#16/#18/#11.
- Exact baseline CI [40067992416](https://github.com/sipratama/alumnibridge/actions/runs/40067992416) completed successfully at **830f463**, with Backend / test,race,vet,build; Frontend / test,lint,build; Integration / PostgreSQL,MinIO,migrations; Repository / whitespace all successful. A new dispatch was unnecessary. The earlier `3d28c8b` run is not used as evidence for the approved SHA.
- Phase C commits are local and have **not** run on GitHub Actions. No push, PR, merge, issue comment/closure, branch-protection change, authentication redesign or Phase D/E work was performed.
- macOS host; Go 1.27.3 on Linux/arm64 in a dedicated tool container. PostgreSQL 17.11 Alpine/musl, Docker reports 10 CPUs and approximately 8 GB RAM. Native web checks used Node 22.23.3; CI uses Node 24.
- Created only the `ab-m4c-test` network and `ab-m4c-pg`, `ab-m4c-minio`, `ab-m4c-go` containers. Test database/object data use disposable tmpfs. Existing AlumniBridge services, development databases, verification records and persistent volumes were untouched.

## Measured findings and changes

| Finding | Priority | Resolution |
| --- | --- | --- |
| Name predicate normalized a users scan before profile eligibility, with high Unicode translation cost | Medium | Ordered eligible boundary, indexed lateral probes and a contract-equivalent ASCII branch |
| Anonymous searches lacked dedicated execution, concurrent admission and source rate budgets | Medium | Scoped request/statement timeouts, separate bounded pool, immediate overload rejection and bounded token buckets |
| Full eligibility materialization improved names but regressed common company searches/browse | Medium, intermediate implementation | Rejected that shape; measured streaming and lateral alternatives before selecting the final SQL |
| Floating-point refill could round a remaining one-second wait to two seconds at 1 request/minute | Low, caught during implementation | Exact integer credits and a slow-refill boundary regression |
| Owner catalog is unbounded | Low under current deployment assumptions | Preserve compatible private contract; measured 301 choices and record a concrete multi-institution follow-up below |

No new Critical/High ownership or logical public-projection defect was identified. This is not a constant-time or complete anti-enumeration guarantee. Issue #11 remains a separate production gate.

## Synthetic fixture and EXPLAIN method

[Performance harness](../apps/api/internal/directory/postgres/performance_integration_test.go) seeds 1,000, 10,000 and 25,000 accounts in separate rolled-back transactions on a dedicated database, using deterministic synthetic IDs and `perf-00000001` canonical slugs. No real accounts or credentials are used. The frozen [Phase B SQL](../apps/api/internal/directory/postgres/testdata/phase_b_search.sql) is a test baseline only.

Fixture distribution: one-third PUBLIC, opt-out every fifth account, moderator-hidden every eleventh, student every thirteenth, pending alumni every seventh remaining account, otherwise verified alumni. Names cycle Siti with NBSP, Budi Santoso, a Unicode circled-capital name, and Agus Wijaya. Professional flags are enabled on alternate accounts; company/title and skills vary; academic selections include independently unpublished program/year flags and adversarial consent rows on ineligible records. All production eligibility and publication predicates remain required. The 25k set has 6,061 PUBLIC/VISIBLE/opted-in candidates and 4,796 verified eligible profiles.

Each case runs `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` for five shapes: `before`, `eligible_only`, `ascii_fence`, `ordered`, `after`. The intermediate shapes isolate full eligibility materialization, ASCII normalization, ordered non-materialized eligibility and indexed lateral probes. [165 machine-readable plan summaries](performance/milestone-4-phase-c-explain.jsonl) retain planning/execution time, rows, scan/index names, rows removed, loops, shared hits/reads and sort methods. Raw JSON plans can be generated with the output option below. Scan rows are per loop: multiply by loops when evaluating work; do not interpret a sum of nested scan rows as distinct people.

EXPLAIN timings cover the search SELECT, excluding transaction/timeout-setup round trips; the burst assessment includes repository setup and HTTP handler work. The academic performance fixture uses the current single program and year 2020; it does not model broad multi-institution selectivity.

These are individual warm-cache observations after fixture insertion and ANALYZE, not percentiles, cold-disk results, sustained-load capacity or production SLOs. Result cardinalities agree across all five shapes. SQL/JWT privacy tests separately verify result contents.

### Execution time (milliseconds)

| Query | 1k before / after | 10k before / after | 25k before / after |
| --- | --- | --- | --- |
| browse | 2.236 / 2.113 | 0.822 / 0.602 | 0.957 / 0.758 |
| name | 58.984 / 5.129 | 624.956 / 3.655 | 1675.189 / 3.755 |
| company | 8.002 / 4.542 | 3.351 / 2.771 | 3.404 / 3.061 |
| job | 6.659 / 2.827 | 3.016 / 1.268 | 3.002 / 1.472 |
| academic | 2.120 / 2.847 | 3.680 / 4.216 | 8.997 / 9.033 |
| skills | 4.053 / 3.190 | 1.066 / 1.301 | 1.097 / 1.492 |
| combined | 66.821 / 12.721 | 671.694 / 10.004 | 1783.992 / 10.263 |
| next | 1.815 / 1.683 | 0.699 / 0.550 | 0.600 / 0.689 |
| no_match | 57.288 / 6.382 | 635.012 / 65.112 | 1651.710 / 176.285 |
| hidden_company | 6.401 / 4.705 | 57.443 / 45.040 | 145.715 / 117.543 |
| absent_company | 6.167 / 4.656 | 57.938 / 44.860 | 146.359 / 119.127 |

The name and combined cases find an early page, so the final query can stop early. No-match searches still examine the eligible population and remain approximately linear. At 25k, name changes from 1,675.189 to 3.755 ms; no-match changes from 1,651.710 to 176.285 ms. Do not generalize the early-page speedup to every prefix or distribution.

### Plan evidence and rejected alternatives

1. Phase B name search used a users sequential scan with normalization before joining eligible profiles. At 25k it examined 25k names, including non-directory accounts.
2. Full eligibility materialization reduced normalization to 4,796 eligible names. At 25k, name took 345.491 ms before the ASCII branch, then 179.502 ms. However, common company search regressed from 3.404 to 116.790 ms and browse from 0.957 to 22.065 ms because every eligible row was collected first.
3. Ordered streaming reduced name to 22.775 ms and company to 20.861 ms, but the planner still chose broad joins and sorts.
4. Final indexed lateral probes keep the outer profile slug index order: one user lookup and at most one verified membership probe per candidate. The eligible subquery's `ORDER BY slug COLLATE "C" OFFSET 0` blocks predicate pushdown across the eligibility boundary; the bounded outer page can stop after `limit + 1`. Projection aggregates run only for that page. One parameterized search statement is used, with no per-result HTTP/database request loop. In the recorded 25k early name page, profile candidates scanned fell to 108 and only 85 eligible names were filtered (21 returned, 64 removed), versus 25k users in the old normalization scan. Shared buffer hits were 8,123 before / 1,112 after; reads were zero in both warm runs; planning was 0.654 / 0.475 ms. The old plan used a top-N sort; the final indexed subquery avoids that full-candidate sort, while bounded projection aggregates still use small sorts.
5. The existing `profiles_directory_slug_idx`, user PK and verified membership indexes support the final plan. Re-run the plan assessment for PostgreSQL/planner upgrades; the execution boundary is verified on PostgreSQL 17, not a constant-time portability guarantee. No new index, migration, stored normalized column, persistent materialized view, synchronization trigger or denormalized search index was justified. The transient query boundary is computed from authoritative rows on every statement. An expression index using database `lower()` would violate the approved Go Unicode contract; an index parameterized by runtime translation tables is not a compatible immutable search definition.

The ASCII branch maps A–Z and all five ASCII whitespace controls to the exact Go equivalents; other values retain the full Go-derived Unicode translation. Literal escaping, whitespace collapse, accent distinctions, simple lowercase, C collation and prefix semantics are unchanged. Unicode/locale, ASCII control-whitespace, NBSP/NEL, wildcard, backslash and case regressions pass. No locale-dependent replacement normalization is introduced.

### Burst assessment

An opt-in real-PostgreSQL test uses committed 25k fixtures on its own fresh database and sends 100 simultaneous searches from distinct direct peer IPs. The final ordered SQL run admitted 8 successful searches and promptly rejected 92 with 503; the maximum handler duration was 38.860 ms. A readiness request executed after search-pool acquisition returned 200 in 0.320 ms using the primary pool. This is an in-process HTTP handler assessment with real PostgreSQL, not a network benchmark or browser E2E. The final burst also shared the disposable PostgreSQL instance with regression tests using another database; these timings are not isolated throughput/SLO measurements.

The fixture retains permanent tombstones after owner cleanup, intentionally exercising the existing reservation policy. A repeated load run on the same database was rejected by those reservations; the harness now explicitly requires a fresh fixture namespace. The successful final assessment used a new disposable database. No tombstone safeguard was weakened.

## Runtime controls and configuration

All new variables have validated defaults, so native startup remains compatible. Compose, `.env.example` and CI fixtures specify them explicitly. `DIRECTORY_CURSOR_KEY` remains mandatory for native startup and unchanged.

| Variable | Default | Accepted range |
| --- | --- | --- |
| `DIRECTORY_REQUEST_TIMEOUT_MS` | 2000 | 100–9000 |
| `DIRECTORY_STATEMENT_TIMEOUT_MS` | 1500 | 1–8999 and shorter than request budget |
| `DIRECTORY_MAX_CONCURRENT` | 8 | 1–64 |
| `DIRECTORY_RATE_PER_MINUTE` | 60 | 1–60000 |
| `DIRECTORY_BURST` | 10 | 1–1000 |
| `DIRECTORY_MAX_CLIENTS` | 10000 | 1–100000 |
| `DIRECTORY_CLIENT_TTL_SECONDS` | 600 | 60–86400; must cover a complete burst refill |

The 2s/1.5s defaults leave margin above observed single-query costs on these fixtures. Eight operations fit this isolated burst assessment. The rate defaults preserve a short interactive burst and average one request/second; no future UI usage or production multi-tenant capacity has been established. Tune using deployment traffic and database capacity before launch.

### Request/statement cancellation and connection isolation

- The HTTP adapter creates a search-specific deadline covering rate checking, validation, admission, cursor processing, pool acquisition and repository execution. Existing global API timeout remains unchanged.
- SQL executes inside a short read-only transaction. `set_config('statement_timeout', milliseconds, true)` scopes PostgreSQL cancellation to that transaction. No global/session default is changed. On success COMMIT restores the original setting; errors/cancellation roll back with a separate cleanup context capped at one second. Response completion can include this bounded cleanup beyond the execution deadline.
- Main wires a dedicated search pool capped at `DIRECTORY_MAX_CONCURRENT`; owner APIs and readiness use the primary pool. Budget **primary max + search max + operational connections** against PostgreSQL's limit. Pool separation prevents directory connection starvation of other endpoints; it cannot isolate shared PostgreSQL CPU/I/O completely.
- Real integration tests hold an ACCESS EXCLUSIVE table lock, trigger the server statement timeout, verify safe domain mapping, then cancel a separate query only after `pg_stat_activity` confirms it is actually waiting inside PostgreSQL. They verify no abandoned active search, usable pool afterward and restored `statement_timeout=0`. A separate exhausted-one-connection-pool test verifies bounded waiting and HTTP timeout behavior.

### Admission and rate limiting

Admission uses a finite channel, acquired before cursor/search/database work, with a non-blocking default rejection. No waiter queue is constructed. Defer releases permits on success, validation/cursor/database errors and cancellation. Other routes do not use these permits.

The token bucket is concurrency-safe, uses exact integer time credits, refills continuously, grants at most the configured burst and computes a rounded-up whole-second `Retry-After`. Authentication grants no exemption. Rate checks precede query validation, so malformed requests also consume the source budget.

Client keys are parsed direct TCP peer `netip.Addr` values: ports removed, IPv4-mapped IPv6 unmapped, IPv6 spellings canonicalized and zones removed. Invalid peer syntax receives a safe 400. No request header or arbitrary string becomes a map key. Both `Forwarded` and `X-Forwarded-For` are ignored **from every peer**: trusted forwarding is not implemented. When Nginx/Vite/proxies are used, browsers share that proxy's API-facing bucket. Configure real client enforcement at a trusted ingress before shared/production deployment; changing a header does not enable trust.

The map retains at most `DIRECTORY_MAX_CLIENTS`. A bounded sweep runs at most once/minute and removes entries inactive for the TTL; expiry is lazy, so inactive state may remain until another request triggers a sweep. Active entries are never evicted for a fresh burst. A full table fails closed with global-overload semantics until space expires. No lock is held across parsing, cursor operations or SQL. IP rotation can defeat a per-address quota but cannot grow the map without bound or bypass global SQL admission. IPv6 privacy-address rotation and NAT sharing require deployment-specific ingress policy. These controls do not prevent all scraping.

### HTTP outcomes

Every result/error preserves `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

| HTTP | Error code | Meaning |
| --- | --- | --- |
| 200 | none | Minimal bounded public page |
| 400 | `invalid_input` | Invalid query/cursor; malformed/expired/tampered cursors remain indistinguishable |
| 400 | `invalid_client` | Unparseable direct peer address |
| 408 | `directory_canceled` | Caller canceled; best-effort response, disconnected clients may receive no body |
| 429 | `directory_rate_limited` | Source bucket exhausted; meaningful token-refill `Retry-After` |
| 503 | `directory_overloaded` | Admission or bounded client table full; `Retry-After: 1` is a retry hint, not a capacity promise |
| 503 | `directory_database_timeout` | PostgreSQL canceled the scoped statement |
| 503 | `directory_timeout` | Search/parent deadline, including pool wait |
| 500 | `internal_error` | Unexpected failure; no raw SQL/error details |

Under pressure, rate rejection can precede validation and admission rejection can precede cursor decryption. The old generic `directory_unavailable` label is replaced by the documented distinct failure codes; successful page/cursor payloads and owner APIs remain compatible.

## Pagination and privacy evidence

AES-256-GCM, random nonces, 15-minute expiration, versioned opaque URL-safe tokens, 2,048-byte cap, normalized filter/page-size binding and public slug anchor are unchanged. No UUIDs or new public identity field were added.

Always-on unit and real PostgreSQL cases cover empty/exact/full-50/multiple pages, static order, invalid/tampered/expired tokens and changed filters/limit. New real service/cursor traversals cover deleted/private/hidden anchors, renamed anchors, inserts before/after the boundary, discovery withdrawal and verification revocation. Each later statement reapplies eligibility and publication; historical reservations do not enter directory results.

Paired hidden fixtures and signed-RSA JWT tests re-run: hidden company/title/skills and academic fields cannot match; academic conditions must share one consented verified membership; PRIVATE/HIDDEN/student/pending/rejected profiles are excluded; foreign memberships cannot be changed; owner authentication rejects invalid, expired and wrong-audience tokens. Public DTO checks exclude UUIDs, email, OIDC subjects, membership IDs, evidence and internal moderation/consent metadata. No totals, facets, autocomplete or bulk export were added. Existing photo/public profile and atomic consent withdrawal/reverification tests pass.

Timing assessment: the 25k hidden-only company prefix returned zero in 117.543 ms; an absent company prefix returned zero in 119.127 ms. Both keep hidden profession normalization behind publication CASE and eligibility. Plans and timings vary with public distributions and selectivity; these single observations do not establish statistical equivalence or constant-time execution. Privacy guarantees here concern matching/projection and bounded work, not elimination of all timing inference.

Live pagination is not a cross-request database snapshot. An insert before the anchor may be missed until restart; a rename can repeat/skip the same person under another slug. A hidden/deleted anchor remains a lexical boundary and grants no access. An already running statement may finish from an earlier snapshot. Already delivered public data cannot be recalled. ADR 0001 / Issue #11 still permits last-writer-wins same-field updates; no optimistic concurrency was implemented.

## Authenticated membership catalog decision

**Keep the existing private response contract for this development baseline; do not silently truncate.** A 301-choice real PostgreSQL fixture returned 75,672 JSON bytes, took approximately 7.35 ms and allocated approximately 285,784 bytes for the query/driver/DTO decode in that test process. This allocation delta is not retained heap or a production per-request memory ceiling. `identity.Me` independently loaded all 301 memberships in approximately 4.74 ms, confirming that adding pagination only to discovery would leave the earlier load unbounded. JWT/HTTP overhead is excluded from these timings.

Current reference inventory has one institution/program, so normal owners have at most one verified alumni choice (plus a possible student membership). Owners cannot create reference programs or grant verification. Expected deployment cardinality is 1–5 trusted alumni affiliations; 301 is an intentionally adversarial fixture. Owner publication remains limited to 20 explicit selections, independently of the selector catalog.

Follow-up gate **before multi-institution onboarding/reference expansion**: re-evaluate at >100 verified memberships per owner, >256 KiB response or >50 ms measured catalog execution. These are review triggers, not enforced runtime limits or promised latency ceilings. Design a compatible bounded private membership selector plus an actor-resolution path that does not eagerly load every membership in `identity.Me`; preserve selected consents across pages and update OpenAPI/UI together. Do not make the selector anonymous, add public membership enumeration or silently hide choices. No new administrator/verification workflow is introduced here.

## Observability and operations

Existing slog emits `directory search` records with bounded outcome class, HTTP status, duration, in-flight count, configured capacity and existing request correlation ID. Rejections expose saturation without recording keys or query contents. The existing HTTP request logger records path without RawQuery. Regression tests check correlation and absence of raw terms/cursors; SQL errors, source IPs, academic values, tokens and encryption material are not added to these events. There is no new telemetry service or public metrics endpoint.

Operators should monitor timeout/overload/rate outcomes and duration, budget both pools, and review ingress URL redaction. Limits are per process, reset on restart and multiply across replicas. Production needs evaluated shared ingress/WAF budgets, trusted proxy topology, log volume/retention and sustained/cold/worst-case workload testing. No distributed rate enforcement or complete anti-scraping protection is claimed.

## Reproducing tests safely

Use disposable PostgreSQL 17 and the existing private MinIO fixture protocol described in [Phase B testing](milestone-4-phase-b-backend.md). Migrate a **fresh separate database** through 000006; never point integration or performance helpers at active development/production data. Fixture failure injection requires isolated resources. No new migration is needed.

From `apps/api`, with the disposable `TEST_DATABASE_URL` and `TEST_OBJECT_STORAGE_*` environment set:

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./cmd/api
```

Use another empty migrated database for EXPLAIN; run alone, without simultaneous integration fixture writers:

```sh
mkdir -p /tmp/alumnibridge-directory-plans
DIRECTORY_EXPLAIN=1 DIRECTORY_EXPLAIN_OUTPUT=/tmp/alumnibridge-directory-plans   go test -count=1 -run '^TestDirectoryExplain$' -v ./internal/directory/postgres
```

The EXPLAIN transaction rolls back its rows. The optional output directory must already exist. For the concurrent burst test, supply a **different fresh migrated database URL** (fixtures must be committed for multiple connections):

```sh
DIRECTORY_LOAD=1 go test -count=1 -run '^TestDirectoryLoad$' -v ./internal/directory/postgres
```

The load test deletes only its synthetic owners/skill afterward; permanent synthetic slug tombstones remain until the disposable database is removed. Always use a fresh database for another load run. Do not bypass the tombstone trigger.

## Files created or modified

- [.env.example](../.env.example)
- [.github/workflows/ci.yml](../.github/workflows/ci.yml)
- [README.md](../README.md)
- [apps/api/cmd/api/main.go](../apps/api/cmd/api/main.go)
- [apps/api/internal/config/config.go](../apps/api/internal/config/config.go)
- [apps/api/internal/config/config_test.go](../apps/api/internal/config/config_test.go)
- [apps/api/internal/directory/directory.go](../apps/api/internal/directory/directory.go)
- [apps/api/internal/directory/http/controls.go](../apps/api/internal/directory/http/controls.go)
- [apps/api/internal/directory/http/controls_test.go](../apps/api/internal/directory/http/controls_test.go)
- [apps/api/internal/directory/http/handler.go](../apps/api/internal/directory/http/handler.go)
- [apps/api/internal/directory/http/handler_test.go](../apps/api/internal/directory/http/handler_test.go)
- [apps/api/internal/directory/postgres/hardening_integration_test.go](../apps/api/internal/directory/postgres/hardening_integration_test.go)
- [apps/api/internal/directory/postgres/load_integration_test.go](../apps/api/internal/directory/postgres/load_integration_test.go)
- [apps/api/internal/directory/postgres/performance_integration_test.go](../apps/api/internal/directory/postgres/performance_integration_test.go)
- [apps/api/internal/directory/postgres/review_integration_test.go](../apps/api/internal/directory/postgres/review_integration_test.go)
- [apps/api/internal/directory/postgres/store.go](../apps/api/internal/directory/postgres/store.go)
- [apps/api/internal/directory/postgres/store_integration_test.go](../apps/api/internal/directory/postgres/store_integration_test.go)
- [apps/api/internal/directory/postgres/testdata/phase_b_search.sql](../apps/api/internal/directory/postgres/testdata/phase_b_search.sql)
- [apps/api/openapi/openapi.yaml](../apps/api/openapi/openapi.yaml)
- [compose.yaml](../compose.yaml)
- [docs/architecture.md](../docs/architecture.md)
- [docs/implementation-plan.md](../docs/implementation-plan.md)
- [docs/milestone-4-directory-design.md](../docs/milestone-4-directory-design.md)
- [docs/milestone-4-directory-plan.md](../docs/milestone-4-directory-plan.md)
- [docs/milestone-4-phase-c-hardening.md](../docs/milestone-4-phase-c-hardening.md)
- [docs/performance/milestone-4-phase-c-explain.jsonl](../docs/performance/milestone-4-phase-c-explain.jsonl)
- [docs/prd.md](../docs/prd.md)

## Executed verification

| Check | Actual result |
| --- | --- |
| `go test -count=1 ./...` with disposable PostgreSQL/MinIO env | PASS |
| `go test -race -count=1 ./...` with same isolated integration env | PASS |
| `go vet ./...` | PASS |
| `go build ./cmd/api` | PASS, Linux/arm64 |
| Web `npm run test` | PASS, 29 tests / 6 files |
| Web `npm run lint` | PASS, no warnings |
| Web `npm run build` | PASS |
| PostgreSQL/MinIO, signed JWT, privacy/concurrency regressions | PASS, included in isolated Go suites |
| Migrations 000001–000006 apply/down/reapply | PASS on an empty disposable database |
| Original 000004 tombstone and 000005 photo rollback refusal guards | PASS, guarded fixtures in rolled-back transactions on a separate disposable database |
| Actual statement timeout, active-query cancellation, pool cleanup/exhaustion | PASS |
| Rate refill/expiry/table bound/concurrent burst/proxy spoof/IPv4/IPv6 | PASS |
| Admission saturation/cancellation/other-route availability | PASS |
| EXPLAIN | PASS, 165 plans / 3 dataset sizes / 5 query shapes |
| 25k / 100-peer burst | PASS, bounded overload and separate-pool readiness |
| `docker compose config --quiet` | PASS; no stack recreated |
| OpenAPI semantic validation | PASS, Swagger Parser 13.1.0, OpenAPI 3.1 / 19 paths; validator installed outside repository |
| `git diff --check` | PASS |
| Exact approved Phase B CI | PASS at `830f463`, four jobs |
| Phase C GitHub Actions | NOT RUN; local commits not pushed |
| Browser E2E, Windows/Podman, assistive technology | NOT RUN; this phase has no new UI |

Initial test failures during implementation were corrected (updated HTTP codes/test quotas, TTL boundary expectation, exact rate refill rounding and fresh load-fixture tombstone isolation). The results above refer to the corrected implementation, not those intermediate runs.

CI retains its existing isolated PostgreSQL 17/private MinIO setup and migration lifecycle. New functional tests are discovered by `go test ./...`; backend commands now disable cached test results and integration fixtures include the runtime defaults. Heavy EXPLAIN/load tests are explicit opt-ins and skipped in routine CI, while their functional timeout/privacy/admission/rate cases always run in the appropriate unit/integration jobs. No local container name or host-specific path is embedded in normal CI tests. Final PR CI must run after an approved push; baseline success is not evidence for these new commits.

## Issue progress and remaining gates

| Issue | Phase C progress / remaining work |
| --- | --- |
| #15 Search API | Existing contract preserved; eligible-first SQL and failure contracts documented. Logical privacy regressions pass; no constant-time guarantee |
| #16 Pagination/performance | Phase C implementation complete locally: boundary/live cursor cases, bounded operations and reproducible EXPLAIN. Await review and final PR CI |
| #18 Abuse/privacy | Phase C backend controls and signed-JWT/privacy tests complete locally. Distributed/proxy enforcement remains production work; browser navigation belongs to D/E; final PR CI still required |
| #11 Optimistic concurrency | Unchanged, OPEN production gate; no stale-write prevention claimed |

No GitHub tracking was modified or issue closed. Next action is maintainer Phase C review. Phase D/E and all UI, verification administration, messaging/connections and other milestones remain unstarted.

Production readiness is **not established**. Remaining gates include #11, real verification/moderation operations, shared ingress abuse controls and topology, sustained larger/cold workload evidence, production OIDC/TLS/secret policy, maintained S3-compatible storage (archived community MinIO remains a risk), backup/restore and consent/legal policy. Windows/Podman/browser/accessibility verification remains for its planned phase. These are distinct from completion of this backend hardening slice.
