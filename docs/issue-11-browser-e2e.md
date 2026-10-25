# Reproducing Issue #11 two-session browser checks

This is an isolated local acceptance check for PR #22. It starts a fresh Docker Compose project with its own PostgreSQL 17, MinIO, Keycloak, API and web containers. It creates a random disposable Keycloak account, onboards it through the browser, verifies its alumni membership only in that disposable database, and removes the project **and all its volumes** at exit. It does not use or alter the running `alumnibridge` development project.

## Requirements and command

- Docker Compose, Node.js compatible with the web package, and a Chromium browser. `playwright` is pinned in `apps/web/package-lock.json`.
- Free local ports 25443 (PostgreSQL), 28080 (API), 25173 (web), 28085 (Keycloak), 29012/29013 (MinIO). Override any with `ISSUE11_POSTGRES_PORT`, `ISSUE11_API_PORT`, `ISSUE11_WEB_PORT`, `ISSUE11_KEYCLOAK_PORT`, `ISSUE11_MINIO_PORT`, or `ISSUE11_MINIO_CONSOLE_PORT`.
- Local development credentials from `.env.example` are used **only** inside the isolated project. The browser user/password are generated in memory on every run. No production credential, user or document is needed.

From the repository root:

```sh
npm --prefix apps/web ci
CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  bash scripts/issue11-browser-e2e.sh
```

On another platform, set `CHROME_PATH` to the local Chrome/Chromium executable. Alternatively install Playwright's Chromium once with `(cd apps/web && npx playwright install chromium)` and omit `CHROME_PATH`. The wrapper refuses an existing project name and chooses a unique `abissue11e2e...` project by default. Do not set `ISSUE11_PROJECT` to a development project. The wrapper applies all migrations through 000008 via the normal up-only migrator and always runs `docker compose down -v` for **its own** project, including on failure.

## Assertions

The browser runner records the HTTP status of each mutation and checks the visible owner UI:

1. Publish with an explicit consent checkbox. Open two separate Keycloak browser contexts. Session B edits an optional field on its original PUBLIC draft. Session A saves PRIVATE (200); a fresh anonymous context receives 404 for the public profile. Session B's old PATCH receives 412, retains its checked draft and comparison view, and cannot blindly Save again. A fresh PRIVATE-to-PUBLIC action requires consent again.
2. Opt into Directory on the fresh PUBLIC profile. Session B selects academic institution disclosure. Session A withdraws Directory consent (200). B's old PATCH receives 412; the academic checkbox remains selected in the draft while Save is blocked for review. A fresh owner context confirms the saved Directory listing and institution disclosure remain off.
3. Create an experience. Session B selects it for deletion. Session A changes its title (200). B's DELETE receives 412; the dialog displays the new title and blocks immediate deletion. Only after the explicit “Use current entry revision for deletion” action does the second DELETE return 204.

The runner writes [the local evidence report](evidence/issue11-browser-local.md) with source HEAD, UTC time, PASS/FAIL, and assertion names/statuses. It stores no browser trace, response body, token, cookie, password, generated account identifier or personal data. A failed run writes `FAIL` and exits nonzero; do not treat an earlier report as proof for a later code SHA. The report is local evidence and does not replace exact-SHA CI or independent security approval.
