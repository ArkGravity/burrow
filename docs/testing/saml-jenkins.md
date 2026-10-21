# SAML / Jenkins 验证记录

本记录区分 Burrow 工程回归、独立 Jenkins 验签、用户手工验收和生产验证。SAML 实现位于 `feat/saml-idp-jenkins`；不将已有 Grafana/Nightingale/Harbor OIDC 用户验收当作 Jenkins 结果。

## 用户手工验收（2026-10-21）

用户确认按照 local-sso 接入文档，成功接入 Grafana、Nightingale（OIDC）和 Jenkins（SAML）三个平台，并提供包含三个应用的 Burrow 门户截图。这是本地浏览器手工验收反馈，与下方自动化结果分别记录，不代表生产验证、远程 CI 或官方协议认证。

随后用户确认门户、管理页面与侧栏的 UI 优化需求已完成，并手动更新[产品截图](../screenshots/README.md)。中英文 README 使用新的协议分组门户、OIDC/SAML 应用创建、角色、用户组和用户创建截图；此次截图与文档同步不重复执行协议回归，也不改变下方已运行检查的历史结果。

## 已运行的检查（2026-10-20 至 2026-10-21）

| 检查                                                                 | 结果与范围                                                                                                                   |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `go test -race ./... -count=1 -timeout=20m`，配置专用 PostgreSQL DSN | PASS；SQLite/PostgreSQL 都实际执行；包含 OIDC/MFA/迁移和 SAML 回归                                                           |
| 最终应用/HTTP/旧会话定向双库 race 测试                               | PASS；含 `TestSessionTimeCompatibility`，不将历史会话重新解释为 UTC                                                          |
| `go vet ./...`                                                       | PASS                                                                                                                         |
| `govulncheck v1.9.0 -test -show verbose ./internal/burrow`           | 无可达漏洞、无已导入包漏洞；模块级提示 GO-2026-5932 是未使用的 `x/crypto/openpgp`，不是已调用路径                            |
| `make web-check`                                                     | 类型检查与 8 个单元测试 PASS；包含用户 ID 显示                                                                               |
| 常规 Chromium 回归                                                   | 新 SAML、MFA 策略与前三项 workspace 用例通过；整轮末项出现浏览器上下文超时，按依赖顺序重跑完整 workspace 文件后 4/4 PASS     |
| `TestSAMLApplication` / `TestSAMLReplayRetention` 双库 race          | PASS；补充信任材料更新的审计回滚、清理/密钥轮换/MFA 重置后的防重放保留，以及跨 SP 请求 ID 隔离                               |
| `make build` / 根 Compose 静默验证                                   | PASS；前端和嵌入式生产二进制实际构建成功                                                                                     |
| local-sso Compose 默认及 `--profile jenkins` 静默验证                | PASS                                                                                                                         |
| 临时离线联调镜像                                                     | PASS；固定 Jenkins 基础镜像、全部 21 个插件的官方 SHA256 校验、插件版本/数量检查；Burrow 使用已缓存模块离线编译 Linux 二进制 |
| 同站 HTTPS、MFA 开启的真实 Jenkins                                   | 10/10 PASS；包括 ForceAuthn、重启与证书替换                                                                                  |
| 跨站 HTTPS、MFA 开启的真实 Jenkins                                   | 10/10 PASS；`jenkins.burrow-sp.test` 与 `sso.burrow.test`                                                                    |
| 同站 HTTPS、MFA 关闭的真实 Jenkins                                   | 10/10 PASS；强制改密仍执行，ForceAuthn 要求新的密码认证                                                                      |
| 标准 Dockerfile 在线构建                                             | 未通过；基础镜像已由用户补齐，但容器下载 Go 模块、Jenkins 更新信息/插件仍超时，不能将离线测试镜像构建记作标准构建通过        |

SDK 测试覆盖原始 Redirect 编码、空/缺失 RelayState、签名/参数篡改、RSA-SHA256 白名单、XML 限制、Response/Assertion 双签和独立篡改，以及真实 AuthnInstant/受会话限制的期限。使用修复版 goxmldsig v1.6.0；测试证书/私钥均动态生成。

数据库与 HTTP 覆盖增量迁移、历史数据保留、唯一约束、应用删除后防重放保留、配置版本变化、浏览器串用、一次性并发消费、审计回滚、ForceAuthn 绑定新会话、密码/MFA/取消/重置续接、权限撤销、账号/应用禁用及 OIDC 协议隔离。HTTP 自测与 SDK 自验证都不替代独立 SP 验签。

## 独立 Jenkins 测试

本地 opt-in 命令：

```bash
make browser-install
make test-e2e-jenkins
# 单独验证跨站 HTTPS POST/Cookie 行为；每次使用全新临时资源
BURROW_SAML_JENKINS_HOST=jenkins.burrow-sp.test make test-e2e-jenkins
# 全新环境，关闭全局 MFA 策略
BURROW_SAML_MFA_ENABLED=false make test-e2e-jenkins
```

使用 Jenkins `2.580.1-jdk21`、SAML `4.623.v7875d61cd9f5`、Matrix Authorization `3.3`，临时 SQLite、随机 Compose 项目名/HTTPS 端口、自签测试证书及 Chromium 域名映射。临时文件位于仓库已忽略的 `tmp/saml-jenkins.*`，兼容 Colima 对工作区的文件共享。不会修改系统 hosts 或信任库。脚本校验项目名和目录后只清理自身资源，默认 E2E 不启动 Jenkins。

测试通过 Burrow 管理 API 建立应用/角色，通过仅限测试的 Jenkins init Groovy 配置 realm/Matrix；ForceAuthn 使用测试管理员的受 CSRF 保护管理接口切换。独立 local-sso 示例仍手动配置，未自动执行其安装向导或代替用户完成隐私窗口验收。

`BURROW_SAML_SKIP_BUILD=true` 可复用本次已构建的 `burrow:saml-test` 与 `burrow-jenkins:saml-test`，不能用于跳过首次构建或掩盖旧镜像。不要复用其他环境的数据卷。

| 真实 SP 用例                                        | 当前状态                                                                                    |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| 临时密码修改 + MFA 后 Jenkins 验签登录              | 同站 PASS                                                                                   |
| persistent NameID、姓名/邮箱和 Matrix 管理/只读边界 | 同站 PASS                                                                                   |
| 被改动断言、已消费响应在全新会话重放被 Jenkins 拒绝 | 同站 PASS；检查 `anonymous` 与实际账号，不能将 Jenkins `authenticated` 字段单独当作登录判据 |
| Jenkins Redirect 签名被篡改时 Burrow 拒绝           | 同站 PASS；使用 Jenkins 实际生成的签名请求                                                  |
| 无应用权限用户及撤销普通角色后的新会话被拒绝        | 同站 PASS                                                                                   |
| 门户复用有效会话、ForceAuthn 重新认证               | 同站 PASS                                                                                   |
| 重启后证书/配置保留、轮换后需替换 Jenkins 信任      | 同站 PASS；旧信任拒绝新证书，替换后恢复登录                                                 |
| 跨站 HTTPS / 关闭 MFA                               | 两组独立新环境各 10/10 PASS；上述实际 SP 用例均在三组环境通过                               |

用户禁用、应用停用、认证版本变化、登录中撤权、过期及浏览器串用等额外边界由双库后端测试验证，不宣称每种组合均在真实 Jenkins 重复运行。Jenkins 侧重放结果与 Burrow 对 AuthnRequest 的独立一次性消费分别记录。

本机在线构建受网络阻塞时，曾在宿主机从官方 `archives.jenkins.io/plugins` 获取锁定版本插件，并逐项与官方 `update-center.actual.json` 的 SHA256 比较；临时 Dockerfile 只复制已校验插件并执行同一版本核对脚本。Burrow 临时镜像使用 Go 基础镜像中的本机缓存模块编译、Debian 运行层和证书包。临时构建上下文不纳入源码；该路径用于完成互操作取证，标准 Dockerfile 的在线下载仍需网络可达。

最终 `TestSAMLKeys` 双库 race 通过，补充验证损坏证书、无活动历史密钥不被静默重建。格式、相对文档链接、shell 语法和 `git diff --check` 均通过；旧迁移 001–005、根 Dockerfile/Compose 没有修改。

完成后已清理各次随机 Compose 项目的容器、网络、卷和临时文件，关闭本次专用 PostgreSQL 测试容器、删除临时编译容器，并确认没有遗留容器。保留用户手工拉取的基础镜像及两个临时联调镜像；没有启动或清理已有 local-sso 部署。

夹具临时凭据不用于真实部署，不打印原始 SAML 请求/响应或密钥；Jenkins Playwright 测试关闭网络 trace 和截图。配置步骤见 [Jenkins 指南](../../examples/local-sso/jenkins/README.md)。这些测试即使通过，也不等于 SAML 官方认证、远程 CI 成功或生产验收。
