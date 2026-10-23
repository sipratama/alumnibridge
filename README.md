# AlumniBridge

AlumniBridge is an open-source professional network for university students and alumni. The first community is Mathematics Education at UHAMKA, Indonesia. The design allows additional universities and study programs later.

The current milestone includes a Go API, a React web app, PostgreSQL, and a dedicated local Keycloak realm for sign-in and student/alumni onboarding. Professional profiles and the public directory are future work.

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
podman compose -p alumnibridge --env-file .env.example up -d postgres keycloak-db keycloak
podman compose -p alumnibridge --env-file .env.example ps
podman compose -p alumnibridge --env-file .env.example exec -T postgres pg_isready -U alumnibridge -d alumnibridge
Invoke-RestMethod http://localhost:18085/realms/alumnibridge/.well-known/openid-configuration | Select-Object issuer,jwks_uri

$env:API_PORT = '8080'
$env:DATABASE_URL = 'postgres://alumnibridge:local_dev_only_change_me@127.0.0.1:5432/alumnibridge?sslmode=disable'
$env:OIDC_ISSUER = 'http://localhost:18085/realms/alumnibridge'
$env:OIDC_AUDIENCE = 'alumnibridge-api'
$env:OIDC_CLIENT_ID = 'alumnibridge-web'
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

The web app is at `http://localhost:5173`; Keycloak is at `http://localhost:18085`; API liveness is `http://localhost:8080/health`; readiness is `http://localhost:8080/ready`. Create an account from the web app's **Create account** button, then select Student or Alumni. Alumni memberships start as `pending`; administrator review is a later milestone. Keycloak's local admin console uses username `localadmin` and password `local_admin_only` from `.env.example`. The browser uses Authorization Code Flow with PKCE; the API validates Keycloak access tokens and never accepts a client-supplied user ID. The example client redirects only to `http://localhost:5173/`. Set `VITE_OIDC_CALLBACK_URI` to this exact URI, including the trailing slash. Existing Keycloak databases are not updated by realm import: in the Keycloak Admin Console, inspect **Clients → alumnibridge-web → Settings → Valid redirect URIs** and set the sole value to `http://localhost:5173/` if it differs. Keep **Web origins** at `http://localhost:5173`. The API does not load `.env` itself: set its environment variables in the process as shown above. If you use `.env`, update the commands and corresponding application environment values together.

## Checks

```powershell
$env:DATABASE_URL = 'postgres://alumnibridge:local_dev_only_change_me@127.0.0.1:5432/alumnibridge?sslmode=disable'
Push-Location apps/api
go test ./...
go build ./cmd/api
$env:TEST_DATABASE_URL = $env:DATABASE_URL
go test ./internal/identity/postgres -count=1
Remove-Item Env:TEST_DATABASE_URL
Pop-Location

Push-Location apps/web
npm ci
npm run test
npm run build
Pop-Location

podman compose -p alumnibridge --env-file .env.example ps
podman compose -p alumnibridge --env-file .env.example exec -T postgres psql -U alumnibridge -d alumnibridge -c 'SELECT 1;'
$env:PGPASSWORD = 'local_dev_only_change_me'
psql -h 127.0.0.1 -p 5432 -U alumnibridge -d alumnibridge -At -c 'SELECT 1;'
Remove-Item Env:PGPASSWORD
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8080/ready
try { Invoke-WebRequest http://localhost:8080/api/v1/auth/me } catch { [int]$_.Exception.Response.StatusCode }
```

The final command should print `401` without a bearer token. The OpenAPI contract is at [apps/api/openapi/openapi.yaml](apps/api/openapi/openapi.yaml). To stop only AlumniBridge services: `podman compose -p alumnibridge --env-file .env.example stop postgres keycloak-db keycloak`. This retains both databases' volumes. Do not use broad Podman cleanup commands on a shared workstation.

## Project documents

- [Product requirements](docs/prd.md)
- [Architecture and proposed repository layout](docs/architecture.md)
- [Implementation plan](docs/implementation-plan.md)
- [Contributor and agent conventions](AGENTS.md)
