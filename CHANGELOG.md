# Changelog

## Unreleased

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

## 0.1.0 - 2026-07-16

- Initial standalone GIA Kit.
- Structured Issue creation from one optimization direction.
- Ready Issue scanning and controlled claiming.
- Independent task branches and validation worktrees.
- SHA-bound smoke/full/release validation.
- Single-writer PR handoff metadata.
- Console, webhook and command notifications.
- Local feedback capture and adversarial test plan.
