# Burrow

单组织、自建 OIDC 身份认证中心。Go + React/TypeScript + Ant Design，前端使用 Bun；生产使用 PostgreSQL，本地开发支持 SQLite。

提供本地账号登录、预关联外部 OIDC 身份、统一登录会话、用户/组/角色/权限管理、APP/Provider 管理、应用门户和登录统计。界面支持简体中文、英文以及浅色、深色、跟随系统主题。

## 本地开发

工具版本：Go **1.27.1**（SQLite 驱动需要 CGO/C 编译器）、Bun **1.4.2**。数据库可直接使用 SQLite；容器部署需要 Docker Compose v2。

从项目根目录执行：

```sh
cp .env.example .env
openssl rand -base64 32
```

将生成值填入 `.env` 的 `BURROW_MASTER_KEY`。保留 `BURROW_ENV=dev`、`BURROW_DB_DRIVER=sqlite`、`BURROW_DB_DSN=burrow.db`。没有固定管理员密码；主密钥丢失后无法解密已保存的 Provider 和签名密钥。

在后端终端执行（`.env` 由你维护，含空格的值必须用引号包围）：

```sh
set -a
. ./.env
set +a
go mod download
go run ./cmd/burrow migrate
go run ./cmd/burrow admin-init --username admin --password-stdin
```

最后一个命令从标准输入读取一行临时密码，至少 12 位。首次登录必须修改密码。重复初始化会拒绝覆盖现有管理员。

```sh
go run ./cmd/burrow serve
```

在另一个终端启动前端：

```sh
cd web
bun install --frozen-lockfile
bun run dev
```

打开 <http://localhost:5173>。开发代理将 API/OIDC 请求转发到 `127.0.0.1:8080`，并对开发请求统一 Origin。外部客户端和 Provider 回调的 issuer 使用 <http://localhost:8080>。

OIDC 联调建议使用单端口构建模式，以便登录页面和授权端点同域：

```sh
bun run --cwd web build
go run ./cmd/burrow serve
```

打开 <http://localhost:8080>。开发构建默认从 `web/dist` 提供页面。需要独立二进制时：

```sh
bun run --cwd web build
mkdir -p bin
go build -tags embedweb -trimpath -o bin/burrow ./cmd/burrow
bin/burrow serve
```

### 本地使用 PostgreSQL

准备独立数据库，然后修改 `.env`：

```dotenv
BURROW_DB_DRIVER=postgres
BURROW_DB_DSN="host=127.0.0.1 port=5432 user=burrow password=YOUR_PASSWORD dbname=burrow sslmode=disable"
```

重新加载环境，执行 `migrate` 和首次管理员初始化。SQLite 与 PostgreSQL 之间不会自动复制数据；生产数据库使用适当的 TLS 配置。

## Docker Compose

部署仅使用根目录 **`docker-compose.yaml`**。所有命令从项目根目录运行。数据库服务通过 `local-db` profile 选择启用；app 与 migrate 共用镜像和环境。

### 本地构建，使用容器 PostgreSQL

复制并修改 `.env.example`：设置随机 `POSTGRES_PASSWORD`、`BURROW_MASTER_KEY`；将 `BURROW_DB_DSN` 改为：

```dotenv
BURROW_DB_DSN="host=postgres port=5432 user=burrow password=YOUR_PASSWORD dbname=burrow sslmode=disable"
BURROW_IMAGE=burrow:local
BURROW_ENV=dev
BURROW_ISSUER=http://localhost:8080
```

DSN 密码必须与 `POSTGRES_PASSWORD` 相同。推荐使用随机十六进制密码，避免 DSN 转义问题。

```sh
docker compose config --quiet
docker compose build
docker compose --profile local-db up -d --no-build --pull never
docker compose run --rm --no-deps -T app admin-init --username admin --password-stdin
docker compose ps
```

初始化命令从标准输入读取一行临时密码。迁移服务成功后应用才启动；迁移失败则阻止应用启动。打开 <http://localhost:8080>。

### 使用固定镜像或外部 PostgreSQL

构建镜像可自行发布至内部镜像仓库；本项目不默认公开发布镜像。将 `BURROW_IMAGE` 设置为已发布的固定版本或 digest。

```dotenv
BURROW_ENV=prod
BURROW_ISSUER=https://identity.example.com
BURROW_IMAGE=registry.example.com/burrow:0.1.0
BURROW_DB_DSN="host=db.example.com port=5432 user=burrow password=YOUR_PASSWORD dbname=burrow sslmode=verify-full"
```

外部数据库无需 `--profile local-db`。TLS 所需根证书应存在于容器信任链中；私有 CA 可按部署环境挂载。

```sh
docker compose config --quiet
docker compose pull app migrate
docker compose up -d --no-build
docker compose run --rm --no-deps -T app admin-init --username admin --password-stdin
```

应用默认只绑定宿主机 `127.0.0.1:8080`。在现有反向代理配置 HTTPS 并转发至该端口，保留浏览器原始 Origin；公开 issuer 必须与用户访问域名完全一致。生产不接受 HTTP issuer 或 SQLite。PostgreSQL 服务不暴露宿主机端口。

环境隔离使用未提交的 `.env.dev` / `.env.prod` 与不同 project，共用同一个 YAML：

```sh
docker compose --env-file .env.dev --project-name burrow-dev --profile local-db up -d --no-build
docker compose --env-file .env.prod --project-name burrow-prod up -d --no-build
```

环境之间分别配置端口、issuer、数据库与主密钥。升级前备份，停止旧应用，再运行新镜像迁移和启动；镜像回滚不会自动回滚数据库。普通 `docker compose down` 保留数据卷，不要对需保留数据的实例使用 `down -v`。

## 使用与接入

1. 管理员创建用户。可以启用本地密码登录，或只关联外部身份。
2. 创建 APP，选择服务端 Web 或浏览器 SPA，配置精确回调地址和登录入口。
3. 创建普通角色并分配 `app:<id>:login` 权限，再直接分配给用户或通过用户组分配。
4. 用户首页只展示已授权应用；点击卡片前往 APP 的登录入口，由 APP 发起标准 OIDC。

后台权限和 APP 登录资格分别控制。用户的角色与组角色权限取并集，默认拒绝；不管理 APP 内部业务权限。内置管理员角色不可修改，并保护最后一位可本地登录的有效管理员。

### OIDC

Discovery：`<issuer>/.well-known/openid-configuration`。

| 项目             | 值                                                                               |
| ---------------- | -------------------------------------------------------------------------------- |
| Flow             | Authorization Code + PKCE S256                                                   |
| Scope            | `openid profile email`                                                           |
| 服务端客户端认证 | `client_secret_basic`                                                            |
| SPA 客户端认证   | `none`，无客户端密钥                                                             |
| Token 签名       | RS256，JWKS 提供公钥                                                             |
| Endpoints        | `/oidc/authorize`、`/oidc/token`、`/oidc/userinfo`、`/oidc/jwks`、`/oidc/logout` |

客户端必须校验 state、ID Token 的签名、issuer、audience、过期时间和 nonce。回调地址不支持通配符。SPA 必须配置对应的浏览器来源。下游密钥仅创建/轮换时展示一次。

不提供 Refresh Token；到期后重新发起授权，有效的 Burrow 会话可以复用。Access Token 仅用于 Burrow UserInfo，不是通用业务 API 凭据。退出 Burrow 会撤销统一会话，**不会同步退出所有 APP 的会话**；离线 ID Token 和 APP 会话由自身有效期控制。应用禁用、用户禁用或撤权会阻止后续授权和授权码兑换。

### 外部 Provider

创建通用 OIDC Provider，配置 issuer、客户端 ID 和密钥。上游回调登记为：

```text
<burrow-issuer>/api/v1/auth/providers/<provider-id>/callback
```

用户详情中预先关联 Provider 与外部 `sub`。系统不会根据邮箱自动匹配或创建用户。APP 中选择允许的 Providers；本地密码登录使用独立开关。

交互式上游登录发送 `prompt=login` 和 `max_age=0`，要求上游返回有效的 `auth_time`；既有 Burrow 会话的复用在发起上游登录之前完成。Provider 默认只允许公开 HTTPS 地址，并限制请求超时及重定向。

## 配置

| 环境变量                        | 默认值 / 说明                                                                 |
| ------------------------------- | ----------------------------------------------------------------------------- |
| `BURROW_ENV`                    | `dev` 或 `prod`                                                               |
| `BURROW_LISTEN_ADDR`            | `:8080`                                                                       |
| `BURROW_ISSUER`                 | `http://localhost:8080`，生产必须 HTTPS                                       |
| `BURROW_DB_DRIVER`              | `sqlite`；生产/Compose 使用 `postgres`                                        |
| `BURROW_DB_DSN`                 | `burrow.db` 或 PostgreSQL DSN                                                 |
| `BURROW_MASTER_KEY`             | 必填，32 字节随机值的 Base64 编码                                             |
| `BURROW_SESSION_TTL`            | `8h`                                                                          |
| `BURROW_TOKEN_TTL`              | `5m`                                                                          |
| `BURROW_AUTH_CODE_TTL`          | `60s`                                                                         |
| `BURROW_LOGIN_TTL`              | `10m`                                                                         |
| `BURROW_EVENT_RETENTION`        | `2160h`（90 天）                                                              |
| `BURROW_STATIC_DIR`             | 非嵌入开发模式的 `web/dist`                                                   |
| `BURROW_TRUSTED_PROXIES`        | 可信代理 CIDR 列表，逗号分隔；只有来自这些地址的 `X-Forwarded-For` 才用于限流 |
| `BURROW_PROVIDER_ALLOWED_CIDRS` | 明确允许访问的内网 Provider CIDR 列表，默认不允许私有地址                     |

进程内限流与清理按单实例运行；不支持横向扩容。`/healthz` 为进程存活探针，`/readyz` 检查数据库迁移状态；启动还校验签名密钥可以解密。

## 验证

```sh
go test ./...
go vet ./...
bun run --cwd web typecheck
bun run --cwd web test
bun run --cwd web build
```

PostgreSQL 测试需要专用测试数据库以及创建 schema 的权限。测试会创建并清理独立 schema：

```sh
export BURROW_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=burrow password=TEST_PASSWORD dbname=burrow_test sslmode=disable'
go test -race ./internal/burrow -count=1
```

未提供该变量时，默认 Go 测试会明确跳过 PostgreSQL 子测试；这不等于完成双库验证。

浏览器测试自动使用临时 SQLite，构建嵌入前端的二进制并绑定 `localhost:18080`。仅测试用账号数据，结束后清理临时数据库：

```sh
cd web
bun x playwright install chromium
cd ..
bun install --cwd examples/spa-client --frozen-lockfile
sh web/e2e/run.sh
```

使用本机 `prettier --write web/src` 格式化前端，不添加项目内 Prettier 依赖。

独立客户端的注册与运行方式见 [examples/README.md](examples/README.md)。

设计和计划位于 `docs/superpowers/`；另见 [实现结构与版本](docs/development/dependencies.md)、[OIDC 适配](docs/development/oidc-adapter.md)、[验证记录](docs/testing/oidc-conformance.md) 和 [备份恢复](docs/operations/recovery.md)。
