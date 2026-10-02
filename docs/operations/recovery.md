# 备份、恢复与升级

## 备份

备份 PostgreSQL 与对应主密钥（`BURROW_MASTER_KEY`、配置中的 `security.master_key` 或 `security.master_key_file`），将主密钥保存在独立受控的秘密存储中。本地开发默认使用公开示例主密钥；将 `security.master_key` 清空后才启用 `data/master.key` 文件模式。生产环境必须使用独立随机主密钥。数据库包含已加密的签名私钥；只有数据库备份不足以恢复服务。

内置数据库可通过 `docker compose --profile local-db exec -T postgres pg_dump -U burrow -d burrow -Fc > burrow.dump` 备份。请按实际数据库用户名修改命令，备份文件不要加入 Git。SQLite 开发实例先停止服务再复制数据库，或使用 SQLite 的在线 backup 功能，不单独复制正在使用的 WAL 主文件。

## 恢复

1. 停止应用写入，准备独立的恢复数据库。
2. 恢复数据库内容和原来的主密钥、issuer、数据库连接配置。
3. 使用与备份兼容的应用版本运行 `migrate`，检查退出码与日志。
4. 启动服务，验证 `/readyz`、JWKS、管理员登录和一个测试 APP 的完整授权流程。
5. 保留原备份，确认恢复成功后再切换入口。

恢复时 `seed` 只补齐默认数据，不覆盖已有管理员密码或状态；不要通过修改 bootstrap 配置尝试重置密码。主密钥不是 OIDC 签名密钥；不能通过直接替换主密钥完成加密密钥轮换。本版本不提供主密钥在线重加密命令。

## 签名密钥轮换

使用相同 YAML 配置执行 `make keys-rotate CONFIG=configs/config.local.yaml`，或在 Compose 中执行 `docker compose run --rm --no-deps app keys-rotate`。新令牌使用新 kid；已有公钥继续发布，因此未过期 ID Token 仍可验证。当前保留历史公钥，不自动删除旧公钥。

## Upgrade to password-only authentication

迁移 `004_remove_upstream.sql` 将 schema v3 升级到 v4，移除上游 Provider、外部身份关联、应用与 Provider 关联、上游登录事务以及 `providers:read` / `providers:write` 权限和对应角色授权。用户与应用的 `localEnabled`、应用的 `providerIds` 字段不再接受；密码认证是唯一登录入口。

升级步骤：

1. 备份数据库、原 master key 和旧版本配置。在旧版本中检查启用的用户和应用是否允许本地登录。
2. 对仍在使用的上游账号设置密码并开启本地登录；将正在使用的应用开启本地登录。无需继续使用的记录可以显式禁用。确认至少一位管理员可通过密码登录。
3. 从新版本使用的 YAML 中删除 `providers` 配置段，并从服务环境移除 `BURROW_PROVIDER_ALLOWED_CIDRS` / `BURROW_ALLOW_PRIVATE_PROVIDERS`。保留原 master key。
4. 停止旧服务器，使用新版本执行 `migrate`。若启用的用户没有密码或禁止本地登录，或启用的应用禁止本地登录，迁移会报出用户/应用数量并整体回滚。在旧版本处理后重试。
5. 迁移成功后运行 `seed`，再启动新服务器并检查管理员密码登录和下游 OIDC 授权。

迁移保留用户、密码、资料、启用状态、应用配置、签名密钥、用户/组/角色关系和其他授权。旧密码会话及关联 Token/授权事务保留；上游或未知认证来源的会话与 Token 被撤销，关联的未完成授权被删除。迁移策略写入同一事务的审计，审计失败也会回滚。已签发的离线 ID Token 及下游应用自己的会话仍按原有期限和应用策略处理。

被禁用、没有密码的旧账号不会被删除或自动启用。升级后管理员可以先重置其临时密码，再显式启用；该用户登录后必须修改临时密码。启用无密码账号会被拒绝。旧应用的禁用状态也保持不变，重新启用意味着允许密码认证。

迁移会删除上游配置和身份关联，旧版本无法直接使用 v4 数据库；回滚须恢复升级前数据库备份及对应配置。不要在真实数据库上通过降版本号回滚。

## 升级失败

当前 schema 为 v4。迁移 `003_custom_roles.sql` 清理空权限 Viewer 的用户/组关联和未使用的旧 Editor/Viewer，保留有成员及实际权限的旧角色为普通角色；权限码保持不变。详见 [角色升级说明](../development/seed.md#upgrade-from-editor-and-viewer)。升级后旧二进制无法运行在 v4 数据库上，回滚需要升级前备份。

数据库迁移具备版本和校验和，修改已经应用的迁移文件会被拒绝。应用启动不会自行修改 schema。迁移失败时先修复原因，再重跑迁移；不要绕过校验或手工提升版本号。

升级前停止旧 app，执行新镜像迁移并检查结果，再启动 app。以后新增迁移必须保持旧代码兼容，或安排明确的停机迁移。仅回退 Docker 镜像不会回退数据库；无法兼容时从升级前备份恢复，并重新评估期间的数据写入。

基础事件记录保留期由 `BURROW_EVENT_RETENTION` 决定。日志不应包含密码、客户端密钥或完整令牌；排查时不要将真实环境文件和认证请求体上传到问题单。
