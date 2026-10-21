# 安装 Burrow

[English](installation.md) · 简体中文

安装建议使用正式发布版，优先选择 Docker Compose。发布镜像和 Linux 二进制均已包含前端，
运行时无需 Go 或 Bun。源码开发见[开发指南](development/getting-started.md)。

## 选择版本与校验

从 [Releases](https://github.com/ArkGravity/burrow/releases) 选择版本并阅读版本说明。
Compose 下载部署包，原生运行下载对应架构的二进制包，同时下载同版本的
`SHA256SUMS`、`IMAGES.txt` 和 `INSTALL.md`。附件中的安装指南已经渲染为该版本的具体步骤。

本文示例固定 `v1.0.0`，选择其他版本时统一替换版本值，并固定镜像 tag 或摘要。
本文使用 amd64/arm64 发布布局；历史 v0.1.0 仅支持 amd64，请按该版本的安装附件操作。

在使用 GNU coreutils 的 Linux 上，解压前执行：

```bash
sha256sum --ignore-missing --check SHA256SUMS
```

该命令只校验当前目录中已下载的文件，确认选定的压缩包显示 `OK`。
下载所有列出的附件后，也可用 `sha256sum --check SHA256SUMS` 校验完整列表。
其他系统使用对应的 SHA-256 工具与校验文件中的条目比对。
`IMAGES.txt` 提供镜像索引及各架构的摘要。

## Docker Compose

准备 Docker Engine 和 Compose v2，下载 `burrow_v1.0.0_deploy.tar.gz` 后解压：

```bash
BURROW_RELEASE=v1.0.0
tar -xzf "burrow_${BURROW_RELEASE}_deploy.tar.gz"
cd "burrow_${BURROW_RELEASE}_deploy"
cp .env.example .env
openssl rand -base64 32 # 主密钥
openssl rand -hex 24    # PostgreSQL 密码
openssl rand -hex 24    # 初始管理员密码
```

为新生产数据库编辑 `.env`：

| 配置                              | 值                                                 |
| --------------------------------- | -------------------------------------------------- |
| `BURROW_ENV`                      | `prod`                                             |
| `BURROW_ISSUER`                   | 实际 HTTPS 地址，例如 `https://sso.example.com`    |
| `BURROW_MASTER_KEY`               | 生成的 Base64 主密钥                               |
| `POSTGRES_PASSWORD`               | 生成的数据库密码                                   |
| `BURROW_DB_DSN`                   | 使用同一数据库密码；内置数据库保留 `host=postgres` |
| `BURROW_BOOTSTRAP_ADMIN_PASSWORD` | 独立生成的初始管理员密码                           |
| `BURROW_TRUSTED_PROXIES`          | 需要转发客户端地址时，填写实际受信任的代理 CIDR    |
| `BURROW_IMAGE`                    | 保留部署包中的固定版本，或固定其镜像索引摘要       |

私密保存 `.env`。配置 TLS 反向代理，将请求转发到本机回环地址的 8080 端口并保留浏览器 Origin。
只信任实际使用的代理 CIDR，issuer 必须与浏览器和 OIDC 客户端访问的公开地址一致。
内置 Compose 不向宿主机暴露 PostgreSQL 端口。

部署包使用 `ghcr.io/arkgravity/burrow:v1.0.0`，同版本也可从
`docker.io/logic3579/burrow:v1.0.0` 获取。Docker 自动选择 Linux amd64 或 arm64。
部署包使用根目录的 [docker-compose.yml](../docker-compose.yml)，不包含 Dockerfile。

```bash
docker compose --profile local-db config --quiet
docker compose --profile local-db pull
docker compose --profile local-db up -d --no-build --pull never
docker compose --profile local-db ps
curl --fail http://127.0.0.1:8080/readyz
```

Compose 按迁移、初始化、服务启动的顺序执行。使用外部 PostgreSQL 时，修改 DSN 的地址、
凭据和 TLS 设置，并在所有命令中省略 `--profile local-db`。
详见[Compose 配置](development/configuration.md#compose)。

## 首次登录与 MFA

访问配置的 HTTPS issuer，使用 `admin` 和 `BURROW_BOOTSTRAP_ADMIN_PASSWORD` 登录，
按提示修改临时密码。修改了 `BURROW_BOOTSTRAP_ADMIN_USERNAME` 时使用对应用户名。
Bootstrap 只创建初始管理员；重复 seed 保留已有账户、密码和授权关系。

MFA 默认关闭。需要所有账户（含管理员）使用 TOTP 时，在 `.env` 中设置
`BURROW_MFA_ENABLED=true`，然后重建应用容器：

```bash
docker compose --profile local-db up -d --no-build --pull never --force-recreate app
```

用户随后需在登录时绑定或验证认证器。开启 MFA 后，密码登录的会话需重新登录；关闭时保留已有绑定。
详见[MFA 策略](development/configuration.md#global-mfa-policy)与
[MFA 恢复](operations/recovery.md#mandatory-mfa-upgrade-and-recovery)。
登录后按 [README 的应用接入步骤](../README_zh.md#接入应用)配置角色与下游应用。

## 独立 Linux 二进制

x86-64 选择 `burrow_v1.0.0_linux_amd64.tar.gz`，AArch64 选择
`burrow_v1.0.0_linux_arm64.tar.gz`。二进制要求 glibc 2.36+，例如 Debian 12；
不支持 Alpine/musl 或更旧的 glibc。解压后进入对应目录并复制配置：

```bash
./burrow version
cp configs/config.yaml configs/config.local.yaml
```

生产环境在本地 YAML 中设置 `env: prod`、HTTPS `server.issuer`、PostgreSQL 连接、
独立的 `security.master_key` 与 `bootstrap.admin_password`。
需要 MFA 时设置 `security.mfa_enabled: true`。原始默认配置使用 SQLite 和公开开发凭据。
所有命令统一使用同一配置：

```bash
./burrow migrate --config configs/config.local.yaml
./burrow seed --config configs/config.local.yaml
./burrow serve --config configs/config.local.yaml
```

原生命令不加载 `.env`。使用服务管理器运行并配置 TLS 反向代理，首次登录使用 YAML 中的用户名
和临时密码，按前面的步骤修改密码。加载、校验与覆盖规则见[配置参考](development/configuration.md)。

## 停止、升级与恢复

```bash
docker compose exec app burrow version
docker compose --profile local-db down
```

停止服务时保留数据库卷。升级期间保留数据库、原始主密钥、配置和 Compose 项目名；
需要保留数据时不要使用 `down --volumes`。新主密钥无法解密已有数据库中的签名密钥或 TOTP 密钥。

升级前备份并阅读目标版本说明，停止旧服务，选择新镜像或二进制，依次执行迁移、初始化和服务启动。
之后重新验证管理员与已配置的下游 OIDC/SAML 登录。切换旧镜像不能直接回滚数据库迁移。

v1.0.0 使用 schema v6。从 v0.1.x 升级会应用迁移 006，保留已有 OIDC 数据、授权和密钥，
并初始化独立加密的 SAML 签名密钥。旧会话使用 SAML 时可能需要重新认证。
迁移完成后旧版二进制不能直接使用 v6 数据库，回滚需恢复升级前备份；
SAML 证书维护与 Jenkins 信任更新见[恢复指南](operations/recovery.md#saml-证书与-schema-v6)。

v0.1.0/v0.1.1 强制 MFA，v0.1.2 及以后默认关闭；要保留原有 MFA 要求，需在新版启动前显式开启。
旧 Provider 数据库升级前必须满足密码认证迁移的前置要求。
完整步骤见[备份、升级与恢复](operations/recovery.md)。
