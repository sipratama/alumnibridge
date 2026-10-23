# Milestone 3 professional profile design (Phase A)

This is a design for future API work, not a list of implemented routes. The API remains a modular monolith. The `profiles` module will own profile, experience, education, skill, slug and photo metadata rules. HTTP and pgx adapters will depend on its domain/application code, not the reverse. The verified identity user UUID is the owner key; no client-supplied owner ID is accepted.

## Data model

Migration `000002_profiles` adds a one-to-one `profiles` row for each user, slug reservations, work experience, education, a shared skill vocabulary and user-skill links. The profile retains editable current company/job title separately from the experience list so members without a detailed work history can still describe their current role. `users.display_name` remains the one source of truth for display name. Education may refer to an existing institution but never changes `memberships`. Photo metadata contains an opaque object key, content type and size; image bytes live outside PostgreSQL.

Profile rows are created lazily when a member first edits or reads their own profile. A new profile is private, and every optional public field flag is false. A public profile requires explicit opt-in. Publishing a profile does not make it eligible for the future verified alumni directory. The public profile endpoint may show a student or pending applicant only if the owner opts in; this policy needs product approval before Phase C. A public DTO must exclude email, OIDC subject, UUIDs not needed for display, membership proof and review data. No raw row may be serialized publicly.

Slugs use lowercase ASCII letters, digits and internal hyphens, 3–64 characters. Normalize a candidate from the display name, fall back to `member-<short random suffix>` if empty, and allocate collisions with a random suffix under a transaction. Reserve `api`, `admin`, `auth`, `in`, `me`, `settings`, `dashboard`, `login`, `logout`, `register`, `onboarding`, `profile`, `profiles`, and offensive terms maintained in an application denylist. Slug history reserves each issued slug for its original owner for the lifetime of that account. A changed slug may redirect to the current slug only when that profile is public; private profiles return 404 for both old and current slugs. The service must insert the reservation and update the current slug in one transaction. `profiles.slug` and `profile_slug_history.slug` each have unique constraints; the application transaction must also check the other table. Do not expose the user UUID in slug URLs. Account deletion and post-deletion slug reuse need a separate policy decision.

Completeness is a server-computed integer percentage based on display name, headline, bio, location, current role/company, at least one experience, at least one education, at least one skill, and a photo. It is private metadata and does not determine public eligibility. Fix the exact weighting in domain tests before implementation.

Experience dates are calendar dates, with an optional end date; current positions require no end date. Education dates are optional but ordered if both exist. Both collections are bounded and ordered by start date descending, then UUID. Skill normalization trims and collapses spaces and case-folds before lookup; the database unique `normalized_name` prevents duplicate vocabulary entries, and `(user_id, skill_id)` prevents duplicate assignments.

## Proposed API contract

All paths are below `/api/v1`. The OpenAPI file will be updated when each route is implemented. Owner routes require the verified Keycloak bearer token; public reads do not.

| Method and path | Purpose | Response |
| --- | --- | --- |
| `GET /users/me/profile` | Owner profile and completeness | Private profile DTO |
| `PATCH /users/me/profile` | Edit text, slug and visibility choices | Private profile DTO |
| `GET /users/me/experiences` | Owner list | Bounded ordered list |
| `POST /users/me/experiences` | Add experience | 201 with owner DTO |
| `PATCH /users/me/experiences/{id}` | Edit owned experience | Owner DTO |
| `DELETE /users/me/experiences/{id}` | Delete owned experience | 204 |
| `GET /users/me/educations` | Owner list | Bounded ordered list |
| `POST /users/me/educations` | Add education | 201 with owner DTO |
| `PATCH /users/me/educations/{id}` | Edit owned education | Owner DTO |
| `DELETE /users/me/educations/{id}` | Delete owned education | 204 |
| `GET /skills?q=&limit=&cursor=` | Search existing skills | Bounded page |
| `GET /users/me/skills` | Owner skills | Bounded list |
| `POST /users/me/skills` | Add or reuse a normalized skill | Owner DTO |
| `DELETE /users/me/skills/{id}` | Remove owned skill link | 204 |
| `POST /users/me/profile/photo` | Upload/replace one validated image | 201 with photo metadata |
| `DELETE /users/me/profile/photo` | Remove image | 204 |
| `GET /profiles/{slug}` | Read an opted-in public profile | Public allowlist DTO or non-disclosing 404 |

Use 400 for malformed input, 401 for missing/invalid token, 404 for absent or non-owned resources, 409 for slug conflicts, 413 for oversized uploads, and 415 for unsupported media. Owner collection responses must have hard limits. Public photos require an access-controlled delivery path designed in Phase C; no direct private object key appears in public DTOs.

## Open decisions before Phase C

- Confirm whether opted-in students and pending alumni may have public profiles. This design proposes yes, but only verified alumni may appear in the later directory.
- Confirm field defaults for public display name/headline and whether education and graduation year need separate consent. This design defaults optional fields hidden and never publishes graduation year from membership data.
- Choose the photo storage adapter and its deletion/retention policy before upload work. Use a private local directory for development if suitable; no work-related MinIO containers.
- Agree on the exact offensive-slug denylist and completeness weights before implementing the related domain logic.
