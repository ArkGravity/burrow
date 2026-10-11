# Local development

For running a published release, use the [installation guide](../installation.md).
This guide covers working on Burrow from a source checkout.

## Requirements and checkout

Use Go **1.27.1**, Bun **1.4.2**, Make and a C compiler for the SQLite CGO driver.
Versions are declared in [.tool-versions](../../.tool-versions),
[go.mod](../../go.mod) and [web/package.json](../../web/package.json).
Use Bun with the existing frozen lockfiles. Docker Engine and Compose v2 are
needed only for container work.

```bash
git clone https://github.com/ArkGravity/burrow.git
cd burrow
make deps
make migrate
make seed
make run       # backend: http://localhost:8080
```

In a second terminal at the repository root:

```bash
make web-dev   # frontend: http://localhost:5173
```

Open http://localhost:5173. Development defaults use SQLite and the public example
credentials `admin` / `Burrow-development-admin-2026`. Change the temporary password
on first login. To customize the bootstrap administrator, edit your local YAML
before seeding a new database. Re-running seed does not reset existing accounts,
passwords or assignments; see [seed boundaries](seed.md).

## Local configuration and MFA

Native commands read YAML and never load `.env`. Configuration precedence is
embedded defaults, the selected YAML file, then exported `BURROW_*` environment
variables. Relative paths resolve from the working directory.

For machine-specific settings, copy the tracked defaults to the ignored local file
and consistently use it for migration, seed and run:

```bash
cp configs/config.yaml configs/config.local.yaml
# Edit the local YAML before using it.
make migrate CONFIG=configs/config.local.yaml
make seed CONFIG=configs/config.local.yaml
make run CONFIG=configs/config.local.yaml
```

MFA is disabled by default. To require TOTP for all accounts, add this to your
selected local YAML, then restart the server:

```yaml
security:
  mfa_enabled: true
```

An exported `BURROW_MFA_ENABLED=true` also enables the policy. Temporary passwords
always require a change, and enabling MFA requires password-only sessions to log
in again. See [configuration and MFA policy](configuration.md).

The tracked master key and administrator password are public development examples;
production rejects them. Keep the original master key when using an existing
database. Development can create a persistent key file when `security.master_key`
is empty; see [master key lifecycle](configuration.md#master-key-lifecycle).

## Embedded build and OIDC integration

Vite serves frontend development. To run the UI, management API and OIDC endpoints
on one port, build the embedded binary:

```bash
make build
./bin/burrow migrate --config configs/config.yaml
./bin/burrow seed --config configs/config.yaml
./bin/burrow serve --config configs/config.yaml
```

Open http://localhost:8080. When using local overrides, pass
`--config configs/config.local.yaml` to all three commands.
`/healthz` reports liveness, while `/readyz` checks database migration state.
Startup verifies signing-key decryption. Signing keys and sessions persist across
restarts. See [Web / SPA clients](../../examples/README.md) for OIDC integration.

## Checks and formatting

Choose checks for the behavior you changed:

| Command                                        | Coverage                                                         |
| ---------------------------------------------- | ---------------------------------------------------------------- |
| `make lint`                                    | Go static analysis                                               |
| `make test`                                    | Go race tests; PostgreSQL subtests skip without a test DSN       |
| `make test-db`                                 | Go race tests with a required dedicated PostgreSQL test database |
| `make web-check`                               | Frontend typecheck and unit tests                                |
| `make check`                                   | Backend and frontend checks                                      |
| `make browser-install`, then `make test-e2e`   | Chromium flows and independent Web / SPA OIDC clients            |
| `make build`                                   | Frontend and embedded production binary                          |
| `make compose-config COMPOSE_ENV=.env.example` | Quiet Compose configuration validation                           |

Set `BURROW_TEST_POSTGRES_DSN` to a dedicated database with schema creation
permission for `make test-db`; never use a production database. Browser tests
create temporary SQLite data and use ports 18080, 19001 and 19002.

Use `make fmt` for Go and frontend formatting with locally installed Prettier;
format changed Markdown files with that same Prettier. Documentation-only changes
need formatting, link checks and `git diff --check`. Run `make help` for all targets.
See [Contributing](../../CONTRIBUTING.md) for review expectations.

## Compose from source

Use the canonical root Compose file. Copy `.env.example` to an untracked `.env`,
set a PostgreSQL password and the matching password in `BURROW_DB_DSN`, then run:

```bash
make compose-config
make compose-build
make compose-up
```

This uses the local image and development environment unless you explicitly change
`.env`. Production configuration and fixed release images are covered in the
[installation guide](../installation.md). `make compose-down` preserves volumes.
Compose isolation and external-database settings are in [configuration](configuration.md#compose).

## Further reading

- [Project conventions](../../AGENTS.md)
- [Dependencies and implementation layout](dependencies.md)
- [CI and image publishing](ci.md)
- [Release preparation](releases.md)
- [Backup, upgrades and key rotation](../operations/recovery.md)
