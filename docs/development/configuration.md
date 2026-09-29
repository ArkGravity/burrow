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

| YAML field                 | Environment override             | Default                                            |
| -------------------------- | -------------------------------- | -------------------------------------------------- |
| `env`                      | `BURROW_ENV`                     | `dev`                                              |
| `server.listen_addr`       | `BURROW_LISTEN_ADDR`             | `:8080`                                            |
| `server.issuer`            | `BURROW_ISSUER`                  | `http://localhost:8080`                            |
| `server.static_dir`        | `BURROW_STATIC_DIR`              | `web/dist` (non-embedded builds)                   |
| `server.trusted_proxies`   | `BURROW_TRUSTED_PROXIES`         | Empty; trust no forwarded client IPs               |
| `database.driver`          | `BURROW_DB_DRIVER`               | `sqlite`                                           |
| `database.dsn`             | `BURROW_DB_DSN`                  | `burrow.db`                                        |
| `security.master_key`      | `BURROW_MASTER_KEY`              | Public development example (production rejects it) |
| `security.master_key_file` | `BURROW_MASTER_KEY_FILE`         | `data/master.key`                                  |
| `session.ttl`              | `BURROW_SESSION_TTL`             | `8h`                                               |
| `oidc.token_ttl`           | `BURROW_TOKEN_TTL`               | `5m`                                               |
| `oidc.auth_code_ttl`       | `BURROW_AUTH_CODE_TTL`           | `60s`                                              |
| `oidc.login_ttl`           | `BURROW_LOGIN_TTL`               | `10m`                                              |
| `providers.allowed_cidrs`  | `BURROW_PROVIDER_ALLOWED_CIDRS`  | Empty; private addresses denied                    |
| `providers.allow_private`  | `BURROW_ALLOW_PRIVATE_PROVIDERS` | `false`; development-only network bypass           |
| `audit.retention`          | `BURROW_EVENT_RETENTION`         | `2160h` (90 days)                                  |

CIDR lists are YAML sequences or comma-separated environment values. Production
requires PostgreSQL and an HTTPS issuer without a path, query or credentials.
Use explicit Provider CIDRs for internal upstream servers instead of a broad
development bypass.

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
decrypt an existing database's signing keys or Provider credentials.

Back up the database and key together. Key rotation through `keys-rotate` rotates
OIDC signing keys, not the master encryption key. See [recovery](../operations/recovery.md).

## Compose

Only Docker Compose reads `.env`. Its interpolation passes settings to app and
migrate as environment variables; containers use embedded defaults for the rest.
The local development YAML is not mounted into containers. Compose always uses
PostgreSQL and receives its master key from `.env`. The example includes the
public development key; replace it before using `BURROW_ENV=prod`.

`make compose-*` accepts `COMPOSE_ENV`, `COMPOSE_PROJECT` and `COMPOSE_PROFILES`.
Defaults are `.env`, `burrow` and `local-db`. Set `COMPOSE_PROFILES=` for an external
database. `make compose-up` uses existing local images; build or pull them first.
