# Jenkins 与 Burrow SAML

这是 `examples/local-sso` 的可选 Jenkins profile。Jenkins 配置通过 UI 手动完成；镜像只预装插件，不自动创建 Burrow 应用或写入 Jenkins 的认证配置。根 Compose 不参与。

## 版本与启动

2026-10-20 核实的版本为 Jenkins LTS `2.580.1-jdk21`，固定多架构 digest：

```text
sha256:a660310e39ade10631f774bacd5219de767dfd08947ea5c32a739b5e5bb382c1
```

[Dockerfile](Dockerfile) 使用该 digest。[plugins.txt](plugins.txt) 锁定 SAML `4.623.v7875d61cd9f5`、Matrix Authorization `3.3` 及全部 21 项插件依赖。构建保留插件工具的下载校验，并检查安装版本和依赖数量与锁文件完全一致。升级时重新解析完整依赖并执行互操作回归；不要把根插件改成 `latest`。

版本来源：[Jenkins LTS 发布](https://www.jenkins.io/changelog-stable/)、[SAML 插件](https://plugins.jenkins.io/saml/)、[Matrix Authorization](https://plugins.jenkins.io/matrix-auth/)。SAML 所选版本包含 [SECURITY-4065 修复](https://www.jenkins.io/security/advisory/2026-09-02/#SECURITY-4065)。这表示版本来源已核实，实际构建/联调状态以[验证记录](../../../docs/testing/saml-jenkins.md)为准。

按[父级指南](../README.md)启动基础服务，在本机域名配置中另加入 `jenkins.yakir.top`：

```text
127.0.0.1 sso.yakir.top jenkins.yakir.top
```

```bash
docker compose -f examples/local-sso/docker-compose.yml --profile jenkins config --quiet
docker compose -f examples/local-sso/docker-compose.yml --profile jenkins up -d --build
docker compose -f examples/local-sso/docker-compose.yml exec jenkins cat /var/jenkins_home/secrets/initialAdminPassword
```

最后一条命令仅供操作者本机读取首次安装密码，不复制到日志或问题单。打开 http://jenkins.yakir.top，完成安装向导并创建本地维护管理员；选择已有插件即可，无须安装推荐的构建插件。Jenkins 数据保存在独立 `jenkins-data` 卷，不发布 50000 或 Docker socket。

在 Jenkins System 中将 **Jenkins URL** 设为 `http://jenkins.yakir.top/`，在 Nodes 的 built-in node 设置执行器数为 **0**。该示例仅验证网页登录。配置 SAML 前备份 Jenkins home，并保留当前管理窗口。

## 配置用户与 Jenkins 权限

1. 在 Burrow 创建两个有姓名和邮箱的普通用户，分别用于 Jenkins 管理员和只读用户。不要用 Burrow Administrator 身份隐式映射 Jenkins 权限。
2. 编辑 Burrow 用户，复制只读的「用户 ID」。该稳定 ID 将作为 SAML persistent NameID 和 Jenkins 账号 ID，用户名更改不会改变它。
3. 打开 [Jenkins Security](http://jenkins.yakir.top/manage/configureSecurity/)，找到「授权策略（Authorization）」区域，选择 **安全矩阵（Matrix-based security）**。这是 Matrix Authorization 插件在简体中文界面的名称。在保存前，先为当前 Jenkins 本地维护管理员保留 `Overall/Administer`，再显式添加上述两个 **User** ID，大小写原样保留。给 Burrow 管理用户 ID `Overall/Administer`，给只读用户 ID `Overall/Read`，不给 anonymous 权限。
4. 在切换 realm 前保留当前本地维护管理员的显式管理权限，保存并验证当前窗口仍可管理。切换到 SAML 后，本地密码登录不再是备用入口；已有窗口也不保证永久可用。恢复方案见下文。

## 配置 Jenkins SAML realm

从 `http://sso.yakir.top/saml/metadata` 复制 XML，在 Jenkins Security 选择 SAML 2.0：

| 设置                            | 值                                                     |
| ------------------------------- | ------------------------------------------------------ |
| IdP Metadata                    | 粘贴 Burrow XML                                        |
| IdP Metadata URL                | 留空；不自动刷新                                       |
| Username Attribute              | **留空**，使用 NameID 作为账号 ID                      |
| Username Case Conversion        | None                                                   |
| Display Name Attribute          | `displayName`                                          |
| Email Attribute                 | `email`                                                |
| Groups Attribute                | 留空                                                   |
| Maximum Authentication Lifetime | `86400` 秒；自定义时至少覆盖 Burrow SessionTTL 和容差  |
| Data Binding Method             | `urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect`   |
| NameIDPolicy Format             | `urn:oasis:names:tc:SAML:2.0:nameid-format:persistent` |
| SP Entity ID                    | 留空，使用 Jenkins 默认值，以导出的 metadata 为准      |
| Authentication Context          | 留空                                                   |
| Force Authentication            | 首次验证关闭；需要每次重新认证时再开启                 |
| Auth Request Signature          | 开启                                                   |
| Wants Assertion Signed          | 开启                                                   |
| Logout URL                      | 留空，不配置 SLO                                       |
| Keystore、Key Alias、密码       | 首次可留空，由插件创建独立 SP 签名密钥                 |

当前锁定版本不再提供旧版 `Use Random relayState` 开关；不要照旧文档写入已移除的字段。联调应检查插件实际发出的随机 RelayState 及其会话绑定，不将 RelayState 用作任意返回 URL。

保存后从 `http://jenkins.yakir.top/securityRealm/metadata` 下载 SP metadata。启用请求签名后，文件包含签名证书。不要将 SP keystore/private key 上传到 Burrow；只上传公开 metadata。配置字段依据为[该版本插件源码](https://github.com/jenkinsci/saml-plugin/tree/4.623.v7875d61cd9f5/src/main/java/org/jenkinsci/plugins/saml)；通用参考见[插件配置说明](https://github.com/jenkinsci/saml-plugin/blob/main/doc/CONFIGURE.md)。

## 在 Burrow 中配置应用

在「应用」创建 Jenkins，协议选择 **SAML 2.0**，应用登录 URL 设置为：

```text
http://jenkins.yakir.top/securityRealm/commenceLogin?from=%2F
```

上传或粘贴 Jenkins metadata，检查服务器提取的 Entity ID、ACS、公开签名证书。支持一个 POST ACS、一张 RSA 签名证书；若同一公钥另列 encryption descriptor，不会误算为第二张签名证书。无需 Client ID、Client Secret 或 PKCE 设置。

应用的 Icon URL 可填写以下 Jenkins SVG 示例：

```text
http://jenkins.yakir.top/static/1ec166e9/images/svgs/logo.svg
```

`static/1ec166e9` 是当前 Jenkins 的静态资源版本路径；升级后若链接失效，从 Jenkins 页面复制新的 Logo 地址并更新应用图标。

创建普通角色，授予 Jenkins 应用登录权限，再分配给上述两个 Burrow 用户或其用户组。Jenkins 的管理员/只读权限仍由它自己的 Matrix 配置决定，Burrow 不发送组、角色或管理员属性。

## 验收

使用隐私窗口从 Jenkins 发起登录。完成 Burrow 密码认证、首次临时密码修改，以及全局 MFA 开启时的绑定/验证。确认返回 Jenkins 后账号 ID 等于 Burrow 用户 ID，姓名/邮箱正确。

分别验证只读用户不能管理 Jenkins、指定管理员可以管理、无应用权限用户不能建立新 Jenkins 会话。退出 Jenkins 后，从 Burrow 门户再次进入，应复用有效 Burrow 会话；开启 Force Authentication 后应重新输入密码并按当前策略验证 MFA。测试撤权时须先退出 Jenkins，已有 Jenkins 会话由 Jenkins 自己管理。

本地 HTTP 示例只用于同站开发域名。SAML 签名证书与 HTTPS/TLS 证书用途不同。跨站 HTTPS Cookie 行为另由隔离测试覆盖，不能把本地 HTTP 成功当作生产验证。

## 轮换、备份与恢复

SP keystore、Jenkins home 中的 secrets、config.xml 和两端 metadata 共同决定信任关系。备份完整 Jenkins home，保留 Burrow 数据库及原 master key。可将本机备份放在忽略目录 `examples/local-sso/jenkins/backups/`，不得提交。

SAML IdP 证书有效期为 365 天，安排到期前维护窗口：暂停新 SAML 登录，使用原 Burrow 配置执行带原因的 `keys-rotate --protocol saml`，复制新的 Burrow metadata 到 Jenkins，保存后重新验收。旧待处理 SAML 请求会失效，历史公开证书保留；Jenkins 只信任一张当前 IdP 证书，不能假设自动无缝轮换。

SP 证书变化时，先取得新 Jenkins metadata，再更新 Burrow 应用。配置版本递增，旧请求不能继续；原应用权限保留。两端自己的已建立会话不因证书轮换而自动退出。

失去 Jenkins 管理访问时，仅停止本例的 Jenkins 服务并从维护备份恢复它的完整 home/config/secrets/keystore，再启动和验收。若备份使用本地 realm，恢复后使用备份对应的本地管理员；不要假设切换 SAML 后还能用旧密码登录。不要清空 Burrow、Grafana、Nightingale 或 Harbor 数据。

```bash
docker compose -f examples/local-sso/docker-compose.yml stop jenkins
# 由操作者恢复本 Jenkins 卷的维护备份
docker compose -f examples/local-sso/docker-compose.yml --profile jenkins up -d jenkins
```
