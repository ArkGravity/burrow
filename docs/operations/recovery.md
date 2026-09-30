# 备份、恢复与升级

## 备份

备份 PostgreSQL 与对应主密钥（`BURROW_MASTER_KEY`、配置中的 `security.master_key` 或 `security.master_key_file`），将主密钥保存在独立受控的秘密存储中。本地开发默认使用公开示例主密钥；将 `security.master_key` 清空后才启用 `data/master.key` 文件模式。生产环境必须使用独立随机主密钥。数据库包含已加密的签名私钥和 Provider 密钥；只有数据库备份不足以恢复服务。

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

## 升级失败

数据库迁移具备版本和校验和，修改已经应用的迁移文件会被拒绝。应用启动不会自行修改 schema。迁移失败时先修复原因，再重跑迁移；不要绕过校验或手工提升版本号。

升级前停止旧 app，执行新镜像迁移并检查结果，再启动 app。以后新增迁移必须保持旧代码兼容，或安排明确的停机迁移。仅回退 Docker 镜像不会回退数据库；无法兼容时从升级前备份恢复，并重新评估期间的数据写入。

基础事件记录保留期由 `BURROW_EVENT_RETENTION` 决定。日志不应包含密码、客户端密钥或完整令牌；排查时不要将真实环境文件和认证请求体上传到问题单。
