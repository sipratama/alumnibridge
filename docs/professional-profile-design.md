# Milestone 3 professional profile design (Phase A)

Phase B implements owner-only profile, experience, education, and skills routes. Phase C implements backend public profile reads, privacy controls and photos. The frontend is Phase D. The API remains a modular monolith. The `profiles` module will own profile, experience, education, skill, slug and photo metadata rules. HTTP and pgx adapters will depend on its domain/application code, not the reverse. The verified identity user UUID is the owner key; no client-supplied owner ID is accepted.

## Data model

Migration `000002_profiles` adds a one-to-one `profiles` row for each user, slug reservations, work experience, education, a shared skill vocabulary and user-skill links. The profile retains editable current company/job title separately from the experience list so members without a detailed work history can still describe their current role. `users.display_name` remains the one source of truth for display name. Education may refer to an existing institution but never changes `memberships`. Photo metadata contains an opaque object key, content type and size; image bytes live outside PostgreSQL.

Profile rows are created lazily when a member first edits or reads their own profile. A new profile is private, and every optional public field flag is false. A public profile requires explicit opt-in. Publishing a profile does not make it eligible for the future verified alumni directory. The public profile endpoint may show a student or pending applicant only if the owner opts in; this policy is approved, and pending alumni never gain a verified badge. A public DTO must exclude email, OIDC subject, UUIDs not needed for display, membership proof and review data. No raw row may be serialized publicly.

Slugs use lowercase ASCII letters, digits and internal hyphens, 3–64 characters. Normalize a candidate from the display name, fall back to `member-<short random suffix>` if empty, and allocate collisions with a random suffix under a transaction. Reserve `api`, `admin`, `auth`, `in`, `me`, `settings`, `dashboard`, `login`, `logout`, `register`, `onboarding`, `profile`, `profiles`, and offensive terms maintained in an application denylist. Slug history reserves each issued slug for its original owner permanently, retaining only a slug tombstone after account deletion. A changed slug may redirect to the current slug only when that profile is public; private profiles return 404 for both old and current slugs. The service must insert the reservation and update the current slug in one transaction. `profiles.slug` and `profile_slug_history.slug` each have unique constraints; the application transaction must also check the other table. Do not expose the user UUID in slug URLs. Migration `000004_slug_reservations` prevents post-deletion reuse and binds each current slug to its owner reservation. Its rollback refuses to discard deleted-account tombstones.

Completeness is a server-computed integer percentage based on nine equally weighted signals: display name, headline, bio, location, current company, current job title, at least one experience, at least one education, and at least one skill. It is private metadata and does not determine public eligibility. Photo remains excluded from the established nine-signal score; changing the weighting is a future UX decision.

Experience dates are calendar dates, with an optional end date; current positions require no end date. Education dates are optional but ordered if both exist. Both collections are bounded and ordered by start date descending, then UUID. Skill normalization trims and collapses spaces and case-folds before lookup; the database unique `normalized_name` prevents duplicate vocabulary entries, and `(user_id, skill_id)` prevents duplicate assignments.

## Proposed API contract

All paths are below `/api/v1`. OpenAPI contains the implemented Phase B routes. Owner routes require the verified Keycloak bearer token; skill search is public. Experience and education PATCH replace their editable fields, while profile PATCH updates only supplied fields. Owner collections are capped at 100 entries with a conflict response at the limit; skill search is capped at 50.

| Method and path | Purpose | Response |
| --- | --- | --- |
| `GET /users/me/profile` | Owner profile and completeness | Private profile DTO |
| `PATCH /users/me/profile` | Edit text and slug (visibility editing deferred) | Private profile DTO |
| `GET /users/me/experiences` | Owner list | Bounded ordered list |
| `POST /users/me/experiences` | Add experience | 201 with owner DTO |
| `PATCH /users/me/experiences/{id}` | Edit owned experience | Owner DTO |
| `DELETE /users/me/experiences/{id}` | Delete owned experience | 204 |
| `GET /users/me/educations` | Owner list | Bounded ordered list |
| `POST /users/me/educations` | Add education | 201 with owner DTO |
| `PATCH /users/me/educations/{id}` | Edit owned education | Owner DTO |
| `DELETE /users/me/educations/{id}` | Delete owned education | 204 |
| `GET /skills?query=&limit=` | Search existing skills | Bounded prefix results |
| `GET /users/me/skills` | Owner skills | Bounded list |
| `POST /users/me/skills` | Add or reuse a normalized skill | Owner DTO |
| `DELETE /users/me/skills/{id}` | Remove owned skill link | 204 |
| `POST /users/me/profile/photo` | Upload/replace one validated image | 201 with photo metadata |
| `DELETE /users/me/profile/photo` | Remove image | 204 |
| `GET /profiles/{slug}` | Read an opted-in public profile | Public allowlist DTO or non-disclosing 404 |

Use 400 for malformed input, 401 for missing/invalid token, 404 for absent or non-owned resources, 409 for slug conflicts, 413 for oversized uploads, and 415 for unsupported media. Owner collection responses must have hard limits. Public photos require an access-controlled delivery path designed in Phase C; no direct private object key appears in public DTOs.

Migration `000003_profile_details` adds employment type and location to experience and description to education. `000002_profiles` was left unchanged because it may already be applied in persistent environments. The PostgreSQL adapter uses an owner-scoped WHERE clause for every item mutation and a transaction for profile/slug updates and skill vocabulary/link creation. Slug reservations use a unique key and retry a random suffix during initialization; explicit conflicting slug updates return 409. Slug history remains internal until Phase C defines public redirects.

## Implemented Phase C decisions and remaining release questions

- Students and pending alumni may explicitly publish; only trusted verified alumni membership grants the badge. Future directory eligibility is a separate policy.
- Public consent shares display name/headline. Optional groups start hidden; membership graduation years are never published.
- Dedicated local MinIO and durable retry cleanup are implemented. Production storage maintenance and retention operations still need release review.
- The existing nine-signal completeness formula is retained. Agree on the offensive-slug moderation policy before public launch.

Phase B review: profile PATCH merges under a row lock; optional empty strings clear text while omitted fields retain it. Null is rejected for non-nullable fields. Experience/education PATCH retains its documented replacement semantics. Requests use a 64 KiB JSON body limit and a ten-second API context deadline. See [review evidence](reviews/milestone-3-phase-b-review.md).

Phase C replaces the deferred publication/storage design with [the implemented privacy and photo lifecycle](phase-c-profile-privacy.md). Privacy uses `/users/me/profile/privacy`, not the earlier suggested visibility route.
