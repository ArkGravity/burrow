# 协议与回归验证记录

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
