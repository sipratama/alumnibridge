# AlumniBridge

AlumniBridge is an open-source professional network for university students and alumni. The first community is Mathematics Education at UHAMKA, Indonesia. The design allows additional universities and study programs later.

This bootstrap contains a Go API, a React web shell, and local PostgreSQL. Authentication and business features are planned in [docs/implementation-plan.md](docs/implementation-plan.md).

## Requirements

- Go 1.27 or newer (bootstrap tested with 1.27.3)
- Node.js 24 or newer and npm (bootstrap tested with Node 24.21.0 and npm 11.19.0)
- Docker Desktop with Compose for PostgreSQL

## Local development: Windows PowerShell

Run these commands from the repository root:

```powershell
Copy-Item .env.example .env
docker compose --env-file .env up -d postgres

$env:API_PORT = '8080'
$env:DATABASE_URL = 'postgres://alumnibridge:local_dev_only_change_me@localhost:5432/alumnibridge?sslmode=disable'
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

The web app is at `http://localhost:5173`. API liveness is `http://localhost:8080/health`; readiness is `http://localhost:8080/ready`. Readiness returns HTTP 503 until PostgreSQL is reachable. Replace the example local password for any shared environment. The API does not read `.env` itself: set the environment variables in its process as shown above.

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

docker compose --env-file .env ps
Invoke-RestMethod http://localhost:8080/health
Invoke-RestMethod http://localhost:8080/ready
```

To stop local PostgreSQL: `docker compose --env-file .env down`. The named database volume remains for subsequent runs.

## Project documents

- [Product requirements](docs/prd.md)
- [Architecture and proposed repository layout](docs/architecture.md)
- [Implementation plan](docs/implementation-plan.md)
- [Contributor and agent conventions](AGENTS.md)
