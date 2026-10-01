# 本地 Burrow + Grafana + Nightingale + Harbor SSO

独立 Docker Compose 示例：Burrow 使用 dev 环境和 SQLite，Grafana、Harbor 使用 OIDC 和 PKCE S256，Nightingale 使用 OIDC 和单应用免 PKCE 兼容，Nginx 代理四个域名。Harbor 使用官方 prepare 镜像生成配置，以独立 Compose 项目加入同一个 SSO 网络。根目录 `docker-compose.yml` 不参与启动。

2026-10-01，用户确认 Grafana、Nightingale 和 Harbor 的本地 OIDC 网页登录均验收通过。夜莺门户根路径可能仍需再点击一次 SSO 按钮，本阶段接受此行为。该结果来自用户实际操作反馈，不代表本轮重复自动化浏览器测试、生产部署验证或官方 OIDC 认证；详细记录见 [验证报告](../../docs/testing/oidc-conformance.md)。

## 启动

确保 Docker 已启动、宿主机 80 端口空闲，并将以下域名解析到本机：

```text
127.0.0.1 sso.yakir.top grafana.yakir.top n9e.yakir.top harbor.yakir.top
```

在仓库根目录运行：

```bash
docker compose -f examples/local-sso/docker-compose.yml config --quiet
docker compose -f examples/local-sso/docker-compose.yml up -d --build
docker compose -f examples/local-sso/docker-compose.yml ps -a
# 需要 Harbor 时再启动；要求 Python 3 和支持 !reset 的 Docker Compose 2.24.4+
python3 examples/local-sso/harbor/deploy.py up
python3 examples/local-sso/harbor/deploy.py ps
```

启动顺序为数据卷权限初始化、迁移、seed、Burrow 服务；Nginx 等待 Burrow 和 Grafana 健康后启动。Nightingale 独立启动，Harbor 按需单独启动。首次部署后，使用各自本地管理员在 Nightingale 和 Harbor UI 中手动配置 SSO，步骤与示例见下文。不会自动创建 Burrow 应用或生成应用密钥。

| 服务        | 地址                     | 本地示例管理员                             |
| ----------- | ------------------------ | ------------------------------------------ |
| Burrow      | http://sso.yakir.top     | `admin` / `Burrow-development-admin-2026`  |
| Grafana     | http://grafana.yakir.top | `admin` / `Grafana-development-admin-2026` |
| Nightingale | http://n9e.yakir.top     | `root` / `root.2020`                       |
| Harbor      | http://harbor.yakir.top  | `admin` / `Harbor-development-admin-2026`  |

首次登录 Burrow 后按提示修改临时密码。Seed 管理员邮箱为 `admin@example.test`，已有数据库中的账号不会被 seed 重置。

此示例使用 HTTP 和公开开发凭据，浏览器入口仅绑定 `127.0.0.1:80`。Burrow、Grafana、Nightingale 和 Harbor HTTP 服务不直接发布宿主机端口；Harbor 保留官方日志组件的 `127.0.0.1:1514` syslog 端口。Docker 网络中四个域名都指向 Nginx，下游访问 OIDC 端点时也使用对外 issuer `http://sso.yakir.top`。

## 手动创建 Grafana 应用

在 Burrow 浏览器管理界面的“应用”中创建：

| 字段                                          | 值                                             |
| --------------------------------------------- | ---------------------------------------------- |
| 名称                                          | Grafana                                        |
| 客户端类型                                    | Web                                            |
| 启用、本地密码登录                            | 开启                                           |
| 应用登录 URL                                  | `http://grafana.yakir.top/login/generic_oauth` |
| Redirect URIs                                 | `http://grafana.yakir.top/login/generic_oauth` |
| 允许不使用 PKCE                               | 关闭                                           |
| Post Logout Redirect URIs、Origins、Providers | 留空                                           |

[grafana.ini](grafana.ini) 预配置 Client ID `grafana-example` 和 Client Secret `grafana-example-secret`。

创建应用时填写 Client ID `grafana-example`、Client Secret `grafana-example-secret`，即可匹配此示例配置。两项留空仍自动生成；若使用生成的凭据，需要将其手动填入 `grafana.ini` 并重启 Grafana：

```bash
docker compose -f examples/local-sso/docker-compose.yml restart grafana
```

本示例不会写入数据库来绕过应用管理 API，也不会自动创建应用。

## 登录验收

1. 先使用 Burrow 管理员完成临时密码修改和应用创建，确认 Grafana 与 Burrow 的应用凭据一致。
2. 创建有姓名和非空邮箱的普通 Burrow 用户，用户名与 Grafana 本地 `admin` 区分；创建普通角色并授予对应应用的登录权限，再分配给用户。新用户默认无角色，需要通过普通角色授予应用权限。
3. 打开 Grafana，点击 “Burrow” 登录按钮，使用上述普通用户完成授权并返回 Grafana。
4. 使用隐私窗口验证普通用户能够登录，未获应用权限的用户被拒绝。
5. 在 Burrow 应用门户点击 Grafana，验证再次登录复用 Burrow 会话。

Grafana 首次 OIDC 登录可自动建立自己的用户记录，默认组织角色为 Viewer；Burrow 管理员身份不会授予 Grafana 管理员。两者会话分别管理，退出一方不会自动退出另一方。详细检查和故障排查见 [Grafana 接入指南](../../docs/operations/grafana.md)。

本示例两边都有 `admin`。使用 Burrow 的 `admin` 首次 OIDC 登录时，可能因 Grafana 已有同名本地账号而出现 `unable to create user: user not found`。保留 Grafana 本地管理员，使用另一个已获应用权限的 Burrow 用户验收。

## Nightingale 配置与验收

固定使用 Nightingale `9.1.1`，SQLite 数据持久化到 `/data/n9e.db`，使用进程内 `miniredis`；其会话和未完成的 SSO 事务在夜莺重启后失效。本次登录示例关闭内置 TSDB 和 Ibex，不需要外置数据库、Redis、Prometheus 或采集器。

### config.toml 能否初始化 SSO

[n9e.config.toml](n9e.config.toml) 只读挂载到 `/app/etc/config.toml`，覆盖 HTTP、数据库、Redis 和日志等运行参数。只挂载这个文件会保留镜像内其他配置和资源。

**v9.1.1 不会读取运行配置中的 `[OIDC]` 或 `[HTTP.OIDC]` 来启用 SSO。** 该版本的 SSO 配置保存在夜莺自己的 `sso_config` 表中。本示例通过夜莺单点登录页面手动维护，直接粘贴下面的 TOML 示例即可。

### 在夜莺 UI 中手动填写 TOML

1. 打开 http://n9e.yakir.top，使用夜莺本地管理员 `root` 登录。
2. 进入“系统配置 → 单点登录”，对应页面 `/system/sso-settings`，选择 OIDC 配置。
3. 将以下独立 TOML 示例粘贴到配置编辑器并保存；已有 OIDC 记录时编辑该记录。
4. 退出夜莺或使用隐私窗口重新打开登录页，确认出现 “Sign in with Burrow”。

```toml
Enable = true
DisplayName = "Sign in with Burrow"
RedirectURL = "http://n9e.yakir.top/callback"
SsoAddr = "http://sso.yakir.top"
SsoLogoutAddr = ""
ClientId = "nightingale-example"
ClientSecret = "nightingale-example-secret"
CoverAttributes = true
DefaultRoles = ["Standard"]
Scopes = ["openid", "profile", "email"]

[Attributes]
Username = "preferred_username"
Nickname = "name"
Phone = ""
Email = "email"
```

以上 TOML 内容供单点登录页面使用；不要把整个运行配置 `n9e.config.toml` 粘贴进去，也不要给此内容加 `[OIDC]` 外层。保存后参数持久化到夜莺数据库，重启和再次运行 Compose 不会覆盖页面中的配置。已有配置的环境可以直接继续使用，无须重新填写。

### 在 Burrow 中创建 Nightingale 应用

| 字段                                          | 值                              |
| --------------------------------------------- | ------------------------------- |
| 名称                                          | Nightingale                     |
| 客户端类型                                    | Server-side Web                 |
| Client ID                                     | `nightingale-example`           |
| Client Secret                                 | `nightingale-example-secret`    |
| 启用、本地密码登录                            | 开启                            |
| 应用登录 URL                                  | `http://n9e.yakir.top/`         |
| Redirect URIs                                 | `http://n9e.yakir.top/callback` |
| 允许不使用 PKCE                               | **开启**                        |
| Post Logout Redirect URIs、Origins、Providers | 留空                            |

夜莺 v9.1.1 的授权码登录没有发送 PKCE，因此仅为这个 Web 应用开启兼容选项，Grafana 保持关闭。配置显式请求 `openid profile email`，没有 `phone` 或 `offline_access`；用户名/姓名/邮箱分别映射 `preferred_username` / `name` / `email`。

1. 使用 Burrow 管理员创建以上应用。
2. 给 `logic` 已持有的普通角色添加 Nightingale 登录权限，或创建普通角色后分配给 `logic`/其用户组；无需添加管理权限。
3. 打开 http://n9e.yakir.top，点击 “Sign in with Burrow”，使用 `logic` 完成登录；已有 Burrow 会话可以复用。
4. 确认夜莺用户为 `logic`，其默认业务角色为夜莺的 `Standard`。首次登录时夜莺会建立自己的用户记录，Burrow 的角色不会映射为夜莺管理员。
5. 使用隐私窗口验证没有该应用权限的用户被拒绝。权限移除后需要退出夜莺并重新发起 OIDC 登录，已建立的夜莺会话不会自动撤销。

夜莺使用自身会话，示例未配置联合登出。用户已验收 `logic` 通过登录页的 “Sign in with Burrow” 成功登录。当前 Burrow 门户仍打开夜莺根路径：没有有效夜莺会话时，需要再点击一次 SSO 按钮；已部署前端不会仅因存在 Burrow 会话而自动发起登录。本阶段接受此行为，不增加自动登录入口或替换夜莺前端。

配置依据：[v9.1.1 运行配置结构](https://github.com/ccfos/nightingale/blob/v9.1.1/conf/conf.go)、[SSO 初始化](https://github.com/ccfos/nightingale/blob/v9.1.1/center/sso/init.go)、[管理 API](https://github.com/ccfos/nightingale/blob/v9.1.1/center/router/router_login.go)、[OIDC 客户端](https://github.com/ccfos/nightingale/blob/v9.1.1/pkg/oidcx/oidc.go)。

## Harbor 配置与验收

固定使用 Harbor `v2.15.2` 的官方 prepare 和服务镜像，部署 log、PostgreSQL、Valkey、registry、registryctl、core、portal、jobservice、proxy 九个基础组件，不启用 Trivy 或 exporter。Harbor 没有 SQLite 部署方式，因此使用它自己的 PostgreSQL；Burrow 仍使用 SQLite。

[harbor/deploy.py](harbor/deploy.py) 调用官方 prepare，生成内部服务配置和 Compose，然后合并网络覆盖文件。它仅负责部署；OIDC 参数在 Harbor UI 中手动保存。Harbor 项目名为 `burrow-local-sso-harbor`，复用 `burrow-local-sso_default` 网络，由现有 Nginx 转发到 `harbor-proxy:8080`。需先启动基础 SSO Compose。官方镜像在本示例中固定使用 `linux/amd64`，Apple Silicon 通过 Docker 的架构模拟运行；本机四核、约 6 GiB Docker 内存已完成启动验证。

### Harbor 文件用途

| 文件或目录                                                        | 用途                                                       | 是否提交 |
| ----------------------------------------------------------------- | ---------------------------------------------------------- | -------- |
| [harbor.yml](harbor/harbor.yml)                                   | 官方 prepare 的部署参数模板                                | 是       |
| [docker-compose.override.yml](harbor/docker-compose.override.yml) | 本地平台、容器名、网络和端口覆盖                           | 是       |
| [deploy.py](harbor/deploy.py)                                     | 调用 prepare，合并 Compose 并管理启动/停止                 | 是       |
| [../.gitignore](.gitignore)                                       | 上层示例统一忽略生成配置、数据、日志和 Python 缓存         | 是       |
| `runtime/`                                                        | 官方生成的完整 Compose、展开路径后的输入文件及内部服务配置 | 否       |
| `data/`、`logs/`                                                  | 持久数据库、内部密钥、registry 数据和运行日志              | 否       |

`runtime/docker-compose.yml` 是官方生成的基础文件，定义九个 Harbor 组件、内部挂载、日志和依赖；`docker-compose.override.yml` 只描述本示例的差异。部署脚本显式以 `-f runtime/docker-compose.yml -f docker-compose.override.yml` 合并两者，最终启动的是同一个 Harbor 项目。这不是两套部署。

保留两层可以在重新运行 prepare 时保留本地网络与端口配置，也避免将本机绝对路径和生成凭据提交到仓库。无需手动修改 `runtime/` 文件。Harbor 目录中的三个维护文件都有独立用途，忽略规则由上层示例统一维护，自动 SSO 配置相关文件已移除。

### 在 Harbor UI 中手动配置 OIDC

运行 `python3 examples/local-sso/harbor/deploy.py up` 后，全新数据库使用本地认证。`harbor.yml` 只配置域名、`external_url`、本地初始管理员和数据库等部署参数，不提供 OIDC 字段。

打开 http://harbor.yakir.top，使用本地 `admin` 登录，进入“配置管理 / Configuration → 认证 / Authentication”，按以下示例逐项填写：

| 字段                        | 值                              |
| --------------------------- | ------------------------------- |
| 认证模式                    | OIDC                            |
| OIDC Provider Name          | `Burrow`                        |
| OIDC Endpoint               | `http://sso.yakir.top`          |
| OIDC Client ID              | `harbor-example`                |
| OIDC Client Secret          | `harbor-example-secret`         |
| OIDC Scope                  | `openid,profile,email`          |
| Verify Certificate          | 开启；本地 HTTP 不涉及 TLS 证书 |
| Automatic onboarding        | 开启                            |
| Username Claim              | `preferred_username`            |
| Group Claim、Admin Group    | 留空                            |
| Primary authentication mode | 关闭，保留本地管理员登录入口    |
| OIDC Logout                 | 关闭                            |

保存前可使用页面的测试按钮检查 Provider 连接，填写完成后保存。退出 Harbor 或使用隐私窗口打开登录页，确认出现 OIDC 登录按钮；完整授权仍须完成 Burrow 配置。配置持久化到 Harbor 数据库，重启和再次执行 `deploy.py up` 不会覆盖 UI 设置。此前已配置 OIDC 的环境继续保留原值且可在 UI 编辑，无须重新填写。

全新 Harbor 只有本地 `admin`，可切换认证模式；如果已有其他本地用户，Harbor 对认证模式切换有限制。本示例保留管理员作为维护入口。

### 在 Burrow 中创建 Harbor 应用

| 字段                                          | 值                                                      |
| --------------------------------------------- | ------------------------------------------------------- |
| 名称                                          | Harbor                                                  |
| 客户端类型                                    | Server-side Web                                         |
| Client ID                                     | `harbor-example`                                        |
| Client Secret                                 | `harbor-example-secret`                                 |
| 启用、本地密码登录                            | 开启                                                    |
| 应用登录 URL                                  | `http://harbor.yakir.top/c/oidc/login?redirect_url=%2F` |
| Redirect URIs                                 | `http://harbor.yakir.top/c/oidc/callback`               |
| 允许不使用 PKCE                               | **关闭**                                                |
| Post Logout Redirect URIs、Origins、Providers | 留空                                                    |

Harbor 原生发送 PKCE S256，无须兼容豁免。应用登录 URL 使用 Harbor 的 OIDC 发起路由，`redirect_url=%2F` 表示成功后返回 Harbor 首页；此入口已验证会跳转到 Burrow。Harbor 的 `external_url` 保证回调使用以上公开地址。

1. 在 Burrow 手动创建应用，给 `logic` 的普通角色增加 Harbor 登录权限，或通过用户组分配角色。
2. 打开 http://harbor.yakir.top，点击 “LOGIN VIA OIDC PROVIDER” / Burrow 登录按钮，使用 `logic` 完成授权。
3. 确认 Harbor 显示当前用户 `logic`。启用自动开户和 Username Claim 后，首次 OIDC 登录会在 Harbor 建立用户记录。
4. 退出 Harbor，再从 Burrow 门户点击 Harbor，确认复用 Burrow 会话并返回 Harbor 首页。
5. 使用未授予 Harbor 登录权限的 Burrow 用户验证拒绝登录；已有 Harbor 会话需要先退出再发起 OIDC。

这里仅验收网页 OIDC 登录，不配置 Docker/Helm CLI 或自动项目授权。Burrow 的角色决定能否登录 Harbor，Harbor 项目成员和业务角色在 Harbor 内单独分配；Burrow 管理员不会自动成为 Harbor 管理员。应用会话分别管理，示例不启用联合登出。

配置依据：[Harbor OIDC 配置](https://goharbor.io/docs/2.15.0/administration/configure-authentication/oidc-auth/)、[v2.15.2 部署配置模板](https://github.com/goharbor/harbor/blob/v2.15.2/make/harbor.yml.tmpl)。

## 运维命令

```bash
# 检查代理配置
docker compose -f examples/local-sso/docker-compose.yml exec nginx nginx -t
# 修改夜莺运行配置后重启
docker compose -f examples/local-sso/docker-compose.yml restart nightingale
# 查看 Harbor 状态、静默检查配置
python3 examples/local-sso/harbor/deploy.py ps
python3 examples/local-sso/harbor/deploy.py config
# 修改 harbor.yml 部署参数后重新生成并启动
python3 examples/local-sso/harbor/deploy.py prepare
python3 examples/local-sso/harbor/deploy.py up
# 停止并保留数据；先停止 Harbor，再关闭它依赖的基础网络
python3 examples/local-sso/harbor/deploy.py down
docker compose -f examples/local-sso/docker-compose.yml down
# 恢复启动
docker compose -f examples/local-sso/docker-compose.yml up -d
python3 examples/local-sso/harbor/deploy.py up
# 修改 Burrow 源码后重建
docker compose -f examples/local-sso/docker-compose.yml up -d --build
```

Compose 项目名为 `burrow-local-sso`，SQLite 数据库、签名密钥和用户保存在 `burrow-local-sso_burrow-data` 卷；Grafana 数据保存在 `burrow-local-sso_grafana-data`，夜莺数据库保存在 `burrow-local-sso_nightingale-data`。增加夜莺不需要清空现有 Burrow/Grafana 数据。不要将这些开发卷用于生产。

Harbor 数据和内部密钥保存在 `harbor/data/`，日志保存在 `harbor/logs/`，生成的 Compose 和配置在 `harbor/runtime/`；本示例的 [.gitignore](.gitignore) 按目录统一忽略这三类本地文件和 Python 字节码缓存。Burrow、Grafana 和夜莺的数据在 Docker 命名卷中，不会生成仓库文件。停止 Harbor 不会删除本地目录。重新生成配置复用已有内部密钥，不要将生成配置、密钥或数据库提交到仓库。基础 Compose 的 `down -v` 不会删除 Harbor 的这些目录。

仅在明确需要清空该示例的全部账号、应用、密钥、Grafana 和夜莺数据时运行：

```bash
docker compose -f examples/local-sso/docker-compose.yml down -v
```

若浏览器自动升级为 HTTPS，确认该域名是否已有 HSTS 记录；本示例仅提供 HTTP，不监听 443。

配置依据：[Grafana Generic OAuth](https://grafana.com/docs/grafana/latest/setup-grafana/configure-access/configure-authentication/generic-oauth/)、[Docker 网络别名](https://docs.docker.com/reference/compose-file/networks/)。镜像固定为 Grafana `13.2.3`、Nightingale `9.1.1`、Harbor `v2.15.2` 和 Nginx `1.30.5-alpine`。
