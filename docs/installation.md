# Install Burrow

English · [简体中文](installation_zh.md)

Use a published release for installation. Docker Compose is recommended; its
images and the standalone Linux binaries include the frontend and need no Go or
Bun at runtime. Source development is covered in the
[development guide](development/getting-started.md).

## Choose and verify a release

Open [Releases](https://github.com/ArkGravity/burrow/releases), choose a version
and read its release notes. Download the deployment archive for Compose, or the
binary archive for your architecture, together with `SHA256SUMS`, `IMAGES.txt`
and the release's `INSTALL.md`. The latter contains instructions rendered for
that exact version. Use the same version for all downloaded files.

The examples below pin `v1.0.0`. Replace this value when deliberately selecting
another release; keep its image tag or digest pinned. This guide describes the
amd64/arm64 release layout; historical v0.1.0 is amd64-only and has its own
attached installation instructions.

On Linux with GNU coreutils, check the downloaded files before extracting:

```bash
sha256sum --ignore-missing --check SHA256SUMS
```

Only files present in the current directory are checked. Ensure the selected
archive reports `OK`. If you download every listed attachment, you can instead
use `sha256sum --check SHA256SUMS`. On other systems, compare the archive's SHA-256
with its entry using your platform's checksum tool. `IMAGES.txt` records the image
index and platform digests.

## Docker Compose

Install Docker Engine and Compose v2. Download `burrow_v1.0.0_deploy.tar.gz`,
then extract the archive and enter its directory:

```bash
BURROW_RELEASE=v1.0.0
tar -xzf "burrow_${BURROW_RELEASE}_deploy.tar.gz"
cd "burrow_${BURROW_RELEASE}_deploy"
cp .env.example .env
openssl rand -base64 32 # master key
openssl rand -hex 24    # PostgreSQL password
openssl rand -hex 24    # initial administrator password
```

For a new production database, edit `.env` with these settings:

| Setting                           | Value                                                                           |
| --------------------------------- | ------------------------------------------------------------------------------- |
| `BURROW_ENV`                      | `prod`                                                                          |
| `BURROW_ISSUER`                   | Your public HTTPS URL, for example `https://sso.example.com`                    |
| `BURROW_MASTER_KEY`               | The generated Base64 master key                                                 |
| `POSTGRES_PASSWORD`               | The generated database password                                                 |
| `BURROW_DB_DSN`                   | Use that same password; retain `host=postgres` for the bundled database         |
| `BURROW_BOOTSTRAP_ADMIN_PASSWORD` | The independently generated administrator password                              |
| `BURROW_TRUSTED_PROXIES`          | Actual proxy CIDRs, if forwarded client addresses are needed                    |
| `BURROW_IMAGE`                    | Keep the fixed version supplied by the release archive, or pin its index digest |

Keep `.env` private. Configure your TLS reverse proxy to forward to Burrow's
loopback port 8080 and preserve the browser Origin. Configure only actual trusted
proxy CIDRs. The issuer must match the public URL used by browsers and OIDC
clients. PostgreSQL is not published to the host by the bundled Compose file.

The archive selects `ghcr.io/arkgravity/burrow:v1.0.0`; the same version is
available from `docker.io/logic3579/burrow:v1.0.0`. Docker selects Linux amd64 or
arm64 automatically. The deployment archive uses the canonical root
[docker-compose.yml](../docker-compose.yml) and contains no Dockerfile.

```bash
docker compose --profile local-db config --quiet
docker compose --profile local-db pull
docker compose --profile local-db up -d --no-build --pull never
docker compose --profile local-db ps
curl --fail http://127.0.0.1:8080/readyz
```

Compose runs migration, seed and the server in order. For an external PostgreSQL
database, update the DSN host, credentials and TLS settings, and omit
`--profile local-db` from every command. See the
[configuration reference](development/configuration.md#compose).

## First login and MFA

Open your configured HTTPS issuer. Sign in as `admin` with the password set in
`BURROW_BOOTSTRAP_ADMIN_PASSWORD`, then change the temporary password. If you
changed `BURROW_BOOTSTRAP_ADMIN_USERNAME`, use that username instead. Bootstrap
settings only create the first administrator; repeated seed preserves existing
accounts, passwords and assignments.

MFA is disabled by default. To require TOTP for every account, including
Administrator, set `BURROW_MFA_ENABLED=true` in `.env` and recreate the application:

```bash
docker compose --profile local-db up -d --no-build --pull never --force-recreate app
```

Users must then bind or verify their authenticator during login. Enabling MFA
requires password-only sessions to log in again; disabling it preserves existing
bindings. See [MFA policy](development/configuration.md#global-mfa-policy) and
[MFA recovery](operations/recovery.md#mandatory-mfa-upgrade-and-recovery).
Use the [README application steps](../README.md#connect-applications) to configure
roles and downstream applications after sign-in.

## Standalone Linux binary

Choose `burrow_v1.0.0_linux_amd64.tar.gz` for x86-64 or
`burrow_v1.0.0_linux_arm64.tar.gz` for AArch64. The binaries require glibc 2.36+
(for example Debian 12); Alpine/musl and older glibc are not supported.
Extract the selected archive, enter its directory and copy the configuration:

```bash
./burrow version
cp configs/config.yaml configs/config.local.yaml
```

For production, edit the local YAML to set `env: prod`, an HTTPS `server.issuer`,
PostgreSQL connection settings, an independent `security.master_key` and
`bootstrap.admin_password`. Set `security.mfa_enabled: true` if you require MFA.
The unchanged defaults use SQLite and public development credentials.
Use the same local configuration for every command:

```bash
./burrow migrate --config configs/config.local.yaml
./burrow seed --config configs/config.local.yaml
./burrow serve --config configs/config.local.yaml
```

Native commands never load `.env`. Run the server under your service manager and
configure your TLS reverse proxy. Follow the first-login steps above using the
username and temporary password from your YAML. See
[configuration](development/configuration.md) for validation and overrides.

## Shutdown, upgrades and recovery

```bash
docker compose exec app burrow version
docker compose --profile local-db down
```

Shutdown preserves database volumes. Keep the database, original master key,
configuration and Compose project name across upgrades; do not use
`down --volumes` when retaining data. A new master key cannot decrypt an existing
database's signing keys or TOTP secrets.

Back up before upgrading, read the selected release notes, stop the old server,
select the new image or binary, then run migration and seed before starting it.
Recheck administrator and configured downstream OIDC/SAML login afterward. A database migration
cannot be rolled back merely by choosing an older image.

v1.0.0 uses schema v6. Upgrading from v0.1.x applies migration 006, preserving
existing OIDC data, grants and keys while initializing independent encrypted SAML
signing keys. Older sessions may need fresh authentication for SAML. A binary
requiring schema v5 cannot use the upgraded database; rollback requires the
pre-upgrade backup. See [SAML certificate recovery and Jenkins trust updates](operations/recovery.md#saml-证书与-schema-v6).

Versions v0.1.0 and v0.1.1 required MFA; v0.1.2 and later default to disabling it.
Explicitly enable the MFA policy before starting a newer release if you want to
preserve that requirement. Older Provider-based databases must satisfy the
password-only migration requirements before upgrading.
See [backup, upgrades and recovery](operations/recovery.md) for the full procedure.
