# Contributing to Burrow

English · [简体中文](#中文说明)

Bug reports, documentation improvements, translations and focused pull requests
are welcome. Read the [project conventions](AGENTS.md) and
[development guide](docs/development/getting-started.md) before changing code.

## Issues and feature proposals

Search [existing issues](https://github.com/ArkGravity/burrow/issues) first.
For a bug, include the Burrow version or commit, operating system, installation
method, reproduction steps, expected behavior and actual behavior. Use a minimal
example and redact passwords, tokens, client secrets, encryption keys and private
configuration from logs and attachments.

For a feature, describe the use case and why existing behavior is insufficient.
Discuss changes to protocols, grants, authentication sources or deployment
components before implementing them. Burrow focuses on a single-organization,
single-instance OIDC service.

Report suspected security vulnerabilities through the private channel in
[SECURITY.md](SECURITY.md).

## Pull requests

1. Fork the repository and create a focused branch.
2. Keep changes scoped to one problem and follow existing source ownership and helpers.
3. Update user-facing strings in both dictionaries in `web/src/lib/i18n.tsx`, and keep English and Chinese READMEs aligned when changing onboarding.
4. Update applicable configuration references and operational documentation when behavior changes.
5. Run appropriate checks and describe the results and limitations in the PR.
6. Open a [pull request](https://github.com/ArkGravity/burrow/pulls) explaining the problem, resulting behavior and relevant validation.

Use Go with `gofmt` and Bun with frozen lockfiles. Use your locally installed
Prettier for frontend and Markdown formatting. Preserve the Go module path
`github.com/ArkGravity/burrow`, explicit checksummed migrations, native SVG logo
and the single-instance architecture. Never edit an applied migration or commit
credentials, local databases or generated integration data.

## Verification

The [development guide](docs/development/getting-started.md#checks-and-formatting)
lists executable checks. Choose them according to the change:

- Documentation: formatting, links and `git diff --check`.
- Backend behavior: relevant Go tests and static analysis.
- Authorization, seed, migration or OIDC changes: relevant tests on SQLite and PostgreSQL, including affected privilege and concurrency boundaries.
- Frontend behavior: typecheck and unit tests; browser/session changes also need applicable end-to-end flows.

Use a dedicated PostgreSQL test database. Report skipped coverage explicitly and
keep local results separate from CI, manual acceptance and official protocol
certification. The PR description should explain what changed, why and how it was
verified; screenshots help reviewers assess visible UI changes.

## 中文说明

欢迎提交问题、文档改进、翻译和范围明确的 PR。开始前阅读
[项目约定](AGENTS.md)与[开发指南](docs/development/getting-started.md)。

- 提交问题前先搜索已有 Issue，提供版本或 commit、运行环境、部署方式、复现步骤和预期/实际结果；删除日志和附件中的敏感数据。
- 新功能说明实际场景；协议、授权模式、认证来源和部署组件的扩展先讨论，再实现。
- 一个 PR 聚焦一个问题，说明变更原因、最终行为、实际检查结果和未覆盖的范围。
- 用户可见文案维护双语字典，入门说明同步中英文 README；行为变化同步配置与运维文档。
- 使用 Go、Bun、gofmt 和本机 Prettier，保留冻结锁文件、现有架构与显式迁移，不提交凭据或本地数据。
- 文档改动检查格式、链接及 `git diff --check`；代码变更按上面的范围选择检查，数据库测试使用专用数据库。
- 漏洞通过[安全报告入口](SECURITY.md)私密报告。
