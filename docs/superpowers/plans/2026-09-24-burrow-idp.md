# Burrow IdP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task after implementation is explicitly authorized. Steps use checkbox (`- [ ]`) syntax for tracking. Do not start subagents unless separately authorized.

**Goal:** 构建单组织、仅 OIDC 的自建 IdP，交付用户/组/RBAC、APP/Provider、双语主题界面及根目录单文件 Docker Compose 部署。

**Architecture:** Go 模块化单体托管 React 静态资源、管理 API 与 OIDC 端点。协议通过 zitadel/oidc/v3 适配，业务授权独立，状态存入 PostgreSQL/SQLite。单实例，无 Redis 或消息队列。

**Tech Stack:** Go、Chi、GORM、zitadel/oidc/v3、React、TypeScript、Vite、Ant Design、Bun、PostgreSQL、SQLite、Docker Compose。

---

## 执行边界与依据

- 已确认规格：`docs/superpowers/specs/2026-09-24-burrow-idp-design.md`。
- 用户随后明确授权“开始实现”，已在本地 `feat/idp` 分支实施。
- 下方 Task 清单保留原始规划，未逐项勾选不代表对应功能缺失；实际文件组织、验证命令和差异以本文执行记录及 `docs/development/`、`docs/testing/` 为准。
- 已初始化本地 Git 仓库，不假设远程地址，不推送。不提交 `.codex`、凭据或无关文件。
- 使用 RTK 的环境下，shell 命令均以 `rtk` 开头；不支持的命令用 `rtk proxy`。面向普通开发者的 README 使用原始命令，不要求安装 RTK。
- 任务按依赖顺序执行。每个任务中的“编写断言→确认失败→最小实现→验证→提交”拆成短步骤；表格/文案等低风险改动使用构建与页面检查，不添加镜像实现的测试。
- 每次只暂存任务列出的文件，用 `rtk git diff --cached --check` 后提交；不要使用无范围的 `git add .`。

## 版本基线与选型验证

### 实施记录（2026-09-28）

| 任务  | 实际交付与证据                                                                    |
| ----- | --------------------------------------------------------------------------------- |
| 1–3   | 工程配置、双库显式迁移、加密密钥、密码与管理员 CLI；Go 测试与镜像构建             |
| 4–7   | RBAC、统一会话、管理 API、APP/Provider；双库回归、权限竞态及事务审计测试          |
| 8–11  | zitadel OP/RP 适配、PKCE 原子兑换、绑定身份、退出与生命周期；协议 HTTP 测试       |
| 12–13 | 双语/主题、登录与管理页面、授权门户；Vitest 与 Chromium 管理流程                  |
| 14    | 独立 Web/SPA 客户端；Chromium 实际 code flow、SSO、UserInfo、退出；未进行官方认证 |
| 15    | 内嵌资源、非 root 镜像、根目录 Compose；真实启动及重启持久化检查                  |
| 16    | README、本地开发/部署、备份恢复、依赖与适配说明、CI 文件和验证记录                |

实际后端集中在 `internal/burrow`，按职责分文件；原计划的多包路径与独立 integration 包命令不再适用。具体差异见 `docs/development/dependencies.md`，实际复现命令见 `docs/testing/oidc-conformance.md`。下方原始细项作为设计过程记录保留，不将未执行的步骤追溯标为通过。

规划时官方来源可见 Go 1.27.1、Bun 1.4.2、zitadel/oidc v3.49.2。将其作为实施起点，不声称已通过组合构建；OIDC 的 v3.49.2 模块声明最低 Go 1.25.0。

任务 1 将这些版本写入 `.tool-versions`、`go.mod` 与 `web/package.json`，并安装当前稳定且互相兼容的 React/TypeScript/Vite/Ant Design、Chi/GORM 驱动版本，使用精确版本与锁文件固化。其他依赖在实际解析前不臆造补丁版本；解析结果写入 `docs/development/dependencies.md`。Go OIDC 库若有安全修复或不兼容，先记录证据和更新基线，再继续。

来源：

- https://go.dev/dl/?mode=json
- https://github.com/oven-sh/bun/releases/tag/bun-v1.4.2
- https://github.com/zitadel/oidc/releases/tag/v3.49.2
- https://github.com/zitadel/oidc/blob/v3.49.2/go.mod
- https://github.com/zitadel/oidc/blob/v3.49.2/pkg/op/storage.go

## 文件职责与边界

| 路径                    | 职责                                                      |
| ----------------------- | --------------------------------------------------------- |
| `cmd/burrow/main.go`    | serve/migrate/admin-init/keys-rotate/healthcheck 命令装配 |
| `internal/config/`      | 读取、校验环境配置，不输出秘密                            |
| `internal/database/`    | GORM 连接、显式迁移、事务/锁及双数据库测试设施            |
| `internal/identity/`    | 用户、组、密码、外部身份绑定                              |
| `internal/authz/`       | 角色/权限、管理员不变量与授权判断                         |
| `internal/session/`     | 统一会话、首次改密限制、认证上下文                        |
| `internal/application/` | 客户端、回调、登录方式、客户端密钥                        |
| `internal/provider/`    | 上游 OIDC 配置、网络访问策略、登录事务                    |
| `internal/oidcserver/`  | ZITADEL OP 适配、协议状态、UserInfo、退出                 |
| `internal/keys/`        | 加密封装、签名密钥保存和轮换                              |
| `internal/event/`       | 事件、统计与清理                                          |
| `internal/httpapi/`     | 路由、统一错误、鉴权/CSRF/限流与模块 handler              |
| `internal/server/`      | 进程生命周期、探针、后台清理                              |
| `web/`                  | React 源码、静态资源嵌入入口、构建与浏览器测试            |
| `tests/integration/`    | HTTP、数据库与协议边界测试                                |
| `examples/`             | 独立客户端实现的 Web/SPA 联调样例                         |
| `docker-compose.yaml`   | 根目录唯一 Compose 配置，支持本地构建和固定镜像           |
| `.env.example`          | 配置变量说明；真实环境文件不提交                          |

模块以内聚的小文件组织，API handler 只做输入输出与业务调用。不要将所有模型放在一个巨型 models.go，也不要让整个业务层使用 op.AuthRequest。

## 公共接口、配置与测试约定

计划中的环境键统一使用 `BURROW_` 前缀：`ENV`、`LISTEN_ADDR`、`ISSUER`、`DB_DRIVER`、`DB_DSN`、`MASTER_KEY`、`TRUSTED_PROXIES`、`PROVIDER_ALLOWED_CIDRS`、`SESSION_TTL`、`TOKEN_TTL`、`AUTH_CODE_TTL`、`LOGIN_TTL`、`EVENT_RETENTION`。时长采用 Go duration，默认 8h、5m、60s、10m、2160h。prod 拒绝 HTTP issuer、缺失主密钥和 SQLite。

初始监听 `:8080`、开发 issuer `http://localhost:8080`。Vite 监听 `:5173`，代理 `/api`、`/oidc`、`/.well-known` 至后端；浏览器回调地址始终基于配置的 issuer。不要从未受信任的 Host/X-Forwarded-* 构建 issuer。

管理接口统一 `/api/v1`；协议端点统一 `/oidc/authorize`、`/oidc/token`、`/oidc/userinfo`、`/oidc/jwks`、`/oidc/logout`，Discovery 使用 `/.well-known/openid-configuration`。固定 issuer URL 不带随请求变化的路径。

REST 错误格式固定为 `{ "error": { "code": "FORBIDDEN", "requestId": "…" } }`；400 输入错误、401 未认证、403 无权限、404 不存在、409 引用/唯一性冲突、429 限流、503 数据库/必要依赖不可用。OIDC 路由保持协议自己的错误格式。

所有集合返回 `{items,total,page,pageSize}`；page 从 1 开始，pageSize 默认 20、最大 100。PUT 更新关联关系使用完整 ID 集合并在事务中替换，禁止批量绑定 JSON 中未声明的权限字段。

后端测试统一命令 `rtk go test ./...`；双库集成测试使用 build tag `integration`，读取 `BURROW_TEST_POSTGRES_DSN`，同时创建临时 SQLite。显式请求集成测试但 DSN 缺失时失败，不将 PostgreSQL 跳过报告为成功。不同测试使用隔离 schema，清理时只删除本测试创建的数据。

前端脚本统一 `dev`、`build`、`typecheck`、`test`、`test:e2e`；测试使用 Vitest/Testing Library 与 Playwright，由 Bun 执行脚本。快照不代替权限和认证行为断言。

## Task 1：版本、工程基础与配置

**依赖：** 无。

**创建：** `.gitignore`、`.tool-versions`、`go.mod`、`go.sum`、`cmd/burrow/main.go`、`internal/config/config.go`、`internal/config/config_test.go`、`web/package.json`、`web/bun.lock`、`web/tsconfig.json`、`web/vite.config.ts`、`web/index.html`、`web/src/main.tsx`、`docs/development/dependencies.md`。

- [ ] 核实官方发布版本与本地工具，记录 Go/Bun/OIDC 版本，不修改全局工具配置。
- [ ] 创建明确命名的 Go module `burrow`，不虚构 GitHub 仓库地址；固定依赖与前端脚本。
- [ ] 为配置写表驱动测试：dev+SQLite 成功；prod+HTTP、prod+SQLite、非法 duration、无主密钥失败。秘密不能进入错误消息。
- [ ] 运行 `rtk go test ./internal/config -v`，预期在尚未实现解析/校验时失败；实现环境读取与校验后重跑通过。
- [ ] 创建最小可构建前端入口与开发代理，先不写业务页面；执行 `rtk proxy bun run --cwd web typecheck`、`rtk proxy bun run --cwd web build`，预期退出 0。
- [ ] 将锁文件和依赖选择写入版本记录，提交 `chore: establish burrow project and configuration`。

配置断言契约：输入 `ENV=prod, ISSUER=http://localhost:8080` 必须被拒绝；输入 `ENV=dev, DB_DRIVER=sqlite` 不要求网络数据库。主密钥为 32 字节随机数据的 Base64 编码，验证解码长度，不使用固定开发默认密钥。

## Task 2：数据库与可重复迁移

**依赖：** 1。

**创建：** `internal/database/open.go`、`internal/database/migrate.go`、`internal/database/migrate_test.go`、`internal/database/migrations/sqlite/0001_identity.sql`、`internal/database/migrations/postgres/0001_identity.sql`、`tests/integration/database_test.go`；**修改：** `cmd/burrow/main.go`。

- [ ] 在 SQLite 与 PostgreSQL 分别定义 users、groups、group_members、roles、permissions、user_roles、group_roles、role_permissions、schema_migrations 和用于管理员不变量的锁记录。
- [ ] 给关联增加外键/唯一约束；用户名以明确的小写规范化值唯一，邮箱不用于自动身份匹配。
- [ ] 写迁移测试：空库升级、再次执行无操作、校验和不匹配拒绝、唯一关系不能重复、回滚事务不留部分数据。
- [ ] 执行 `rtk go test ./internal/database -v` 确认断言失败，再实现连接与迁移。SQLite 开启外键、busy timeout；迁移使用数据库写锁，PostgreSQL 使用迁移专用 advisory lock。
- [ ] 增加 `migrate` 命令和 schema 版本检查。serve 不自动执行 DDL。
- [ ] 执行 `rtk go test -tags=integration ./tests/integration -run TestDatabase -v`，预期两库通过，提交 `feat: add explicit database migrations`。

## Task 3：密钥、密码与管理员初始化

**依赖：** 2。

**创建：** `internal/keys/cipher.go`、`internal/keys/signing.go`、`internal/keys/keys_test.go`、`internal/identity/password.go`、`internal/identity/password_test.go`、`internal/identity/bootstrap.go`、`internal/identity/bootstrap_test.go`、`internal/database/migrations/sqlite/0002_keys.sql`、`internal/database/migrations/postgres/0002_keys.sql`；**修改：** `cmd/burrow/main.go`。

- [ ] 先写密码校验、错误主密钥/篡改密文失败、轮换后旧公钥保留、重复初始化拒绝的测试；`rtk go test ./internal/keys ./internal/identity -v` 预期失败。
- [ ] 密码使用 Argon2id PHC 编码，随机盐；解码时限制参数避免恶意哈希触发无界资源消耗。高熵随机客户端密钥另用 SHA-256 验证哈希和常量时间比较。
- [ ] 使用 AES-256-GCM 封装上游密钥与 RSA 私钥，AAD 绑定用途和对象 ID；RSA 3072 位、RS256、随机 kid。提供持久化读取与显式轮换，不每次启动生成。
- [ ] 添加 `admin-init --username admin --password-stdin`，数据库事务创建管理员和内置权限。密码从 stdin 输入，stdout/日志不回显，失败不留半个账号；初始化账号也要求首次改密。
- [ ] 添加 `keys-rotate` 运维命令；私钥退役后旧公钥至少保留令牌有效期加 60 秒容差。
- [ ] 测试通过后提交 `feat: add credentials keys and admin bootstrap`。

## Task 4：RBAC 与最后管理员保护

**依赖：** 3。

**创建：** `internal/authz/permissions.go`、`internal/authz/service.go`、`internal/authz/admin_guard.go`、`internal/authz/service_test.go`、`tests/integration/admin_guard_test.go`。

- [ ] 内置权限固定 `dashboard:read`，用户/组/角色/权限/APP/Provider 各模块 `:read`，用户/组/APP/Provider `:write`，角色及权限关联使用 `authorization:write`。新增 APP 生成 `app:<id>:login`。
- [ ] 写表驱动测试：直接角色、组角色、并集、无角色拒绝、禁用用户拒绝；角色普通名称为 admin 不产生内置身份。
- [ ] 运行 `rtk go test ./internal/authz -v` 确认失败，实现实时查询和合并，不加权限缓存。
- [ ] 所有影响最后管理员的写入先锁定公共 guard 行，再检查变更后的有效管理员数；SQLite 使用立即写事务。用户本地登录开关、密码状态、禁用、删除、组成员/角色变更都走同一保护路径。
- [ ] 普通授权管理员不得通过编辑用户请求夹带内置管理员角色；只有现有内置管理员能授予/撤销该角色。
- [ ] 执行 `rtk go test -tags=integration ./tests/integration -run TestAdminGuard -v`，断言两管理员并发降权最多一方成功且至少保留一人。
- [ ] 提交 `feat: enforce rbac and administrator invariants`。

## Task 5：统一会话与本地登录

**依赖：** 4。

**创建：** `internal/session/model.go`、`internal/session/service.go`、`internal/session/service_test.go`、`internal/httpapi/auth.go`、`internal/httpapi/csrf.go`、`internal/httpapi/errors.go`、`internal/httpapi/router.go`、`internal/database/migrations/sqlite/0003_sessions.sql`、`internal/database/migrations/postgres/0003_sessions.sql`、`tests/integration/session_test.go`。

- [ ] 定义 `/api/v1/auth/login`、`/auth/logout`、`/auth/csrf`、`/me`、`/me/password`；所有状态变更拒绝跨站请求并验证 CSRF。
- [ ] 写 HTTP 测试：错误密码与不存在用户同类错误；会话凭据轮换；过期/撤销返回 401；首次改密前后台返回 403；重置密码撤销旧会话。
- [ ] `rtk go test ./internal/session ./internal/httpapi -v` 先失败，再实现随机 256 位会话、哈希存储、绝对 TTL、来源与认证时间字段。
- [ ] 浏览器 Cookie 使用 host-only、HttpOnly、SameSite=Lax，prod Secure；登录前 CSRF 使用独立短时绑定机制。首次改密建立受限会话，只允许必要个人/改密/退出端点。
- [ ] 外部来源会话的启用状态每次使用时检查；统一入口允许已启用 Provider，APP 登录仍按该 APP 配置限制。
- [ ] 双库会话集成测试通过后提交 `feat: add local login and persistent sso sessions`。

## Task 6：用户、组、角色和权限 API

**依赖：** 5。

**创建：** `internal/identity/users.go`、`internal/identity/groups.go`、`internal/httpapi/users.go`、`internal/httpapi/groups.go`、`internal/httpapi/roles.go`、`internal/httpapi/permissions.go`、`tests/integration/management_test.go`。

- [ ] 为 users/groups/roles 提供分页 CRUD，permissions 只读系统目录；用户/组角色与角色权限通过显式关联端点更新，禁止任意模型绑定。
- [ ] 用户端点包括状态切换、临时密码重置、会话撤销；用户被删除前事务撤销会话与相关授权，保留事件。
- [ ] 先写 HTTP 权限矩阵：401 未登录，403 无权限，普通读者 GET=200/写入=403；组/角色被引用时删除=409；并发更新遵守管理员 guard。
- [ ] `rtk go test -tags=integration ./tests/integration -run TestManagement -v` 确认失败，再按 HTTP→service→事务存储实现，业务不依赖 React 页面。
- [ ] 校验分页上下界、名字长度、重复 ID、跨字段非法组合；错误消息不暴露 SQL 或密码哈希。
- [ ] 全部矩阵通过后提交 `feat: add identity and authorization management api`。

## Task 7：APP 与 Provider 配置

**依赖：** 6。

**创建：** `internal/application/model.go`、`internal/application/service.go`、`internal/provider/model.go`、`internal/provider/service.go`、`internal/httpapi/applications.go`、`internal/httpapi/providers.go`、`internal/httpapi/external_identities.go`、`internal/database/migrations/sqlite/0004_apps_providers.sql`、`internal/database/migrations/postgres/0004_apps_providers.sql`、`tests/integration/apps_providers_test.go`。

- [ ] 定义 APP/Provider CRUD、APP 客户端密钥重置、用户外部身份关联端点，增加精确 URI 白名单与 SPA allowed_origins 字段。
- [ ] 写测试：公共客户端无密钥；机密客户端密钥仅生成时出现；回调通配符拒绝；APP 自动获得对应权限；绑定同一外部身份到第二用户失败；已有外部身份的 Provider issuer 变更=409。
- [ ] `rtk go test -tags=integration ./tests/integration -run TestAppsProviders -v` 先失败，再实现事务性保存及加密上游凭据。
- [ ] APP 必须至少有一种可用登录方式；禁用 Provider/修改登录方式后旧配置不能继续授权。图标使用受限 HTTPS URL 或内置图标，不新增上传/对象存储。
- [ ] Provider GET 不返回密钥；修改密钥使用单独字段语义，空值不意外清空已有秘密。下游密钥重置立即停止旧密钥认证。
- [ ] 删除 APP 清理对应权限关联并失效未完成授权；已有引用的 Provider 删除=409。
- [ ] 全部测试通过后提交 `feat: manage oidc applications and upstream providers`。

## Task 8：OIDC 库适配与协议元数据

**依赖：** 7。

**创建：** `internal/oidcserver/provider.go`、`internal/oidcserver/client.go`、`internal/oidcserver/storage.go`、`internal/oidcserver/auth_request.go`、`internal/oidcserver/keys.go`、`internal/oidcserver/discovery_test.go`、`internal/database/migrations/sqlite/0005_oidc.sql`、`internal/database/migrations/postgres/0005_oidc.sql`、`docs/development/oidc-adapter.md`。

- [ ] 阅读锁定版本 op.Storage、op.Client、授权/令牌处理调用顺序，写明每个接口对应的业务服务、事务边界及失败响应。使用接口编译断言，不能复制内存示例存储上线。
- [ ] 实现固定 issuer 和端点、数据库持久化 AuthRequest、key set 与客户端元数据适配。Refresh Token 等必需但未启用的方法显式失败，不创建该类状态。
- [ ] 写 Discovery/JWKS 测试：仅 code、authorization_code、S256、RS256、basic/none；不宣告离线访问或未启用端点；JWKS 不含私钥；端点 URL 与配置 issuer 一致。
- [ ] 运行 `rtk go test ./internal/oidcserver -run 'TestDiscovery|TestJWKS' -v`，由失败推进至通过。
- [ ] 若库默认注册额外端点，关闭或阻断并同步元数据，确认不存在绕过入口。
- [ ] 提交 `feat: integrate oidc provider metadata and storage`。

## Task 9：授权码、PKCE 与令牌原子消费

**依赖：** 8。

**创建：** `internal/oidcserver/authorize.go`、`internal/oidcserver/token.go`、`internal/oidcserver/userinfo.go`、`tests/integration/oidc_code_test.go`、`tests/integration/oidc_concurrency_test.go`；**修改：** `internal/oidcserver/storage.go`、`internal/oidcserver/auth_request.go`。

- [ ] 写完整 HTTP 流程测试：本地登录→授权→code→token→userinfo；nonce、aud、iss、sub 和 scope 对应 claims 正确，不含后台权限。
- [ ] 实现 APP 权限检查和登录来源判断，prompt=none 无会话返回 login_required，需换身份源时返回适当交互错误；prompt=login/max_age 不静默使用旧认证。
- [ ] 兑换时先经库校验客户端及 PKCE，然后在存储事务中重新检查状态/权限，条件更新消费标记，并创建 token record。关键条件为 `consumed_at IS NULL AND expires_at > now`，必须检查受影响行数等于 1。
- [ ] 根据 Task 8 调用顺序将消费放在实际发令牌前的适配点，不在读取 AuthRequest 时就消费，也不依赖最后 DeleteAuthRequest 防重放。事务错误时不返回令牌；响应签名失败后允许用户重新授权，不恢复可重放的 code。
- [ ] 写两库并发测试：同一 code 同时 20 次兑换最多一次 200，其余 invalid_grant；错误 PKCE 不得获得令牌；跨 client、过期、二次兑换全部失败。
- [ ] 加入“授权后撤权/禁用/改登录源再兑换”的测试；无权限的管理员也不绕过 APP 状态检查。
- [ ] `rtk go test -tags=integration ./tests/integration -run 'TestOIDCCode|TestOIDCConcurrency' -v` 全通过后提交 `feat: implement secure oidc code flow`。

## Task 10：上游 OIDC 登录与绑定

**依赖：** 9。

**创建：** `internal/provider/login.go`、`internal/provider/callback.go`、`internal/provider/transport.go`、`internal/provider/transport_test.go`、`tests/integration/upstream_oidc_test.go`；**修改：** `internal/session/service.go`、`internal/httpapi/router.go`。

- [ ] 测试使用独立 mock OIDC 上游，包含受控签名、公钥、Discovery 和 token 端点，不连接真实第三方账号。
- [ ] 上游登录事务同时绑定浏览器随机值、Provider、下游授权请求或门户目的地、state/nonce/PKCE，10 分钟过期且一次使用。回跳目标存服务器，不信任任意 return URL。
- [ ] 通过 zitadel RP 处理 discovery/兑换/验证，只接受有效签名、正确 issuer/aud/nonce；按预关联 sub 找用户，不以邮箱创建或合并账号。
- [ ] 断言未知身份失败、Provider 禁用失败、篡改 state/nonce 失败、不同浏览器回调失败、回调重放失败、不同 APP 登录来源限制生效。
- [ ] 网络层强制超时和受控重定向；解析及实际拨号均验证地址，覆盖 IPv4/IPv6、回环、metadata、DNS 变化；内网 Provider 必须显式 CIDR 放行。
- [ ] 运行 `rtk go test ./internal/provider -v` 与 `rtk go test -tags=integration ./tests/integration -run TestUpstreamOIDC -v`，通过后提交 `feat: support prelinked upstream oidc identities`。

## Task 11：退出、事件与服务生命周期

**依赖：** 10。

**创建：** `internal/oidcserver/logout.go`、`internal/event/service.go`、`internal/event/cleanup.go`、`internal/server/server.go`、`internal/server/health.go`、`internal/httpapi/ratelimit.go`、`internal/httpapi/dashboard.go`、`internal/database/migrations/sqlite/0006_events.sql`、`internal/database/migrations/postgres/0006_events.sql`、`tests/integration/lifecycle_test.go`。

- [ ] 退出验证 ID Token hint、当前会话匹配和 post_logout_redirect_uri；无充分绑定信息时要求本站确认，不能根据任意 userID 退出别人的会话。
- [ ] 撤销会话后 UserInfo 拒绝关联令牌，新授权/兑换停止；不尝试删除 APP 自己的 Cookie。
- [ ] 登录/管理变更记录事件；敏感管理变更与成功事件在同一事务内，失败登录单独记录。日志脱敏测试用唯一哨兵凭据检查不出现明文。
- [ ] 实现内存限流、请求体大小限制、请求 ID、panic 边界；只信任配置代理给出的客户端地址。
- [ ] 实现 90 天事件清理和短期协议状态清理；后台任务随进程退出取消。存活不依赖数据库，就绪依赖数据库/schema/可用签名密钥。
- [ ] `rtk go test -tags=integration ./tests/integration -run TestLifecycle -v` 验证重启后会话与 JWKS 延续、数据库故障失败关闭、过期清理、统计权限。
- [ ] 提交 `feat: add logout audit events and runtime health`。

## Task 12：前端登录、布局与偏好

**依赖：** 5、11。

**创建：** `web/src/app/router.tsx`、`web/src/app/App.tsx`、`web/src/api/client.ts`、`web/src/auth/session.tsx`、`web/src/pages/LoginPage.tsx`、`web/src/pages/ChangePasswordPage.tsx`、`web/src/pages/LogoutPage.tsx`、`web/src/pages/ProfilePage.tsx`、`web/src/i18n/index.ts`、`web/src/i18n/zh-CN.json`、`web/src/i18n/en.json`、`web/src/theme/ThemeProvider.tsx`、`web/src/auth/session.test.tsx`、`web/e2e/auth.spec.ts`、`web/playwright.config.ts`。

- [ ] 路由按 `/login`、`/change-password`、`/logout`、`/profile` 与管理布局组织，session API 返回有效权限、受限改密状态、用户偏好。
- [ ] API client 使用同域 Cookie 与 CSRF header；401 清理前端 session，403 显示权限错误，503 显示可重试失败；不将令牌写入 localStorage。
- [ ] 登录页读取服务器认可的事务上下文，显示合法 Providers；外部错误映射稳定翻译，不能将任意 URL 用作继续登录地址。
- [ ] 使用 Ant Design 浅/深色算法；跟随系统监听 matchMedia 并清理 listener。用户未登录时偏好存本地，登录后使用用户配置。
- [ ] 添加行为测试：受限账号只能改密；普通用户没有管理入口；切换语言、主题后刷新仍正确；系统主题变化即时生效。
- [ ] 运行 `rtk proxy bun run --cwd web test`、`rtk proxy bun run --cwd web typecheck`、`rtk proxy bun run --cwd web test:e2e -- auth.spec.ts`，通过后提交 `feat: add bilingual login and theme shell`。

## Task 13：管理页面与 APP 门户

**依赖：** 6、7、11、12。

**创建：** `web/src/pages/HomePage.tsx`、`web/src/pages/UsersPage.tsx`、`web/src/pages/GroupsPage.tsx`、`web/src/pages/RolesPage.tsx`、`web/src/pages/PermissionsPage.tsx`、`web/src/pages/ApplicationsPage.tsx`、`web/src/pages/ProvidersPage.tsx`、`web/src/components/PermissionGate.tsx`、`web/src/components/SecretOnce.tsx`、`web/e2e/management.spec.ts`；**修改：** 路由与两份语言资源。

- [ ] 首页从 `/api/v1/me/apps` 获取已过滤列表，看板有权限才请求 `/api/v1/dashboard`；卡片导航 APP 登录入口，不携带 token。
- [ ] 分别完成用户/组成员与角色编辑、角色权限、只读权限目录；关联编辑采用明确选择器，禁止靠手填任意权限字符串。
- [ ] APP 表单区分 public/confidential，配置回调、来源和登录方式；密钥一次展示并允许复制，关闭即清除前端值。
- [ ] Provider 表单只显示已设置秘密状态；用户详情提供 Provider/sub 关联、会话撤销与临时密码重置。
- [ ] UI 处理 409 引用冲突和最后管理员保护；删除必须展示对象及影响，错误不清空已填表单。
- [ ] Playwright 覆盖“建用户→建组→分配角色→登录 APP”以及读者不能写、密钥不二次显示、中英文/主题正常；不为每个静态标签写单元测试。
- [ ] `rtk proxy bun run --cwd web test:e2e -- management.spec.ts` 与 typecheck/build 通过，提交 `feat: add identity administration and app portal`。

## Task 14：独立客户端联调与协议验收

**依赖：** 9、10、13。

**创建：** `examples/web-client/main.go`、`examples/spa-client/package.json`、`examples/spa-client/bun.lock`、`examples/spa-client/src/main.ts`、`examples/spa-client/index.html`、`examples/README.md`、`tests/integration/oidc_interop_test.go`、`docs/testing/oidc-conformance.md`。

- [ ] Web 示例使用 coreos/go-oidc + x/oauth2 作为独立 RP，SPA 使用 oidc-client-ts，固定兼容版本；不复制 Burrow 协议实现作为唯一验收客户端。
- [ ] 示例通过管理员配置客户端并授予测试用户权限，不自动注册生产客户端、不硬编码生产秘密。
- [ ] 完成 Web→SPA 复用会话、过期再授权、nonce/PKCE 检验、Provider 来源限制、退出边界的联调记录。
- [ ] 对 Discovery、code flow、UserInfo、JWKS、RP logout 执行适用一致性测试；记录工具版本、测试范围、结果及不适用项，不用“认证通过”代替实际结果。
- [ ] 执行 `rtk go test -tags=integration ./tests/integration -run TestOIDCInterop -v`；示例文档以真实复现命令和观测结果验收。
- [ ] 提交 `test: verify oidc interoperability with independent clients`。

## Task 15：静态嵌入、镜像与根目录 Compose

**依赖：** 13、14。

**创建：** `web/embed.go`、`internal/server/static.go`、`Dockerfile`、`.dockerignore`、`docker-compose.yaml`、`.env.example`；**修改：** `cmd/burrow/main.go`、`.gitignore`。

- [ ] 使用前端构建阶段→Go 编译阶段→非 root 运行阶段；前端构建在 Go embed 前完成。开发采用 build tag 分离未构建的 dist，避免 `go test` 依赖不存在的产物。
- [ ] Go 静态托管支持 SPA fallback，但 `/api`、`/oidc`、`/.well-known` 未知路径不返回 HTML；hash 静态资源长期缓存、index 不长期缓存。
- [ ] 应用镜像内提供 healthcheck 子命令，不依赖运行镜像包含 shell/curl；编译 SQLite 驱动时明确 CGO/运行库需求，并实测镜像不能只在宿主运行。
- [ ] 根目录 `docker-compose.yaml` 定义 app、migrate 和 profile 为 local-db 的 PostgreSQL；不让 app 强制 depends_on 一个可选数据库服务。migrate 有界重试等待数据库，app 依赖 migrate 成功。
- [ ] app 和 migrate 使用相同 `BURROW_IMAGE`，共享配置与构建定义。本地显式执行 `docker compose build` 后使用 `--pull never --no-build` 启动；生产先拉取固定镜像，再使用 `--no-build` 启动，不能意外构建生产镜像。实际执行命令在 README 列出。
- [ ] 提供单个根目录 `.env.example`，说明配置、镜像、数据库与主密钥；真实 `.env`、`.env.dev`、`.env.prod` 均忽略，显式保留 `.env.example`。构建上下文也排除真实凭据和数据库文件。
- [ ] 默认读取 `.env`；需要 dev/prod 隔离时，以 `--env-file` 和 `--project-name` 复用同一 YAML，不创建环境覆盖 YAML、固定 container_name 或跨 project 共用数据卷。数据库默认不映射宿主机端口。
- [ ] 实现迁移锁和幂等检查，应用启动仅检查 schema；升级迁移兼容旧版本，镜像回滚不自动回滚数据库。初始化管理员作为独立命令，不能每次启动重新创建。
- [ ] 在准备实际本地配置后运行 `rtk proxy docker compose -f docker-compose.yaml config --quiet`，再使用隔离环境文件检查 `--env-file`、`--project-name` 和 local-db profile 组合，预期均退出 0；不输出包含秘密的渲染配置。
- [ ] 在隔离 project 验证本地构建/固定镜像、内置/外部 PostgreSQL、迁移失败阻止应用启动、初始化、登录、重启后会话/JWKS 保持；不得连接用户生产数据库。
- [ ] 提交 `feat: package burrow with root compose configuration`。

## Task 16：本地文档、持续验证与交付检查

**依赖：** 1—15。

**创建：** `README.md`、`docs/operations/recovery.md`、`.github/workflows/ci.yml`；**修改：** 依赖记录、OIDC 接入文档。

- [ ] 根 README 给出工具版本、Bun 安装、复制环境、主密钥生成、SQLite/PostgreSQL 切换、migrate、admin-init、Go/Bun 开发启动、前端代理、测试与构建命令。按全新目录顺序实际复现。
- [ ] 说明临时密码改密、首位管理员、APP/Provider 配置、独立客户端接入、token/会话有效期以及不保证跨 APP 同步退出。
- [ ] 恢复文档覆盖 PostgreSQL 备份+主密钥保管、签名轮换、数据库恢复、迁移失败与回滚限制，不在示例中包含真实凭据。
- [ ] CI 包含 Go unit/vet、双库 integration、前端类型/行为/构建、浏览器关键路径与根目录 Compose 配置检查。CI service 数据库不新增部署 YAML，不进行生产部署。
- [ ] 执行 `rtk go test ./...`、`rtk go vet ./...`、`rtk go test -race -tags=integration ./tests/integration`、前端 test/typecheck/build/e2e 和部署检查；仅在新失败或新修改后重复相关检查。
- [ ] 对照下表记录实际命令/版本/结果；不能将未运行的容器冒烟或一致性测试标为通过。交付时明确仍待外部环境验证的项。
- [ ] 提交 `docs: document local development deployment and acceptance`。

## 规格覆盖与自检

| 规格内容                                  | 任务           |
| ----------------------------------------- | -------------- |
| 单组织、固定栈、无额外中间件              | 1、2、15       |
| 用户/组/角色/权限及最后管理员             | 3、4、6、13    |
| 本地登录、首次改密、统一会话              | 3、5、12       |
| APP/Provider 与预关联身份                 | 7、10、13      |
| Code+PKCE、Discovery/JWKS、claims         | 8、9、14       |
| 并发兑换、撤权与认证来源限制              | 4、9、10       |
| 退出/UserInfo/失效边界                    | 11、14         |
| 密钥保护、限流、CSRF、网络访问限制        | 3、5、10、11   |
| 首页、门户、中英文、主题                  | 11、12、13     |
| 两库迁移与持久化                          | 2、5、8、9、11 |
| 本地 README、根目录单文件 Compose、单副本 | 15、16         |
| 日志、备份、探针、验收                    | 11、14、16     |

自检结论：任务覆盖规格；部署使用根目录单个 docker-compose.yaml，没有部署子目录、环境覆盖 YAML 或新增运行时服务。文件路径、配置前缀、默认 TTL 和端点在任务间一致。关键协议适配以锁定版本真实接口为依据，任务 8 先记录调用顺序，任务 9 再实现原子消费，避免推测库的事务行为。未执行实现或测试，当前没有测试通过的声明。

## 执行交接

计划完成后停在文档阶段。用户明确要求开始实现时，在当前会话按 executing-plans 顺序推进并报告每个阶段的验证证据；若用户另行选择委派，再采用 subagent-driven-development。不把继续审阅或计划确认自动视为部署到外部环境的授权。
