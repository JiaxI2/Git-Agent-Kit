# Changelog

## Unreleased

- **docs**: 增加由 Microsoft Visio 工程化绘制的 GIA 多 Agent Git 隔离
  工作流图、可编辑源文件和可复现 Diagram IR。

## 0.1.0 - 2026-07-16

- **test**: 最终 T2 真实 GitHub、T3 对抗场景和 T4 用户体验全部通过；
  T3 全量 35/35、发布候选 targeted 4/4，T4 核心目标 3/3 且无意外恢复。
- **docs**: 增加 claim 后 switch/edit/commit/push 快速路径、最终验证报告和
  `0.1.0` Release Notes 草案。
- **fix(cli)**: 失败路径只输出一个结构化 JSON 文档，console notify 改写
  stderr，避免污染机器可读 stdout；JSON 保留可读的 HTML 字符。
- **release**: 二进制改由 `v0.1.0` Tag workflow 使用固定 Go 1.22.12
  clean checkout 构建，发布 Windows/Linux amd64 资产和顶层 `SHA256SUMS`。
- **fix(github)**: 折叠多行 direction 的标题空白；Issue dispatch 对同一 Issue 串行执行并 upsert 单一分类评论，避免 opened/labeled 重复评论和 risk label/评论冲突。
- **security**: validation 优先绑定当前分支 upstream；没有 upstream 时仅接受唯一远端 ref tip，拒绝用其他 ref 上相同 SHA 冒充当前 PR/ref。
- **feat**: 增加 `init --format json|yaml|yml`、严格 unknown-field 配置解析和
  required commands 的显式 `--config` 加载。
- **feat**: 增加 `gia pr request`，仅允许已认领且 executor 匹配的 Issue 在
  clean、已推送且远端 SHA 一致的任务分支上申请 Draft PR。
- **security**: 限制 Draft PR body file 必须解析到仓库内，并在 doctor 中明确
  GitHub App/ruleset 才是审批、merge 和 release 的硬权限边界。
- 增加 `gia help`、`gia --help`、主要命令帮助和 `gia issue list`。
- 空 Issue 查询保留机器可读结果，并增加查询条件、原因与恢复建议。
- `init` 输出增加 `.gia/` 提交或忽略的明确下一步。
- `workflow_dispatch` 支持安全的可选 Issue 输入，不再因缺少 Issue payload 恒定跳过。
- Windows 本地测试清理增加临时根绝对路径校验和 `ShouldProcess`。
- 统一 README 的 Windows/Linux PATH 快速开始，并补充 CLI、配置、Git 和通知测试。
- Issue、claim 与 handoff 多行正文统一通过临时 `gh --body-file` 传递，并在成功、失败和取消后清理。
- Issue 创建前自动补齐 lifecycle、risk、executor labels；无法创建或复核时失败关闭。
- 使用结构化 GitHub 输出复核新 Issue 的编号、URL 和 labels，并提供 `List`/`ListReady` API。
- 增加平台中立的 Draft PR 申请 API：始终创建 Draft、复核远端字段并返回 `PENDING_USER_APPROVAL`，不提供审批、合并或发布能力。
- Claim 改用 `gia/claims/<issue>` 远端 ref 原子租约，重复和并发认领失败关闭。
- Claim 元数据错误完整传播，并补偿标签、任务分支和租约分支。
- Validation 要求 expected SHA 是配置 remote 的当前 ref tip，拒绝未推送和过期 SHA。
- `protected.branches`、`protected.paths` 和 `rejectForcePush` 进入运行时门禁。
- 配置支持 JSON/YAML/YML 单一来源加载，多格式并存失败关闭。
- 新增平台中立 permissions policy；Draft PR 可由执行器申请，批准、合并和发布默认仅 `user`。

- Initial standalone GIA Kit.
- Structured Issue creation from one optimization direction.
- Ready Issue scanning and controlled claiming.
- Independent task branches and validation worktrees.
- SHA-bound smoke/full/release validation.
- Single-writer PR handoff metadata.
- Console, webhook and command notifications.
- Local feedback capture and adversarial test plan.
