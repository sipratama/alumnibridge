# AlumniBridge

AlumniBridge is an open-source professional network for university students and alumni. The first community is Mathematics Education at UHAMKA, Indonesia. The design allows additional universities and study programs later.

This bootstrap contains a Go API, a React web shell, and local PostgreSQL. Authentication and business features are planned in [docs/implementation-plan.md](docs/implementation-plan.md).

## Requirements

- Go 1.27 or newer (bootstrap tested with 1.27.3)
- Node.js 24 or newer and npm (bootstrap tested with Node 24.21.0 and npm 11.19.0)
- Podman with a running Podman machine and a working `podman compose` provider (verified with Podman 5.8.3 and Docker Compose provider 5.1.3; Docker Desktop is not required)
- PostgreSQL `psql` client only if you want to test the Windows host connection directly

## Local development: Windows PowerShell

Run these commands from the repository root. The explicit project name keeps the container, network, and volume under `alumnibridge`:

```powershell
podman machine list
podman compose version
podman compose -p alumnibridge --env-file .env.example up -d postgres
podman compose -p alumnibridge --env-file .env.example ps
podman compose -p alumnibridge --env-file .env.example exec -T postgres pg_isready -U alumnibridge -d alumnibridge

$env:API_PORT = '8080'
$env:DATABASE_URL = 'postgres://alumnibridge:local_dev_only_change_me@127.0.0.1:5432/alumnibridge?sslmode=disable'
Push-Location apps/api
go mod download
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

The web app is at `http://localhost:5173`. API liveness is `http://localhost:8080/health`; readiness is `http://localhost:8080/ready`. Readiness returns HTTP 503 until PostgreSQL is reachable. `.env.example` contains a local-only example password. For your own settings, copy it to the ignored `.env`, change the password in both `POSTGRES_PASSWORD` and `DATABASE_URL`, and use `--env-file .env` in the Podman commands. The API does not read `.env` itself: set its environment variables in its process as shown above.

## Checks

```powershell
Push-Location apps/api
go test ./...
go build ./cmd/api
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
```

To stop only AlumniBridge PostgreSQL: `podman compose -p alumnibridge --env-file .env.example stop postgres`. This retains its data volume for subsequent runs.

## Project documents

- [Product requirements](docs/prd.md)
- [Architecture and proposed repository layout](docs/architecture.md)
- [Implementation plan](docs/implementation-plan.md)
- [Contributor and agent conventions](AGENTS.md)
