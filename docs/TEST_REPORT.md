# GIA Kit 0.1.0 多智能体验证报告

执行日期：2026-07-16

总体结论：**未达到正式集成或发布准入条件**。T0/T1 本地核心隔离能力通过；T2 真实 GitHub、T3 对抗测试和 T4 用户体验暴露多项阻塞缺陷。

## 测试方法

- T2、T3、T4 由三个独立智能体并行执行，主线程负责权限边界、证据回读和清理。
- T2/T4 使用 `JiaxI2` 账号下的专用私有临时仓库；证据回读后已删除。
- T3 使用本地 bare remote 和进程级认证隔离，不修改全局 GitHub 凭据。
- 未 merge、未 Tag、未 Release、未 force push，也未修改 AiCoding、Codex-Skills 或 GIA 源文件。

## T0 静态与单元测试

结果：**PASS_WITH_GAPS**

- `gofmt`：通过，10 个 Go 文件无格式漂移。
- `go test ./...`：通过。
- `go test -race ./...`：通过。
- `go vet ./...`：通过。
- Linux amd64 构建：通过。
- Windows amd64 构建：通过。
- 发布二进制 SHA-256：与 `SHA256SUMS` 一致。
- 单元测试覆盖率偏低：
  - `internal/issue`：23.8%；
  - `internal/validate`：6.2%；
  - `internal/workflow`：11.2%；
  - CLI、config、gitx、notify：0%。
- Go module 已绑定 canonical 身份 `github.com/JiaxI2/git-isolated-agent-kit`；发布前仍需完成真实 T2/T3/T4 回归并确认 Tag/Release 证据。

## T1 本地沙箱集成

结果：**PASS_WITH_GAP**

已验证：

- 临时 bare remote 和 clone 创建成功；
- 稳定工作区保持在原分支；
- 验证 worktree 独立创建；
- 错误或缺失 expected HEAD 被拒绝；
- 正确 expected HEAD 的 smoke 验证通过；
- dirty repository 被拒绝验证；
- dirty worktree 被拒绝删除；
- clean worktree 可正常移除；
- 所有测试临时目录均已清理。

PowerShell AST 与 PSScriptAnalyzer 通过，但 Safety gate 失败：

- `scripts/test-local.ps1:34` 删除测试脏文件时缺少 `ShouldProcess`、`-WhatIf` 或明确的安全封装；
- `scripts/test-local.ps1:38` 递归删除测试临时目录时缺少同类保护。

## T2 真实 GitHub 端到端测试

结果：**FAIL**

临时仓库：

- `JiaxI2/gia-kit-t2-20260716-202052`，测试完成后已删除；
- Issue `#1`；
- Draft PR `#2`；
- 成功的 Issue dispatch run：`29497940187`；
- 被跳过的 workflow dispatch run：`29498059002`。

通过或部分通过：

- 私有测试仓库、默认分支和 GIA 配置创建成功；
- `gia doctor` 的 Git、GitHub CLI、Go、仓库、配置和认证检查通过；
- 手工创建 labels 后，`scan` 能发现 `agent:ready` Issue；
- Issue dispatch 能完成低风险分类并写入安全评论；
- `claim` 能创建并推送独立任务分支；
- Draft PR 保持 Open/Draft；
- `status` 能确认稳定仓库仍位于 `main` 且保持 clean。

失败与缺陷：

1. `issue create` 返回成功，但 Issue 正文只剩 `## 背景与方向`，多行结构化正文被截断。
2. `issue create` 未添加 `agent:ready`、`risk:*`、`executor:*` labels，首次 `scan` 返回空结果，opened dispatch 被跳过。
3. claim 评论只保留 `GIA claimed this task.` 首行。
4. handoff 评论只保留 `<!-- GIA:HANDOFF:START -->`，SHA、状态和执行器审计信息全部丢失。
5. `gia issue list` 未实现。
6. workflow 声明 `workflow_dispatch`，但 job 依赖 `github.event.issue.labels`，手工 dispatch 恒定 skipped。

## T3 对抗与失败关闭测试

结果：**13 PASS / 3 FAIL**

通过：

- 恶意 Issue 中的 shell、PowerShell 删除命令和提示注入未被执行；
- 错误或缺失 expected HEAD 被拒绝；
- dirty repository 和 dirty worktree 被拒绝；
- 无效 remote、缺失/无效 GitHub 认证、远端 non-fast-forward 冲突均失败关闭；
- detached HEAD 被状态输出明确暴露；
- 子模块 HEAD 漂移使仓库变 dirty，并被 validation 拒绝；
- 高风险方向关键词可将 risk 推断为 `high`。

失败与缺陷：

1. 同一 Issue 重复 claim 仍可成功；单写者所有权没有基于 Issue 状态或 label 的原子校验。
2. `gh issue edit` 或 `gh issue comment` 失败时，错误被丢弃，命令仍返回 `CLAIMED` 成功。
3. clean 但未推送的 commit 可以通过 validation；没有验证目标 SHA 是否存在于远端或可从 PR ref 到达。

附加代码审查发现：`protected.paths` 和 `rejectForcePush` 当前只存在于配置模型，没有运行时使用点，尚不能形成真正的 protected-path 门禁。

## T4 用户体验测试

结果：**PARTIAL / NOT ACCEPTED**

临时仓库 `JiaxI2/gia-kit-t4-20260716-202216`，测试完成后已删除。

量化结果：

- 主流程使用 7 条 GIA 命令；
- 主流程耗时 118.2 秒；
- 含证据复核和清理共 200.1 秒；
- 三个用户目标中 2 个部分或完全成功；
- 需要 4 次人工决策，其中 2 次属于意外恢复操作。

主要体验问题：

1. P1：Issue 创建报告成功但没有 labels，导致核心 `Issue -> scan` 路径失败。
2. P1：结构化 Issue 正文被截断，用户无法获得预期的目标、非目标、验收和回滚说明。
3. P2：README 在 Windows 示例中混用 `.\bin\gia.exe` 和未加入 PATH 的 `gia`。
4. P2：`scan` 返回 `[]` 时没有查询条件、原因或修复建议。
5. P2：`init` 创建未跟踪 `.gia/` 后仓库立即显示 dirty，但没有提交配置或下一步提示。
6. P3：`--help` 和 `help` 均失败，只有无参数调用显示顶层命令列表。

## 阻塞缺陷

正式集成前至少需要解决：

1. Windows 多行参数安全传递，保证 Issue/评论/handoff 内容完整。
2. label bootstrap 或原子失败策略，禁止创建无标签任务后报告成功。
3. claim 原子所有权校验，并传播所有 GitHub 元数据错误。
4. validation 增加远端 SHA/PR ref 可达性检查。
5. 让 `protected.paths`、受保护分支和 force-push 拒绝配置真正进入运行时门禁。
6. 修复 `workflow_dispatch` 条件或移除不可执行入口。
7. 修复 PowerShell 测试脚本 Safety gate。
8. 补齐 CLI/config/gitx/notify 和远程流程测试覆盖率。
9. canonical module 身份已改为 `github.com/JiaxI2/git-isolated-agent-kit`；仍需建立远端仓库、Tag 和 Release，确保测试证据可追溯。

## 清理结果

- T2/T4 私有远程测试仓库已删除并确认不再存在。
- T3 本地 bare remote、认证隔离目录和所有沙箱已清理。
- 本轮构建和测试产生的临时二进制、worktree 和 `%TEMP%\gia-test-*` 已清理。
