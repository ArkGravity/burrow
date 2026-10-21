#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
export BURROW_SAML_TMP
mkdir -p tmp
fixture_root="$(pwd -P)/tmp"
BURROW_SAML_TMP=$(mktemp -d "$fixture_root/saml-jenkins.XXXXXX")
export BURROW_SAML_PROJECT
BURROW_SAML_PROJECT="burrow-saml-$(openssl rand -hex 5)"
export BURROW_SAML_MASTER_KEY
BURROW_SAML_MASTER_KEY=$(openssl rand -base64 32)
compose_file=web/e2e/fixtures/jenkins/docker-compose.yml
cleanup() {
    case "$BURROW_SAML_PROJECT:$BURROW_SAML_TMP" in
        burrow-saml-*:"$fixture_root"/saml-jenkins.*) ;;
        *) echo 'Refusing cleanup outside this test fixture' >&2; return 1 ;;
    esac
    docker compose -p "$BURROW_SAML_PROJECT" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$BURROW_SAML_TMP"
}
trap cleanup EXIT INT TERM
mkdir -p "$BURROW_SAML_TMP/tls" "$BURROW_SAML_TMP/public"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=burrow-saml-test \
    -addext 'subjectAltName=DNS:sso.burrow.test,DNS:jenkins.burrow.test,DNS:jenkins.burrow-sp.test' \
    -keyout "$BURROW_SAML_TMP/tls/test.key" -out "$BURROW_SAML_TMP/tls/test.crt" >/dev/null 2>&1
if [ "${BURROW_SAML_SKIP_BUILD:-false}" != true ]; then
    docker build -t burrow:saml-test .
    docker build -t burrow-jenkins:saml-test examples/local-sso/jenkins
fi
docker compose -p "$BURROW_SAML_PROJECT" -f "$compose_file" config --quiet
docker compose -p "$BURROW_SAML_PROJECT" -f "$compose_file" up -d nginx
BURROW_SAML_HTTPS_PORT=$(docker compose -p "$BURROW_SAML_PROJECT" -f "$compose_file" port nginx 443 | sed 's/.*://')
export BURROW_SAML_HTTPS_PORT
docker compose -p "$BURROW_SAML_PROJECT" -f "$compose_file" up -d --wait burrow
bun web/e2e/fixtures/jenkins/setup.ts
bun run --cwd web test:e2e --config playwright.jenkins.config.ts
