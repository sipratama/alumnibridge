# Milestone 4 — Alumni Directory & Discovery (planning)

Status: **Planned; implementation not started**.

This milestone has been explicitly reprioritized by the maintainer to follow Milestone 3. The old roadmap in `docs/implementation-plan.md` uses a different Milestone 4 label ("Identity operations"); update the canonical plan when the new scope is approved, retaining those identity/verification operations as deferred release work.

## Objective

Enable privacy-safe discovery of eligible student/alumni professional profiles. Build on the explicit PUBLIC consent and field-level privacy controls from Milestone 3 without revealing private academic records or Keycloak information.

## GitHub work items

- [#14 Directory product policy and consent](https://github.com/sipratama/alumnibridge/issues/14)
- [#15 Privacy-safe directory search API](https://github.com/sipratama/alumnibridge/issues/15)
- [#16 Pagination and query limits](https://github.com/sipratama/alumnibridge/issues/16)
- [#17 React directory search UI](https://github.com/sipratama/alumnibridge/issues/17)
- [#18 Abuse resistance and privacy regression](https://github.com/sipratama/alumnibridge/issues/18)
- [#19 E2E and documentation](https://github.com/sipratama/alumnibridge/issues/19)

## Decisions required before implementation

1. Whether PUBLIC profile consent automatically enables directory inclusion, or directory discovery requires an additional opt-in.
2. Whether Student, pending Alumni and verified Alumni can all appear in the directory, and what trusted labels they may display.
3. Which academic membership details (institution, program, cohort) may be searchable/visible; do not assume private membership fields are automatically public.
4. Whether anonymous search is permitted, and how filters/rate limits apply to non-authenticated visitors.
5. Stable pagination and sorting contract, plus privacy behavior when results change between pages.

## Guardrails

- Never expose NIM, membership evidence, Keycloak subject, non-consented contact data or internal UUIDs.
- Derive verified badges exclusively from trusted membership status.
- Hide PRIVATE and moderated HIDDEN profiles in all search/filters/counts.
- Use bounded, parameterized SQL; do not implement unbounded scraping-friendly endpoints.
- Keep architecture modular monolith with Chi + pgx/PostgreSQL and typed React API integration.
- Preserve Milestone 3 behavior and the development-only concurrency decision tracked in #11.
- Do not claim production readiness until storage, abuse controls, concurrency, moderation and backup/restore gates are satisfied.

## Development process

1. Finalize #14 design and acceptance criteria.
2. Implement backend and pagination with PostgreSQL integration tests.
3. Add responsive UI and real browser coverage.
4. Run CI established by #10 and complete release review.
5. Open a PR from `feature/milestone-4-alumni-directory` to `master`; do not merge automatically.

**Branch ancestry note:** The branch was created from `master` before the CI Foundation draft PR was merged. Refresh/rebase the branch onto the latest `master` before starting implementation, without discarding local changes.
