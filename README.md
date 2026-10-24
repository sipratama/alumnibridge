# AlumniBridge

AlumniBridge is an open-source professional network for university students and alumni. The first community is Mathematics Education at UHAMKA, Indonesia. The design allows additional universities and study programs later.

The current milestone includes a Go API, a React web app, PostgreSQL, and a dedicated local Keycloak realm for sign-in and student/alumni onboarding. The backend also supports owner profile management, explicitly public profiles, privacy controls and controlled photo delivery. The professional profile UI is implemented; the public alumni directory remains future work.

## Requirements

- macOS with Docker Desktop and Docker Compose (for the full Docker stack)
- Go 1.27 or newer (bootstrap tested with 1.27.3)
- Node.js 24 or newer and npm (bootstrap tested with Node 24.21.0 and npm 11.19.0)
- Podman with a running Podman machine and a working `podman compose` provider (verified with Podman 5.8.3 and Docker Compose provider 5.1.3; Docker Desktop is not required)
- PostgreSQL `psql` client only if you want to test the Windows host connection directly

## Local development: macOS with Docker

From the repository root, run:

```sh
docker compose -p alumnibridge --env-file .env.example up -d --build
docker compose -p alumnibridge --env-file .env.example ps
```

The web app is at `http://localhost:5173`, the API at `http://localhost:8080`, and Keycloak at `http://localhost:18085`. Compose builds the Go API and React web app, runs the database migration, and starts PostgreSQL and Keycloak. The example credentials are for local development only. To stop the stack while retaining database data, run `docker compose -p alumnibridge --env-file .env.example down`. To inspect startup issues, run `docker compose -p alumnibridge --env-file .env.example logs --tail=100 api web migrate keycloak`.

## Local development: Windows PowerShell

Run these commands from the repository root. These commands start only AlumniBridge services. The Keycloak database has its own persistent volume and is separate from application data. Default credentials are development-only examples; copy `.env.example` to the ignored `.env` and change passwords for any shared workstation.

```powershell
podman machine list
podman compose version
podman compose -p alumnibridge --env-file .env.example up -d postgres keycloak-db keycloak minio
podman compose -p alumnibridge --env-file .env.example ps
podman compose -p alumnibridge --env-file .env.example exec -T postgres pg_isready -U alumnibridge -d alumnibridge
Invoke-RestMethod http://localhost:18085/realms/alumnibridge/.well-known/openid-configuration | Select-Object issuer,jwks_uri

$env:API_PORT = '8080'
$env:DATABASE_URL = 'postgres://alumnibridge:local_dev_only_change_me@127.0.0.1:5432/alumnibridge?sslmode=disable'
$env:OIDC_ISSUER = 'http://localhost:18085/realms/alumnibridge'
$env:OIDC_AUDIENCE = 'alumnibridge-api'
$env:OIDC_CLIENT_ID = 'alumnibridge-web'
$env:OBJECT_STORAGE_ENDPOINT = 'localhost:19000'
$env:OBJECT_STORAGE_ACCESS_KEY = 'alumnibridge_dev_only'
$env:OBJECT_STORAGE_SECRET_KEY = 'local_minio_only_change_me'
$env:OBJECT_STORAGE_BUCKET = 'alumnibridge-profile-photos'
$env:OBJECT_STORAGE_SECURE = 'false'
$DirectoryKeyBytes = New-Object byte[] 32
$DirectoryRng = [Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $DirectoryRng.GetBytes($DirectoryKeyBytes)
    $env:DIRECTORY_CURSOR_KEY = [Convert]::ToBase64String($DirectoryKeyBytes)
} finally { $DirectoryRng.Dispose() }
Push-Location apps/api
go mod download
go run ./cmd/migrate up
go run ./cmd/api
Pop-Location
```

Run the web app in a **second** PowerShell terminal:

```powershell
Push-Location apps/web
npm ci
npm run dev
Pop-Location
```

The web app is at `http://localhost:5173`; Keycloak is at `http://localhost:18085`; API liveness is `http://localhost:8080/health`; readiness is `http://localhost:8080/ready`. Create an account from the web app's **Join the community** button, then select Student or Alumni. Alumni memberships start as `pending`; administrator review is a later milestone. Keycloak's local admin console uses username `localadmin` and password `local_admin_only` from `.env.example`. The browser uses Authorization Code Flow with PKCE; the API validates Keycloak access tokens and never accepts a client-supplied user ID. The example client redirects only to `http://localhost:5173/`. Set `VITE_OIDC_CALLBACK_URI` to this exact URI, including the trailing slash. Existing Keycloak databases are not updated by realm import: in the Keycloak Admin Console, inspect **Clients → alumnibridge-web → Settings → Valid redirect URIs** and set the sole value to `http://localhost:5173/` if it differs. Keep **Web origins** at `http://localhost:5173`. The API does not load `.env` itself: set its environment variables in the process as shown above. If you use `.env`, update the commands and corresponding application environment values together.

## Directory backend configuration

Milestone 4 Phase B adds anonymous `GET /api/v1/directory` and authenticated owner `GET/PATCH /api/v1/users/me/profile/discovery`. There is no Directory UI yet. Existing PUBLIC profiles are not listed until a verified alumni owner explicitly opts in. Academic filters require separate membership consent; onboarding cannot grant verification.

`DIRECTORY_CURSOR_KEY` is mandatory for native API startup: canonical base64 of 32 random bytes. On macOS/Linux use `export DIRECTORY_CURSOR_KEY="$(openssl rand -base64 32)"`; Windows generation is included above. Store the generated value in your environment/secret manager if cursors should survive restarts. Key rotation invalidates outstanding 15-minute cursors safely. Compose and `.env.example` contain an explicitly development-only fallback; replace it for shared deployments. Never print real keys or cursor tokens in logs. See [Phase B backend and isolated testing](docs/milestone-4-phase-b-backend.md).

Phase C adds configurable per-process search controls: `DIRECTORY_REQUEST_TIMEOUT_MS=2000`, `DIRECTORY_STATEMENT_TIMEOUT_MS=1500`, `DIRECTORY_MAX_CONCURRENT=8`, `DIRECTORY_RATE_PER_MINUTE=60`, `DIRECTORY_BURST=10`, `DIRECTORY_MAX_CLIENTS=10000`, and `DIRECTORY_CLIENT_TTL_SECONDS=600`. These are validated optional native defaults and explicit Compose/CI settings. Search has a separate connection pool capped at the admission limit; include it alongside the primary pool in PostgreSQL capacity planning. Rate exhaustion returns 429 with `Retry-After`; overload and timeouts have distinct 503 error codes.

Client identity is the direct TCP peer, with canonical IPv4/IPv6 addresses. `Forwarded` and `X-Forwarded-For` are ignored, including from proxies. With Nginx/Vite/another proxy, clients share the proxy's API-facing bucket. Enforce real source-IP limits at the ingress before shared deployment; setting a forwarding header does not configure a trusted proxy. Limits are per process and reset on restart. See [Phase C measurements, configuration and isolated test instructions](docs/milestone-4-phase-c-hardening.md).

## Checks

Run backend unit/race/vet/build checks, frontend test/build/lint, and PostgreSQL/MinIO integration against an isolated disposable database. Profile integration tests install failure-injection triggers; never point `TEST_DATABASE_URL` at a development or production database with active users.

See [Windows PowerShell verification](docs/windows-verification.md) for exact, project-scoped service, migration, isolated-test and browser commands. These commands were reviewed on macOS during the Milestone 3 final review; this is not a claim that Windows/Podman execution passed. The [final review report](docs/reviews/milestone-3-final-review.md) records executed results and limitations.

## Project documents

- [Product requirements](docs/prd.md)
- [Architecture and proposed repository layout](docs/architecture.md)
- [Implementation plan](docs/implementation-plan.md)
- [Contributor and agent conventions](AGENTS.md)
- [Professional profile frontend and screenshots](docs/phase-d-professional-profile-frontend.md)
- [Milestone 3 final review](docs/reviews/milestone-3-final-review.md)

## Local profile photo storage

Compose builds a dedicated MinIO instance with the `alumnibridge_minio_data` volume. The S3 API is at `http://localhost:19000` and its local console at `http://localhost:19001`; both bind to loopback. Example credentials in `.env.example` are local placeholders. Use `.env` overrides for shared machines. API object storage configuration is mandatory; native Go processes need the `OBJECT_STORAGE_*` variables shown above.

The MinIO server is built from published community release `RELEASE.2025-10-15T17-29-55Z`, pinned source commit `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`. Official registry images were unavailable during implementation. The first build downloads and compiles that source. This archived community release is for isolated local development; assess a maintained S3-compatible deployment before production. The application depends only on its replaceable storage port. Sources: [MinIO server](https://github.com/minio/minio), [Go SDK](https://github.com/minio/minio-go).

The API creates a private `alumnibridge-profile-photos` bucket and refuses buckets with anonymous access policies. Upload raw JPEG/PNG/WebP bytes to `POST /api/v1/users/me/profile/photo` with the matching Content-Type, up to 5 MiB. The API normalizes pixels, strips metadata, and returns its own read URL. Object keys and signed object URLs are never returned. Owner reads require authentication; public reads require public visibility, visible moderation status, and `show_photo=true`. Members manage profiles at `/profile`, privacy at `/settings/privacy`, and explicitly public profiles are available anonymously at `/in/:slug`.

A cleanup worker runs every minute, deleting at most 20 obsolete objects per sweep. Failed uploads/activations remain registered and expire after 15 minutes; failed deletes retain retry intent. Removal revokes API access immediately while storage deletion may retry later. Keep database and object storage backups consistent. See [Phase C design and verification](docs/phase-c-profile-privacy.md).
