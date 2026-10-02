# OIDC 适配边界

协议层继续使用 `github.com/zitadel/oidc/v3`。Burrow 实现 `op.Storage`、客户端与授权请求接口，并在 HTTP 边界收窄到首版支持的能力。协议库负责标准请求处理和令牌签名；Burrow 负责用户、密码与 TOTP 认证、统一会话和 APP 权限。

| 位置                                 | 约束                                                                            |
| ------------------------------------ | ------------------------------------------------------------------------------- |
| `CreateAuthRequest`                  | 只接受 code flow、已登记客户端与 scopes；默认要求 PKCE S256，Web 可显式允许省略 |
| `client`                             | Web 使用 basic，SPA 无密钥；精确回调、退出 URL                                  |
| `/oidc/login`                        | 校验浏览器绑定事务、会话、用户/APP 状态及当前登录资格                           |
| `SaveAuthCode` / `AuthRequestByCode` | 数据库保存授权码摘要与过期时间                                                  |
| `CreateAccessToken`                  | 事务内重新检查授权状态，一次性消费授权码，禁止并发重复兑换                      |
| `SetUserinfoFromToken`               | 检查 Token、用户状态及对应 APP 的浏览器来源                                     |
| `SigningKey` / `KeySet`              | 加密持久化私钥、发布公钥与 kid，重启不更换密钥                                  |
| `oidc_http.go`                       | 显式 Discovery 能力、请求限制、受保护的退出确认                                 |

不支持的 grant 与扩展显式拒绝，不开放动态注册、Refresh Token、Client Credentials、密码授权或 introspection。Access Token 仅供本站 UserInfo 使用。退出撤销 Burrow 会话，不控制 APP 自身会话；已签发的离线 ID Token 仍按其过期时间处理。

## 应用和密码认证

Applications 登记使用 Burrow 登录的下游客户端（如 Grafana、Harbor、Nightingale）。Burrow 在此充当 IdP，应用使用 Burrow 签发的凭据调用其 OIDC 端点。具有 `applications:write` 权限的管理用户可在创建时填写可选 `clientId`、Web `clientSecret`；省略或空字符串自动生成。Client ID 只接受最多 128 个字母、数字和 `-._~`，必须唯一；Secret 为 16–256 个不含空格的可打印 ASCII 字符，仅存哈希并在创建响应中返回一次。SPA 不接受非空 Secret。Client ID/类型创建后不可修改，更新请求不能提交 Secret；通过独立重置接口生成新密钥。审计不记录明文密钥。无需数据库迁移。

Burrow 使用本地账号密码认证用户，向下游应用提供 OIDC 登录。上游 Provider、外部身份绑定和用户/应用认证来源配置已移除。创建用户必须设置临时密码；密码通过后仅创建五分钟受限事务；强制改密及 MFA 绑定/验证完成前不签发正式会话、OIDC 授权码或 Token。正式会话要求 MFA 完成且匹配当前用户认证版本，授权、换码和 UserInfo 均检查当前会话及用户/应用权限。升级到 schema v4 的前置检查和恢复方式见 [升级说明](../operations/recovery.md#upgrade-to-password-only-authentication)。

## Web 应用 PKCE 兼容

应用字段 `allowWithoutPkce` 默认为 `false`。只有 Administrator 可修改此设置，具有 `applications:write` 的普通角色可以维护应用其他字段。SPA 不允许开启。新建应用省略字段时保持强制 PKCE，编辑时省略字段保留原值；迁移 `002_pkce_compatibility.sql` 为已有应用设置 `false`。设置变化与管理审计同事务提交，事件 `Details` 记录 `allowWithoutPkce.before` 和 `allowWithoutPkce.after`，审计失败回滚修改。

开启后仅允许 Web 授权请求同时省略 challenge 和 method；携带 PKCE 时仍完整验证 S256，拒绝 `plain`、参数不完整、非法 challenge、缺失或错误 verifier。没有 challenge 的换码请求也不能额外提交 verifier。Web 仍必须使用 `client_secret_basic`。精确回调、授权码有效期和原子一次性消费、当前用户/会话/APP/权限检查保持生效。关闭兼容后，未兑换的无 PKCE 授权码立即不能兑换；不追溯撤销已经签发的 Token 或 APP 会话。Discovery 仍只声明 S256。

此选项降低授权码注入防护，用于兼容不能修改的服务端客户端，不提供与 PKCE 等价的保护。客户端应正确绑定和验证 state；采用 nonce 防护时必须在使用令牌前验证其与浏览器事务匹配。它不替客户端完成这些检查，也不增加 Refresh Token 或跨应用退出。

Nightingale v9.1.1 可作为兼容目标：登记为 Web，显式开启此选项，Scopes 设置为 `openid profile email`（去掉默认 `phone`），用户名映射 `preferred_username`、昵称映射 `name`、邮箱映射 `email`。该版本 OIDC 登录未显式使用 PKCE 或 nonce，仍需按部署环境评估剩余风险。源码：[OIDC 客户端](https://github.com/ccfos/nightingale/blob/v9.1.1/pkg/oidcx/oidc.go)。本仓库独立客户端回归不代表已完成真实夜莺联调。

独立互操作客户端见 [examples](../../examples/README.md)，实际覆盖见 [协议验证记录](../testing/oidc-conformance.md)。本项目未进行 OpenID Foundation 认证，不能据此宣称认证通过。

OIDC `amr` 如实返回 `pwd` 和 `otp`，`auth_time` 是全部认证步骤完成时刻；不声明未定义的 ACR。有效 MFA 会话支持 SSO；`prompt=login` 或触发 `max_age` 的请求需重新完成密码与 MFA，检查精确到请求创建时间，避免同一秒内复用旧会话。`prompt=none` 在受限登录期间返回 `login_required`。MFA 重置撤销共享会话、Access Token 和未兑换授权码，UserInfo 及换码重新检查 MFA/版本；下游自有会话和离线 ID Token 保持既有边界。参见 [MFA](mfa-proposal.md)。
