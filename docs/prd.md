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
3. **Public alumni directory:** Visitors can view discoverable, verified alumni through paginated results and public profile pages. Search supports profession, company, graduation year, and skills. Unverified claims never enter public results.
4. **Membership review:** Authorized administrators can inspect restricted verification information, approve or reject a claim, and see who made the decision and when. Claimants can see their own review status.
5. **Connections:** Signed-in members can send, accept, decline, or withdraw connection requests and see their own connection state. Duplicate and self-requests are rejected.
6. **Privacy controls:** Private contact details, academic proof, review notes, and account identifiers are excluded from public responses. Members can control whether their verified alumni profile is listed publicly.

## Acceptance criteria

- A visitor can search the directory without signing in and receives only public fields for discoverable, verified alumni.
- A student or unverified alumni applicant does not appear in public alumni search, even if a profile contains a graduation year.
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
- Store timestamps in UTC and present dates in the viewer's locale. Use UUIDs for application primary keys.
- Provide accessible forms, clear verification status, and responsive layouts. English is the source language for code and docs; interface localization can be planned separately.

## Open product decisions

1. **Discoverability:** Confirm whether public listing requires explicit opt-in, and which profile fields may be public by default. Proposed default: verified alumni opt in; display name, headline, profession, company, selected skills, and graduation year are individually publishable.
2. **Verification proof:** Confirm acceptable evidence, reviewers, appeal path, and deletion period. Proposed default: restricted submission with administrator review; do not publish or reuse evidence.
3. **Student eligibility:** Confirm whether any authenticated person may register as a student or whether enrollment also needs verification.
4. **Connections:** Confirm whether connection requests are allowed to unverified applicants, and whether private profile/contact fields become visible after acceptance. Proposed default: connection status changes only; no automatic disclosure of private fields.
5. **Language and legal notices:** Confirm launch language, privacy notice, consent text, and data deletion process with the project owner before public release.
