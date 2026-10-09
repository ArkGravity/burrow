# Configuration

Burrow uses `configs/config.yaml` for native development. The same file is
embedded at build time so a standalone binary and the container have identical
defaults. No `.env` loader is used by the application.

## Loading and precedence

1. Embedded defaults from `configs/config.yaml`.
2. `configs/config.yaml` in the current working directory, when present, or the
   explicitly selected `--config` file.
3. `BURROW_*` environment variables, intended for Compose and service managers.

An explicit file must exist. YAML files can contain a subset of fields; omitted
fields retain their embedded defaults. Unknown fields, multiple YAML documents,
invalid CIDRs, nonpositive durations and invalid production settings fail startup.
An environment variable that is set but empty overrides the file; empty values
are rejected for required fields and durations. Empty CIDR variables clear lists.
Relative file paths are resolved against the current working directory.

Use `make run CONFIG=configs/config.local.yaml` or
`go run ./cmd/burrow serve --config configs/config.local.yaml` for local overrides.
Local configuration files are excluded from Git and Docker build contexts. Do not
put secrets in the tracked default file: it is embedded into the binary.

## Fields

| YAML field                 | Environment override     | Default                                            |
| -------------------------- | ------------------------ | -------------------------------------------------- |
| `env`                      | `BURROW_ENV`             | `dev`                                              |
| `server.listen_addr`       | `BURROW_LISTEN_ADDR`     | `:8080`                                            |
| `server.issuer`            | `BURROW_ISSUER`          | `http://localhost:8080`                            |
| `server.static_dir`        | `BURROW_STATIC_DIR`      | `web/dist` (non-embedded builds)                   |
| `server.trusted_proxies`   | `BURROW_TRUSTED_PROXIES` | Empty; trust no forwarded client IPs               |
| `database.driver`          | `BURROW_DB_DRIVER`       | `sqlite`                                           |
| `database.dsn`             | `BURROW_DB_DSN`          | `burrow.db`                                        |
| `security.master_key`      | `BURROW_MASTER_KEY`      | Public development example (production rejects it) |
| `security.master_key_file` | `BURROW_MASTER_KEY_FILE` | `data/master.key`                                  |
| `security.mfa_enabled`     | `BURROW_MFA_ENABLED`     | `false`                                            |
| `session.ttl`              | `BURROW_SESSION_TTL`     | `8h`                                               |
| `oidc.token_ttl`           | `BURROW_TOKEN_TTL`       | `5m`                                               |
| `oidc.auth_code_ttl`       | `BURROW_AUTH_CODE_TTL`   | `60s`                                              |
| `oidc.login_ttl`           | `BURROW_LOGIN_TTL`       | `10m`                                              |
| `audit.retention`          | `BURROW_EVENT_RETENTION` | `2160h` (90 days)                                  |

CIDR lists are YAML sequences or comma-separated environment values. Production
requires PostgreSQL and an HTTPS issuer without a path, query or credentials.

Upstream Provider settings were removed. Delete the `providers` section from
older YAML files before running the new version: unknown YAML fields are rejected.
Remove `BURROW_PROVIDER_ALLOWED_CIDRS` and `BURROW_ALLOW_PRIVATE_PROVIDERS` from
service environments; these variables no longer have an effect.

## Master key lifecycle

The master key is 32 bytes encoded as Base64. Both `configs/config.yaml` and
`.env.example` contain the same public development example. Production rejects
this key whether it comes from YAML, environment variables or a key file.
Generate an independent production value with `openssl rand -base64 32`.

An explicit `master_key` takes precedence over `master_key_file`. To use a key
file, clear `security.master_key` in your local YAML (or set `BURROW_MASTER_KEY`
to an empty value). Development creates a missing file once with mode `0600`;
simultaneous initializations reuse the same file. An existing file is never
replaced, including when its contents are invalid. Production never generates
a missing key.

For an existing database, reuse its original key. Put the previous value in the
ignored local YAML, or clear `master_key` and point `master_key_file` at the
original file. A new key, including the public development default, cannot
decrypt an existing database's signing keys.

Back up the database and key together. Key rotation through `keys-rotate` rotates
OIDC signing keys, not the master encryption key. See [recovery](../operations/recovery.md).

## Global MFA policy

MFA is disabled by default for every account, including Administrator. To use
MFA, manually set `security.mfa_enabled: true` in the selected YAML file or export
`BURROW_MFA_ENABLED=true`. Use `false` to disable it again.
Invalid or empty environment boolean values fail startup.
The environment override takes precedence over YAML. Restart the server after
changing this setting; all processes using the same database must use the same policy.

Disabling MFA skips both setup and verification, including the administrator's
code during MFA reset. Temporary passwords still require a change before full
access; reset still requires Administrator permission and an audit reason.
Existing encrypted bindings and replay counters are preserved. Re-enabling MFA
rejects password-only sessions, their outstanding authorization codes and online
Access Token use. Users log in again to verify an existing binding or set up a new
one. Applications retain their own sessions, and offline ID Tokens keep their expiry.
OIDC `amr` reports `pwd` for password-only sessions and `pwd` plus `otp` for
sessions that completed MFA.

For native commands, use an ignored `configs/config.local.yaml` override:

```yaml
security:
  mfa_enabled: true
```

For Compose, set `BURROW_MFA_ENABLED=true` in `.env` and recreate the application
container with the usual `make compose-up` command. `.env` does not affect native commands.

## Compose

Only Docker Compose reads `.env`. Its interpolation passes settings to app and
migrate and seed as environment variables; containers use embedded defaults for the rest.
The local development YAML is not mounted into containers. Compose always uses
PostgreSQL and receives its master key from `.env`. The example includes the
public development key; replace it before using `BURROW_ENV=prod`.

`make compose-*` accepts `COMPOSE_ENV`, `COMPOSE_PROJECT` and `COMPOSE_PROFILES`.
Defaults are `.env`, `burrow` and `local-db`. Set `COMPOSE_PROFILES=` for an external
database. `make compose-up` uses existing local images; build or pull them first.

## Administrator bootstrap

`make seed` reads the `bootstrap` section. It creates the Administrator role and
the initial administrator without stdin prompts or password output. The username
is `admin`, display name `Administrator`, email empty and development password
`Burrow-development-admin-2026`. Override these settings before first creation:

| YAML field                 | Environment override              |
| -------------------------- | --------------------------------- |
| `bootstrap.admin_username` | `BURROW_BOOTSTRAP_ADMIN_USERNAME` |
| `bootstrap.admin_name`     | `BURROW_BOOTSTRAP_ADMIN_NAME`     |
| `bootstrap.admin_email`    | `BURROW_BOOTSTRAP_ADMIN_EMAIL`    |
| `bootstrap.admin_password` | `BURROW_BOOTSTRAP_ADMIN_PASSWORD` |

Production first-time seed requires an independent 12–256 character password.
Development falls back to the example if the password is empty. These settings
do not change an existing administrator. Keep the original bootstrap username
when upgrading or restoring an existing instance. See [seed](seed.md).

The OIDC `login_ttl` setting applies to OIDC authorization requests. Password/MFA
restricted login transactions expire after a fixed five minutes and never extend
when a temporary password is changed. MFA follows the global policy above.
Preserve the existing master key, which encrypts active and pending
TOTP secrets as well as signing keys. `mfa-reset` loads configuration with the same
precedence as other native commands. See [MFA recovery](../operations/recovery.md#mandatory-mfa-upgrade-and-recovery).
