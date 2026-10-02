#!/usr/bin/env bash
set -euo pipefail

image=${1:?Provide the locally loaded release image}
export BURROW_ENV=prod
export BURROW_DB_DRIVER=postgres
export BURROW_DB_DSN=${BURROW_TEST_POSTGRES_DSN:?Use a dedicated test database}
export BURROW_MASTER_KEY
BURROW_MASTER_KEY=$(openssl rand -base64 32)
export BURROW_BOOTSTRAP_ADMIN_PASSWORD
BURROW_BOOTSTRAP_ADMIN_PASSWORD=$(openssl rand -hex 24)
export BURROW_ISSUER=https://release-smoke.invalid
export BURROW_LISTEN_ADDR=127.0.0.1:18081
container="burrow-release-smoke-${GITHUB_RUN_ID:-$$}-${GITHUB_RUN_ATTEMPT:-1}"
cleanup() { docker rm -f "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT
runtime=(--rm --network host --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges:true
  --env BURROW_ENV --env BURROW_DB_DRIVER --env BURROW_DB_DSN --env BURROW_MASTER_KEY
  --env BURROW_BOOTSTRAP_ADMIN_PASSWORD --env BURROW_ISSUER --env BURROW_LISTEN_ADDR)
docker run "${runtime[@]}" "$image" migrate
docker run "${runtime[@]}" "$image" seed
docker run "${runtime[@]}" "$image" seed
docker run -d --name "$container" "${runtime[@]}" "$image" serve >/dev/null
ready=false
for ((attempt=0; attempt<30; attempt++)); do
  if curl --fail --silent http://127.0.0.1:18081/readyz >/dev/null; then ready=true; break; fi
  sleep 1
done
if [[ $ready != true ]]; then docker logs "$container"; exit 1; fi
curl --fail --silent http://127.0.0.1:18081/healthz >/dev/null
curl --fail --silent http://127.0.0.1:18081/ | grep -q '/assets/'
docker exec --env BURROW_HEALTHCHECK_URL=http://127.0.0.1:18081/readyz "$container" burrow healthcheck
echo 'Release image: production startup, PostgreSQL, readiness and embedded UI passed.'
