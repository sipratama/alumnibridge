# AlumniBridge: Product Requirements Document

## Purpose and audience

AlumniBridge helps students discover alumni career paths and build professional connections. It launches for Mathematics Education students and alumni of UHAMKA, while its institution and study-program concepts must support later communities. This document defines the first product release; it does not prescribe database tables or API shapes.

## User roles

| Role | Main capabilities |
| --- | --- |
| Visitor | Browse the public alumni directory and public profile fields; use public search. |
| Student | Register, maintain a profile, and request connections after signing in. |
| Alumni applicant | Register, maintain a profile, and submit membership information for review. |
| Verified alumni | Appear in the public directory when discoverability is enabled; use authenticated interactions. |
| Administrator | Review alumni membership claims and record a decision; manage access to verification information. |

An identity may be both a student and an alumnus over time. An alumni claim is not a privilege until an administrator verifies it. A Keycloak account alone does not establish alumni status.

## First-release capabilities

1. **Identity and registration:** Students and alumni can create accounts through Keycloak, sign in, and select their community affiliation. The application keeps professional and membership state separate from Keycloak credentials.
2. **Professional profile:** An authenticated member can manage a display name, headline, current profession and company, work experience, education, and skills. Education and graduation year are subject to a field-level publication decision.

Milestone 3 also provides a bio, location, photo, shareable slug, private-by-default profile visibility, and server-computed completeness. Public profile access is now consent-based for students, pending alumni and verified alumni; optional groups require separate publication choices. Verification badges come only from trusted memberships.

Milestone 3 Phase B provides authenticated owner-only profile, experience, education, and skills operations. Profiles default private. Phase C adds backend public reads, owner privacy controls, normalized photos and separate moderation hiding. Students and pending alumni may explicitly opt into public profiles, but pending alumni must never be represented as verified alumni. Phase D provides responsive owner editing, photo and field privacy controls, and anonymous `/in/:slug` pages. Public preview uses the actual anonymous API after publication; no private draft is presented as public.

3. **Public alumni directory (Milestone 4):** Visitors can view explicitly opted-in, verified alumni through paginated results and public profile pages. PUBLIC visibility alone does not enable discovery; directory consent defaults false for existing and new profiles. Backend filters cover name, university, study program, graduation year, current profession/company and skills. University, program and graduation year require additional per-verified-membership publication consent; professional filters honor existing field flags. Unverified claims never enter directory results. See the [directory design](milestone-4-directory-design.md) and [policy ADR](decisions/0002-directory-discoverability.md), implemented by the [Phase B backend](milestone-4-phase-b-backend.md) and [Phase D frontend](milestone-4-phase-d-frontend.md). [Phase C hardening](milestone-4-phase-c-hardening.md) adds bounded execution, per-process admission and source-IP rate limits; these reduce abuse without guaranteeing prevention of copying or distributed scraping.
4. **Membership review:** Authorized administrators can inspect restricted verification information, approve or reject a claim, and see who made the decision and when. Claimants can see their own review status.
5. **Connections:** Signed-in members can send, accept, decline, or withdraw connection requests and see their own connection state. Duplicate and self-requests are rejected.
6. **Privacy controls:** Private contact details, academic proof, review notes, and account identifiers are excluded from public responses. Members can control whether their verified alumni profile is listed publicly.

## Acceptance criteria

- A visitor can search the directory without signing in and receives only public fields for discoverable, verified alumni.
- A student or unverified alumni applicant does not appear in public alumni search, even if a profile contains a graduation year.
- A PUBLIC verified profile without directory opt-in does not appear in search. Private, moderator-hidden and deleted profiles never appear; signing in does not bypass these rules.
- A private or non-consented field cannot influence directory matching or public counts. Academic filters must match one verified membership whose relevant fields were explicitly published. Pending affiliations cannot inherit another affiliation's verified label.
- Authenticated members can edit only their own profile and connection requests; administrators alone can decide membership claims.
- The server checks permissions independently of the web UI and does not use a user-supplied ID as proof of identity.
- Search results are paginated and deterministic for equal filters; empty and invalid filters have documented behavior.
- A connection state transition cannot create duplicate active requests or self-connections.
- Verification evidence and reviewer notes are absent from public API schemas, logs, search indexes, and generated frontend types.
- Core authorization, verification, and connection state transitions have Go tests; the main browser journeys have integration or end-to-end coverage before release.

## Out of scope for the first release

Messaging, jobs, recommendations, social feeds, invitations, payment, institution self-service administration, and native mobile apps. Supporting multiple institutions in the model does not mean launching multiple communities immediately.

## Privacy and operational requirements

- Collect only information needed for registration, professional discovery, and membership review. Keep verification material in a restricted workflow with a defined retention period before production launch.
- Public visibility is explicit at the profile level. The public API uses an allowlist of fields; private fields never depend on frontend hiding.
- Use HTTPS in production, least-privilege database and Keycloak credentials, secret-free repository files, and audit records for administrative decisions.
- Protected application operations must reject an existing suspended, deleting or deleted identity even while its previously issued OIDC access token remains cryptographically valid. Account deletion, recovery, marker retention and public withdrawal require separate approved lifecycle policies before production use.
- Store timestamps in UTC and present dates in the viewer's locale. Use UUIDs for application primary keys.
- Provide accessible forms, clear verification status, and responsive layouts. English is the source language for code and docs; interface localization can be planned separately.

## Open product decisions

1. **Discoverability consent copy:** The approved and implemented policy uses separate default-false directory opt-in, verified-only anonymous results and additional academic publication consent ([ADR 0002](decisions/0002-directory-discoverability.md)). Phase D provides owner-facing consent copy and controls; review legal wording before launch. Direct PUBLIC student/pending profiles remain allowed; directory consent withdrawal does not itself make those direct links private. Display name/headline remain existing PUBLIC core fields; optional professional fields retain their existing flags.
2. **Verification proof:** Confirm acceptable evidence, reviewers, appeal path, and deletion period. Proposed default: restricted submission with administrator review; do not publish or reuse evidence.
3. **Student eligibility:** Confirm whether any authenticated person may register as a student or whether enrollment also needs verification.
4. **Connections:** Confirm whether connection requests are allowed to unverified applicants, and whether private profile/contact fields become visible after acceptance. Proposed default: connection status changes only; no automatic disclosure of private fields.
5. **Language and legal notices:** Confirm launch language, privacy notice, consent text, and data deletion process with the project owner before public release.
