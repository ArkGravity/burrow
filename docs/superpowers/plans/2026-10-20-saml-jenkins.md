# Burrow SAML IdP and Jenkins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task after the user explicitly authorizes implementation. Track delivery in the checklist below and preserve the original detailed actions as review input. Do not start subagents unless separately authorized. Do not commit, push, publish, or delete existing local resources without the applicable user authorization.

**Goal:** 实现用户已选择的方案 A：Burrow 下游 SAML 2.0 IdP、原有密码/MFA与应用授权复用，以及可从空环境复现的 Jenkins local-sso 示例。

**Architecture:** 保持 `internal/burrow` 单包模块化单体；新增显式 schema v6、SAML 配置/事务/密钥，HTTP 与 XML 签名适配分文件。使用 ZITADEL SAML 的公开接口处理协议编码和签名，Burrow掌握认证、授权、一次性签发和审计。Jenkins 只通过独立示例的按需 profile 启动。

**Tech Stack:** 现有 Go/Chi/GORM/PostgreSQL/SQLite、React/TypeScript/Ant Design/Bun，新增 `github.com/zitadel/saml`（验证基线 v0.4.1）；Jenkins LTS、SAML 与 Matrix Authorization 插件、Docker Compose、Playwright。

---

## 输入、状态与执行约束

- 设计依据：[2026-10-20 SAML/Jenkins 设计](../specs/2026-10-20-saml-jenkins-design.md)。用户在本轮选择 A 并要求 Superpowers 规划；前文已完成范围和选型讨论。
- 用户后续以“继续任务”授权开始执行；当前在 `feat/saml-idp-jenkins` 实施，未提交或推送。下列任务清单保留执行契约，实际证据见本节记录。
- 规划时源码基线：`3dd57a9`，schema v5；实际实施前重新查看工作区差异和最新源码，不将历史 2026-09-24 计划中的已移除 Provider/多包结构当作当前架构。
- 用户要求已落实：从本地 `main` 的 `3dd57a9d65552cbc15d8024452ee7093b90c49c4` 新建并切换到 `feat/saml-idp-jenkins`，未提交/推送。开始任一代码任务前执行 `rtk git branch --show-current`，必须为该功能分支；不在 main/dev 直接实现，也不重新建立同名分支。
- 第一项任务是 SDK 选型门槛，不以协议配置降级绕开失败；通过后再实现迁移并在隔离测试库验证，不对现有生产或开发数据库运行迁移。
- 每个勾选项作为一个动作执行；测试使用“新增行为断言 → 确认失败原因 → 最小实现 → 验证”闭环。仅文案/表格/布局等低风险改动不添加机械镜像测试。
- 项目要求优先使用 RTK。README 给开发者的命令不依赖 RTK；本计划的代理执行命令使用 RTK。
- 当前授权不含 git commit/push，任务结束只做范围化 diff review。即使技能模板建议频繁提交，也须以仓库和用户授权为准。
- 不启动、迁移或清理用户已有数据库/部署。PostgreSQL 只使用专用测试 DSN 与既有隔离 schema 测试设施。
- 保持单进程，不新增 Redis、队列、上游 IdP、SonarQube、生产部署组件或新的 Go 包层。

### 执行记录（2026-10-20 至 2026-10-21）

- Task 1 编解码门槛通过；goxmldsig 从 SDK 默认 v1.4.0 升至修复版 v1.6.0，公开 API 无需 fork。证据记录于依赖说明。
- Task 2 / Task 4 基础实现完成；SQLite、隔离 PostgreSQL 的迁移、唯一约束、应用级联删除保留防重放记录、密钥与 CLI race 测试实际通过。PostgreSQL 使用本次创建的临时容器，无持久卷，与已有部署无关。
- Task 3 基础实现完成；应用创建/编辑、metadata 反例、内部 ClientID 隔离与旧 OIDC 凭据回归已在双库通过。
- Task 5–9 已实现并通过双库后端、前端单元测试与常规浏览器覆盖；完整双库 race 套件通过，新增信任配置审计回滚和防重放保留的定向双库测试也通过。
- Task 6 实现细化：SAMLTransaction 增加 `failure_status`，持久化已验签请求的能力错误。否则签名/审计暂时失败后，续接无法知道原请求不支持的选项，可能错误签发成功断言。该字段只保存白名单状态码，不保存原始请求 XML；加入新的 006 迁移，001–005 不变。
- Task 2 增加旧会话兼容标记 `sessions.utc_timestamps`：新会话/SAML 时间采用 UTC，旧 OIDC 会话仍按原语义校验，不批量撤销或延长旧会话。双库时间兼容测试通过。
- Task 10–11 的可选 profile、版本锁、手动配置指南和独立测试已完成。同站/跨站 HTTPS、MFA 开启，以及同站 MFA 关闭的真实 Jenkins 各 10/10 通过，涵盖权限、篡改/重放、门户复用、ForceAuthn、重启、受控证书替换。
- Jenkins 测试夹具使用 Burrow 管理 API 和仅限测试的 init Groovy，替代原计划的完整 UI 初始化，以避免临时安装管理员成为额外依赖；local-sso 产品示例仍手工配置。测试临时目录放在已忽略的工作区 tmp 下，兼容 Colima 文件共享。
- Task 12 的 native build、静默 Compose 与既有回归已通过。容器内依赖下载仍超时；本机采用官方 SHA256 核验插件及缓存模块构建临时离线镜像完成互操作，标准 Dockerfile 在线构建仍列为环境限制。具体证据见[验证记录](../../testing/saml-jenkins.md)。
- 没有用户手工验收、生产验证或远程 CI 结果。当前无可调用 Mem0 工具，未读取或保存项目记忆。

### 交付进度

2026-10-21，用户确认按文档成功接入 Grafana、Nightingale 和 Jenkins，并授权提交和推送本功能分支，再从其基础新建界面优化分支。前文未提交/未推送的状态描述属于此前实施阶段；手工验收证据见验证记录。

原始细分动作保留为审查输入；下面按最终交付物勾选。部分测试名称和文件合并到当前职责文件，命令与未覆盖边界以验证记录为准，不将规划动作自动视为已运行。

- [x] Task 1：公开 SDK、原始 Redirect 验签与双签门槛及依赖审查。
- [x] Task 2：schema v6、旧 OIDC 数据和会话兼容、双库迁移/约束。
- [x] Task 3：SAML metadata 管理、协议隔离、权限与审计事务。
- [x] Task 4：独立加密密钥、显式初始化、轮换、CLI 与恢复说明。
- [x] Task 5：密码/改密/MFA 类型化续接、取消和撤销。
- [x] Task 6：浏览器绑定、独立防重放、并发消费和审计回滚。
- [x] Task 7：范围内 IdP 端点、签名响应和错误契约。
- [x] Task 8：双库安全边界、真实 SP 篡改/重放拒绝；未声称所有反例组合均在 SP 重复运行。
- [x] Task 9：双语管理 UI、登录续接、前端及常规浏览器回归。
- [x] Task 10：可选 Jenkins profile、精确版本闭包、人工配置指南。
- [x] Task 11：隔离资源、三组各 10 项真实 Jenkins 自动互操作、失败清理。
- [x] Task 12：native build、静默 Compose、格式/链接/diff 审阅与准确验证报告。
- [ ] 环境限制：标准 Dockerfile 在线构建需 Go/Jenkins 下载站恢复可达；临时离线镜像互操作已通过。
- [ ] 用户后续验收：按 local-sso README 手工完成安装向导与隐私窗口验收；自动夹具不替代该结果。

## 文件结构与职责

以下路径是实施职责映射；以当前源码为准。个别测试合并到职责对应文件，实际命令/结果见执行记录。

| 路径                                                                                                                          | 动作与职责                                               |
| ----------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| `go.mod`, `go.sum`                                                                                                            | 锁定通过门槛的 SDK 及经过审查的传递依赖                  |
| `internal/burrow/saml_codec.go`                                                                                               | 有界 XML、Redirect 验签、metadata 与双签响应编码         |
| `internal/burrow/saml_codec_test.go`                                                                                          | SDK 门槛、编码与签名反例                                 |
| `internal/burrow/saml_applications.go`                                                                                        | SP metadata 导入、提取、配置验证和批量引用               |
| `internal/burrow/saml_applications_test.go`                                                                                   | 管理 API、配置变化与跨协议反例                           |
| `internal/burrow/saml_keys.go`, `saml_keys_test.go`                                                                           | 独立证书、加密密钥、初始化、轮换与持久化                 |
| `internal/burrow/saml_storage.go`, `saml_storage_test.go`                                                                     | 请求登记、浏览器绑定、有效期与原子消费                   |
| `internal/burrow/saml_http.go`, `saml_http_test.go`                                                                           | 端点、跳转、协议错误及签发事务                           |
| `internal/burrow/saml_security_test.go`                                                                                       | 双库权限竞态、浏览器串用、撤销和审计回滚                 |
| `internal/burrow/login_request.go`, `login_request_test.go`                                                                   | 类型化认证续接的最小公共函数                             |
| `internal/burrow/models.go`, `core.go`                                                                                        | 新模型、迁移注册、schema检查及清理                       |
| `internal/burrow/migrations/006_saml_idp.sql`                                                                                 | v6 增量 DDL；历史迁移保持原字节                          |
| `internal/burrow/saml_migration_test.go`                                                                                      | 双库升级、历史校验和和数据保留                           |
| `internal/burrow/admin.go`, `mfa.go`, `http.go`, `mutation.go`                                                                | 管理、会话、删除/撤销和事务集成                          |
| `internal/burrow/oidc_http.go`, `oidc_storage.go`                                                                             | OIDC 协议隔离与续接适配，不重写 OP                       |
| `internal/burrow/security_test.go`                                                                                            | 扩展现有 fixture，明确 OIDC Protocol，初始化测试 SAML 键 |
| `cmd/burrow/main.go`, `main_test.go`                                                                                          | migrate 初始化、keys-rotate 协议参数及 CLI 回归          |
| `web/src/lib/api.ts`, `i18n.tsx`                                                                                              | 协议/API 类型、两种语言词条                              |
| `web/src/pages/Resources.tsx`, `Auth.tsx`, `Home.tsx`                                                                         | 应用表单、身份标识、登录续接与门户                       |
| `web/src/components/SAMLApplicationFields.tsx`                                                                                | 范围内的 SAML metadata 上传/粘贴与只读摘要               |
| `web/src/lib/login-request.ts`, `login-request.test.ts`                                                                       | 前端解析、保存和传播 requestType                         |
| `web/src/components/SAMLApplicationFields.test.tsx`                                                                           | 真实用户输入与提交契约，不测试布局细节                   |
| `web/vite.config.ts`, `web/e2e/saml.spec.ts`                                                                                  | 代理与常规浏览器管理/续接覆盖                            |
| `examples/local-sso/jenkins/Dockerfile`, `plugins.txt`, `README.md`                                                           | 固定镜像、完整插件版本锁与人工配置/恢复指南              |
| `examples/local-sso/docker-compose.yml`, `nginx.conf`, `README.md`                                                            | 按需 Jenkins、网络与示例总入口                           |
| `web/e2e/jenkins-saml.spec.ts`, `run-jenkins-saml.sh`                                                                         | opt-in 的真实 Jenkins 互操作；独立临时资源               |
| `web/e2e/fixtures/jenkins/docker-compose.yml`, `nginx.conf.template`                                                          | 仅包含 Burrow/Jenkins/Nginx 的独立测试拓扑与动态端口模板 |
| `Makefile`, `.github/workflows/ci.yml`                                                                                        | opt-in 命令和现有覆盖扩展，保留默认依赖边界              |
| `docs/development/saml-adapter.md`, `docs/operations/jenkins.md`, `docs/testing/saml-jenkins.md`                              | 协议、运维、准确的验证证据                               |
| `README.md`, `README_zh.md`, `docs/README.md`, `docs/development/dependencies.md`, `docs/operations/recovery.md`, `AGENTS.md` | 与最终行为同步；修改前确认对应文件存在                   |

不把 `saml_codec.go` 变为业务层：它只能消费已明确的输入和返回可验证的 XML/metadata，不查用户角色、不创建共享会话、不消费事务。

## 依赖与关键数据契约

实际执行顺序：**1 → 2 → 4 → 3 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12**。Task 4 的密钥可用性是 Task 3 新增/启用 SAML 应用的前提，因此先做密钥，再做应用管理。任务编号保留原规划编号，依赖顺序以本段为准；默认单代理执行。

新增类型在 Task 2 定义，字段与设计规格保持一致：

```go
type SAMLApplication struct {
	ApplicationID      string `gorm:"primaryKey" json:"-"`
	EntityID           string `gorm:"uniqueIndex;not null" json:"entityId"`
	ACSURL             string `gorm:"column:acs_url" json:"acsUrl"`
	SigningCertificate string `json:"signingCertificate"`
	MetadataXML        string `json:"metadataXml"`
	ConfigVersion      int64  `json:"configVersion"`
}

type SAMLTransaction struct {
	ID            string `gorm:"primaryKey"`
	ApplicationID string `gorm:"uniqueIndex:saml_request"`
	SPRequestID   string `gorm:"column:sp_request_id;uniqueIndex:saml_request"`
	BrowserHash   string
	RelayState    string
	ACSURL        string `gorm:"column:acs_url"`
	ConfigVersion int64
	ForceAuthn    bool
	FailureStatus string
	ReceivedAt    time.Time
	ExpiresAt     time.Time
	Consumed      bool
	UserID        string
	SessionID     string
}

type SAMLRequestReplay struct {
	SPEntityID  string `gorm:"primaryKey;column:sp_entity_id"`
	SPRequestID string `gorm:"primaryKey;column:sp_request_id"`
	ReplayUntil time.Time
}

type SAMLSigningKey struct {
	ID             string `gorm:"primaryKey"`
	PrivateCipher  string `json:"-"`
	CertificatePEM string `gorm:"column:certificate_pem"`
	Active         bool
	CreatedAt      time.Time
}

func (SAMLApplication) TableName() string   { return "saml_applications" }
func (SAMLTransaction) TableName() string   { return "saml_transactions" }
func (SAMLRequestReplay) TableName() string { return "saml_request_replays" }
func (SAMLSigningKey) TableName() string    { return "saml_signing_keys" }
```

DDL 负责外键与唯一约束；GORM 标签不代替迁移。所有缩略字段显式设置列名以匹配 DDL：`SPRequestID→sp_request_id`、`ACSURL→acs_url`、`SPEntityID→sp_entity_id`，避免 GORM 推导分歧。待认证事务 UserID/SessionID 使用空字符串且不建立空串外键；签发前在事务中加载和验证真实引用。

`SAMLRequestReplay` 不引用应用或用户；其生存期不能被删除应用、撤销登录事务或清理已消费事务缩短。它保存已通过签名检查的外部 SP 请求已登记的事实，与 SAMLTransaction 在同一事务创建。

API 输入采用 `protocol` 和嵌套 `saml.metadataXml`；EntityID、ACS、证书、ConfigVersion 从 XML 提取并只读返回。显式输入这些派生字段拒绝。SAML 与 OIDC 专属字段混用拒绝；更新省略的字段沿用原值。协议创建后不可修改。

## 执行批次与检查点

| 批次 | 按顺序执行        | 可以交付的结果                                  | 继续条件                                     |
| ---- | ----------------- | ----------------------------------------------- | -------------------------------------------- |
| 1    | Task 1 → 2 → 4    | SDK 证据、v6 兼容迁移、独立密钥与 CLI           | 所有 SDK 门槛、双库迁移和密钥回滚测试通过    |
| 2    | Task 3 → 5 → 6    | 应用管理、协议隔离、类型化续接、请求登记/防重放 | 旧 OIDC、MFA 流程与并发请求登记通过          |
| 3    | Task 7 → 8 → 9    | 完整 IdP 端点、安全边界、管理 UI                | 双签/权限竞态/审计回滚、前端与常规浏览器通过 |
| 4    | Task 10 → 11 → 12 | Jenkins 镜像/配置指南、独立互操作和最终证据     | 实际 Jenkins 验签通过，构建与完整回归通过    |

检查点是记录并报告证据的位置，不为常规下一任务重复索取许可。SDK 门槛不满足、需改变 A 范围或需要修改/删除现有用户部署时，才停止依赖工作并说明具体原因。每项功能测试首次补齐后确认测试名确实匹配，`no tests to run` 不能作为通过结果。

开始首批代码工作前确认已获得实施授权，运行以下只读检查并记录结果；当前规划回合不代替实施授权：

```bash
rtk git branch --show-current
rtk git status --short --untracked-files=all
rtk git log -1 --oneline
```

分支必须为 `feat/saml-idp-jenkins`。若发现之后产生的无关用户编辑，保留它们并收紧本次编辑范围，不 reset、覆盖或重新切回 main。

## HTTP 与错误响应契约

管理 API 沿用当前 `{error:{code,requestId}}` 格式；SAML 协议错误与管理错误分开。以下结果作为 Task 3/7/8 的测试输入，不在实现时临时选择行为。

| 条件                                                       | HTTP / 输出                            | 数据库效果                              |
| ---------------------------------------------------------- | -------------------------------------- | --------------------------------------- |
| 未知协议、派生 SAML 字段、混用 OIDC 字段                   | 400 `invalid_request`                  | 不修改应用                              |
| 无应用写权限                                               | 403 `forbidden`                        | 不修改应用                              |
| 缺授权关系写权限但提交 roleIds                             | 403 `forbidden`                        | 应用和关联一起不变                      |
| 重复已登记的 SP Entity ID                                  | 409 `conflict`                         | 不建立第二条配置                        |
| 无可用 IdP 密钥但请求启用应用                              | 503 `unavailable`                      | 旧状态不变，可保存停用配置              |
| URL/XML 超限、重复参数、未知/停用 SP、验签失败、未登记 ACS | 本地 400，无 ACS 表单                  | 不登记可签发事务                        |
| 已接受过的外部请求重新提交                                 | 本地 400 `invalid_transaction`         | 原防重放记录与期限不变                  |
| 另一浏览器、已消费或到期内部事务续接                       | 本地 400 `invalid_transaction`         | 不签发，不替换原绑定                    |
| 已验签且 ACS 可信，但 IsPassive=true                       | 200，签名失败响应，NoPassive           | 防重放登记；响应一次性消费              |
| 已验签且 ACS 可信，但 NameIDPolicy 不支持                  | 200，签名失败响应，InvalidNameIDPolicy | 防重放登记；响应一次性消费              |
| 已验签且 ACS 可信，但认证上下文不支持                      | 200，签名失败响应，NoAuthnContext      | 防重放登记；响应一次性消费              |
| 可信请求认证完成后无应用登录权限                           | 200，签名失败响应，RequestDenied       | 记录无权限结果，不建立 SP 身份断言      |
| 会话失效且请求允许交互                                     | 303，同站 Burrow 登录续接              | 复用原 SAML 事务，不延长有效期          |
| 数据库、密钥读取、签名或审计失败                           | 本地 503，无 ACS 表单                  | 回滚本次消费/审计；已登记防重放记录保留 |
| 正常签发                                                   | 200，no-store 的双签 POST 表单         | 消费标记和成功审计同一事务提交          |

可信能力错误先登记请求再返回一次性失败响应。首次 `/saml/sso` 登记与后续 `/saml/login` 消费是不同事务：后者失败可由同一浏览器续接重试，不能通过重投外部 AuthnRequest 新开事务。

Response 和 Assertion 的 XML ID 均使用新的高熵 ID；SessionIndex 使用独立随机值，不把 Burrow 共享会话凭据或数据库 SessionID 暴露给 Jenkins。断言与表单只返回规格约定属性，不携带角色、组或内部认证版本。

## Task 1：证明 SDK 与 Jenkins 请求/响应契约

**Files:** 新增 `saml_codec_test.go`，修改 `go.mod`, `go.sum`, `docs/development/dependencies.md`；适配器写入 `saml_codec.go`。本任务在获授权实施后执行。

- 检查当前 Go/Bun 与工作区，记录 SDK 官方版本、许可证和传递依赖；读取 v0.4.1 的公开 signing/XML API，不根据旧回答猜测函数签名。
- 运行 `rtk proxy go get github.com/zitadel/saml@v0.4.1`，审阅两个模块文件差异；若工具链或依赖安全检查不接受，记录具体版本证据后停止。
- 用 `crypto/rsa`、`crypto/x509`、`t.TempDir()` 动态生成测试专用 SP/IdP 证书，禁止提交真实私钥；记录 fixture 的用途、有效期和固定时钟。
- 编写 `TestSAMLSDKRedirectSignature`：签名有效、RelayState 篡改、SAMLRequest 篡改、Signature 缺失、SigAlg 错误、重复参数、等价但不同 URL 编码、空 RelayState 与缺失 RelayState。
- 运行 `rtk proxy go test ./internal/burrow -run '^TestSAMLSDKRedirectSignature$' -count=1 -v`，确认新增适配器未实现导致断言失败，不把编译器拼写错误当作红灯证据。
- 通过公开签名接口实现最小验签适配，优先按原始 Redirect 查询中经过验证的原始编码构造签名输入；逐项重跑，不能用 Decode→Encode 改变待签名字节。
- 编写 `TestSAMLSDKDoubleSignature`：对 Response/Assertion 分别篡改内容；验证两个独立 Signature 引用正确 ID，重复 XML ID 或注入节点拒绝。
- 用 SDK 公开接口实现响应编码，注入真实 AuthnInstant、SessionNotOnOrAfter、Recipient、Audience、InResponseTo 与 persistent NameID；不复制 SDK 默认生成“现在认证”的结果。
- 执行 `rtk proxy go test ./internal/burrow -run '^TestSAMLSDK' -count=1 -v`，预期所有上述用例 PASS。
- 在依赖说明写出公开 API 的确切调用、依赖解析结果及门槛证据。缺任一门槛停止后续任务，输出失败用例及需要的适配，不自动 fork、换库或放宽验签。

**退出标准：** 公开 API 足以完成所选范围；只引入库和测试/编码适配，没有挂载额外协议端点。此阶段仍未宣称 Jenkins 已实际验收。

## Task 2：schema v6 与兼容迁移

**Files:** 新增 `006_saml_idp.sql`, `saml_migration_test.go`；修改 `models.go`, `core.go`, `security_test.go`。

- 使用 `openTestStore` 在 SQLite/PostgreSQL 建立 v5 数据，保留用户、角色、应用、会话、TokenRecord、OIDC AuthTransaction 与 SigningKey 的快照。
- 编写 `TestSAMLMigration` 子用例：空库到 v6、v5 到 v6、幂等、旧校验和拒绝、已存在的 OIDC 数据/密钥不变；执行 `rtk proxy go test ./internal/burrow -run '^TestSAMLMigration$' -count=1 -v` 确认缺 schema 时失败。
- 新迁移增加 `applications.protocol TEXT NOT NULL DEFAULT 'oidc'`、`login_transactions.request_type TEXT NOT NULL DEFAULT 'oidc'` 与四个 SAML 表；增加 SP EntityID 唯一、ApplicationID/SPRequestID 复合唯一、独立防重放复合主键、单活动键部分唯一索引、期限索引和应用外键。使用下方完整 DDL 契约。
- 在 `Application.Protocol` 增加 GORM 默认值，更新 API 默认创建与 `testApp` fixture；不让旧 fixture 的空字符串绕过 `protocol=oidc` 过滤。
- 将 v6 加入迁移列表，重写 checksum 分派时保持 v1–v5 的结果逐字节一致；`Health` 检查 v6，不编辑旧 SQL。
- 测试原始数据库下重复 SP 请求插入失败、多活动签名键插入失败、删除应用级联清理配置/事务但保留防重放记录，不能靠应用层“先查再插”替代唯一约束。
- `Cleanup` 纳入 SAML 过期事务与防重放记录；按 ExpiresAt 清理登录事务，按 ReplayUntil 清理防重放记录，不能共用一个截止时间。
- 使用有效测试 DSN 运行 `rtk proxy go test -race ./internal/burrow -run 'TestSAMLMigration|TestMigrationIntegrity' -count=1 -v`；两种 driver 都应实际 PASS。未配置 DSN 的 SKIP 记录为未覆盖。

迁移 DDL 契约（完整内容；执行前核对 Task 1 是否产生经过验证的字段变更）：

```sql
ALTER TABLE applications
ADD COLUMN protocol TEXT NOT NULL DEFAULT 'oidc'
CHECK (protocol IN ('oidc', 'saml'));

ALTER TABLE login_transactions
ADD COLUMN request_type TEXT NOT NULL DEFAULT 'oidc'
CHECK (request_type IN ('oidc', 'saml'));

ALTER TABLE sessions ADD COLUMN utc_timestamps BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE saml_applications (
    application_id TEXT PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
    entity_id TEXT NOT NULL UNIQUE,
    acs_url TEXT NOT NULL,
    signing_certificate TEXT NOT NULL,
    metadata_xml TEXT NOT NULL,
    config_version BIGINT NOT NULL DEFAULT 1 CHECK (config_version > 0)
);

CREATE TABLE saml_transactions (
    id TEXT PRIMARY KEY,
    application_id TEXT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    sp_request_id TEXT NOT NULL,
    browser_hash TEXT NOT NULL,
    relay_state TEXT NOT NULL DEFAULT '',
    acs_url TEXT NOT NULL,
    config_version BIGINT NOT NULL,
    force_authn BOOLEAN NOT NULL DEFAULT FALSE,
    failure_status TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    consumed BOOLEAN NOT NULL DEFAULT FALSE,
    user_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX saml_transactions_request
ON saml_transactions(application_id, sp_request_id);
CREATE INDEX saml_transactions_expiry ON saml_transactions(expires_at);
CREATE INDEX saml_transactions_user ON saml_transactions(user_id);

CREATE TABLE saml_request_replays (
    sp_entity_id TEXT NOT NULL,
    sp_request_id TEXT NOT NULL,
    replay_until TIMESTAMP NOT NULL,
    PRIMARY KEY (sp_entity_id, sp_request_id)
);
CREATE INDEX saml_request_replays_expiry ON saml_request_replays(replay_until);

CREATE TABLE saml_signing_keys (
    id TEXT PRIMARY KEY,
    private_cipher TEXT NOT NULL,
    certificate_pem TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX saml_signing_keys_active
ON saml_signing_keys(active) WHERE active = TRUE;
```

**退出标准：** 只运行新迁移，旧 OIDC 会话/令牌/签名公钥不批量失效，schema/readiness 与测试版本一致。

## Task 3：SP 应用管理、metadata 校验与协议隔离

**Files:** 新增 `saml_applications.go`, `saml_applications_test.go`；修改 `admin.go`, `models.go`, `oidc_http.go`, `oidc_storage.go`, `protocol_security_test.go`。

- 写管理 API 测试：协议默认 OIDC；合法 SAML metadata 创建；协议不可改；派生字段不可写；跨协议字段拒绝；SAML secret-reset 拒绝；缺写权限拒绝。
- 写 metadata 表驱动用例：EntityDescriptor 数量、POST ACS 数量、证书数量/算法/期限、DTD/实体、重复 ID、超 256 KiB、深度超过 32、无效生产 URL；同一公钥出现在 signing/encryption KeyDescriptor 时，按用途提取签名证书并给 SDK 生成最小描述，保留原 metadata。
- 执行 `rtk proxy go test ./internal/burrow -run '^TestSAMLApplication' -count=1 -v`，预期在未支持 protocol/metadata 时失败。
- 在现有 `authorizationTx` 管理路径创建/更新 SAML 配置，保留 `authorization:write` 的关系变更检查；导入 XML 的有界解析在锁外完成，写入前验证当前应用与 ConfigVersion。
- SAML 应用内部生成唯一 ClientID，ClientType=saml，不生成 SecretHash，公共 UI/API 不把它作为 SP EntityID；OIDC 输入仍按现有 Web/SPA 规则处理。
- 调用 Task 4 的密钥读取验证，在管理事务内拒绝缺失/损坏/过期密钥下新增启用应用或从停用转为启用；失败保留旧状态和审计原子性，停用配置允许保存。
- metadata 变更递增 ConfigVersion，并使之前待处理 SAML 请求不能续接；审计失败时配置版本和更新一起回滚。
- 为列表、个人门户和权限紧凑引用分别构造返回值；metadata 只返回给应用管理读取，权限列表的 samlEntityId 通过一次批量读取补齐。
- 写 `TestSAMLProtocolIsolation`：SAML 内部 ClientID 调用 OIDC authorize/token/get-client/userinfo 失败；OIDC 应用不能匹配 SAML Issuer。
- 所有 OIDC 应用查找增加 Protocol 约束，尤其是 token 前置检查、GetClientByClientID、授权码兑换与在线 token 授权复查；不能只修改一个端点。
- 运行 `rtk proxy go test -race ./internal/burrow -run 'TestSAMLApplication|TestSAMLProtocolIsolation|TestApplicationCredentials|TestPermission' -count=1 -v`；配专用 DSN覆盖两库。

**退出标准：** 应用管理可独立配置两种协议；默认旧客户端兼容；服务器阻止跨协议使用。

## Task 4：独立 SAML 密钥与运维命令

**Files:** 新增 `saml_keys.go`, `saml_keys_test.go`, `cmd/burrow/main_test.go`；修改 `cmd/burrow/main.go`, `core.go`, `http.go`, `security_test.go`。

- 写 `TestSAMLKeys`：幂等初始化、重启保存、私钥加密、错误主密钥、损坏/过期证书、密钥不匹配、单活动键、历史公开证书保留。
- 执行 `rtk proxy go test ./internal/burrow -run '^TestSAMLKeys$' -count=1 -v`，确认不存在初始化/读取能力时失败。
- 实现 `EnsureSAMLSigningKey`，在锁外生成候选 RSA 3072/X.509（365 天），锁内再次检查，仅在无历史键且无活动键时插入；有损坏历史数据时拒绝而非自动替换。
- 使用现有 `seal/unseal` 保存私钥，解密后检查 PKCS8、私钥与证书匹配及有效期；日志只含非敏感键 ID/错误类别。
- `migrate` 完成现有 OIDC 密钥初始化后再调用 SAML 初始化；修正现有“OIDC 缺键时提前 return”的控制流。
- 为 `keys-rotate` 增加 `--protocol`，默认为 oidc；仅 saml 要求非空 `--reason`。实现活动键替换、旧待处理 SAML 事务失效与运维审计原子提交，OIDC keyset 不变。
- readiness 根据启用 SAML 应用检查活动键；serve 不隐式生成。补齐所有会创建启用 SAML 应用的 fixture，避免把初始化依赖藏进 handler。
- 写 CLI 测试：默认 OIDC、未知协议、缺 SAML reason、已有配置 precedence、重复 migrate；测试临时路径和环境使用 `t.TempDir/t.Setenv`。
- 执行 `rtk proxy go test -race ./internal/burrow ./cmd/burrow -run 'TestSAMLKeys|TestKeysRotate|TestMigrate' -count=1 -v`，审计故障时不得切换任何键。

**退出标准：** 新键生命周期独立可恢复；文档明确手动轮换维护窗口，不暗示 SDK 支持 SP 多证书。

## Task 5：类型化登录续接与双协议撤销

**Files:** 新增 `login_request.go`, `login_request_test.go`；修改 `models.go`, `mfa.go`, `http.go`, `oidc_http.go`, `mfa_test.go`, `mfa_config_test.go`, `admin.go`。

先固定续接纯函数的完整实现契约：

```go
package burrow

import (
	"errors"
	"net/url"
)

type LoginRequestRef struct {
	Type string
	ID   string
}

func normalizeLoginRequest(kind, id string) (LoginRequestRef, error) {
	if kind == "" {
		kind = "oidc"
	}
	if kind != "oidc" && kind != "saml" {
		return LoginRequestRef{}, errors.New("invalid_request_type")
	}
	if kind == "saml" && id == "" {
		return LoginRequestRef{}, errors.New("invalid_transaction")
	}
	return LoginRequestRef{Type: kind, ID: id}, nil
}

func loginRequestURL(ref LoginRequestRef) (string, error) {
	ref, err := normalizeLoginRequest(ref.Type, ref.ID)
	if err != nil {
		return "", err
	}
	if ref.ID == "" {
		return "/", nil
	}
	return "/" + ref.Type + "/login?requestId=" + url.QueryEscape(ref.ID), nil
}
```

该纯函数不做授权。使用它之前必须按类型查服务器端事务并验证浏览器绑定，不能因为生成的路径是同站就允许任意事务。

对应的直接行为测试：

```go
func TestLoginRequestURL(t *testing.T) {
	for _, tc := range []struct {
		kind string
		id   string
		want string
		bad  bool
	}{
		{"", "", "/", false},
		{"", "old", "/oidc/login?requestId=old", false},
		{"saml", "new", "/saml/login?requestId=new", false},
		{"saml", "", "", true},
		{"https://example.test", "new", "", true},
		{"saml", "a&b", "/saml/login?requestId=a%26b", false},
	} {
		got, err := loginRequestURL(LoginRequestRef{Type: tc.kind, ID: tc.id})
		if (err != nil) != tc.bad || got != tc.want {
			t.Fatalf("kind=%q id=%q got=%q err=%v", tc.kind, tc.id, got, err)
		}
	}
}
```

- 先添加上述纯函数测试与两协议 continuation/revocation HTTP 测试；执行 `rtk proxy go test ./internal/burrow -run 'TestLoginRequest|TestSAMLLoginContinuation' -count=1 -v` 确认失败。
- 实现纯函数，给登录输入/状态/改密续接加入 requestType；省略继续按 OIDC，未知类型或 SAML 空 ID 失败。
- `authContext` 从 OIDC 专属文件移动到最小公共续接文件或公共 HTTP 文件，由服务端按类型加载；返回应用名称，不泄露 SP metadata。
- `validateLoginRequest`、`loginStateDB` 与 `finishLogin` 用类型化引用查对应表，并在所有 MFA/改密步骤保留类型和原截止时间。
- 将 `invalidateAuthentication` 的 keepRequest 参数改为 `LoginRequestRef`，逐一修改所有调用：密码重置、个人改密、MFA API/CLI reset、禁用账户、强制改密。
- 撤销时先按 request_type 分别获取待处理请求 ID，再删除对应表；强制改密只保留本协议当前请求。先获取再删除 LoginTransaction，不能丢失尚未写 UserID 的待处理请求关联。
- 取消 SAML 登录时将该请求作废并保留防重放记录至到期；删除用户/应用同步失效对应事务，不误删其他用户数据。
- `finishLogin` 在创建 Session 的同一事务内更新本次 SAMLTransaction 的 SessionID/UserID；ForceAuthn 最终要求当前会话等于该绑定，不只比较秒级时间。旧共享会话、另一个请求完成的登录都不能满足。
- 执行 `rtk proxy go test -race ./internal/burrow -run 'TestLoginRequest|TestSAMLLoginContinuation|TestSAMLRevocation|TestMFA|TestCSRFAndForcedPassword' -count=1 -v`；双库覆盖。

**退出标准：** 首次改密、TOTP 绑定/验证、取消与重置均恢复/清理正确协议；老 OIDC 请求无需新增字段仍可运行。

## Task 6：请求登记、浏览器绑定与原子消费

**Files:** 新增 `saml_storage.go`, `saml_storage_test.go`；修改 `saml_codec.go`, `mfa.go`, `core.go`。

- 写 `TestSAMLRequestRegistration`：唯一请求 ID、跨 SP 相同 ID、过期/未来时间、另一浏览器、被消费请求、配置版本变化。
- 编写 `TestSAMLAtomicConsume`：两个并发请求只有一个签发成功，失败的审计写入不能把请求消耗掉；使用 existing driver 循环覆盖 SQLite/PostgreSQL。
- 执行 `rtk proxy go test ./internal/burrow -run 'TestSAMLRequestRegistration|TestSAMLAtomicConsume' -count=1 -v`，确认未实现存储操作时失败。
- 在认证锁/数据库事务内登记请求，用唯一索引最终裁决，捕获重复错误并返回固定错误类型，不重用另一浏览器的已有事务。
- 同一事务先插入 `SAMLRequestReplay` 再插入 `SAMLTransaction`，以 SPEntityID/SPRequestID 复合主键阻止删除重建应用后的重放；存储错误导致两项一起回滚。
- 保存接收时间、外部请求 ID、原始 RelayState、浏览器哈希、SP 配置版本和有效期；严格按后附 `samlRequestDeadlines` 计算两个截止时间，重复请求不能延长任何一期限。
- 实现按内部 ID + BrowserHash 查续接；过期/已消费/不匹配统一 invalid_transaction，不透露事务属于谁。
- 消费使用条件更新并检查 RowsAffected=1；成功后同一事务内写审计；只有提交成功才允许把已缓冲的表单写给客户端。
- 增加 `TestSAMLReplayRetention`，覆盖事务过期先被清理、登录取消、用户重置、证书轮换、删除并重建同 EntityID 应用后重放原请求，均拒绝；跨 SP 同请求 ID 可分别登记。
- 后台按各表期限清理；防重放记录不在应用、用户或会话撤销路径删除，未来有效窗口内仍受唯一主键阻止。
- 执行 `rtk proxy go test -race ./internal/burrow -run 'TestSAMLRequestRegistration|TestSAMLAtomicConsume' -count=1 -v`，重复同类测试仅在修改/失败需要时运行。

原子更新契约（所有前置检查和审计在同一事务）：

```go
result := tx.Model(&SAMLTransaction{}).
	Where("id = ? AND browser_hash = ? AND consumed = ? AND expires_at > ? AND config_version = ?",
		request.ID, request.BrowserHash, false, now, appConfig.ConfigVersion).
	Update("consumed", true)
if result.Error != nil {
	return result.Error
}
if result.RowsAffected != 1 {
	return errors.New("invalid_transaction")
}
```

`request` 为本事务已加载的 SAMLTransaction，`appConfig` 为同事务重新加载的 SAMLApplication，`now` 为本次注入时钟。不能用请求里的 ConfigVersion 代替数据库当前版本。

**退出标准：** 并发只有一次签发，审计失败可以安全重试未消费事务，成功消费后不能重发。

Task 6 的期限函数完整契约，放入 `internal/burrow/saml_storage.go`；时钟由调用者传入，不调用 time.Now，也不改变已有 OIDC TTL：

```go
package burrow

import (
	"errors"
	"time"
)

func samlRequestDeadlines(receivedAt, issueInstant time.Time) (time.Time, time.Time, error) {
	if receivedAt.IsZero() || issueInstant.IsZero() ||
		issueInstant.Before(receivedAt.Add(-5*time.Minute)) ||
		issueInstant.After(receivedAt.Add(time.Minute)) {
		return time.Time{}, time.Time{}, errors.New("invalid_saml_request_time")
	}
	requestWindowEnd := issueInstant.Add(6 * time.Minute)
	expiresAt := receivedAt.Add(5 * time.Minute)
	if requestWindowEnd.Before(expiresAt) {
		expiresAt = requestWindowEnd
	}
	replayUntil := requestWindowEnd
	if expiresAt.After(replayUntil) {
		replayUntil = expiresAt
	}
	return expiresAt, replayUntil, nil
}
```

其边界测试放在 `internal/burrow/saml_storage_test.go`，并纳入 Task 6 的定向运行命令：

```go
package burrow

import (
	"testing"
	"time"
)

func TestSAMLRequestDeadlines(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		offset time.Duration
		life   time.Duration
		replay time.Duration
		bad    bool
	}{
		{"current", 0, 5 * time.Minute, 6 * time.Minute, false},
		{"future-boundary", time.Minute, 5 * time.Minute, 7 * time.Minute, false},
		{"old-boundary", -5 * time.Minute, time.Minute, time.Minute, false},
		{"too-future", time.Minute + time.Nanosecond, 0, 0, true},
		{"too-old", -5*time.Minute - time.Nanosecond, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expiresAt, replayUntil, err := samlRequestDeadlines(now, now.Add(tc.offset))
			if (err != nil) != tc.bad {
				t.Fatalf("unexpected error classification: %v", err)
			}
			if tc.bad {
				if !expiresAt.IsZero() || !replayUntil.IsZero() {
					t.Fatal("invalid request received deadlines")
				}
				return
			}
			if !expiresAt.Equal(now.Add(tc.life)) || !replayUntil.Equal(now.Add(tc.replay)) {
				t.Fatalf("incorrect deadlines: life=%v replay=%v", expiresAt.Sub(now), replayUntil.Sub(now))
			}
		})
	}
	if _, _, err := samlRequestDeadlines(now, time.Time{}); err == nil {
		t.Fatal("zero IssueInstant accepted")
	}
	if _, _, err := samlRequestDeadlines(time.Time{}, now); err == nil {
		t.Fatal("zero received time accepted")
	}
}
```

执行 `rtk proxy go test -race ./internal/burrow -run 'TestSAMLRequestDeadlines|TestSAMLRequestRegistration|TestSAMLAtomicConsume|TestSAMLReplayRetention' -count=1 -v`，预期四项实际匹配并通过；数据库相关测试显式包含 sqlite/postgres 子用例。

## Task 7：挂载 SAML HTTP 与签发流程

**Files:** 新增 `saml_http.go`, `saml_http_test.go`；修改 `http.go`, `saml_codec.go`, `saml_storage.go`, `mutation.go`。

- 写 metadata/certificate/SSO/login HTTP 测试；metadata 仅声明实际支持绑定，所有 SLO/Attribute/POST-SSO 等未实现路由失败且不回落 SPA。
- 运行 `rtk proxy go test ./internal/burrow -run '^TestSAMLHTTP' -count=1 -v`，确认端点缺失或无签发能力时失败。
- 显式挂载四个 SAML GET 端点，保留当前 Cookie/CSRF/Origin 策略；禁止把 SDK Handler 整体挂载到通配前缀。
- 实现 32 KiB 原始 URL、256 KiB 解压 XML、80 字节 RelayState、深度 32 限制，重复关键参数拒绝，并复用现有内存限流边界。
- 使用登记 SP 查公钥后验签；再检查时间、Destination、ACS/索引、NameIDPolicy、响应绑定和认证请求选项，不从未验签请求信任配置。
- 无有效会话时进入带 requestType=saml 的原登录页；有有效会话且无 ForceAuthn 时复用；不合条件时重新认证而非签发。
- 在 `authenticationTx` 内最终加载应用与配置、用户与会话、当前权限与 MFA，构造并双签、缓冲 HTML、消费并写审计，提交后再写网络。
- 表单 action 只用登记 ACS；使用 `html/template` 转义表单字段；成功/失败响应 no-store；CSP 仅允许该 ACS 的 form-action，自动提交脚本使用 nonce。
- 不可信 SP/签名/ACS 输入在本地返回 400；可信请求的授权/能力错误才生成签名失败响应；内部服务故障返回 503，错误/日志无协议秘密。
- 验证 AuthnInstant=实际 Session.AuthTime，Assertion 截止时间不超 Session.ExpiresAt；密码和密码+MFA都不伪造未支持的 assurance profile。
- 执行 `rtk proxy go test -race ./internal/burrow -run 'TestSAMLHTTP|TestSAMLSDK|TestSAMLProtocolIsolation' -count=1 -v`。

**退出标准：** 全部协议声明与实际行为一致，断言不会在提交前输出，SDK 默认端点/认证时间不会泄漏到生产行为。

## Task 8：安全竞态、重放和撤销回归

**Files:** 新增 `saml_security_test.go`；扩展 `concurrency_test.go`, `mfa_config_test.go`, `protocol_security_test.go`。

- 用 existing interleave/认证锁方法构造“已开始登录后撤权，再完成签发”的测试，不使用真实 sleep。
- 覆盖用户/应用禁用、角色撤销、密码重置、MFA 重置、auth_version 变化、SP metadata 更新、签名键轮换和共享会话过期。
- 覆盖 MFA 从关闭切换为开启后 password-only 会话不能签发；强制改密未完成不能签发；TOTP replay 仍受原时间步限制。
- 覆盖另一浏览器续接、改变 requestType 串表、伪造直接 `/saml/login`、重复 RelayState/Signature 参数、请求 ID 重放与并发消费。
- 在 IssueInstant 为未来 60 秒的允许边界构造请求，登录事务到期并清理后、ReplayUntil 到期前仍拒绝同请求；删除/重建应用后也不能绕过。
- 覆盖 DB/audit/signing failure：无可用表单输出，数据库消费标记和审计一致；管理 metadata 更新审计失败也回滚。
- 增加 “正确 Response 中替换 Assertion/添加重复 XML ID” 反例，分别测试签名覆盖及提交到 SP 的实际拒绝行为。
- 执行 `rtk proxy go test -race ./internal/burrow -run 'TestSAMLSecurity|TestSAMLAtomicConsume|TestSAMLRevocation|TestSAMLProtocolIsolation' -count=1 -v`；确认两个 driver 实际运行，未有 DSN则如实标记限制。

**退出标准：** 授权变更与签发有明确事务顺序，接受范围不能因缓存或浏览器续接绕过。

## Task 9：管理表单、续接 UI 与双语显示

**Files:** `Resources.tsx`, `Auth.tsx`, `Home.tsx`, `api.ts`, `i18n.tsx`, `web/vite.config.ts`；新增 `SAMLApplicationFields.tsx` 及测试、`login-request.ts` 及测试、`web/e2e/saml.spec.ts`。

- 定义 API 类型：Application 的 protocol/saml 和 ApplicationRef 的 protocol/samlEntityId；LoginState 增加 requestType，缺省兼容旧 OIDC 响应。
- 写前端 requestType 测试：默认 OIDC、SAML 续接、未知类型拒绝、保存/读取状态时同时比较类型与 ID，不因相同 ID 续接另一个协议。
- 写组件行为测试：选择 SAML 后只提交公共字段与 metadataXml；切回 OIDC 清理残留 SAML 输入；编辑时协议不可改；错误和服务器摘要可读。
- 运行 `rtk proxy make web-check`，确认新行为断言失败后实现；不添加测试来只断言某个 CSS class。
- 新增 metadata 上传/粘贴组件，在客户端限制大小并显示文件名/摘要；服务器仍执行所有安全检查。界面只显示必要配置，不输出内部兼容 ClientID。
- 创建页面默认 OIDC，编辑按 protocol 显示；SAML 不展示 ClientSecret/PKCE/Origins；权限/角色选择器按 protocol 显示当前 EntityID，保留已有紧凑批量引用。
- 登录、改密、MFA 状态和恢复页面完整传播 requestType；重用 API/session/access helpers，前端不作最终授权判断。
- 门户继续使用 LoginURL 跳到 Jenkins，不在前端生成 SAMLResponse；显示应用协议仅限有意义的管理场景。
- 两种语言字典同步词条，检查 light/dark/system；Vite 代理增加 `/saml` 且不改变现有 `/oidc` 代理。
- 在现有浏览器环境加入创建/编辑 SAML 应用、登录上下文和拒绝续接测试，夹具只做测试，不新增独立产品示例。
- 执行 `rtk proxy make web-check` 与 `rtk proxy make test-e2e`，报告前端/浏览器和 OIDC 回归的实际结果。

前端传输契约：

```ts
type LoginRequest = { requestType: "oidc" | "saml"; requestId: string };

export function readLoginRequest(search: string): LoginRequest {
  const query = new URLSearchParams(search);
  const requestType = query.get("requestType") || "oidc";
  const requestId = query.get("requestId") || "";
  if (requestType !== "oidc" && requestType !== "saml") {
    throw new Error("invalid_request_type");
  }
  if (requestType === "saml" && !requestId) {
    throw new Error("invalid_transaction");
  }
  return { requestType, requestId };
}
```

**退出标准：** 用户可通过 UI 完成应用配置；OIDC 表单/API兼容；语言/主题支持保留。

## Task 10：Jenkins profile、版本锁与人工配置指南

**Files:** 新增 `examples/local-sso/jenkins/{Dockerfile,plugins.txt,README.md}`；修改示例 Compose、Nginx、README 与必要的忽略规则。

- 从官方发布/安全资料选择一个当前受支持的 Jenkins LTS，核实 SAML 插件兼容，记录确切 tag、digest 和目标平台；不把“最低 2.504.3”当作推荐部署版本。
- 用 Jenkins 插件安装工具解析 SAML、Matrix Authorization 完整依赖；将全部精确版本写 `plugins.txt`，镜像构建时验证已安装版本与锁一致，避免自动拉入浮动传递依赖。
- Dockerfile 基于固定 LTS 镜像，仅预装上述插件；使用非 root jenkins 用户，不安装 Docker、不挂 socket、不暴露 50000，不自动写认证配置。
- Compose 新增 `profiles: [jenkins]` 与 `jenkins-data`，由 Nginx 代理内部 8080；不增加 root Compose 服务，不修改现有卷或默认 depends_on。
- Nginx 新增 jenkins.yakir.top 虚拟主机和网络别名，变量 DNS 解析允许未启动 profile；继续关闭访问日志以避免协议参数进入日志。
- 在新 README 写完整人工步骤：空 Jenkins 安装、执行器设为 0、建立 Burrow 用户并授予普通角色、取得稳定用户 ID、预设 Matrix 管理员/读用户、配置 metadata/签名/NameID、导入 SP metadata、配置门户、隐私窗口验收。
- 明确 Jenkins Username Attribute 留空，DisplayName=displayName，Email=email，Groups 留空；使用锁定版本的 RelayState 行为（旧随机开关已移除，不写入旧字段）；NameIDPolicy=persistent；Wants Assertion Signed 和请求签名开启。
- 指明签名证书与 TLS 不同；Maximum Authentication Lifetime 至少覆盖当前 Burrow SessionTTL 和容差；Jenkins 账户管理仍由其自身授权策略处理。
- 写受控证书轮换和 Jenkins 配置备份/恢复步骤；操作仅限 Jenkins 新卷，不把切换 realm 后的本地密码当作可用备用入口。
- 执行 `rtk proxy docker compose -f examples/local-sso/docker-compose.yml config --quiet` 和 `rtk proxy docker compose -f examples/local-sso/docker-compose.yml --profile jenkins config --quiet`，两种配置均应退出 0，禁止打印完整解析配置。
- 明确后续启动命令 `docker compose -f examples/local-sso/docker-compose.yml --profile jenkins up -d --build` 只有在获授权实施和联调阶段才执行，不清理其他示例资源。

**退出标准：** Jenkins 可选，默认示例不依赖它；用户可以按文档从空环境配置，版本/恢复边界明确。

## Task 11：独立 Jenkins 互操作与资源隔离

**Files:** 新增 `web/e2e/jenkins-saml.spec.ts`, `web/e2e/run-jenkins-saml.sh`, `web/e2e/fixtures/jenkins/docker-compose.yml`, `web/e2e/fixtures/jenkins/nginx.conf.template`, `docs/testing/saml-jenkins.md`；修改 `Makefile`, `web/playwright.config.ts`。

- 增加 opt-in `make test-e2e-jenkins`，默认 `make test-e2e` 明确排除 Jenkins 专属 spec，原有 OIDC 流程保留；不使用未配置 Jenkins 的 SKIP 冒充验收通过。
- 脚本创建 mktemp 目录、唯一 Compose project、独立网络/卷与临时端口；不复用 local-sso 的固定 project/命名卷，不占既有 18080/19001/19002；不更改系统 hosts。
- 固定采用独立 Nginx HTTPS 测试夹具，浏览器使用 Chromium host-resolver-rules，将 `sso.burrow.test` 与 `jenkins.burrow.test` 映射到 127.0.0.1；容器网络这两个名字均指向测试 Nginx。脚本选择空闲临时端口，Nginx 内部监听与本机发布使用相同端口，两侧 URL 一致；如启动时发现端口被抢占，最多重新分配并重试三次，不清理占用进程。
- 使用本机已有 OpenSSL 在临时目录生成包含测试域 SAN 的短期证书，仅挂给测试 Nginx；仅该 Playwright 测试 context 设置 ignoreHTTPSErrors，不改系统信任、全局环境或产品 TLS 校验。人工 local-sso 仍使用原 HTTP 示例。
- `.test` 主机用于默认隔离验收；另用独立测试域 `jenkins.burrow-sp.test` 在同一 HTTPS 夹具下做跨站 POST 用例，配置同样的浏览器/容器映射。验证 Jenkins 在其支持的 Cookie 配置下完成请求关联，不修改 Burrow Cookie 为宽泛 Domain 或 SameSite=None，也不通过关闭浏览器 Cookie 检查规避问题。
- 真实测试从空 Burrow/Jenkins 数据开始，通过管理 API/UI 建立应用与角色，通过 Jenkins UI 配置 SAML和Matrix；测试可以自动化 UI，但产品示例不增加后台自动导入服务。
- 正常用例：初次改密、MFA 关闭/开启与绑定/验证、共享会话复用、门户进入、ForceAuthn、姓名与稳定账户 ID。
- 反例：无应用权限、用户/应用禁用、登录中撤权、重复提交同一 SAMLResponse、篡改签名；确认 Jenkins 自身拒绝，不能只检查 Burrow 错误页面。
- 检查普通用户无 Overall/Administer；指定管理用户有管理权限，Burrow Administrator 不自动转换为 Jenkins 管理员。
- 重启临时 Burrow/Jenkins 后再次登录，验证配置/证书持久化；运行受控证书替换验收。
- trap 只清理本次创建且带唯一 project 标识的资源，先校验项目名和临时目录；不运行全局 prune，不删除已有 local-sso 卷。
- Playwright 默认关闭此认证用例的原始 trace/响应体录制，失败截图/诊断去除断言、密钥和协议参数；报告仅记录非敏感 ID、状态和版本。
- 执行 `rtk proxy make test-e2e-jenkins`，保存实际版本、命令、结果和限制；没有 Docker/浏览器/许可条件时明确未运行，不标记 PASS。
- 用人工隐私窗口复核 README 全新部署步骤，区分自动化结果与用户报告。

**退出标准：** 独立 Jenkins 确实验签和创建会话，常规测试与已有示例数据不受该联调生命周期影响。

测试环境约定：

| 项目             | 固定选择                                                                                 |
| ---------------- | ---------------------------------------------------------------------------------------- |
| Compose project  | `burrow-saml-test-` 加本次随机后缀，禁止等于 `burrow-local-sso`                          |
| 临时目录         | `mktemp -d` 创建，启动前保存规范化绝对路径用于清理校验                                   |
| 拓扑             | migrate、seed、burrow、jenkins、nginx；不启动 Grafana/Nightingale/Harbor                 |
| issuer           | `https://sso.burrow.test:` 加选定端口；通过测试环境变量显式传入                          |
| Jenkins Base URL | `https://jenkins.burrow.test:` 加同一端口                                                |
| 跨站用例         | Jenkins Base URL 改为 `https://jenkins.burrow-sp.test:` 加同一端口，并同步其 SP metadata |
| TLS              | 临时证书 SAN 包含三个测试域；只对专用 Playwright context 忽略该自签证书错误              |
| 初始账号         | 仅测试环境生成临时密码，参数不打印，trace 与表单响应录制关闭                             |
| 证书信任         | 手动 metadata 的浏览器自动化操作，不引入网络 metadata 刷新服务                           |
| 清理             | 精确 project 的 compose down，删除本次卷/网络与临时目录；失败也执行                      |
| 失败证据         | 只保留脱敏状态、版本、错误类别；不用原始 SAMLResponse/秘密日志                           |

实际端口由运行时分配，不将本次端口或运行数据写回 tracked 配置。此测试夹具与人工 local-sso 共享版本锁和产品行为要求，不共享运行数据。

## Task 12：运维文档、回归与交付检查

**Files:** 新增 SAML adapter/operations/testing 文档；修改文件职责表中存在的 README、索引、依赖、恢复、AGENTS；仅在需要时调整 CI。

- 将新协议范围写入 `AGENTS.md`：仅下游 SAML A、单 IdP、多协议隔离、双签和 Jenkins 目标；保留原 OIDC PKCE、安全和历史验收约定。
- 写 adapter 文档，标明 SDK公开 API 组合、未挂载的默认端点、RequestType 兼容与真实 AuthnInstant，不宣称通用 SAML 全特性。
- 写运维指南：数据库/master key 配套备份、证书到期检查、轮换维护窗口、Jenkins realm 切换恢复、双方会话/撤销边界。
- 更新产品说明和双语 README，不重写旧版历史设计记录；在 SAML 测试报告记录自动化/人工/构建/CI 四类证据，保持旧验收记录原意。
- 确认现有 CI 双库任务会发现所有新 Go 测试，常规 browser 能覆盖 SAML UI；opt-in Jenkins 互操作入口清晰，不因加入巨型新 job 改变既有发布复用策略。
- 执行 `rtk proxy make check`，配专用 PostgreSQL DSN 时执行 `rtk proxy make test-db`；如果刚跑过相同完整后端套件且无新变化，复用本轮证据而非无理由重复。
- 执行 `rtk proxy make test-e2e`、`rtk proxy make build`、`rtk proxy make compose-config COMPOSE_ENV=.env.example`，记录实际结果。
- 执行 Task 10 两种示例 Compose 的 quiet 检查，确认构建 Jenkins 镜像与真实启动分别有证据；没有执行的检查明确列出。
- 使用本机已有 Prettier 格式化新增前端/文档/配置，并 gofmt 修改的 Go 文件；执行 `rtk git diff --check`。
- 审阅范围化 diff、依赖锁、迁移字节、日志与样例，确认无真实凭据、本地数据、Jenkins HOME、私钥或抓包文件被纳入版本控制。
- 更新本计划勾选状态和执行记录，向用户报告已改内容、测试、限制与具体待审阅差异；提交/推送仅在用户要求后执行。

**退出标准：** A 范围的功能与 Jenkins 示例交付可复现，有双库与独立 SP 证据，OIDC 行为未被扩展或弱化，报告不夸大验证结果。

## 覆盖自检与执行交接

| 规格要求                                       | 任务       |
| ---------------------------------------------- | ---------- |
| SDK公开接口、双签、原始 Redirect 编码          | 1、7、8    |
| v6 迁移、数据保留与双库                        | 2、8、12   |
| SP metadata、安全导入、跨协议隔离              | 3、7、8    |
| 密钥持久化、证书与轮换                         | 4、10、11  |
| 密码/MFA、类型化续接与撤销                     | 5、7、8、9 |
| 浏览器绑定、独立防重放期限、并发消费、审计回滚 | 2、6、7、8 |
| UI 双语/主题、紧凑引用与门户                   | 3、9       |
| 可选 Jenkins、人工配置、独立资源与恢复         | 10、11     |
| 真实 SP 验证、OIDC 回归与准确证据              | 11、12     |

计划自检已对照规格覆盖上述任务；关键接口/字段保持一致，没有把 SDK 门槛、容器镜像选择或未来测试写成已通过结果。版本解析属于 Task 1/10 的明确动作，不能在未解析前伪造镜像 digest 或插件依赖锁。

最初规划回合只执行文档检查；当前实施结果见开头执行记录及验证报告。当前无可调用 Mem0 工具，未读取或保存项目记忆。

### 规划修订记录（2026-10-20）

- 已按用户要求创建并切换 `feat/saml-idp-jenkins`，从本地 main 的 `3dd57a9` 起步；不是已提交或已推送的分支。
- 将执行次序固定为 1→2→4→3→5→6→7→8→9→10→11→12，明确四个交付批次。
- 补齐完整 v6 DDL、GORM 列名约定、HTTP/协议错误矩阵和纯函数边界测试。
- 将登录事务和防重放记录分离，增加未来 IssueInstant 边界、撤销、清理以及删除重建应用的重放反例。
- 初始规划时所有实施勾选项均未完成；该规划回合没有编写 `.go/.tsx/.sql` 功能文件或安装依赖。

用户随后授权实施，按约定选择 `superpowers:executing-plans` 在当前会话按依赖顺序执行；只有用户另行授权子代理工作，才改用 `subagent-driven-development`。不在规划交付时隐式切换到实现。
