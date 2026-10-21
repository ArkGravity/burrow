# Burrow SAML IdP 与 Jenkins 示例设计

日期：2026-10-20；实施更新：2026-10-21。状态：方案 A 已在功能分支实现并完成同站/跨站 Jenkins 自动互操作；当前检查证据与网络构建限制见[验证记录](../../testing/saml-jenkins.md)。

## 1. 目标与授权边界

在现有 Burrow 单实例模块化单体中增加下游 SAML 2.0 IdP，以 Jenkins 为唯一首期互操作验收目标。在 `examples/local-sso` 增加按需启动的 Jenkins 示例，复用账号、密码、MFA、共享会话、应用门户和 `app:<application ID>:login` 权限。

最初回合只编写设计与实施计划；用户随后以“继续任务”授权实施和隔离联调。当前未提交、推送、合并或发布。用户确认的 A 方案作为本规格输入，不重新进行选型问答。

用户随后明确要求在新分支进行 SAML 接入。已从本地 `main` 的 `3dd57a9d65552cbc15d8024452ee7093b90c49c4` 创建并切换到 `feat/saml-idp-jenkins`；规划文档和未来功能变更都留在该分支。此操作不包含提交、推送、合并或功能实现。

## 2. 已确定的最小范围

| 项目            | 首期行为                                                            |
| --------------- | ------------------------------------------------------------------- |
| 协议角色        | Burrow 为 IdP；Jenkins 为 SP                                        |
| 登录发起        | SP 发起；门户转到 Jenkins 登录入口                                  |
| AuthnRequest    | HTTP-Redirect；强制 RSA-SHA256 请求签名                             |
| SAMLResponse    | HTTP-POST；Response 与 Assertion 双签；RSA-SHA256、SHA256 digest    |
| SP 配置         | 一个 Entity ID、一个 POST ACS、一张 RSA 请求签名证书                |
| 配置交换        | 上传或粘贴 SP metadata XML；手动复制 Burrow metadata XML 到 Jenkins |
| NameID          | persistent；值为 Burrow 不变的用户 ID                               |
| 身份属性        | 固定 `username`、`displayName`、`email`；不输出组或角色             |
| Jenkins 账户 ID | NameID；Username Attribute 留空；姓名、邮箱通过对应属性展示         |
| 认证            | 当前 Burrow 密码、强制改密与全局 MFA 策略                           |
| 会话复用        | 满足当前用户认证版本和 MFA 策略的共享会话可复用                     |
| 强制认证        | 支持 ForceAuthn，不能接受早于该请求的认证                           |
| 被动认证        | 首期拒绝 IsPassive=true，不创建交互登录流程                         |
| 认证上下文      | 支持 PasswordProtectedTransport；不宣称特定 MFA assurance profile   |
| 请求有效期      | IssueInstant 不早于接收时间 5 分钟、不晚于接收时间 60 秒            |
| 登录事务        | 最多 5 分钟，且不得超过请求 IssueInstant + 5 分钟 + 60 秒容差       |
| 断言期限        | 签发后最多 3 分钟，且不晚于共享会话到期时间                         |
| 时钟容差        | 接收请求允许 60 秒；Jenkins 配置并验收 60 秒响应校验容差            |
| 退出            | 各自管理会话；不提供跨应用退出                                      |

不包含：IdP 发起的 unsolicited Response、POST AuthnRequest、多 ACS、组同步、任意属性映射、断言加密、SLO、Artifact、SCIM、上游 SAML 登录、SonarQube 或其他新增集成。metadata 不声明这些能力；未实现的 SAML 路径返回 404 或明确的协议错误。

## 3. SDK 选择与实施前门槛

优先使用 `github.com/zitadel/saml`，验证基线为 `v0.4.1`。它作为 Go 依赖嵌入进程，不部署 ZITADEL 服务。现有 `github.com/zitadel/oidc/v3` 保留。

已比较的备选：`crewjam/saml` 支持 SP/IdP，但本次检查的 IdP 请求签名验证存在缺口；自行组合 XML/签名实现扩大维护责任。选择 ZITADEL SAML 的依据是其 IdP 能力与接口，不是供应商品牌一致。

SDK 的能力不等于 Burrow 已完成协议安全性：Burrow负责浏览器绑定、授权、状态期限、防重放、并发消费和审计。

实施 Task 1 必须先证明以下条件：

1. 能验证 Jenkins 的原始 Redirect 签名，包括原始查询编码、RelayState 是否存在的区别；重复关键参数拒绝，不能重编码后错误验签。
2. 能使用公开 XML/签名接口构造 Response 与 Assertion 双签，篡改任一签名覆盖的内容均验签失败。
3. 能控制 AuthnInstant、SessionNotOnOrAfter、Recipient、Audience、InResponseTo、NameID 格式和认证上下文。
4. 请求认证、会话、事务和 HTTP 路由由 Burrow 管理；不挂载 SDK 的整个默认路由集合来意外开放 SLO/Attribute Query。
5. 不需要 fork SDK、访问私有字段或编写自己的 XML canonicalization / XMLDSig 算法。
6. Go 工具链和依赖安全检查可接受，具体解析版本写入依赖文档。

允许在 `internal/burrow/saml_codec.go` 组合 SDK 的公开 XML/签名能力；不强制复用其默认 Response 构造器。该构造器默认使用签发时间作为认证时间，不能直接代表共享会话的真实认证时间。

实施时必须先通过这些门槛；不得放宽签名、伪造认证时间、静默扩展范围或直接改用另一库。现已通过门槛，并将传递签名依赖 goxmldsig 固定到修复版 v1.6.0，证据见依赖说明。

## 4. 应用与存储

保持现有 `internal/burrow` 包，按职责加文件。新增迁移 `006_saml_idp.sql`，同时更新迁移列表、各版本校验和、schema readiness 和相关测试；不改动 001–005。迁移不批量吊销现有 OIDC 会话或令牌。

新会话使用 UTC 时间并设置 `sessions.utc_timestamps=true`；迁移为旧会话保留默认 false，按原数据库时间语义检查到期与 OIDC 认证时间，避免延长或提前终止旧会话。SAML 要求一次新的 UTC 会话认证后再复用历史会话。

### 4.1 应用

- `Application.Protocol` 为 `oidc` 或 `saml`，默认 `oidc`，创建后不可修改。
- SAML 应用仍使用公共应用 ID、名称、状态、图标、门户登录 URL 和角色授权。
- SAML 应用继续生成唯一内部 ClientID，以满足原表 `NOT NULL UNIQUE` 及兼容引用；它不是 SAML Entity ID，不要求用户输入，不用于协议认证，SAML UI 不展示为客户端配置。
- SAML 应用 ClientType 存储为 `saml`，不生成 SecretHash；OIDC 专属回调、Origins、PKCE、密钥重置输入在服务端拒绝。
- 所有 OIDC 查找、授权、token、userinfo 和历史协议事务访问都限制 `protocol=oidc`；不能只依赖 ClientType 或 UI 隐藏。
- `ApplicationRef` 增加 `protocol` 和 `samlEntityId`，使用批量查询填充。OIDC 显示 Client ID，SAML 显示 Entity ID；不为权限选择器引入 N+1 或额外权限要求。

`saml_applications` 保存 ApplicationID（一对一外键）、EntityID（唯一）、ACSURL、SigningCertificate、MetadataXML、ConfigVersion。证书与 metadata 是公开配置，仍仅在有应用读取权限的管理流程中返回，不附加到个人门户响应。

导入只接受单个 EntityDescriptor，提取一个 POST ACS、一张签名证书。证书算法为 RSA、至少 2048 位；校验可用期和证书用途，不将自签证书当作需要公共 CA 信任的 TLS 证书。Entity ID 作为精确匹配标识，不强制为可访问 URL。ACS 遵循现有开发 HTTP／生产 HTTPS URL 校验。

metadata 大小最多 256 KiB，拒绝 DTD、实体声明、重复 ID、深度超过 32 的 XML；不自动抓取证书、schema、metadata URL。不清除现有应用授权关系。提取签名用途的证书时，不把同一公钥的 encryption KeyDescriptor 误算成第二张签名证书；传给 SDK 的配置采用经过验证的最小 signing/ACS 描述，并保留上传原文用于管理展示。

修改 SP metadata、ACS、证书或 Entity ID 时递增 ConfigVersion 并使旧未消费 SAML 事务失败，防止在新信任配置下继续旧请求；修改与审计原子提交。授予角色仍要求 `authorization:write`。

### 4.2 SAML 事务与登录续接

`saml_transactions` 保存：内部随机 ID、ApplicationID、SPRequestID、BrowserHash、RelayState、ACSURL、ConfigVersion、ForceAuthn、FailureStatus、ReceivedAt、ExpiresAt、Consumed、UserID、SessionID。FailureStatus 仅持久化已验签请求的白名单能力错误，保证签名/审计失败后重试不会转成成功响应，不保存原始请求 XML。ApplicationID + SPRequestID 唯一；内部 ID 不等于外部请求 ID。

另用小型 `saml_request_replays` 表保存 SPEntityID、SPRequestID、ReplayUntil，前两项为复合主键，不依赖应用/用户外键。它与登录事务在同一事务内插入，用于保存请求已被接受的事实，不保存浏览器凭据或断言。SPEntityID 最多 1024 字节，SPRequestID 最多 256 字节，大小检查在入库前完成。

`LoginTransaction.RequestType` 新增默认值 `oidc`。登录 API、登录状态、认证上下文和页面续接均携带 `requestType=oidc|saml`；旧调用省略该字段继续按 OIDC 处理。外部输入先规范化，再持久化，未知值拒绝；无 RequestID 时不得携带 `saml`。

续接只由服务端用类型枚举和已验证内部事务 ID 生成同站路径；不接受用户提供的返回 URL。所有 MFA/改密步骤保留原协议、请求和浏览器绑定。`finishLogin` 创建共享 Session 时，将该 SessionID/UserID 原子绑定到本次 SAMLTransaction；ForceAuthn 最终要求当前共享会话与该绑定一致，不能仅比较时间戳。

密码重置、MFA 重置、账号禁用、用户/应用删除、登录取消、后台过期清理涵盖 SAML。强制改密仅保留当前登录事务和其对应协议请求，不用单个无类型 keepRequest 在两张表中做全局排除。

登录事务与防重放记录采用不同期限：设 R 为接收时间、I 为 IssueInstant，接收要求 `R - 5m <= I <= R + 60s`；`ExpiresAt = min(R + 5m, I + 5m + 60s)`；`ReplayUntil = max(ExpiresAt, I + 5m + 60s)`。请求一旦登记，即使登录取消、事务清理、用户重置、证书轮换或应用删除，防重放记录也保留至 ReplayUntil。删除应用再创建相同 Entity ID，不能重新接受有效窗口内相同的 SPRequestID。

已消费登录事务可在 ExpiresAt 后清理；防重放记录只有在 ReplayUntil 之后才清理。服务端一次性签发与 SP 的 Response/Assertion 重放校验是两件事，均须测试。现有数据库足以保存这些短期记录，不新增缓存或其他服务。

## 5. 端点与流程

| 端点                                                      | 用途                                                       |
| --------------------------------------------------------- | ---------------------------------------------------------- |
| `GET /saml/metadata`                                      | IdP EntityDescriptor，Entity ID 固定为配置 issuer 加此路径 |
| `GET /saml/certificate`                                   | 当前 IdP 公共签名证书 PEM                                  |
| `GET /saml/sso`                                           | 接收已签名 Redirect AuthnRequest                           |
| `GET /saml/login?requestId=...`                           | 检查浏览器绑定、恢复或完成已保存的 SAML 事务               |
| `GET /api/v1/auth/context?requestType=saml&requestId=...` | 使用原认证上下文端点展示应用名                             |

issuer 只取配置，不从任意 Host 或 Forwarded 头生成。Vite 代理增加 `/saml`；未支持的 `/saml/*` 不回落到 SPA 页面。现有 Cookie 的 host-only、HttpOnly、SameSite=Lax 和生产 Secure 约束保留。

处理顺序：

1. 限流；原始 URL 总长不超过 32 KiB，压缩前后请求解码均有上限，解压 XML 不超过 256 KiB。
2. 拒绝重复 SAMLRequest、RelayState、SigAlg、Signature；RelayState 最多 80 字节，Jenkins 开启随机 RelayState。
3. 从有界 XML 取出 Issuer，查找已登记且启用的 SAML 应用；Issuer 的解析不等于信任。
4. 验证请求签名、SAML Version、请求 ID、IssueInstant、Destination、POST 响应绑定、NameIDPolicy 和 ACS。
5. ACS URL 如显式提交必须精确匹配；如省略，用唯一登记 ACS；如提交索引，只能匹配 metadata 中唯一 ACS 的实际索引。未知索引拒绝。
6. NameIDPolicy 省略或 persistent 接受，其他格式返回 InvalidNameIDPolicy；IsPassive 或不支持的认证上下文请求失败，不能忽略。
7. 建立 browser cookie；在同一数据库事务中插入防重放记录及服务器端登录事务，用复合主键裁决重复请求，303 到同站续接路径。
8. 无有效共享会话时进入原登录页面并带 requestType=saml。ForceAuthn 要求会话认证晚于本请求建立，并绑定本次限制登录完成结果；不通过删除共享 cookie 模拟认证。
9. 进入认证锁与数据库事务，复查当前用户、会话、认证版本、MFA、应用协议/状态/权限、配置版本、证书和请求有效期。
10. 先在内存构造并签名表单，写入消费标记和 `saml:issue` 审计，提交后再将表单写入 HTTP 响应。签名或审计失败均回滚，不输出可用断言。

将步骤 9–10 与现有管理/认证变更共用 `authenticationTx` 的锁。不得在锁内读网络请求体、下载 metadata 或向网络写数据。允许在短事务内使用已加载配置做一次签名，以使授权决策、签名与消费有明确的线性化点。客户端断开后不重发已消费事务，重新从 Jenkins 发起。

AuthnInstant 使用 Session.AuthTime，SessionNotOnOrAfter 使用 Session.ExpiresAt；签发新断言不刷新认证年龄。MFA 仍严格按全局策略检查；PasswordProtectedTransport 只表示最低密码认证能力，不把 TOTP 描述为未经验证的 MFA profile。首期 RequestedAuthnContext 只接受省略或 exact PasswordProtectedTransport，其他返回 NoAuthnContext。

错误分两类：未知 SP、错误签名、未登记 ACS 等在 Burrow 本地终止，不向请求地址发回身份；只有确定可信 SP/ACS 后，才允许返回签名的标准失败响应。数据库或审计不可用失败关闭。日志不写原始查询、AuthnRequest、RelayState、SAMLResponse、私钥或用户秘密。

在可信 SP/ACS 已确定后，能力错误等标准失败响应也要完成防重放登记和一次性响应处理；签名或审计故障不回滚此前已提交的请求防重放记录，但回滚本次签发的 Consumed 与审计，使同一已登记的浏览器续接可重试。重新提交 `/saml/sso` 的同一外部请求始终拒绝。

## 6. 密钥生命周期

独立 `saml_signing_keys` 保存 ID、加密 PKCS8 私钥、证书 PEM、Active、CreatedAt。只有一个活动键，使用数据库约束保证；保留历史公钥/证书供恢复与诊断。RSA 3072 位、自签签名证书有效期 365 天；生产入口 TLS 证书与此证书分开管理。

`migrate` 完成 DDL 后显式调用幂等初始化：已有键原样保留，不因重启或重复迁移生成替代键，不覆盖 master key。`serve` 不生成键；存在启用 SAML 应用时，缺失、损坏、过期或无法解密的活动键使启动/就绪失败。无 SAML 应用时可继续服务原 OIDC，但 SAML metadata/签发在缺键时失败。

新增或启用 SAML 应用时，在管理事务内验证当前 IdP 密钥可用；缺失、损坏或过期时拒绝启用，而不是先写入启用状态再让整站 readiness 失败。停用状态允许保存 SP 配置，便于运维准备。

扩展现有 `keys-rotate --protocol=oidc|saml`，默认 `oidc`。SAML 轮换要求提供非空 `--reason`，替换活动键、失效旧 SAML 事务与运维审计原子完成，不改变 OIDC JWKS。

首期使用维护窗口内轮换：暂停 Jenkins 新 SAML 登录，轮换、更新 Jenkins 信任的 IdP metadata、验签、恢复登录。SP 证书变更则重新导入 Jenkins metadata。因 SDK 单 SP 证书限制，不宣称自动双证书平滑切换。备份必须同时保留数据库和原 master key。

## 7. Jenkins 本地示例

- 原 `examples/local-sso/docker-compose.yml` 增加 `jenkins` profile 与独立 `jenkins-data`；根目录生产 Compose 不增加 Jenkins。
- 入口 `http://jenkins.yakir.top`，仅复用 Nginx 的本机 80 端口，Jenkins 内部 8080 不直接发布；不开放 agent 端口。
- `examples/local-sso/jenkins/Dockerfile` 预装固定版本插件；`plugins.txt` 固定 SAML、Matrix Authorization 及全部解析依赖，镜像固定 Jenkins LTS 标签与 digest。版本锁在实施前解析并验证，不以最低兼容版本充当当前推荐版本。
- 不挂 Docker socket，不建构建代理/任务，控制器执行器设为 0。保留标准初次安装流程，用 Jenkins 本地管理员先配置权限再切换 SAML。
- Nginx 使用变量解析 Jenkins 上游，未启动 profile 时不阻止其他服务启动；不把 Jenkins 加为默认 Nginx depends_on。增加网络域名别名，保留现有 OIDC 示例。
- 不自动创建 Burrow 应用、用户或角色。人工配置顺序：建立已授权的 Burrow 普通用户与管理用户；复制 Burrow metadata；Jenkins 开启请求签名、随机 RelayState、persistent NameID；取得 SP metadata 并在 Burrow 建 SAML 应用；设置门户启动地址；进行隐私窗口验收。
- Jenkins Username Attribute 留空，Display Name Attribute=displayName，Email Attribute=email，Group Attribute 留空；Jenkins 用 NameID 作授权主体，指南提供从 Burrow 管理界面取得用户 ID 的路径。
- 使用 Matrix 授权：指定管理用户 Overall/Administer，普通验收用户 Overall/Read，anonymous 无权限；Burrow Administrator 不自动获得 Jenkins 管理权限。
- 切换 Security Realm 前备份 Jenkins 配置，文档给出仅操作 Jenkins 卷的停机恢复步骤；不宣称本地密码管理员在 SAML realm 下仍可直接登录，也不自动关闭安全控制。

Jenkins ACS 为 `/securityRealm/finishLogin`，metadata 为 `/securityRealm/metadata`，门户目标使用已核实的 `/securityRealm/commenceLogin` 入口。Jenkins 的 Maximum Authentication Lifetime 与 Burrow 会话 TTL 对齐并覆盖容差，不能通过伪造 AuthnInstant 避免过期检查。

## 8. 测试与交付条件

| 层次       | 必须覆盖                                                                                           |
| ---------- | -------------------------------------------------------------------------------------------------- |
| 双数据库   | v5→v6、空库、幂等、旧校验和、唯一性、分离期限清理、原子消费、审计回滚                              |
| 协议       | 正确双签、请求编码、签名/参数篡改、证书/算法、ACS/索引、Audience、Recipient、时间与 NameIDPolicy   |
| 认证授权   | 密码/MFA/强制改密、MFA 重新启用、ForceAuthn、认证版本、用户/应用禁用、角色撤销、删除、浏览器串用   |
| 生命周期   | 密码/MFA 重置清理两协议请求，证书或 SP 配置变更使旧请求失败，重启持久化                            |
| 跨协议     | SAML ClientID 不可用于 OIDC authorize/token/userinfo；OIDC 应用不可用于 SAML；requestType 不能串表 |
| 浏览器     | 中英文、三种主题、应用配置、续接；使用内存或测试夹具模拟 SP 只验证 UI，不当作 Jenkins 验收         |
| 独立互操作 | 实际固定版本 Jenkins 接收并验证断言，正常/撤权/重复响应验收；不以 SDK 自测替代                     |
| 回归       | 原 Web/SPA OIDC 与权限边界；原 local-sso 配置不依赖 Jenkins profile                                |

常规浏览器测试保持临时 SQLite 和既有端口。真实 Jenkins 联调通过单独 opt-in 入口运行，使用测试专用 Compose project 和卷，清理只能针对本次创建的资源。常规测试不能依赖人工配置好的 local-sso 数据。

独立 Jenkins 浏览器夹具使用临时 Nginx HTTPS 入口与临时自签证书，覆盖同站和跨站 SAML POST 的请求关联；浏览器错误忽略仅限该测试 context，不能关闭 Cookie 或签名校验。人工 `examples/local-sso` 继续使用既有 HTTP 模式，两者不共享数据或更改系统 hosts/证书信任。

验收报告分别记录自动化测试、人工 Jenkins 操作、容器构建和远程 CI；本设计不预先声明任何结果通过。已发行版本和 2026-10-01 OIDC、2026-10-02 MFA 的用户验收记录保持历史属性。

## 9. 文件归属与实施顺序

| 顺序 | 文件职责                                                                        |
| ---- | ------------------------------------------------------------------------------- |
| 1    | SDK 测试夹具、`saml_codec.go` 与双签/验签测试契约                               |
| 2    | `006_saml_idp.sql`、`models.go`、`core.go`：schema、事务、清理、readiness       |
| 3    | `saml_applications.go`、`admin.go`：metadata 导入、管理验证、紧凑引用、协议隔离 |
| 4    | `saml_keys.go`、`cmd/burrow/main.go`：密钥初始化、验证与轮换                    |
| 5    | `login_request.go`、`mfa.go`、`http.go`、`oidc_http.go`：类型化续接和撤销       |
| 6    | `saml_http.go`、`saml_storage.go`：入口、签发、消费、审计                       |
| 7    | `Resources.tsx`、`Auth.tsx`、`Home.tsx`、API 类型与 i18n：应用配置、显示与登录  |
| 8    | Jenkins profile、镜像、版本锁、Nginx、README 和 opt-in 浏览器互操作             |
| 9    | 双库/浏览器/构建回归、SAML 运维文档、AGENTS 协议边界和准确验证报告              |

详细任务与命令见 [实施计划](../plans/2026-10-20-saml-jenkins.md)。执行时先完成 SDK 门槛，再依赖顺序推进；不主动开启子代理。

## 10. 资料与自检

- [ZITADEL SAML v0.4.1](https://github.com/zitadel/saml/releases/tag/v0.4.1)
- [SDK SP 验签与单证书限制](https://github.com/zitadel/saml/blob/v0.4.1/pkg/provider/serviceprovider/serviceprovider.go)
- [SDK Response 默认认证时间与上下文](https://github.com/zitadel/saml/blob/v0.4.1/pkg/provider/response.go)
- [Jenkins SAML 插件](https://plugins.jenkins.io/saml/)
- [Jenkins SAML 配置](https://github.com/jenkinsci/saml-plugin/blob/main/doc/CONFIGURE.md)
- [Jenkins 认证与授权边界](https://www.jenkins.io/doc/book/security/access-control/)
- [OWASP SAML 安全说明](https://cheatsheetseries.owasp.org/cheatsheets/SAML_Security_Cheat_Sheet.html)

规格自检：确认只有方案 A 和 Jenkins；没有恢复上游身份；认证、签发与 SP 会话边界分开；metadata 和 Cookie 声明与实际范围一致；历史验收与未来计划分开；所有 SDK 门槛均为尚未执行的验证工作。当前会话无可调用 Mem0 工具，未声称读取或保存项目记忆。
