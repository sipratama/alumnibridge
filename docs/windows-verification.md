# AlumniBridge verification on Windows and Podman

These are exact PowerShell steps for a Windows reviewer. They were inspected on macOS, **not executed on Windows**. Use a dedicated local AlumniBridge checkout and the repository's development example credentials. If you use a customized `.env`, change the Compose file argument and matching test/API credentials together. Do not run these against production.

## Inspect and start only this project

Run from the repository root. Select an already approved running Podman machine; do not create, stop or replace a shared machine to run these checks.

```powershell
$ErrorActionPreference = 'Stop'
podman machine list
podman compose version
podman compose -p alumnibridge --env-file .env.example config --quiet
if ($LASTEXITCODE -ne 0) { throw 'Compose validation failed' }
podman compose -p alumnibridge --env-file .env.example up -d --build postgres keycloak-db keycloak minio migrate api web
if ($LASTEXITCODE -ne 0) { throw 'Project startup failed' }
podman compose -p alumnibridge --env-file .env.example ps
podman compose -p alumnibridge --env-file .env.example exec -T postgres pg_isready -U alumnibridge -d alumnibridge
podman compose -p alumnibridge --env-file .env.example exec -T postgres psql -U alumnibridge -d alumnibridge -c 'SELECT version, dirty FROM schema_migrations;'
Invoke-WebRequest http://localhost:19000/minio/health/ready -UseBasicParsing | Select-Object StatusCode
Invoke-RestMethod http://localhost:18085/realms/alumnibridge/.well-known/openid-configuration | Select-Object issuer,jwks_uri
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8080/ready
Invoke-RestMethod 'http://localhost:8080/api/v1/directory?limit=1' | Select-Object items,next_cursor
Invoke-WebRequest http://localhost:5173/ -UseBasicParsing | Select-Object StatusCode
Invoke-WebRequest http://localhost:5173/alumni -UseBasicParsing | Select-Object StatusCode
try { Invoke-WebRequest http://localhost:8080/api/v1/users/me/profile -UseBasicParsing } catch { [int]$_.Exception.Response.StatusCode }
```

Expected for this repository revision: healthy databases/Keycloak/MinIO, migrate completes successfully, migration version 8 with `dirty=false`, health/readiness, anonymous Directory and web routes return 200, unauthenticated owner profile returns 401. This is an expected result, not evidence that the procedure ran on Windows. Startup applies only outstanding migrations through the existing migration runner; never use a reset, `force`, volume deletion or table truncation to fix startup. If a command fails, inspect only project logs with `podman compose -p alumnibridge --env-file .env.example logs --tail=100 api migrate minio keycloak` and resolve the actual error.

For existing Keycloak databases, import does not overwrite the client. Read `alumnibridge-web` in the admin console: public client, standard flow enabled, PKCE S256, sole Valid Redirect URI `http://localhost:5173/`, web origin `http://localhost:5173`, direct grants and implicit flow disabled. Do not broaden the allowlist to repair an authentication failure. The running Mac client was inspected separately in the [final review](reviews/milestone-3-final-review.md).

The original native API/Vite startup remains available in [README](../README.md). Choose either the container API/web above or native processes; do not bind two processes to the same host ports.

## Isolated PostgreSQL and storage integration tests

Install the Go version specified by `apps/api/go.mod`, Node 24/npm, and a C toolchain if running Go's race detector on a supported Windows architecture. Inspect `go version`, `go env GOOS GOARCH CGO_ENABLED`, `node --version`, and `npm --version`. If race detection is unsupported or its C compiler is unavailable, record that check as unexecuted; do not report it passed.

The profile integration suite installs temporary failure-injection triggers. It must not use the live development database. The block below creates a unique temporary database, applies/rolls back/reapplies migrations before tests, and drops only that database in `finally`. The MinIO tests create and remove unique buckets; they do not reuse the application bucket.

```powershell
$ReviewDb = 'alumnibridge_review_' + [guid]::NewGuid().ToString('N')
$ReviewDbCreated = $false
$PreviousTestEnvironment = @{}
$TestKeys = @('TEST_DATABASE_URL', 'TEST_OBJECT_STORAGE_ENDPOINT', 'TEST_OBJECT_STORAGE_ACCESS_KEY', 'TEST_OBJECT_STORAGE_SECRET_KEY')
foreach ($Key in $TestKeys) { $PreviousTestEnvironment[$Key] = [Environment]::GetEnvironmentVariable($Key, 'Process') }
function Invoke-ReviewSqlFile([string]$Database, [string]$File) {
    Get-Content -LiteralPath $File -Raw | podman compose -p alumnibridge --env-file .env.example exec -T postgres psql -v ON_ERROR_STOP=1 -U alumnibridge -d $Database
    if ($LASTEXITCODE -ne 0) { throw "Migration SQL failed: $File" }
}
try {
    podman compose -p alumnibridge --env-file .env.example exec -T postgres createdb -U alumnibridge $ReviewDb
    if ($LASTEXITCODE -ne 0) { throw 'Temporary database creation failed; nothing will be dropped' }
    $ReviewDbCreated = $true
    $UpFiles = Get-ChildItem infrastructure/migrations/*.up.sql | Sort-Object Name
    $DownFiles = Get-ChildItem infrastructure/migrations/*.down.sql | Sort-Object Name -Descending
    foreach ($File in $UpFiles) { Invoke-ReviewSqlFile $ReviewDb $File.FullName }
    foreach ($File in $DownFiles) { Invoke-ReviewSqlFile $ReviewDb $File.FullName }
    foreach ($File in $UpFiles) { Invoke-ReviewSqlFile $ReviewDb $File.FullName }

    $env:TEST_DATABASE_URL = "postgres://alumnibridge:local_dev_only_change_me@127.0.0.1:5432/${ReviewDb}?sslmode=disable"
    $env:TEST_OBJECT_STORAGE_ENDPOINT = '127.0.0.1:19000'
    $env:TEST_OBJECT_STORAGE_ACCESS_KEY = 'alumnibridge_dev_only'
    $env:TEST_OBJECT_STORAGE_SECRET_KEY = 'local_minio_only_change_me'
    Push-Location apps/api
    try {
        go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Backend tests failed' }
        go test -race ./...
        if ($LASTEXITCODE -ne 0) { throw 'Race tests failed or are unsupported; record the exact outcome' }
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Vet failed' }
        go build ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw 'API build failed' }
    } finally { Pop-Location }
} finally {
    foreach ($Key in $TestKeys) { [Environment]::SetEnvironmentVariable($Key, $PreviousTestEnvironment[$Key], 'Process') }
    if ($ReviewDbCreated) {
        podman compose -p alumnibridge --env-file .env.example exec -T postgres dropdb -U alumnibridge --if-exists $ReviewDb
        if ($LASTEXITCODE -ne 0) { Write-Warning "Inspect and remove only the temporary database $ReviewDb" }
    }
}

Push-Location apps/web
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    npm run test
    if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed' }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    npm run lint
    if ($LASTEXITCODE -ne 0) { throw 'Frontend lint failed' }
} finally { Pop-Location }
git diff --check
if ($LASTEXITCODE -ne 0) { throw 'Diff whitespace check failed' }
```

Rollback is exercised before test data creates permanent tombstones. A rollback guard rejecting an unsafe populated rollback is intentional and must not be bypassed.

## Real browser verification

The harness needs an installed Chrome and Playwright. Install the pinned test utility into a unique temporary directory, not the app dependency graph. Use example admin credentials only for the dedicated local realm: cleanup deletes only the unique synthetic users created by the harness.

```powershell
$BrowserTools = Join-Path ([System.IO.Path]::GetTempPath()) ('alumnibridge-browser-' + [guid]::NewGuid().ToString('N'))
npm install --prefix $BrowserTools --no-save --package-lock=false playwright@1.64.1
if ($LASTEXITCODE -ne 0) { throw 'Browser tooling installation failed' }
$env:PLAYWRIGHT_MODULE = Join-Path $BrowserTools 'node_modules/playwright'
$env:CHROME_EXECUTABLE = Join-Path $env:ProgramFiles 'Google\Chrome\Application\chrome.exe'
if (-not (Test-Path -LiteralPath $env:CHROME_EXECUTABLE)) { throw 'Set CHROME_EXECUTABLE to the installed Chrome executable' }
$env:CONTAINER_CLI = 'podman'
$env:KEYCLOAK_ADMIN_USERNAME = 'localadmin'
$env:KEYCLOAK_ADMIN_PASSWORD = 'local_admin_only'
node apps/web/e2e/phase-d.cjs
if ($LASTEXITCODE -ne 0) { throw 'Browser verification failed; inspect its cleanup outcome' }
```

Also manually register Student and Alumni through `http://localhost:5173`, complete onboarding, edit each professional field, create/update/delete experience and education, reuse skills, upload/replace/delete JPEG/PNG/WebP, explicitly publish with selected fields, view `/in/:slug` in an anonymous browser, confirm hidden fields and no false badge, rename the slug, then switch to Private and retry both old/new public links and photo reads. Refresh `/profile`, `/account`, `/dashboard`, `/settings/privacy` and `/onboarding`; sign out, revisit protected routes, then sign in and confirm the intended destination restores. Record any `redirect_uri` error or loop, rather than changing client wildcards.

Keyboard/screen-reader steps are in the [final review checklist](reviews/milestone-3-final-review.md#manual-accessibility-checklist). Do not infer WCAG compliance from automation or screenshots.

Stop only the services started for this verification when finished, retaining their volumes:

```powershell
podman compose -p alumnibridge --env-file .env.example stop web api postgres keycloak keycloak-db minio
```

Do not run prune, system reset, machine removal or `down -v` on a shared workstation.
