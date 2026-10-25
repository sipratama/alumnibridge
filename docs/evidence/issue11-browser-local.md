# Issue #11 disposable two-session browser evidence

- Result: **PASS**
- Application source HEAD: `3988340d3cc3ff28e1cb5e55aceeb0b587a41ff1`
- UTC: 2026-10-25T00:25:45.761Z
- Command: `ISSUE11_POSTGRES_PORT=25444 ISSUE11_MINIO_PORT=29014 ISSUE11_MINIO_CONSOLE_PORT=29015 CHROME_PATH=<local Chrome executable> bash scripts/issue11-browser-e2e.sh`
- Environment: isolated Docker Compose project; PostgreSQL 17, MinIO, Keycloak, API and web; disposable verified alumni identity.
- Authentication: two independent Playwright browser contexts with one disposable Keycloak account; an additional anonymous context checks public denial.
- Artifact policy: HTTP status and UI assertions only. No trace, response body, token, cookie, password, account identifier or personal data is stored.

## Assertions

| Check | HTTP |
| --- | ---: |
| seed_public | 200 |
| private_withdrawal | 200 |
| private_public_read | 404 |
| stale_public_draft | 412 |
| fresh_explicit_public | 200 |
| seed_directory_consent | 200 |
| directory_withdrawal | 200 |
| stale_academic_draft | 412 |
| experience_create | 201 |
| experience_change | 200 |
| stale_experience_delete | 412 |
| reviewed_experience_delete | 204 |

UI assertions passed: original PUBLIC/academic drafts remain visible on 412; conflict review blocks blind Save; PRIVATE public read is 404; a fresh owner view keeps Directory disclosure off; stale history DELETE shows the new title and requires explicit review before 204.

The wrapper removes the disposable Compose project and all of its volumes after this run. The result is local evidence; GitHub CI on a later commit must be reported separately.
