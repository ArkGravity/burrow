<img src="web/public/logo.svg" alt="Burrow logo" width="64" height="64" />

# Burrow

English · [简体中文](README_zh.md)

A lightweight, single-organization OpenID Connect identity provider: shared SSO
sessions, password authentication with optional TOTP MFA, users, groups, roles, permissions and
application access. The Go backend and React web UI ship as one `burrow` binary.

Burrow helps small teams and operators of self-hosted services manage accounts
and application access in one place. It is inspired by Casdoor and independently
implemented, with a focused OIDC scope and a single-instance deployment.

- Repository: https://github.com/ArkGravity/burrow
- [Configuration](configs/config.yaml) · [Documentation](docs/README.md) · [Project conventions](AGENTS.md) · [OIDC examples](examples/README.md)

The UI supports English and Simplified Chinese, with light, dark and system themes.
PostgreSQL is used in production; SQLite is available for local development.

Burrow uses the [MIT license](LICENSE). [Version `v0.1.3`](https://github.com/ArkGravity/burrow/releases/tag/v0.1.3) is available;
see [release notes](docs/releases/v0.1.3.md), [installation instructions](https://github.com/ArkGravity/burrow/releases/download/v0.1.3/INSTALL.md)
and the [changelog](CHANGELOG.md) for Linux amd64/arm64 binaries, container images and
deployment packages.

## Features

- **Shared SSO:** connect Web and SPA applications with OpenID Connect and PKCE S256.
- **Password and MFA:** TOTP disabled by default; enable it manually for all accounts through configuration, temporary-password changes and administrator recovery.
- **Access management:** users, groups, custom roles, permissions and audited management operations.
- **Application portal:** users see the applications they are allowed to access.
- **Simple deployment:** one binary with an embedded UI, or Docker Compose with PostgreSQL; no Redis or queue.
- **Localized UI:** English and Simplified Chinese, with light, dark and system themes.

For a first installation, use the [v0.1.3 installation guide](https://github.com/ArkGravity/burrow/releases/download/v0.1.3/INSTALL.md).
For downstream integrations, see the [Web and SPA examples](examples/README.md)
and [Grafana, Nightingale and Harbor setup](examples/local-sso/README.md).

## Screenshots

Captured by the maintainer on October 9, 2026 during manual browser acceptance of the [local SSO example](examples/local-sso/README.md), in English and dark mode. The `logic` user's portal shows the assigned Grafana and Nightingale applications.

![Burrow application portal for the logic user, showing Grafana and Nightingale](docs/screenshots/user-app-overview.jpg)

<details>
<summary>Create an application</summary>

![Create a Nightingale Web application in Burrow](docs/screenshots/create-application.jpg)

</details>

<details>
<summary>Configure role permissions</summary>

![Configure a role with Grafana and Nightingale login permissions](docs/screenshots/create-roles.jpg)

</details>

<details>
<summary>Create a group</summary>

![Create a group and assign its shared role in Burrow](docs/screenshots/create-group.jpg)

</details>

<details>
<summary>Create a user</summary>

![Create the logic user with role and group assignments in Burrow](docs/screenshots/create-user.jpg)

</details>

## Layout

- `cmd/burrow/`: server, migrations, administrator bootstrap and key rotation.
- `internal/burrow/`: identity, RBAC, sessions, OIDC OP adapters and database tests.
- `configs/`: default YAML configuration, also embedded in standalone binaries.
- `web/`: React, TypeScript, Ant Design and Vite, built with Bun.
- `examples/`: independent Web and SPA OIDC clients.
- `docs/`: configuration, protocol, operations, verification and design records.
- Repository root: `Makefile`, `Dockerfile` and `docker-compose.yml`.

## Development

Requirements: Go **1.27.1**, a C compiler for the SQLite driver, Bun **1.4.2** and
Make. Docker Compose v2 is needed only for container deployment. Run commands
from the repository root.

```bash
make deps
make migrate
make seed         # default roles and administrator from configuration
make run          # backend on http://localhost:8080
```

In another terminal:

```bash
make web-dev      # Vite on http://localhost:5173
```

Open http://localhost:5173 with development credentials `admin` /
`Burrow-development-admin-2026`, then change the password and bind your authenticator when prompted.
Customize `bootstrap.admin_username`, `admin_name`, `admin_email` and
`admin_password` in your ignored local YAML before seeding a new database.
`make seed` is noninteractive and never prints a password. Re-running it preserves
existing administrator credentials, account state and user assignments.

Native commands read `configs/config.yaml` directly. No `.env` file, shell
sourcing or exported variables are needed. The YAML and `.env.example` share a
public development master key. Production rejects this value; replace it with
an independent key generated by `openssl rand -base64 32`.

To use a persistent local key file instead, set `security.master_key: ""` in your
local YAML. Development then creates `data/master.key` once with mode `0600`.
For an existing database, keep its original master key: changing to the new
default cannot decrypt previously stored signing keys.

Use an ignored configuration file for machine-specific settings:

```bash
cp configs/config.yaml configs/config.local.yaml
# Edit the local file, for example database.driver and database.dsn.
make migrate CONFIG=configs/config.local.yaml
make run CONFIG=configs/config.local.yaml
```

Configuration precedence is embedded defaults, the selected YAML file, then
`BURROW_*` environment overrides. An explicit missing config file is an error;
without `--config`, a standalone binary falls back to embedded defaults if
`configs/config.yaml` is absent. Relative paths resolve from the working
directory. Native commands never read `.env`; it is reserved for Docker Compose.
See the [configuration reference](docs/development/configuration.md).

Common commands:

```bash
make help         # list all targets
make build        # frontend + bin/burrow with the UI embedded
make test         # Go tests with race detection
make lint         # go vet
make web-check    # frontend typecheck and unit tests
make check        # backend and frontend checks
make fmt          # gofmt and your locally installed Prettier
make browser-install
make test-e2e     # Chromium and independent Web/SPA OIDC clients
```

`make test-db` requires `BURROW_TEST_POSTGRES_DSN` to point to a dedicated test
database with schema creation permission. Without it, PostgreSQL subtests are
skipped by `make test`. Browser tests use temporary SQLite data and local ports
18080, 19001 and 19002.

## Single binary

```bash
make build
./bin/burrow migrate --config configs/config.yaml
./bin/burrow seed --config configs/config.yaml
./bin/burrow serve --config configs/config.yaml
./bin/burrow version
```

Open http://localhost:8080. The binary serves the UI, management API and OIDC
endpoints on one port. Use this mode for end-to-end OIDC integration; Vite is for
UI development. Seed can be repeated to add missing default roles while preserving
existing administrator credentials.

`/healthz` reports process liveness; `/readyz` checks database migration state.
Startup also verifies that signing keys can be decrypted. Signing keys and
sessions persist across restarts. Rotate signing keys with `make keys-rotate`;
historical public keys remain available for existing tokens.

## Containers and deployment

Compose uses the root `docker-compose.yml` and reads `.env`. Copy the example,
set `POSTGRES_PASSWORD` and the matching password in `BURROW_DB_DSN`. The example
master key works for development; replace `BURROW_MASTER_KEY` for production:

```bash
cp .env.example .env
openssl rand -base64 32  # replace the example master key for production
openssl rand -hex 24     # PostgreSQL password
make compose-config
make compose-build
make compose-up
make compose-seed       # optional repeat; existing credentials are preserved
```

The default `local-db` profile runs PostgreSQL 17. The migration service retries
database connections during startup, then seed initializes the default roles
and administrator. The app starts only after both one-shot services succeed. Open http://localhost:8080. The application binds to host loopback by
default; PostgreSQL is not published to the host.

For production, set `BURROW_ENV=prod`, an HTTPS `BURROW_ISSUER`, an independent
`BURROW_BOOTSTRAP_ADMIN_PASSWORD` for initial seeding and a PostgreSQL DSN with
appropriate TLS settings. Production seed rejects the public example password. Place the app behind your TLS reverse proxy,
preserve the browser Origin, and configure trusted proxy CIDRs for forwarded
client addresses. Burrow runs as a single instance and needs no Redis or queue.

Use separate, untracked `.env.dev` and `.env.prod` files and project names to
isolate deployments. Set `BURROW_IMAGE` to a fixed image tag or digest, such as
`ghcr.io/arkgravity/burrow:main-<short-sha>` or
`docker.io/logic3579/burrow:main-<short-sha>` (the default Docker Hub namespace).
Version releases use `vX.Y.Z` tags in both registries and include a standalone
Compose deployment archive. Current workflows build Linux amd64 and arm64
images under the same tag and provide separate binaries for both architectures;
the published `v0.1.0` remains amd64-only. Use the [release installation guide](docs/releases/INSTALL.md)
for deployment without a source checkout; development images remain available.
For an external PostgreSQL database, omit the local profile:

```bash
docker compose --env-file .env.prod --project-name burrow-prod pull app migrate seed
make compose-up COMPOSE_ENV=.env.prod COMPOSE_PROJECT=burrow-prod COMPOSE_PROFILES=
```

`make compose-down` preserves database volumes. Preserve the database, master key
and Compose project name across upgrades. Read the [backup and recovery guide](docs/operations/recovery.md)
before migrating or rotating keys. CI publishes images from `main` after all
checks pass and the registry credentials are configured; see the
[CI and image publishing guide](docs/development/ci.md).

## OIDC and access management

1. Create users, groups and ordinary roles.
2. Register a Web or SPA application with exact callback URLs and a login URL.
3. Assign its `app:<id>:login` permission to a role, then assign the role directly
   to users or through groups.
4. Users see authorized applications in their portal. Each application starts
   its own OIDC authorization request.

Management APIs and OIDC authorization enforce current permissions server-side.
Authorization-code exchange checks access again. The last enabled
administrator with a password is protected. Burrow manages identity and application access;
applications manage their own business permissions.

Administrator is the only seeded built-in role. Other roles are custom permission
sets assigned to users or groups. New users have no roles by default; authenticated
users can still access their profile and portal. Application login requires an
explicit grant through a custom role, unless the user is an administrator.

Application permissions display the current application name and Client ID in
lists and role selectors, while retaining stable `app:<application ID>:login`
identifiers. Migration 003 removes empty Viewer assignments and unused legacy
Editor/Viewer roles; assigned legacy roles with permissions become custom roles
without changing their grants. Seed does not recreate Editor or Viewer. The Users
list shows group names so administrators can inspect membership directly.
See [seed and default roles](docs/development/seed.md).

| Capability            | Supported behavior                                                                 |
| --------------------- | ---------------------------------------------------------------------------------- |
| Discovery             | `<issuer>/.well-known/openid-configuration`                                        |
| Flow                  | Authorization Code + PKCE S256 by default; administrator-controlled Web exceptions |
| Client authentication | Web: `client_secret_basic`; SPA: `none`                                            |
| Scopes                | `openid profile email`                                                             |
| ID Token signing      | RS256, public keys through JWKS                                                    |
| Endpoints             | `/oidc/authorize`, `/oidc/token`, `/oidc/userinfo`, `/oidc/jwks`, `/oidc/logout`   |
| Sessions              | Shared Burrow SSO; APP sessions remain under APP control                           |

Administrators may enable **Allow login without PKCE** for an individual Web
application (`allowWithoutPkce`, default `false`). SPA clients always require
S256; supplied PKCE is always validated. This compatibility setting reduces
authorization code protection. Disabling it blocks outstanding codes issued
without PKCE, while existing tokens and APP sessions retain their normal
lifecycle. Run `make migrate` before starting an upgraded server.

Access Tokens are for Burrow UserInfo. Refresh Tokens, implicit flow, machine
clients, dynamic registration and cross-application logout are outside the
first release. Clients must verify state, nonce, signature, issuer, audience and
expiry. Register exact SPA origins for browser access.

Burrow authenticates users with its own passwords; upstream Providers and external
identity linking have been removed. MFA is disabled by default and requires manual activation. When enabled, all users must complete TOTP. New users require a temporary password and
must change it before full access. Schema v4 removes the old Provider data and
settings. Before upgrading, use the previous version to configure passwords and
enable local login for active users and applications, or disable unused records.
Migration refuses incompatible active records and rolls back rather than silently
changing access. See [Provider-removal upgrade](docs/operations/recovery.md#upgrade-to-password-only-authentication) and [MFA upgrade and recovery](docs/operations/recovery.md#mandatory-mfa-upgrade-and-recovery).
See the [adapter notes](docs/development/oidc-adapter.md) and [client examples](examples/README.md).

## Documentation and CI

[docs/README.md](docs/README.md) indexes configuration, dependencies, operations,
protocol verification and historical design documents.

[CI](.github/workflows/ci.yml) runs Go race tests against SQLite and PostgreSQL 17,
static analysis, frontend checks and builds, Chromium flows, Compose configuration
validation and container builds for PRs, `dev` pushes and manual runs. Backend,
browser and image checks run natively on Linux amd64 and arm64. Main pushes reuse
successful PR regression only when the recorded tested Git tree matches exactly,
and fall back to full regression otherwise. Main always builds and smoke-tests
both production images before publishing
multi-platform images to GHCR and Docker Hub without rebuilding. See [CI setup](docs/development/ci.md) for
triggers, image tags and registry credentials.
Local verification scope and limitations are recorded in the
[verification report](docs/testing/oidc-conformance.md). Engineering interoperability
tests are not an OpenID Foundation certification.

Version tags additionally run the [release workflow](.github/workflows/release.yml)
to prepare tested amd64/arm64 binaries, a shared deployment archive, checksums and versioned multi-platform images
in a Release draft. See [release preparation and publication](docs/development/releases.md).

With MFA enabled, setup is mandatory at first login and after an Administrator resets MFA.
Password login alone creates only a five-minute restricted transaction. Temporary
password changes and authenticator verification must finish before portal, management
or OIDC access. Administrators reset MFA with their own unused code and an audit reason;
this preserves the target password and revokes Burrow sessions/tokens. Password reset
preserves the existing MFA binding. Operators can recover a lost administrator authenticator
with `burrow mfa-reset --username admin --reason "Lost authenticator"` using the existing
configuration and master key. Details: [MFA authentication and recovery](docs/development/mfa-proposal.md).

**MFA is disabled by default** (`security.mfa_enabled: false` and
`BURROW_MFA_ENABLED=false`). To use MFA, manually enable it in the selected YAML file:

```yaml
security:
  mfa_enabled: true
```

Alternatively, set `BURROW_MFA_ENABLED=true` in the service environment (Compose:
`.env`). Restart the server or recreate the Compose application container after
changing this setting. Enabling MFA applies to every account, including Administrator.
Temporary passwords always require a change, and disabling MFA preserves existing
bindings. Enabling MFA requires users with password-only sessions to log in again
and complete MFA. See
[global MFA policy](docs/development/configuration.md#global-mfa-policy). This switch
was introduced in v0.1.2; historical v0.1.0/v0.1.1 artifacts require MFA.
