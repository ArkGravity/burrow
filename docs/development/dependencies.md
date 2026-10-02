# 依赖与实现结构

版本由 `.tool-versions`、`go.mod`、`go.sum` 及两个前端工程的 `package.json` / `bun.lock` 固定。下列版本已用于本地构建，不代表未来版本选择建议。

| 用途                           | 版本               |
| ------------------------------ | ------------------ |
| Go / Bun                       | 1.27.1 / 1.4.2     |
| Chi / GORM                     | 5.3.2 / 1.31.2     |
| GORM PostgreSQL / SQLite       | 1.6.3 / 1.6.0      |
| zitadel/oidc/v3                | 3.49.2             |
| YAML 配置解析 gopkg.in/yaml.v3 | 3.0.1              |
| React / Ant Design             | 19.3.0 / 6.6.5     |
| TypeScript / Vite              | 7.0.2 / 8.3.1      |
| Vitest / Playwright            | 5.0.1 / 1.63.0     |
| 独立 Web RP：coreos/go-oidc/v3 | 3.21.0             |
| 独立 SPA RP：oidc-client-ts    | 3.5.0              |
| Compose 数据库镜像             | postgres:17-alpine |

SQLite 通过 CGO 驱动访问，构建需要 C 编译器。容器最终镜像为 Debian bookworm-slim，以非 root 用户运行，静态资源嵌入 Go 二进制。Prettier 使用开发者本机工具，没有新增项目依赖。

## 实现结构

为控制首版复杂度，后端使用一个 `internal/burrow` 包，按职责拆文件，避免初版计划中多个细包之间的循环依赖。所有持久状态位于数据库，运行单实例，不需要其他中间件。

| 文件                                  | 职责                                             |
| ------------------------------------- | ------------------------------------------------ |
| `cmd/burrow/main.go`                  | 启动、迁移、seed 初始化、签名密钥轮换、健康检查  |
| `config.go`、`configs/`               | YAML 默认配置、环境覆盖及开发主密钥文件          |
| `core.go`、`models.go`、`migrations/` | 数据库、模型、显式迁移、密码及加密               |
| `http.go`                             | 会话、本地登录、偏好、主页、CSRF、静态资源       |
| `admin.go`、`mutation.go`             | 管理 API、RBAC、管理员保护、事务内权限复查与审计 |
| `oidc_storage.go`、`oidc_http.go`     | 下游 OP 库适配、授权、Token、JWKS 与退出         |
| `network.go`                          | 可信代理与客户端地址解析                         |
| `cors.go`                             | 精确 SPA 来源校验                                |
| `web/src/`                            | React 页面、API、双语、主题与权限导航            |
| `web/e2e/`、`examples/`               | 浏览器测试及独立协议客户端                       |

相对原始任务步骤的差异：RSA 密钥为 2048 位、RS256；AES-256-GCM 使用格式版本 AAD `burrow:v1`，没有实现按对象 ID 区分的 AAD；用户名按输入精确匹配。初版的集成覆盖由 Go HTTP 测试和 Playwright 两种独立客户端联调承担，没有单独的 `tests/integration` 包。提交按最终可验证实现汇总，没有为每个规划步骤各建一个提交。
