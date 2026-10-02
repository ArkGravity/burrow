#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
BURROW_E2E_TMP=$(mktemp -d)
BURROW_E2E_PID=
cleanup() {
  if [ -n "$BURROW_E2E_PID" ]; then kill "$BURROW_E2E_PID" 2>/dev/null || true; wait "$BURROW_E2E_PID" 2>/dev/null || true; fi
  rm -rf "$BURROW_E2E_TMP"
}
trap cleanup EXIT INT TERM
export BURROW_ENV=dev
export BURROW_MASTER_KEY=$(openssl rand -base64 32)
export BURROW_DB_DRIVER=sqlite
export BURROW_DB_DSN="$BURROW_E2E_TMP/burrow.db"
export BURROW_LISTEN_ADDR=127.0.0.1:18080
export BURROW_ISSUER=http://localhost:18080
export BURROW_E2E_URL=http://localhost:18080
export BURROW_BOOTSTRAP_ADMIN_USERNAME=admin
export BURROW_BOOTSTRAP_ADMIN_PASSWORD=Initial-admin-password-2026
bun run --cwd web build
if [ -z "${BURROW_E2E_BINARY:-}" ]; then
  go build -tags embedweb -o "$BURROW_E2E_TMP/burrow" ./cmd/burrow
  BURROW_E2E_BINARY="$BURROW_E2E_TMP/burrow"
fi
test -x "$BURROW_E2E_BINARY"
go build -o "$BURROW_E2E_TMP/web-client" ./examples/web-client
export BURROW_E2E_BINARY
export BURROW_E2E_WEB_BINARY="$BURROW_E2E_TMP/web-client"
"$BURROW_E2E_BINARY" migrate
"$BURROW_E2E_BINARY" seed
"$BURROW_E2E_BINARY" seed
"$BURROW_E2E_BINARY" serve >"$BURROW_E2E_TMP/server.log" 2>&1 &
BURROW_E2E_PID=$!
BURROW_E2E_READY=0
for attempt in 1 2 3 4 5 6 7 8 9 10; do
  if curl --fail --silent http://localhost:18080/readyz >/dev/null; then BURROW_E2E_READY=1; break; fi
  sleep 1
done
if [ "$BURROW_E2E_READY" != 1 ]; then cat "$BURROW_E2E_TMP/server.log"; exit 1; fi
bun run --cwd web test:e2e
