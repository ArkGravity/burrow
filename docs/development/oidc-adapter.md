# OIDC 适配边界

协议层继续使用 `github.com/zitadel/oidc/v3`。Burrow 实现 `op.Storage`、客户端与授权请求接口，并在 HTTP 边界收窄到首版支持的能力。协议库负责标准请求处理和令牌签名；Burrow 负责用户、统一会话、认证来源和 APP 权限。

| 位置                                 | 约束                                                       |
| ------------------------------------ | ---------------------------------------------------------- |
| `CreateAuthRequest`                  | 只接受 code flow、PKCE S256、已登记客户端与 scopes         |
| `client`                             | Web 使用 basic，SPA 无密钥；精确回调、退出 URL             |
| `/oidc/login`                        | 校验浏览器绑定事务、会话、用户/APP 状态及当前登录资格      |
| `SaveAuthCode` / `AuthRequestByCode` | 数据库保存授权码摘要与过期时间                             |
| `CreateAccessToken`                  | 事务内重新检查授权状态，一次性消费授权码，禁止并发重复兑换 |
| `SetUserinfoFromToken`               | 检查 Token、用户状态及对应 APP 的浏览器来源                |
| `SigningKey` / `KeySet`              | 加密持久化私钥、发布公钥与 kid，重启不更换密钥             |
| `oidc_http.go`                       | 显式 Discovery 能力、请求限制、受保护的退出确认            |

不支持的 grant 与扩展显式拒绝，不开放动态注册、Refresh Token、Client Credentials、密码授权或 introspection。Access Token 仅供本站 UserInfo 使用。退出撤销 Burrow 会话，不控制 APP 自身会话；已签发的离线 ID Token 仍按其过期时间处理。

上游使用同库 RP 实现，但拥有独立的浏览器绑定事务、state、nonce 与 PKCE。回调必须匹配预关联的 Provider、issuer、sub，不按邮箱自动绑定。交互式上游登录要求新认证时间；Provider 网络访问通过地址检查、可选 CIDR 白名单及受限 HTTP 客户端执行。

独立互操作客户端见 [examples](../../examples/README.md)，实际覆盖见 [协议验证记录](../testing/oidc-conformance.md)。本项目未进行 OpenID Foundation 认证，不能据此宣称认证通过。
