# ADR 0002 — Separate directory discovery and academic publication consent

Date: 2026-10-24 (Asia/Jakarta).
Status: **Chosen Phase A design; awaiting maintainer review before implementation**.
Scope: Milestone 4 — Alumni Directory & Discovery. No runtime or schema change is made by this ADR.

## Context

Milestone 3 lets students, pending alumni and verified alumni explicitly publish a professional profile for direct anonymous access. Optional fields have separate publication flags. A public profile is not permission to aggregate it into a searchable directory or to disclose private academic membership data.

The [PRD](../prd.md) limits the alumni directory to verified alumni. The directory plan at base `1537c88` left discoverability and academic consent undecided; the [current plan](../milestone-4-directory-plan.md) records this phase's choices. Membership verification remains application state, independent of Keycloak roles and self-entered education.

## Decision

1. **Separate discovery opt-in, default false.** Existing and new profiles are not discoverable until their owner explicitly enables directory consent. Do not backfill consent from PUBLIC visibility. The consent copy explains anonymous search, the public alumni verification indicator and scraping limitations.
2. **Verified alumni only.** Effective eligibility requires an existing profile, PUBLIC visibility, moderator VISIBLE state, discovery consent and at least one trusted membership with `role='alumni' AND status='verified'`. Student-only, pending-only and rejected-only accounts are excluded. A person with several roles can qualify through a verified alumni membership; pending memberships are never labeled verified.
3. **Anonymous browsing.** Visitors and authenticated users receive the same minimal public directory projection. Signing in, including as a future administrator, does not bypass directory privacy. Future administrative searches require separate authorized APIs.
4. **Additional academic consent.** University, study program and graduation year are separately publishable per verified membership, each defaulting false. Program or graduation-year publication requires institution publication. Academic filters and returned affiliations use only those consented fields on the same verified membership. `show_educations` does not authorize publication of membership records. A verification badge alone reveals no institution, program or year.
5. **Existing professional flags govern search as well as display.** Company, job title and skills cannot match if their existing publication flag is false. Name and headline are the existing public profile core; Phase A proposes name-prefix search only. Other biography/history/location fields are outside directory search and cards.
6. **Withdrawal is effective in SQL.** PRIVATE, HIDDEN, deleted or no-longer-verified accounts fail every directory query, including subsequent cursor pages. Moving to PRIVATE also clears discovery and academic consents atomically; returning to PUBLIC requires fresh opt-in. Removing discovery consent clears academic consents. Hiding suppresses access without changing owner consent; authorized unhide can restore an otherwise eligible listing. Future verification changes must clear discovery and academic consent when the last verified alumni membership is lost, requiring fresh consent after reverification.
7. **Canonical slugs only in results.** Historical slugs retain existing direct-profile redirects, subject to current visibility, but never produce duplicate directory entries or search matches. Permanent anonymous tombstones are not directory data and cannot be reassigned.
8. **No stronger revocation claim.** Recheck eligibility and field consent in each SQL statement. Already delivered data and an earlier in-flight database snapshot cannot be recalled. [ADR 0001](0001-last-writer-wins-development.md) still applies: atomic stale-write rejection is not implemented, and [#11](https://github.com/sipratama/alumnibridge/issues/11) remains a production gate, including new consent writes.

## Consequences and alternatives

| Choice | Benefit | Cost / rejected alternative |
| --- | --- | --- |
| Separate opt-in | Preserves the difference between sharing a link and broad discovery | Extra owner action; PUBLIC-implies-discoverable would unexpectedly list existing users |
| Verified-only directory | Matches the PRD and makes alumni status trustworthy | Sparse/empty directory until verification operations exist; listing students/pending applicants would require an explicit later product change |
| Per-membership academic flags | Supports multiple institutions without publishing an entire academic record | More consent controls and SQL tests than one global academic flag |
| Anonymous equal-access projection | Supports public career discovery without making login a privacy bypass | Public data remains scrapeable; bounded queries and rate controls reduce abuse but cannot prevent copying |
| SQL projection on each page | Avoids stale public search indexes and duplicated privacy sources | More database work than a cached/materialized directory; measure before optimizing |

No verifier/admin workflow is added to make the directory appear populated. Backend tests can seed verified memberships only in disposable databases; shared or production accounts must never be promoted for demonstrations.

## Compatibility and implementation gate

Existing anonymous `/in/:slug` and `/api/v1/profiles/{slug}` access for PUBLIC students/pending alumni remains valid. Discovery withdrawal alone does not privatize a directly public profile or revoke its public photo. Existing owner privacy requests retain their current required `visibility` contract; directory consent uses a separate owner endpoint, with transactional withdrawal integration. No client-supplied owner, moderation state or verification flag is accepted.

Before Phase B, the maintainer reviews this policy, especially verified-only eligibility, consent wording and academic consent granularity. The [design](../milestone-4-directory-design.md) specifies the candidate API, schema, tests and bounded pagination. OpenAPI will describe executable endpoints when Phase B adds them; this planning ADR does not claim they exist.
