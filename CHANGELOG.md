# Changelog

## Unreleased

- 增加 `gia help`、`gia --help`、主要命令帮助和 `gia issue list`。
- 空 Issue 查询保留机器可读结果，并增加查询条件、原因与恢复建议。
- `init` 输出增加 `.gia/` 提交或忽略的明确下一步。
- `workflow_dispatch` 支持安全的可选 Issue 输入，不再因缺少 Issue payload 恒定跳过。
- Windows 本地测试清理增加临时根绝对路径校验和 `ShouldProcess`。
- 统一 README 的 Windows/Linux PATH 快速开始，并补充 CLI、配置、Git 和通知测试。

## 0.1.0 - 2026-07-16

- Initial standalone GIA Kit.
- Structured Issue creation from one optimization direction.
- Ready Issue scanning and controlled claiming.
- Independent task branches and validation worktrees.
- SHA-bound smoke/full/release validation.
- Single-writer PR handoff metadata.
- Console, webhook and command notifications.
- Local feedback capture and adversarial test plan.
