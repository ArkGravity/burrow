# OIDC 接入示例

两个示例仅用于本地联调，不维护生产 APP 会话。启动 Burrow 的单端口模式（`localhost:8080`），在管理界面创建两个 APP，并给测试用户授予登录权限。

| 配置       | Web                               | SPA                               |
| ---------- | --------------------------------- | --------------------------------- |
| 客户端类型 | web                               | spa                               |
| 登录入口   | `http://localhost:19001/login`    | `http://localhost:19002/`         |
| 回调地址   | `http://localhost:19001/callback` | `http://localhost:19002/callback` |
| 退出回调   | `http://localhost:19001/`         | `http://localhost:19002/`         |
| 浏览器来源 | 无需配置                          | `http://localhost:19002`          |

Web 使用独立的 `coreos/go-oidc` 和 Go OAuth2 客户端校验令牌：

```sh
export OIDC_ISSUER=http://localhost:8080
export OIDC_CLIENT_ID=YOUR_WEB_CLIENT_ID
export OIDC_CLIENT_SECRET=YOUR_WEB_SECRET
go run ./examples/web-client
```

Web 示例默认使用 PKCE S256。测试兼容流程时，由管理员为该 Web APP 开启“允许不使用 PKCE 登录”，并设置 `OIDC_USE_PKCE=false` 后启动示例。示例仍绑定浏览器 state 并验证 nonce、ID Token 签名与 claims；SPA 始终使用 PKCE。此配置只用于兼容验证，不表示其他客户端具有相同的安全检查。

SPA 使用 `oidc-client-ts`，不配置客户端密钥：

```sh
cd examples/spa-client
bun install --frozen-lockfile
VITE_OIDC_CLIENT_ID=YOUR_SPA_CLIENT_ID bun run dev
```

先登录 Web，验证回调展示的已验证 claims；再打开 SPA 登录，应复用同一 Burrow 会话。SPA 提供退出按钮，跳转到 Burrow 确认后返回登记的退出地址。样例仅在 sessionStorage 保存客户端状态；生产应用应结合自己的威胁模型选择会话方案。

需要使用相同的主机名 `localhost`，不要在浏览器入口和登记地址之间混用 `127.0.0.1`。生产 APP 应维护自己的会话与注销逻辑，不将 Burrow 的 ID Token 当作 API 访问令牌。
