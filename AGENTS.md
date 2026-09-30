# Project conventions

These instructions apply to the entire repository. Follow the current code and
executable documentation when historical design records disagree with them.

## Product scope

Burrow is an independently implemented, lightweight, single-organization OIDC
identity provider inspired by Casdoor. Keep the implementation focused on shared
SSO, personal profiles, application portals, users, groups, roles, permissions,
application configuration and upstream OIDC providers.

- Preserve the single-instance modular monolith. Avoid additional middleware,
  Redis, queues or deployment components without a concrete requirement.
- Support English and Simplified Chinese, and light, dark and system themes.
- Use PostgreSQL in production and support SQLite for development and tests.
- Keep deployment configuration in the root `docker-compose.yml`.
- Do not add protocols, grants or account-provisioning features beyond the agreed
  OIDC scope unless explicitly requested.

## Architecture and source ownership

The backend uses Go, Chi, GORM and `github.com/zitadel/oidc/v3`. The frontend uses
Bun, React, TypeScript, Ant Design and Vite. Use the versions declared in
`go.mod`, package manifests, lockfiles and CI; update related declarations
together when changing toolchains or dependencies.

The Go module is currently `github.com/logic3579/burrow`, although the repository
is hosted at `ArkGravity/burrow`. Preserve existing imports unless a module
migration is explicitly part of the task.

| Location                                          | Responsibility                                                        |
| ------------------------------------------------- | --------------------------------------------------------------------- |
| `cmd/burrow/main.go`                              | `serve`, `migrate`, `seed`, `keys-rotate` and `healthcheck` commands  |
| `configs/config.yaml`, `configs/embed.go`         | Tracked development defaults and embedded configuration               |
| `internal/burrow/config.go`                       | YAML loading, environment overrides and validation                    |
| `internal/burrow/core.go`, `models.go`            | Database lifecycle, identity models, password handling and RBAC       |
| `internal/burrow/seed.go`                         | Idempotent default-role and administrator initialization              |
| `internal/burrow/admin.go`, `mutation.go`         | Management APIs, transactional authorization and auditing             |
| `internal/burrow/http.go`                         | Authentication, sessions, personal resources, portal and HTTP routing |
| `internal/burrow/oidc_storage.go`, `oidc_http.go` | Downstream OIDC storage and protocol handlers                         |
| `internal/burrow/upstream.go`                     | Upstream OIDC transactions and linked identities                      |
| `internal/burrow/network.go`, `cors.go`           | Proxy trust, Provider network restrictions and browser origins        |
| `internal/burrow/migrations/`                     | Explicit, checksummed SQL migrations                                  |
| `web/src/main.tsx`, `pages/`                      | UI layout, authentication, home and resource management               |
| `web/src/lib/`, `components/`                     | API/session/access helpers, translations and shared components        |
| `web/e2e/`, `examples/`                           | Browser regression tests and independent Web/SPA OIDC clients         |
| `docs/`, `.github/workflows/ci.yml`               | References, operational instructions and CI                           |

Keep backend changes in the existing package, split by responsibility. Do not
introduce package layers merely to reorganize files. Production builds embed the
frontend with `-tags embedweb`; build `web/dist` first, normally through
`make build`.

## Authentication and authorization

- Authenticate against the current enabled user and valid shared session.
  Enforce management permissions on the server; UI visibility is supplementary.
- Recheck user, application and authentication-source access during both OIDC
  authorization and authorization-code exchange.
- Effective permissions combine direct user roles and group roles. There are no
  nested groups or inherited roles. The Administrator role ID grants administrator
  access; the `Builtin` flag alone must never grant that privilege.
- Protect the last enabled local administrator. Require `authorization:write`
  for explicit authorization relationship changes. `users:write` alone must not
  enable promotion, administrator password reset or administrator identity linking.
- Preserve transactional mutation handling: serialize decisions and writes,
  revalidate the actor's current permissions within the transaction, and commit
  the audit record with the mutation. Audit failure must roll back the mutation.
- Keep session cookies host-only, HttpOnly and SameSite=Lax, with Secure enabled
  in production. Preserve CSRF and Origin checks on unsafe browser requests.
- Use the existing Argon2id password handling and bounded parameters. Seeded or
  reset temporary passwords require a password change before full access.
- Never log passwords, client secrets, tokens, encryption keys or full secret
  configuration. Use quiet Compose validation to avoid printing resolved secrets.

## OIDC boundaries

- Continue using `zitadel/oidc/v3` through the existing OP/RP adapters.
- Support Authorization Code with mandatory PKCE S256. Web clients use
  `client_secret_basic`; SPA clients use `none`.
- Preserve exact callback, logout and allowed-origin matching. Do not introduce
  wildcard redirects or permissive CORS.
- Preserve browser-bound state, nonce and PKCE transactions, expiry checks and
  atomic one-time authorization-code consumption.
- Upstream identities require a pre-linked Provider, issuer and subject. Do not
  provision users automatically or match accounts by email. Preserve fresh
  upstream authentication and `auth_time` validation.
- Restrict Provider network access and trust forwarded addresses only from
  configured proxy CIDRs. Do not globally allow private Provider networks in
  production.
- Keep downstream client secrets hashed and Provider secrets/private signing keys
  encrypted with the existing master key. Preserve signing keys across restarts
  and historical public keys during rotation.
- Burrow logout revokes its shared session. Applications own their sessions;
  already issued offline-verifiable ID Tokens have their own expiry.
- Refresh Tokens, implicit/password grants, machine clients, dynamic registration
  and cross-application logout are outside the current implementation.

## Seed and default roles

`make seed` is explicit, noninteractive and idempotent. Run migration first. It
adds missing built-in roles and creates the configured administrator only in an
empty user database. Repeated execution must preserve existing passwords,
profiles, account states and user assignments. Reject reserved-role conflicts
and an existing bootstrap username that is not already an administrator.

- **Administrator:** all management and application access, subject to account,
  application and authentication-source restrictions.
- **Editor:** read management resources and maintain applications/Providers;
  no user management, password reset, identity linking or authorization changes.
- **Viewer:** personal resources and the authorized application portal only;
  no management permissions and no implicit application-login grants.

Built-in roles are immutable. Application-login permissions belong to ordinary
roles and can reach users directly or through groups. Do not attach blanket APP
permissions to the built-in roles.

When a new user's `roleIds` is omitted, assign Viewer and preselect it in the
creation form. Respect explicit roles, including an empty list. Do not backfill
existing users or reassign Viewer when editing a user. Additional explicitly
assigned roles may grant more permissions through the normal union.

The Users list returns group IDs and compact group references using batched
queries. Preserve this capability under `users:read` without requiring access to
the full Groups API. Avoid per-user membership queries.

## Configuration and persistence

- Configuration precedence is embedded YAML defaults, the selected YAML file,
  then exported `BURROW_*` environment overrides.
- Native commands never load `.env`; it is only for Docker Compose. Local
  development must work from YAML defaults without manual environment exports.
- Use ignored `configs/config.local.yaml` for machine-specific configuration.
  Explicit missing configuration paths are errors; relative paths resolve from
  the working directory.
- Tracked configuration and `.env.example` contain public development examples,
  not deployment credentials. Production rejects the example master key and
  initial administrator password. Never commit real credentials or local data.
- Preserve the original master key for an existing database. Key-file fallback
  applies only when the inline key is empty; do not overwrite existing key files.
- Coordinate config changes across YAML defaults, parsing/validation, CLI,
  `.env.example`, Compose, tests and documentation as applicable.
- Use explicit migrations, not runtime AutoMigrate. Never edit an applied
  checksummed migration. New versions also require corresponding migrator support.
- Startup order is migration, seed, then server. Keep readiness and signing-key
  validation meaningful. Compose shutdown must preserve database volumes.

## Development workflow

Run commands from the repository root. Use RTK for agent shell commands when it
is available, following any host-provided RTK instructions. For unsupported
commands use `rtk proxy`, for example `rtk proxy make web-check`. In environments
without RTK, use the corresponding ordinary commands.

```bash
make deps
make migrate
make seed
make run       # backend: localhost:8080
make web-dev   # separate terminal; frontend: localhost:5173
```

Use `CONFIG=configs/config.local.yaml` consistently for migration, seed and run
when using local overrides. Inspect `make help` for available actions.

- Use Bun with frozen lockfiles for frontend and SPA example dependencies. Do not
  introduce npm, Yarn or pnpm lockfiles.
- Use `gofmt` for Go and the locally installed Prettier for frontend/docs/config
  formatting. Do not install Prettier when the local tool is already available.
- Put user-facing translations in both language dictionaries in
  `web/src/lib/i18n.tsx`. Keep language keys aligned and preserve theme support.
- Reuse existing API, session and access helpers. Keep security decisions in the
  backend and do not expose implementation details in product flows.
- Keep fixes scoped, preserve unrelated user edits, and synchronize README,
  configuration references and operational guides when behavior changes.

## Codex and project memory

- Track project MCP settings in `.codex/config.toml`; keep other `.codex/`
  local state ignored. Supply `MEM0_API_KEY` and `CONTEXT7_API_KEY` through the
  environment, never as literal secrets in tracked configuration.
- Mem0's platform Project is `ArkGravity`, shared by the Burrow and Optimus
  repositories. Distinguish repository memories with `metadata.project`:
  `burrow` for this repository and `optimus` for Optimus. Do not create a separate
  platform Project or rename these metadata values to `ArkGravity`.
- Write Burrow memories with `user_id=logic`, `app_id=burrow` and
  `metadata.project=burrow`. Explicitly filter reads and searches by
  `user_id=logic` and `metadata.project=burrow`; optionally also filter by
  `app_id=burrow`. Do not rely on the MCP server's default user scope.
- Recall relevant Burrow memories before substantive project work. Treat current
  source, executable documentation and user instructions as authoritative when
  stored memories disagree. Save durable decisions, conventions and verified
  checkpoints after completing work; update existing topic memories when useful
  instead of creating duplicates. Never store credentials or local secret data.
- The project Mem0 server uses `default_tools_approval_mode = "approve"`, a
  15-second startup timeout and a 60-second tool timeout. This enables automatic
  tool approval within the user's authorized task; it does not authorize unrelated
  changes or deletion of other projects' memories.
- Report failed memory operations accurately. Do not claim a checkpoint was
  saved until the operation succeeds, and verify important writes by reading them
  back. Keep historical test results distinct from checks run in the current task.

## Verification and delivery

| Command                                        | Coverage                                                |
| ---------------------------------------------- | ------------------------------------------------------- |
| `make lint`                                    | Go static analysis                                      |
| `make test`                                    | Go race tests; PostgreSQL tests skip without a test DSN |
| `make test-db`                                 | Go race tests with required `BURROW_TEST_POSTGRES_DSN`  |
| `make web-check`                               | Frontend typecheck and unit tests                       |
| `make check`                                   | Backend and frontend checks                             |
| `make browser-install`, `make test-e2e`        | Chromium flows and independent OIDC clients             |
| `make build`                                   | Frontend and embedded production binary                 |
| `make compose-config COMPOSE_ENV=.env.example` | Quiet deployment configuration validation               |

Use a dedicated PostgreSQL test database with schema creation permission; never
point tests at production. Browser tests use temporary SQLite data and ports
18080, 19001 and 19002. Preserve temporary-resource cleanup.

Choose checks appropriate to the change. Authorization, seed, migration and
protocol changes need relevant backend coverage on both database drivers;
browser/session changes need applicable end-to-end flows. Verify concurrent
consumption, revocation and privilege boundaries when those behaviors change.
For documentation-only changes, check formatting, links and `git diff --check`;
do not repeat an unchanged, already verified full suite without a reason.

Report the checks actually run and material limitations. Local engineering
interoperability tests do not imply official OIDC certification or successful
remote CI. Do not claim a container build or deployment that was not performed.
Commit and push only when requested, using scoped staging and descriptive commit
messages. Never force-push or discard unrelated changes without authorization.

## References

- [README and common commands](README.md)
- [Documentation index](docs/README.md)
- [Configuration reference](docs/development/configuration.md)
- [Seed and role boundaries](docs/development/seed.md)
- [OIDC adapter boundaries](docs/development/oidc-adapter.md)
- [Backup, recovery and key rotation](docs/operations/recovery.md)
- [Verification report](docs/testing/oidc-conformance.md)
