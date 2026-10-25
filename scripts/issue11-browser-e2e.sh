#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

# A unique project protects the running development stack and its volumes.
export ISSUE11_PROJECT=${ISSUE11_PROJECT:-abissue11e2e$(date +%s)$$}
if [[ ! $ISSUE11_PROJECT =~ ^abissue11e2e[a-z0-9]+$ ]]; then
  echo 'ISSUE11_PROJECT must be a fresh abissue11e2e project name' >&2
  exit 2
fi
if [[ -n $(docker compose -p "$ISSUE11_PROJECT" -f compose.yaml ps -aq) ]] ||
   [[ -n $(docker volume ls -q --filter "label=com.docker.compose.project=$ISSUE11_PROJECT") ]]; then
  echo 'Refusing to reuse an existing Compose project or volume' >&2
  exit 2
fi

export POSTGRES_PORT=${ISSUE11_POSTGRES_PORT:-25443}
export API_PORT=${ISSUE11_API_PORT:-28080}
export WEB_PORT=${ISSUE11_WEB_PORT:-25173}
export KEYCLOAK_PORT=${ISSUE11_KEYCLOAK_PORT:-28085}
export MINIO_PORT=${ISSUE11_MINIO_PORT:-29012}
export MINIO_CONSOLE_PORT=${ISSUE11_MINIO_CONSOLE_PORT:-29013}
export VITE_OIDC_CALLBACK_URI="http://localhost:$WEB_PORT/"
export ISSUE11_REPORT=${ISSUE11_REPORT:-$repo_root/docs/evidence/issue11-browser-local.md}

cleanup() {
  local status=$?
  trap - EXIT
  docker compose -p "$ISSUE11_PROJECT" --env-file .env.example -f compose.yaml down -v --remove-orphans >/dev/null || true
  if [[ $status -ne 0 ]]; then
    echo 'Disposable browser run failed; its isolated containers and volumes were removed.' >&2
  fi
  exit "$status"
}
trap cleanup EXIT

docker compose -p "$ISSUE11_PROJECT" --env-file .env.example -f compose.yaml config --quiet
docker compose -p "$ISSUE11_PROJECT" --env-file .env.example -f compose.yaml up -d --build

# Compose health checks cover PostgreSQL, MinIO and Keycloak. Wait for the API
# and web listeners before opening the two browser sessions.
for attempt in $(seq 1 60); do
  if curl --fail --silent --output /dev/null "http://localhost:$API_PORT/health" &&
     curl --fail --silent --output /dev/null "http://localhost:$WEB_PORT/"; then
    break
  fi
  if [[ $attempt -eq 60 ]]; then
    echo 'Disposable API/web did not become ready' >&2
    exit 1
  fi
  sleep 1
done

npm --prefix apps/web run test:e2e:issue11
echo "Sanitized evidence: $ISSUE11_REPORT"
