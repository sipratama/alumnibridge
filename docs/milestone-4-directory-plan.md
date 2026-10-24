# Milestone 4 — Alumni Directory & Discovery

Date: 2026-10-24 (Asia/Jakarta).
Status: **Phase E integrated acceptance completed locally with conditions**.

Current implementation/test evidence: [Phase B backend](milestone-4-phase-b-backend.md) and [Phase C hardening](milestone-4-phase-c-hardening.md). The repository/GitHub observations below record the original Phase A alignment.
Phase C and Phase D reviews are recorded in their [Phase C](reviews/milestone-4-phase-c-review.md) and [Phase D](reviews/milestone-4-phase-d-review.md) reports. The [Phase D frontend](milestone-4-phase-d-frontend.md) and [Phase E integrated acceptance](milestone-4-phase-e-acceptance.md) are complete locally. Final-SHA CI, maintainer merge approval and production approval remain separate.

## Objective and documents

Enable privacy-safe anonymous discovery of opted-in verified alumni, initially UHAMKA Mathematics Education and later other institutions/programs. Direct PUBLIC student/pending profiles remain supported without directory inclusion. PUBLIC is not automatic directory consent.

- [Detailed directory design](milestone-4-directory-design.md): privacy matrix, API, database/query/pagination, backend/frontend, security and tests.
- [ADR 0002: discoverability](decisions/0002-directory-discoverability.md): separate default-false opt-in and academic consent.
- [PRD](prd.md), [architecture](architecture.md), [canonical roadmap](implementation-plan.md).
- [ADR 0001: development concurrency](decisions/0001-last-writer-wins-development.md) and [M3 final review](reviews/milestone-3-final-review.md): retained historical decisions and launch gates.

## Repository alignment evidence

Inspection on 2026-10-24:

| Check | Observed result |
| --- | --- |
| Repository/auth | `sipratama/alumnibridge`; origin HTTPS; active GitHub CLI account `sipratama` |
| Initial branch/tree | `master`, clean; no uncommitted changes to overwrite |
| Fetched master/base | `1537c8899fa8fff51ecc7fac01db1d1f8814526c` |
| Existing remote feature | `feature/milestone-4-alumni-directory` at `d41c471`; 0 ahead / 7 behind fetched master |
| Safe synchronization | Created local tracking checkout of the existing remote branch, then `git merge --ff-only origin/master`; no conflict or reset |
| Base comparison | Feature at master base before planning edits; `git diff origin/master...HEAD` empty at that point |
| [PR #12](https://github.com/sipratama/alumnibridge/pull/12) | MERGED 2026-10-24 04:18:37 UTC; merge `167d81f`; all four PR checks passed |
| [PR #13](https://github.com/sipratama/alumnibridge/pull/13) | MERGED 2026-10-24 04:18:53 UTC; merge `1537c88` |
| Latest master CI | [Run 40061849822](https://github.com/sipratama/alumnibridge/actions/runs/40061849822), completed **success** at master `1537c88`; four jobs successful |
| Actual enforcement | Branch API reports `protected=false`; protection endpoint returns “Branch not protected”; rulesets API returns `[]`. No required checks are currently enforced |

Successful CI job names: `Backend / test, race, vet, build`; `Frontend / test, lint, build`; `Integration / PostgreSQL, MinIO, migrations`; `Repository / whitespace`. These results describe the fetched master base, not unpushed Phase A commits. The previous master run for `167d81f` was canceled; it is not counted as passed. No new remote branch was created, and this phase does not push, create a PR or merge one.

## GitHub milestone and tracking reconciliation

The paginated `gh api` milestone inspection initially found only Milestone 3. Created exactly one [Milestone 4 — Alumni Directory & Discovery, milestone number 2](https://github.com/sipratama/alumnibridge/milestone/2). Milestone number 2 is GitHub's numeric identifier, not the product milestone ordinal.

Issues #14–#19 are associated with that milestone and remain OPEN. Existing issue titles/numbers were reused; no duplicate issue or milestone, issue closure or comment was created.

| Issue | Assessment / action / recommendation |
| --- | --- |
| [#2 Profile/slugs](https://github.com/sipratama/alumnibridge/issues/2) | Keep OPEN. M3 evidence covers ownership, normalization/reserved slugs, collisions, history/tombstones and completeness. Resolve the additional “inappropriate” slug vocabulary/moderation criterion, or explicitly split it into launch follow-up before closure. No denylist is invented in M4 Phase A |
| [#5 Skills](https://github.com/sipratama/alumnibridge/issues/5) | Keep OPEN for maintainer reconciliation. Implemented/documented contract is `GET /api/v1/skills?query=&limit=` with bounded prefix search, not the issue sketch's `q`/`cursor`. M3 normalization/reuse/ownership tests and new master CI are evidence. Recommend approving the existing no-cursor vocabulary contract and correcting the issue scope before closure; directory pagination is a separate contract |
| [#8 Frontend/integration](https://github.com/sipratama/alumnibridge/issues/8) | Keep OPEN. M3 final report contains real Chrome/macOS flows, 29 web tests and backend/privacy evidence; CI Foundation now supplies the previously absent CI. Windows/Podman, real screen-reader sign-off, incomplete manual contrast checks and final user acceptance remain distinct tasks. Reconcile these explicitly; no new browser pass is claimed |
| [#10 CI Foundation](https://github.com/sipratama/alumnibridge/issues/10) | Keep OPEN. Workflow merged; PR #12 and current master actually pass backend/frontend/isolated integration/migration checks. Maintainer evaluation of required branch protection is not recorded as complete, and no protection is configured. Recommend selecting the four exact job names above and reviewing enforcement/bypass policy; no repository protection settings changed |
| [#11 Atomic concurrency](https://github.com/sipratama/alumnibridge/issues/11) | **Reopened** from CLOSED. Atomic stale-write rejection, privacy revocation safety, frontend conflict handling and compatibility tests remain unimplemented. Merged ADR 0001 accepts only a development limitation; it does not satisfy the issue. Carry as production gate, including directory consent, without blocking local M4 design/development |

The M3 reports are historical evidence at their recorded SHAs. Their statements that CI was absent or all issues were open are not rewritten retroactively; this table records the current reconciliation. No #2/#5/#8/#10 acceptance checkbox/body was silently changed.

## Chosen Phase A product and architecture decisions

1. Directory requires PUBLIC + moderator VISIBLE + explicit discovery consent + trusted verified alumni membership. All existing/new profiles default to no discovery; students/pending alumni retain their direct public links.
2. Anonymous and signed-in directory reads share the same allowlist and limits. No administrator privacy bypass, Connections or Messaging.
3. Academic institution/program/year need separate per-verified-membership consent. Program/year require institution consent. Combined filters match the same membership; private academic records cannot influence results/counts.
4. Company/title/skill filters honor existing publication flags. Cards omit biography/history/private identifiers. Verification remains trusted application data.
5. Phase A selected `GET /api/v1/directory` plus owner `GET/PATCH /api/v1/users/me/profile/discovery`; Phase B now implements and documents these endpoints in OpenAPI.
6. PostgreSQL live projection, bounded literal-prefix/exact filters, canonical slug ordering, authenticated opaque cursor with no internal IDs, no public totals/facets. No search service, materialized private-field index or new infrastructure.
7. Phase A proposed additive consent columns/table/indexes; Phase B now adds migration 000006 and tests it on isolated databases only. Existing running development schema, tombstones, photo registry and old migration files remain intact.
8. PRIVATE/opt-out clear discovery/academic consent atomically in the Phase B implementation. Live queries recheck current publication state. Stale owner-write races remain #11; delivered data cannot be recalled.

## Phases and issue mapping

| Phase | Scope | GitHub issues | Completion evidence required |
| --- | --- | --- | --- |
| A — Alignment/design | Repository/GitHub alignment, roadmap, policy/ADR, API/schema/query design | [#14](https://github.com/sipratama/alumnibridge/issues/14); designs for #15–#19 | Documents and tracking verified; maintainer policy review before code |
| B — Backend | Add consent state/owner operations, SQL eligibility/projection/filtering, bounded first cursor, OpenAPI and regression tests | [#15](https://github.com/sipratama/alumnibridge/issues/15), #14; baseline #16/#18 | Isolated migration/SQL tests and signed-JWT ownership/privacy matrix pass |
| C — Pagination/performance/abuse | Cursor adversarial cases, measured indexes, timeouts/limits and privacy abuse controls | [#16](https://github.com/sipratama/alumnibridge/issues/16), [#18](https://github.com/sipratama/alumnibridge/issues/18) | Static pages deterministic; withdrawals excluded on new snapshots; EXPLAIN/limit/rate evidence |
| D — Frontend | Public directory route, filters/cards/paging, owner discovery controls, responsive keyboard/caching behavior | [#17](https://github.com/sipratama/alumnibridge/issues/17), #18 | Component/API tests and real navigation evidence |
| E — Final verification | Regression/E2E/platform/accessibility/security review, docs and PR preparation after approval | [#19](https://github.com/sipratama/alumnibridge/issues/19); final #14–#18 acceptance | Actual PR CI, review report, executed browser evidence or explicit gaps |

Phase C hardens a bounded API; Phase B must not expose an unbounded temporary search endpoint. #14 stays open until policy review and implemented OpenAPI criteria are satisfied. Do not auto-close any M4 issue when a planning document is committed.

## Recommended Phase B sequence after user review

1. Confirm ADR/API details and inspect the then-current schema tip. Add only new reversible consent migration(s), with false defaults and FK/CHECK constraints; test against a disposable database.
2. Extend profiles publication service/store for owner discovery state, owner-scoped membership consent, atomic withdrawal integration and signed-JWT ownership/rollback tests. Preserve the existing required `visibility` PATCH contract.
3. Add directory service/consumer repository/pgx/HTTP adapters and minimal DTO; reuse publication policy and apply the entire privacy matrix in SQL. No per-result calls to full public profile reads.
4. Implement bounded filters and initial cursor contract with a validated public slug anchor; synchronize OpenAPI and add empty/invalid/filter-combination/hidden-field tests.
5. Run existing backend/frontend/isolated infrastructure checks as applicable to changed executable files and inspect actual CI on a later approved PR. Defer measured performance/rate hardening and React implementation to their planned phases.

## Validation and limits (Phase A historical record)

Executed documentation checks:

- Six changed Markdown files; all 40 repository-relative file links resolve. There are no fragment links requiring anchor checks. Fences and table boundaries are balanced, the example response parses as JSON, and no developer-local file URLs are introduced.
- All 15 current issue titles are unique. The two GitHub milestones contain exactly one Milestone 4 (number 2); #14–#19 are OPEN and assigned to it. #2/#5/#8/#10/#11 remain OPEN.
- One Mermaid flowchart parsed and rendered to SVG using Mermaid 12.2.0 in headless Chrome. A plain Node parse first failed because Mermaid requires a browser DOM; rerunning in Chrome resolved that tooling limitation. Validation dependencies were installed in a temporary directory outside the repository, with no project dependency/lockfile changes.
- `git diff --check` passed; only documentation is changed on the synchronized feature branch. Latest master CI remains completed/success for `1537c88`.

Local application unit/build/lint, runtime, database, application-browser E2E, Windows/Podman and screen-reader tests are **not run in Phase A**; the master CI and M3 reports are separately attributed evidence. Rendering the documentation diagram is not an application browser test. No container, network, volume, database or verification state is changed.

The maintainer approved the Phase A eligibility/academic consent policy and contracts; Phase B now implements the backend foundation. Phase B review and approval preceded Phase C; Phase C now awaits review. Conditions before milestone merge: full implemented tests/review/CI and resolution or explicit acceptance of remaining platform/accessibility gaps. Conditions before production: atomic concurrency #11, trusted verification/moderation operations, evaluated branch protection #10, measured abuse controls, production OIDC/TLS/storage/secret policy, consent/legal/retention and backup/restore gates. Empty inventory is acceptable until genuine verified memberships exist; do not weaken verification to fill the directory.
