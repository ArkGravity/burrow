# Grafana OIDC 接入验收

以下步骤面向自建 Grafana，示例使用 Burrow `https://sso.example.com` 和 Grafana `https://grafana.example.com`。替换为实际地址。Grafana 使用 Generic OAuth、PKCE S256 和 `client_secret_basic`，不需要开启 Burrow 的 PKCE 兼容例外。

## 部署前检查

- 升级 Burrow 前备份数据库并保留原 master key，按 [恢复与升级说明](recovery.md)操作。先迁移，再 seed，再启动服务；当前 schema 为 v4，升级到 v4 后旧二进制不能直接运行在该数据库上。
- 生产使用 PostgreSQL、HTTPS 和独立的生产凭据。确认 `server.issuer` / `BURROW_ISSUER` 是 Burrow 的实际对外地址。
- 浏览器和 Grafana 后端都必须能访问 Burrow。容器内的 `localhost` 不是 Burrow 地址。
- 检查 `<issuer>/.well-known/openid-configuration`，确认 issuer 和各端点没有指向开发地址。
- 确认 Grafana 版本支持下面的配置项，尤其是 `validate_id_token` / `jwk_set_url`；不要把不支持的配置当成已生效。

## Burrow 配置

管理员在“应用”中创建 Grafana：

| 字段                      | 值                                                |
| ------------------------- | ------------------------------------------------- |
| 名称                      | Grafana                                           |
| 客户端类型                | 服务端 Web                                        |
| 启用                      | 开启                                              |
| 应用登录 URL              | `https://grafana.example.com/login/generic_oauth` |
| Redirect URIs             | `https://grafana.example.com/login/generic_oauth` |
| Post Logout Redirect URIs | 首轮留空                                          |
| Origins                   | 留空                                              |
| 允许不使用 PKCE 登录      | 关闭                                              |
| 本地密码登录              | 开启，用于本地用户验收                            |

保存 Client ID 和一次性显示的 Client Secret。密钥丢失时重置并同步更新 Grafana。

创建时可选填写固定 Client ID 和 Web Client Secret，留空自动生成。Client ID 最多 128 个字母、数字或 `-._~`；Secret 为 16–256 个不含空格的可打印 ASCII 字符。创建后 Client ID 不可修改，编辑表单不修改 Secret；轮换使用“重置密钥”。本地示例见 [独立 Docker Compose](../../examples/local-sso/README.md)，生产应使用独立的随机密钥。

创建有姓名和有效邮箱的普通测试用户，启用账户和本地登录，先完成临时密码修改。创建普通角色 `Grafana Users`，授予对应 `app:<应用 ID>:login` 权限，再直接或通过组分配给测试用户。新用户默认无角色，需要通过普通角色授予应用登录权限；不要只用管理员验证授权。

## Grafana 配置

合并到实际加载的 `grafana.ini` 对应配置段，Linux deb/RPM 通常为 `/etc/grafana/grafana.ini`。不修改 `defaults.ini`，不提交真实密钥。

```ini
[server]
root_url = https://grafana.example.com/

[auth]
disable_login_form = false

[auth.basic]
enabled = true

[users]
auto_assign_org = true
auto_assign_org_role = Viewer

[auth.generic_oauth]
enabled = true
name = Burrow
client_id = YOUR_BURROW_CLIENT_ID
client_secret = YOUR_BURROW_CLIENT_SECRET
scopes = openid profile email
auth_url = https://sso.example.com/oidc/authorize
token_url = https://sso.example.com/oidc/token
api_url = https://sso.example.com/oidc/userinfo
auth_style = InHeader
use_pkce = true
use_refresh_token = false
login_attribute_path = preferred_username
name_attribute_path = name
email_attribute_path = email
allow_sign_up = true
auto_login = false
skip_org_role_sync = true
allow_assign_grafana_admin = false
validate_id_token = true
jwk_set_url = https://sso.example.com/oidc/jwks
```

首次登录在 Grafana 创建本地用户记录，默认组织角色为 Viewer。Burrow 当前不输出 groups/roles 声明，由 Grafana 维护业务角色；Burrow 管理员不会自动成为 Grafana 管理员。保留本地管理员登录，便于配置失败时恢复。

子路径部署例如 `/grafana/` 时，修改 `root_url` 并按部署设置 `serve_from_sub_path`，Burrow 登录 URL 和回调改为完整的 `/grafana/login/generic_oauth` 地址。域名、协议、端口、路径必须精确匹配。

Docker 可以使用 `GF_<配置段>_<配置项>` 环境变量覆盖，例如 `GF_AUTH_GENERIC_OAUTH_USE_PKCE=true`、`GF_AUTH_GENERIC_OAUTH_AUTH_STYLE=InHeader`。改变 Compose 环境变量后用 `docker compose up -d grafana` 重新创建容器，仅 restart 不会更新环境变量。原生服务修改 INI 后重启实际 Grafana 服务。

## 验收步骤

1. 隐私窗口打开 Grafana 登录页，确认出现 Burrow 登录按钮。
2. 点击按钮，用有应用权限的普通 Burrow 用户登录，返回 Grafana。
3. 核对用户名、姓名、邮箱和首次分配的 Viewer 角色。
4. 只退出 Grafana，保留 Burrow 会话，再次 OIDC 登录，应复用 SSO 会话。
5. 从 Burrow 应用门户点击 Grafana，确认能完成登录。
6. 新隐私窗口使用没有该应用权限的普通用户，确认登录被拒绝。
7. 测试撤权或禁用时，清除 Grafana 会话后重新发起 OIDC 登录。已有 Grafana 会话不会因 Burrow 撤权而自动失效。

首轮不设置联动退出。Grafana 和 Burrow 分别管理自己的会话，Burrow 不提供 Refresh Token 或跨应用同步退出。

## 常见问题

| 现象               | 检查                                                  |
| ------------------ | ----------------------------------------------------- |
| 没有 Burrow 按钮   | 配置是否加载、enabled、服务是否重启或容器是否重建     |
| S256 PKCE required | `use_pkce=true` 是否生效                              |
| invalid_client     | Client ID、Secret、InHeader、是否重置密钥             |
| invalid_scope      | 仅 `openid profile email`，不加 offline_access/groups |
| 回调不匹配         | root_url 与 Burrow redirect URI 完全一致              |
| access_denied      | 账户/应用启用状态、应用权限、认证来源                 |
| 用户开户失败       | 用户邮箱、email Scope、已有用户冲突                   |
| 网络/证书错误      | Grafana 后端访问 Burrow 的 DNS、连接、证书信任        |
| 签名验证失败       | Grafana 版本、JWKS URL、公钥可访问性                  |

查看正常级别的 Grafana 错误日志，不记录或分享 Secret、授权码、Token。此前本地验证使用独立 Web/SPA 客户端，尚未实际验收 Grafana、Harbor 或 Nightingale；远端生产验收由部署环境另行记录。

本地示例曾定位到 `unable to create user: user not found`：Burrow 的 `admin` 经 `preferred_username` 映射后，与 Grafana 已有本地 `admin` 重名，而账号未绑定该 OAuth 身份。使用不同用户名且已获应用权限的普通用户验收。`token is not in JWT format` 是 Grafana 尝试解析不透明 Access Token 的警告，需要结合后续错误判断；本次失败发生在用户同步阶段。

参考：[Grafana Generic OAuth](https://grafana.com/docs/grafana/latest/setup-grafana/configure-access/configure-authentication/generic-oauth/)、[Grafana 配置](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/)、[Burrow 协议边界](../development/oidc-adapter.md)。
