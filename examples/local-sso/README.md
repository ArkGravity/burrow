# 本地 Burrow + Grafana SSO

独立 Docker Compose 示例：Burrow 使用 dev 环境和 SQLite，Grafana 使用 Generic OAuth、PKCE S256 和 Basic 客户端认证，Nginx 代理两个域名。根目录 `docker-compose.yml` 不参与启动。

## 启动

确保 Docker 已启动、宿主机 80 端口空闲，并将以下域名解析到本机：

```text
127.0.0.1 sso.yakir.top grafana.yakir.top
```

在仓库根目录运行：

```bash
docker compose -f examples/local-sso/docker-compose.yml config --quiet
docker compose -f examples/local-sso/docker-compose.yml up -d --build
docker compose -f examples/local-sso/docker-compose.yml ps -a
```

启动顺序为数据卷权限初始化、迁移、seed、Burrow 服务；Nginx 等待 Burrow 和 Grafana 健康后启动。没有自动创建应用或生成应用密钥的脚本。

| 服务    | 地址                     | 本地示例管理员                             |
| ------- | ------------------------ | ------------------------------------------ |
| Burrow  | http://sso.yakir.top     | `admin` / `Burrow-development-admin-2026`  |
| Grafana | http://grafana.yakir.top | `admin` / `Grafana-development-admin-2026` |

首次登录 Burrow 后按提示修改临时密码。Seed 管理员邮箱为 `admin@example.test`，已有数据库中的账号不会被 seed 重置。

此示例使用 HTTP 和公开开发凭据，仅绑定 `127.0.0.1:80`。Burrow 和 Grafana 不直接发布宿主机端口。Docker 网络中两个域名都指向 Nginx，Grafana 访问 token、UserInfo 和 JWKS 时也使用对外 issuer `http://sso.yakir.top`。

## 手动创建应用

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

## 运维命令

```bash
# 检查代理配置
docker compose -f examples/local-sso/docker-compose.yml exec nginx nginx -t
# 停止并保留数据
docker compose -f examples/local-sso/docker-compose.yml down
# 恢复启动
docker compose -f examples/local-sso/docker-compose.yml up -d
# 修改 Burrow 源码后重建
docker compose -f examples/local-sso/docker-compose.yml up -d --build
```

Compose 项目名为 `burrow-local-sso`，SQLite 数据库、签名密钥和用户保存在 `burrow-local-sso_burrow-data` 卷；Grafana 数据保存在 `burrow-local-sso_grafana-data`。不要将这些开发卷用于生产。

仅在明确需要清空该示例的全部账号、应用、密钥和 Grafana 数据时运行：

```bash
docker compose -f examples/local-sso/docker-compose.yml down -v
```

若浏览器自动升级为 HTTPS，确认该域名是否已有 HSTS 记录；本示例仅提供 HTTP，不监听 443。

配置依据：[Grafana Generic OAuth](https://grafana.com/docs/grafana/latest/setup-grafana/configure-access/configure-authentication/generic-oauth/)、[Docker 网络别名](https://docs.docker.com/reference/compose-file/networks/)。镜像固定为 Grafana `13.2.3` 和 Nginx `1.30.5-alpine`。
