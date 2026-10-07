# 协议与回归验证记录

## 2026-10-07：v0.1.1 双架构正式发布

- 按用户要求提交并推送双架构实现 `abac1be` 和版本准备 `b2fe059`，在精确提交 `b2fe05991d284504b90075f333142cd813300935` 的[完整 main CI](https://github.com/ArkGravity/burrow/actions/runs/37595876423) 成功后创建并推送 annotated tag `v0.1.1`。该 tag 后续保持不变，文档状态同步使用独立提交。
- CI 的原生 amd64/arm64 SQLite/PostgreSQL race tests、Chromium/MFA/OIDC 回归、镜像构建与静默 Compose 校验均通过；前端检查/构建、Go 静态分析和 29 项发布脚本回归也通过。保存的镜像成功发布到 GHCR 和 Docker Hub，双架构索引及各架构 digest 一致。
- [v0.1.1 Release workflow](https://github.com/ArkGravity/burrow/actions/runs/37596734846) 的 validate、两个原生 prepare 和 draft 全部通过。两个架构均从各自镜像提取真实二进制，验证版本/commit、打包后运行 SQLite/MFA/OIDC 浏览器回归，并对同一容器执行 PostgreSQL 生产模式迁移、重复 seed、启动、readiness、healthcheck 与内嵌前端检查。发布复用保存的镜像，不重新构建。
- 六个 Release 附件全部下载复核，GitHub API 提供的附件 digest 和本地 SHA256 一致；`SHA256SUMS` 覆盖其余五个附件。两个二进制归档的 ELF 架构、执行位、许可证、配置与版本化安装说明正确；Go 元信息确认 Go 1.27.1、Linux、对应 GOARCH、CGO 和 embedweb。部署包保留原封不动的根 Compose，镜像行固定为 `ghcr.io/arkgravity/burrow:v0.1.1`。
- 下载程序 SHA256：amd64 为 `b241dfc94abf6b93669a3176e3ee489808d57864e6276b779cc66d822c23a185`，arm64 为 `5798a7f77d0e7560a1710307bfa5d2efd7e1ff99dbe3458aaed30db03408852b`。本机只检查 ELF 和构建元信息，没有执行这些 Linux 程序；它们的实际运行测试由上述原生 Release jobs 完成。
- 两个仓库的 `v0.1.1` manifest 均复核为恰好包含 `linux/amd64` 和 `linux/arm64`，共同 index digest 为 `sha256:23604067892397b5a75469f8c6caf7da8987d98cb2778acfd2f032a02fe0ccb6`；amd64 manifest 为 `sha256:db118831c5de30a07a593cac13f55b715f40b7514880e884c6ea5a0a28857f3c`，arm64 manifest 为 `sha256:edd1721e9b66aba49978a94656951264765f771c22e2333d196c0873b18c49d6`。Workflow 的匿名索引访问检查通过。
- [Burrow v0.1.1](https://github.com/ArkGravity/burrow/releases/tag/v0.1.1) 于 `2026-10-07T09:02:21Z` 正式发布。GitHub API 确认 `draft=false`、`prerelease=false`、Latest 为 `v0.1.1`，六个附件均为 uploaded。认证、权限和 schema v5 未改变，旧 v0.1.0 发布产物未替换。
- 上述结果是本轮远端工程验证与附件复核，不代表生产部署、重新验收 Grafana/Nightingale/Harbor 或官方 OIDC 认证。此前用户报告的三应用与 MFA 人工验收仍为独立历史检查点。

## 2026-10-02：v0.1.0 正式发布

- 用户明确要求正式发布后，[Burrow v0.1.0](https://github.com/ArkGravity/burrow/releases/tag/v0.1.0) 于 `2026-10-02T14:53:59Z` 发布。GitHub API 确认 `draft=false`、`prerelease=false`，Latest Release 为 `v0.1.0`，五个附件均为 uploaded。版本 tag 指向 `523ff526043b7bc430c88baf53a396f2861595f9`；后续文档状态同步不移动该 tag。
- 发布准备 [PR #4](https://github.com/ArkGravity/burrow/pull/4) 在[完整 PR CI](https://github.com/ArkGravity/burrow/actions/runs/37018008591) 通过后 squash 合并；该提交的 [main CI](https://github.com/ArkGravity/burrow/actions/runs/37018533674) 全部通过，包括前端检查/构建、Go 静态检查、17 项发布门禁/打包测试、SQLite/PostgreSQL race、Chromium、Linux amd64 容器构建、静默 Compose 校验及双仓库镜像发布。
- [v0.1.0 Release 工作流](https://github.com/ArkGravity/burrow/actions/runs/37020493982) 的 validate、prepare、draft 全部通过。对镜像中提取并打包的真实 Linux amd64 二进制运行四条 Chromium/MFA/OIDC 流程；独立 PostgreSQL 17 验证生产模式镜像的迁移、重复 seed、启动、readiness、healthcheck 与内嵌前端。这些临时工程验证不代表生产部署或官方 OIDC 认证；此前三应用/MFA 人工验收保持独立。
- 五个附件（Linux 二进制归档、Compose 部署归档、`INSTALL.md`、`IMAGES.txt`、`SHA256SUMS`）全部下载复核，校验和、归档清单、MIT、配置样例、原封不动根 Compose 和固定镜像行均正确。下载二进制在容器内实际执行 `version`，返回 `v0.1.0`、上述提交和 `2026-10-02T16:14:55+02:00` 构建时间；Go 元信息确认 Linux/amd64、CGO 与 embedweb。镜像程序与下载包程序 SHA256 均为 `2b4140d784a53f4fc932ff279aaa9357acf2c401af1742a80ae7225d239ba1cb`。
- `ghcr.io/arkgravity/burrow:v0.1.0` 与 `docker.io/logic3579/burrow:v0.1.0` 的匿名 manifest 请求均 HTTP 200，共同 digest 为 `sha256:73b84e43a4ac697efdb5326b7c2eb1309cb02f58bd05a2ad050e7e8daa968d02`。本机实际拉取 GHCR 版本镜像并运行下载程序成功，临时容器已清理；没有修改真实数据库、包可见性或部署生产。
- 正式发布确认回合核查 Release/Latest 状态、附件与文档；上面的 Go、浏览器和容器结果来自同一版本提交已成功执行的工作流，不重复未变化的产品套件。

## 2026-10-02：v0.1.0 发布准备（本地实现阶段）

- 用户确认采用 MIT。新增 `burrow version`、构建版本注入、固定版本发布工作流、安装/升级说明、部署归档和校验文件；认证逻辑与迁移 001–005 未修改。
- 发布门禁及打包共 17 项本地测试通过，覆盖错误 tag、未合并提交、错误提交/分支/事件的 CI、失败/缺失 CI、API 失败、重复发布、草稿重试、缺失说明、归档文件清单、许可证、固定镜像和 SHA256。打包测试使用 ELF 头部夹具，只验证打包行为，不代表真实 Linux 二进制已运行。
- `make build VERSION=v0.1.0` 与 `make lint` 通过；本机产物为 macOS arm64。版本命令在错误配置/环境下仍可读取构建信息，非法参数被拒绝。通过 `BURROW_E2E_BINARY` 指定此二进制，四条 Chromium 回归全部通过，覆盖用户/权限/双语主题、Web/SPA SSO、单 Web PKCE 兼容、MFA 过期/重置及 CLI 恢复；使用临时 SQLite，未修改真实数据库。
- actionlint 1.7.12、Shell 语法、Prettier、55 个本地文档链接、静默根 Compose 配置与 diff 检查通过；浏览器临时服务已退出，测试端口与 Docker 无遗留容器。本轮未重跑完整双数据库 race 套件；发布工作流要求最终提交的完整 main CI，并另验证打包后的 Linux 二进制与生产模式 PostgreSQL 容器启动。
- 本地实现阶段尝试 Linux amd64 镜像构建，但 Docker Hub 的 Bun、Go、Debian 基础镜像元数据访问超时，构建未完成。当时尚未提交/推送、创建版本 tag、执行远端发布工作流、上传附件或验证新版本镜像公开拉取，没有发布 Release 或部署生产；随后远端验证与正式发布结果见上节。

## 2026-10-02：强制 MFA 实现与回归

- Provider 移除已通过 [PR #1](https://github.com/ArkGravity/burrow/pull/1) squash 合并到 `main`（`617cd9e`）；按用户要求直接合并，未检查 CI。本地 main 更新后创建 `feat/mandatory-mfa`，完成以下 MFA 实现与验证。
- 2026-10-02，用户确认 MFA 功能已在浏览器上人工验收通过，并授权提交、推送和 PR 合并。此项为用户人工验收反馈，以下自动化检查结果分别记录。
- 新增迁移 005/schema v5，001–004 未修改；全员首次登录强制 TOTP，密码通过仅建立五分钟受限事务。临时密码、绑定或 MFA 验证全部完成后才签发正式会话；已有绑定的密码重置先验证原 MFA 再改密。新增管理员重置（自身未使用动态码及原因）和 CLI 恢复，同事务撤销目标会话/Token/未完成事务并审计；密码和 MFA 分别重置。
- 使用独立临时 PostgreSQL 17 与 SQLite 运行完整 `make test-db lint`，race 测试与静态分析通过。最终新增保护和边界修改另运行相应双库 race 用例，覆盖账号级限速不能通过新事务绕过、改密轮换凭据且不延长期限、实际 ID Token 的 `pwd`/`otp` 和 `auth_time`、受限改密资格检查、退出及新鲜认证。TOTP 官方向量、±1 周期、防重放、并发单次消费/绑定、浏览器绑定、权限/认证版本复查、禁用/应用撤权/请求过期、迁移和审计失败回滚、CLI 恢复均有后端覆盖。
- `make test-e2e` 四条 Chromium 流程通过：管理员/普通用户首次绑定、刷新恢复、中文/主题/权限；Web 从应用发起的密码+MFA 授权继续、Web/SPA 共享 SSO/RP 退出；单 Web 无 PKCE 兼容；受限事务过期、在线重置后重新绑定、密码重置保留 MFA 的验证顺序，以及唯一管理员的实际 CLI 恢复。运行器构建前端和嵌入前端的 Go 二进制，全部使用临时 SQLite 数据。
- `make web-check` 类型检查与两条单测、静默 Compose 配置校验通过；文档格式、本地链接与 diff 检查通过。测试容器/服务完成后清理；未修改真实项目数据库，未构建 Burrow 容器镜像或部署生产，也未重新验收 Grafana/Nightingale/Harbor 或验证 MFA 远端 CI。

## 上游 Provider 移除（2026-10-02）

- 在 `feat/remove-upstream-providers` 分支移除上游 OIDC、Provider 管理、外部身份关联和用户/应用认证来源配置；保留下游 Applications、共享 SSO、应用授权和 PKCE 策略。当前 schema 为 v4，历史迁移 001–003 未修改。
- 迁移 004 在同一事务内清理上游数据与 Provider 权限、撤销非密码来源会话/Token 和未完成授权，并记录审计。启用用户需要密码且用户/应用必须允许本地登录，否则迁移拒绝并回滚。升级须先在旧版本准备这些记录；禁用的无密码账号保留，管理员可重置密码后显式启用。当时 MFA 尚未实现；随后经用户确认，实施情况见上面的本轮 MFA 记录及 [认证与恢复说明](../development/mfa-proposal.md)。
- 使用临时 PostgreSQL 17 容器的独立数据库和测试隔离 schema，`make test-db lint` 通过完整 SQLite/PostgreSQL race 测试和 Go 静态检查。针对迁移和密码认证的双数据库 race 测试也通过，覆盖旧 schema v1/v2/v3 升级、数据/密钥/授权保留、错误迁移拒绝、审计失败回滚、旧上游 API 与字段拒绝、禁用旧账号恢复、未知认证来源在 API/授权/换码/UserInfo 的拒绝。
- `make web-check` 通过类型检查和两条单元测试。`make test-e2e` 通过三条 Chromium 流程，包括移除旧 UI 入口、用户临时密码改密、从独立 Web 应用发起的密码登录、Web/SPA 共享 SSO 和 RP 退出，以及独立 Web 客户端的单应用无 PKCE 兼容。浏览器运行器构建并运行嵌入前端的 Go 二进制。
- `make compose-config COMPOSE_ENV=.env.example` 静默校验通过。格式、当前文档的本地链接和 `git diff --check` 通过。本轮未构建 Burrow 容器镜像、部署生产、重新部署 Grafana/Nightingale/Harbor 或验证远端 CI。此前三应用用户验收和以下旧版本测试记录仍是历史检查点，不代表本轮重新验收。

## 历史检查点

本记录对应 2026-09-28 的本地验收，使用版本见 [依赖文档](../development/dependencies.md)。这是工程回归与互操作验证，不是 OpenID Foundation 官方一致性认证。

## 已验证范围

- Go HTTP 测试覆盖 Discovery、授权码、S256、错误 verifier、一次性消费、并发兑换、撤权/禁用后的拒绝、浏览器事务绑定、认证来源限制、退出及精确重定向。
- PostgreSQL 测试使用独立 schema 和真正的连接池；SQLite 与 PostgreSQL 均运行迁移和主要权限/授权码流程，启用 Go race detector。
- 上游模拟 OP 验证预绑定身份及 state/nonce/PKCE、过旧或缺失 `auth_time` 的拒绝。
- 新增管理回归覆盖并发禁用时偏好修改不能恢复账号、旧密码不能覆盖管理员重置、普通管理者不能向刚晋升的管理员绑定身份、审计落库失败时敏感修改回滚。
- Chromium 测试覆盖管理员初始化后改密、用户/组/角色/APP 授权、普通门户、越权 API 拒绝、中文及深色偏好、退出。
- 独立 Web 客户端使用 coreos/go-oidc 验证签名和 claims；独立 SPA 使用 oidc-client-ts 验证 code + PKCE、SSO、跨来源 UserInfo 与 RP logout。

## 复现命令

本地执行结果：`go test -race ./... -count=1` 在配置 PostgreSQL DSN 后通过；`go vet ./...` 通过；Vitest 2 项通过；Chromium 2 项通过；前端与 Docker 镜像构建通过。根目录 Compose 的隔离实例通过配置校验、迁移、页面、readyz、Discovery、管理员初始化/改密与重启后会话、JWKS 持久化检查。

```sh
export BURROW_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=burrow password=TEST_PASSWORD dbname=burrow_test sslmode=disable'
go test -race ./... -count=1
go vet ./...
bun run --cwd web test
bun run --cwd web build
bun install --cwd examples/spa-client --frozen-lockfile
# 首次运行先在 web 下安装 Playwright Chromium
sh web/e2e/run.sh
```

浏览器测试启动三个本地端口：Burrow 18080、Web RP 19001、SPA RP 19002。测试自动清理临时 SQLite 数据库与进程。未配置 PostgreSQL DSN 时数据库子测试会跳过，不能作为双库通过的证据。

## 范围限制

未运行官方认证服务、生产反向代理/HTTPS 基础设施及远程 GitHub Actions。CI 工作流已提供，但本地结果不代表远程工作流已经执行。前端构建通过，仍有单个入口 bundle 超过 500 kB 的体积提示。Refresh Token、其他授权类型、多实例与跨 APP 同步退出不在首版范围。

## Seed, default roles and user groups (2026-09-30)

- `make test-db lint` with a dedicated PostgreSQL 17 test DSN passed the full
  SQLite/PostgreSQL race suite and Go static analysis.
- New tests cover repeated seed preserving passwords/status, rejection of an
  ordinary-account username collision and production example credentials, Viewer
  defaults versus explicit role choices, role boundaries, group summaries for a
  users-only reader, and rejection of blanket APP grants to built-in roles.
- `make web-check` passed typecheck and two unit tests.
- `make test-e2e` passed both Chromium scenarios, including two noninteractive
  seed invocations, Viewer preselection/persistence, the Users Groups column,
  ordinary portal access and independent Web/SPA OIDC clients.
- `make compose-config COMPOSE_ENV=.env.example` validated the migrate → seed →
  app dependency chain. No container image build/start was performed for this
  change; the browser runner built and exercised the embedded Go binary.

## Web PKCE compatibility (2026-09-30)

- `make test-db lint` passed the full SQLite/PostgreSQL race suite and Go static
  analysis using an isolated, temporary PostgreSQL 17 database. After the final
  administrator-session and test changes, the dual-driver race tests were rerun
  for `TestPKCE` and `TestCSRFAndForcedPassword` and passed.
- New coverage includes default enforcement, SPA rejection, confidential Web
  login without PKCE or nonce, client-secret checks, rejection of malformed or
  downgraded PKCE, unexpected verifier rejection, replay and concurrent code
  consumption, current access/session/policy rechecks, and existing-token
  validity after the compatibility option is disabled.
- Administration coverage verifies administrator-only policy changes (including
  concurrent role revocation), Editor updates to other fields, before/after
  audit details, rollback on audit failure, and version-one migration with data,
  default enforcement, checksum validation and idempotence.
- `make web-check` passed typecheck and two unit tests. `make test-e2e` passed
  all three Chromium scenarios, including Web/SPA shared SSO, SPA hiding the
  compatibility option, default-off state, administrator provisioning and an
  independent coreos/go-oidc Web client completing login without PKCE.
- The browser runner built and exercised the embedded Go binary. No Burrow
  container image build/deployment or real Nightingale/Harbor deployment was
  performed. Compatibility mode lowers authorization-code protection; the
  independent Web example still verifies nonce and does not represent the
  security behavior of Nightingale v9.1.1.

## Application permission labels and custom roles (2026-10-01)

- Optional application credentials passed dual-driver race coverage for defaults,
  explicit values, duplicate/invalid values, Web code exchange, SPA secret
  rejection, management permissions, secret rotation and audit rollback. The
  browser provisioning flow also exercised manual Client ID/Secret input.

- `make test-db` passed the full SQLite/PostgreSQL 17 race suite against an
  isolated temporary database. Final additional tests for new-user defaults on
  both drivers and migration audit rollback also passed. `make lint` passed.
- Permission coverage checks application-name/Client-ID search, duplicate display
  names, compact references under `permissions:read`, rename preserving grants
  and successful OIDC authorization with the unchanged permission code.
- Schema-v2 → v3 coverage checks checksum enforcement, repeat migration, empty
  Viewer cleanup, unused Editor cleanup, preservation of assigned legacy grants
  and custom roles, migration auditing and rollback when audit insertion fails.
  New users have no default role; explicit role assignments retain authorization
  checks. Seed only supplies Administrator and preserves existing credentials.
- `make web-check` passed typecheck and two unit tests. `make test-e2e` passed all
  three Chromium scenarios, including selecting an application permission by its
  display name and granting ordinary-user portal access through a group role.
- The Burrow Docker image was built. The independent local SSO Compose was
  reset at the user's request and started with fresh SQLite/Grafana volumes.
  Applications remain manually configured. The user subsequently reported that
  the ordinary `logic` user successfully logged into Grafana through Burrow OIDC;
  this is user-reported acceptance, not a repeated automated browser check.
  Production deployment has not been verified. This is local engineering
  validation, not official OIDC certification.

## Local Nightingale example checkpoint (2026-10-01)

- Added Nightingale `9.1.1` to the independent `examples/local-sso` Compose with
  persistent SQLite data, in-process miniredis and Nginx routing for
  `n9e.yakir.top`. Existing Burrow/Grafana containers and data were retained;
  the root deployment Compose was unchanged.
- Verified upstream source: runtime `config.toml` does not initialize OIDC.
  The current example documents manual OIDC TOML entry in Nightingale's UI,
  without direct database writes or automatic Burrow application creation.
- Quiet Compose validation, Nginx configuration validation, formatting and
  `git diff --check` passed. All four persistent services were healthy, and the
  original one-shot SSO initialization exited successfully (that initialization
  service and script have since been removed). A repeated import skipped the
  unchanged configuration. After restarting Nightingale, its login-button
  configuration persisted and its generated authorization request had the
  expected issuer, Client ID, exact callback, scopes and no PKCE challenge.
  The proxied Nightingale homepage returned HTTP 200.
- The user subsequently reported that `logic` successfully logged into
  Nightingale by clicking "Sign in with Burrow" on its login page. This is
  user-reported acceptance, not a repeated automated browser check. The portal
  still opens Nightingale's root URL, which does not initiate OIDC automatically
  in the deployed frontend. The user accepted this behavior and deferred both
  automatic-entry approaches. The README now includes the standalone TOML and
  manual OIDC UI steps, distinct from the mounted runtime `config.toml`.
  No product unit/race suite or browser regression was run for this example.

## Local Harbor example checkpoint (2026-10-01)

- Added Harbor `v2.15.2` using its official prepare image and nine base service
  images, without Trivy/exporter. The separate `burrow-local-sso-harbor` Compose
  project joins the existing SSO network through core and proxy; the existing
  Nginx serves `harbor.yakir.top`. Existing Burrow/Grafana/Nightingale containers
  and data were retained; only Nginx was recreated. The root deployment Compose
  remains unchanged. Official amd64 images ran under emulation on this arm64
  Docker host (four CPUs, approximately 6 GiB memory).
- The current deployment helper only generates internal service configuration
  and merges the network overlay. Harbor OIDC settings are maintained manually
  in its UI, using the README example. `harbor.yml` supplies deployment settings,
  not OIDC parameters; Burrow Applications are also created manually.
- Successfully generated official configuration, pulled and started all nine
  healthy services, and checked the proxied homepage (HTTP 200) and Harbor health
  API (all components healthy). The actual OIDC login route returned HTTP 302 to
  Burrow with the expected Client ID, exact public callback, only
  `openid profile email`, and an S256 challenge. No PKCE exception is needed.
- During the original deployment, switching between startup overrides and UI
  maintenance verified persistence and editability. Startup overrides and the
  mode-switch option have since been removed; the helper now always uses UI
  maintenance. The README documents manual OIDC and Burrow application/role setup.
- Generated service configuration, internal keys, database and logs are ignored
  local data. Outer and generated Harbor proxy access logs are disabled to avoid
  recording authorization-code callback URLs. Quiet Compose validation, Nginx
  configuration validation, formatting and `git diff --check` passed.
- The user subsequently confirmed Harbor's OIDC web-login acceptance, together
  with Grafana and Nightingale; see the final acceptance checkpoint below.
  No product unit/race suite, automated
  browser regression, production deployment or Docker/Helm CLI authentication
  was performed for this example. This is local engineering validation, not
  official OIDC certification.

## Manual SSO configuration cleanup (2026-10-01)

- At the user's request, Nightingale and Harbor now use manual SSO configuration
  in their own UIs, with complete examples in the local SSO README. Removed the
  Nightingale one-shot service, API import script and standalone OIDC file, and
  removed Harbor's OIDC Compose overlay and deployment mode argument. The Harbor
  helper retains official deployment preparation and network configuration only.
- Removed the exited Nightingale initialization container. Recreated Harbor core
  without its startup configuration override; existing OIDC settings persisted
  and were editable. Burrow, Grafana and Nightingale containers and all existing
  databases were preserved. No fresh database or manual UI save was exercised in
  this cleanup. The user subsequently confirmed all three integrations passed.
- Harbor jobservice was unhealthy during verification and was restarted. The
  subsequent homepage and aggregate health API checks passed, all nine Harbor
  containers and the existing SSO services were healthy, and the actual OIDC
  redirect retained its exact callback, expected scopes and S256 challenge.
- Quiet validation passed for both Compose projects; the base service list no
  longer contains a Nightingale SSO initializer. Python syntax, README TOML and
  local file links, formatting and `git diff --check` passed. `git check-ignore`
  verified Harbor data, logs and runtime directories; a local rule also ignores
  Python bytecode caches. No generated data was staged or deleted. The root
  deployment Compose remains unchanged. No product test suite or browser
  regression was rerun for this example cleanup.

## Local SSO file layout cleanup (2026-10-01)

- Moved Nightingale's runtime configuration to `examples/local-sso/n9e.config.toml`
  and removed the empty `nightingale/` directory. Updated the Compose bind mount
  and README. Recreated only Nightingale; it became healthy, its mount references
  the new file and its original named data volume remains attached.
- During that cleanup, a check for the original OIDC button text failed because the
  then-current persisted
  OIDC record has `Enable=false`. Read-only inspection confirmed the record still
  exists and its update time predates this recreation. No SSO setting was changed
  or automatically enabled during this cleanup.
- Reviewed Harbor's files: its generated runtime Compose contains the nine base
  service definitions, while the maintained overlay changes local platform,
  names, networks and ports. Renamed the overlay to `docker-compose.override.yml`
  and updated the helper. The README explains each maintained/generated file and
  why the two Compose layers belong to one deployment. Harbor was not recreated.
- Both quiet Compose validations, Python/TOML syntax, README local links,
  formatting and `git diff --check` passed. No product suite or browser regression
  was rerun; existing data and root deployment Compose were preserved.

## Grafana, Nightingale and Harbor acceptance (2026-10-01)

- The user confirmed that Grafana, Nightingale and Harbor all passed local OIDC
  web-login acceptance through Burrow. This supersedes the earlier pending Harbor
  acceptance and the temporary Nightingale disabled-configuration observation;
  no current database setting was read or modified to record this report.
- The final independent example uses Burrow dev SQLite and Nginx, Nightingale's
  flat `n9e.config.toml` runtime file, and Harbor's official prepare output plus
  local Compose overrides. Applications and login-role assignments are maintained
  in Burrow's UI; Nightingale and Harbor OIDC settings are maintained in their UIs.
- Grafana and Harbor keep S256 PKCE enforcement; Nightingale v9.1.1 uses only its
  per-Web-application exception. The accepted Nightingale portal behavior still
  allows an additional SSO button click. Harbor acceptance covers web login only.
- This is user-reported acceptance, not a repeated automated browser run or a
  claim that every negative-access scenario in the example was executed. Production
  deployment, Docker/Helm CLI authentication and official certification remain
  outside this checkpoint. Earlier engineering checks above remain historical.

## Branding and acceptance resource cleanup (2026-10-01)

- Added a native SVG mark combining a burrow entrance and keyhole, using the
  existing forest-green and pale-green palette. The favicon, README and shared
  `BrandMark` component use the same asset. Login branding and the signed-in
  sidebar now use this mark; the sidebar's duplicated wordmark was removed in
  response to the user's screenshot, leaving one top home-link brand entry.
- At the user's explicit request, removed the two local acceptance Compose
  projects: 16 containers, three named data volumes, nine Harbor anonymous volumes
  and two networks. Removed the 14 image tags used by the example and its official
  prepare helper, plus Harbor's ignored data, logs and runtime directories.
  Docker's default networks and unrelated cached images were retained; no global
  prune or forced image deletion was used. Maintained example files remain intact.
- Final inventory found no remaining Docker containers or volumes and only the
  three default networks. Harbor's three generated directories were absent and
  its maintained deployment files remained. The previously accepted three-app
  login checkpoint remains historical; a future local run starts with fresh data.
- As requested for this small branding change, no tests, typecheck, build or
  browser regression was run. Delivery checks were limited to formatting, diff
  review and verification of the scoped resource cleanup. No authentication or
  authorization logic, root production Compose or remote deployment was changed.
