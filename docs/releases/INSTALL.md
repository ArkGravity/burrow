# Install Burrow

This release supports Linux amd64 and arm64. Download attachments from
[GitHub Releases](https://github.com/ArkGravity/burrow/releases). Verify the
downloaded files with `sha256sum --check SHA256SUMS` in the download directory.
Download every listed attachment before checking the complete checksum file.
`IMAGES.txt` lists the multi-platform version tags and platform references
(`repository@sha256:...`) with their registry digests. New publications create
only the multi-platform version tag. The historical v0.1.0 release supports
amd64 only; follow its attached installation guide when installing that version.

## Containers (recommended)

Extract `burrow_@VERSION@_deploy.tar.gz` and enter the extracted directory. It
contains the canonical root Compose file, an environment example and the MIT
license. Docker Engine and Compose v2 are required; Go and Bun are not required.

```bash
cp .env.example .env
openssl rand -base64 32 # generate an independent master key
openssl rand -hex 24    # generate a PostgreSQL password
openssl rand -hex 24    # generate an initial administrator password
```

Edit `.env` before starting. Use the generated values for `BURROW_MASTER_KEY`,
`POSTGRES_PASSWORD` and the password in `BURROW_DB_DSN`, and
`BURROW_BOOTSTRAP_ADMIN_PASSWORD`. Set `BURROW_ENV=prod`, an HTTPS
`BURROW_ISSUER` matching your public URL and the appropriate trusted proxy CIDRs.
Keep `.env` private. Place Burrow behind your TLS reverse proxy; its host port
binds to loopback by default. Production uses PostgreSQL.

The deployment example selects `ghcr.io/arkgravity/burrow:@VERSION@`. You can instead
set `BURROW_IMAGE=docker.io/logic3579/burrow:@VERSION@`, or pin the matching index digest
from `IMAGES.txt`. Change the tag when installing a later release. The archive
does not contain a Dockerfile; use the published image without building locally.
Docker selects amd64 or arm64 automatically from the same tag or index digest.

```bash
docker compose --profile local-db config --quiet
docker compose --profile local-db pull
docker compose --profile local-db up -d --no-build --pull never
docker compose --profile local-db ps
curl --fail http://127.0.0.1:8080/readyz
```

Compose starts migration, seed and the server in order. Sign in as `admin` with
your configured temporary password and change it. MFA is disabled by default;
to require TOTP for all accounts, manually set `BURROW_MFA_ENABLED=true` in `.env`
and recreate the application container. Users then bind or verify their authenticator
at login. Re-running seed preserves existing accounts and credentials. For an external PostgreSQL database, update the DSN with its
host/credentials/TLS settings and omit `--profile local-db` from these commands.

```bash
docker compose exec app burrow version
docker compose --profile local-db down
```

Shutdown preserves database volumes. Keep the Compose project name stable;
do not use `down --volumes` when keeping data.

## Standalone binary

Choose `burrow_@VERSION@_linux_amd64.tar.gz` for x86-64 or
`burrow_@VERSION@_linux_arm64.tar.gz` for AArch64, then enter its extracted
directory. Each binary embeds the frontend and configuration defaults. It
requires Linux on the matching architecture with glibc 2.36 or newer (for example
Debian 12); Alpine/musl and older glibc are not supported. It is built with CGO using the
same Debian bookworm build as the container. Go and Bun are not needed to run it.

```bash
./burrow version
cp configs/config.yaml configs/config.local.yaml
```

For production, edit the local YAML to set production mode, an HTTPS issuer,
PostgreSQL connection details, a new master key and an independent initial
administrator password. Use the same configuration for every command:

```bash
./burrow migrate --config configs/config.local.yaml
./burrow seed --config configs/config.local.yaml
./burrow serve --config configs/config.local.yaml
```

Native commands never read `.env`. The unchanged example YAML uses SQLite and
public development credentials; use it only for local development. Start the
server under your service manager and put it behind your TLS reverse proxy.

## Upgrades and recovery

Back up the database, original master key and configuration before upgrading.
Versions v0.1.0 and v0.1.1 required MFA; v0.1.2 defaults to disabling it. To keep
requiring MFA, explicitly set `BURROW_MFA_ENABLED=true` in the service environment
(Compose: `.env`), or `security.mfa_enabled: true` in the selected YAML, before
starting the upgraded server. Existing authenticator bindings are preserved.
Stop the old server, select the new image/binary, run migration and seed, then
start the new server. Never generate a replacement master key for an existing
database. Keep the old backup until administrator and downstream OIDC login
have been verified.

Upgrading older development versions to schema v5 invalidates old Burrow
sessions/tokens and unfinished authorizations. Users sign in again with their
existing passwords and, when MFA is enabled, bind TOTP. Older Provider-based databases must first
satisfy the password-only migration requirements. Applications keep their own
sessions. Database migration cannot be undone merely by selecting an older
image; restore a compatible database backup and its original configuration/key.

See the [configuration reference](https://github.com/ArkGravity/burrow/blob/@VERSION@/docs/development/configuration.md)
and [backup, upgrade and MFA recovery guide](https://github.com/ArkGravity/burrow/blob/@VERSION@/docs/operations/recovery.md).

## License and verification scope

Burrow is released under the included MIT license. These are early releases;
future releases may require configuration, API or database migrations. Automated
OIDC interoperability checks and historical manual acceptance do not constitute
OpenID Foundation certification or validation of your production deployment.
