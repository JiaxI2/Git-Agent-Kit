# 架构

## 第一性原理

多 Agent 开发的根本问题不是“谁写代码更快”，而是多个不完全可信执行器同时操作共享状态。最小安全解法是：

1. Git commit 是不可变交付单元；
2. branch 是短期写入租约；
3. PR 是审查和验证事务；
4. worktree 是本地文件系统隔离；
5. Issue 是需求真源；
6. commit SHA 是验证结果唯一锚点。

## 控制面

GIA 定义：任务规格、风险等级、执行器权限、分支规则、交接状态、验证等级、通知和反馈。

权限配置是平台中立 workflow policy：执行器默认可提交 Draft PR 申请，但批准、
合并和发布默认只属于 `user`。该策略用于 GIA 的决策与审计，不是凭据边界；硬隔离
必须由独立 GitHub App/token、分支保护和发布环境实现。

## 执行面

`web-agent`、`local-agent`、`ci`、`human` 都是可替换执行器。执行器不得隐式共享本地状态，只能通过 GitHub Issue、分支、commit、PR 和评论交接。

## 信任面

- L0：执行器自检；
- L1：PR CI；
- L2：本地隔离 Full；
- L3：人工审查；
- L4：Release 验证。

## Issue 生命周期

`READY -> CLAIMED -> DRAFT_PR -> CI_PASSED -> LOCAL_VALIDATION -> READY_FOR_REVIEW -> MERGED -> RELEASED`

失败可进入 `REWORK` 或 `BLOCKED`，但不得跳过验证等级。

### GitHub I/O 完整性

创建 Issue 前，GIA 会检查并补齐配置声明的生命周期标签以及当前任务的
`risk:*`、`executor:*` 标签。标签查询、创建或最终复核失败时，Issue 创建
必须停止；现有同名标签不会被覆盖。

Issue 正文和 claim/handoff 评论通过私有临时文件传给 `gh --body-file`，
避免 Windows command shim 截断多行参数。临时文件在成功、失败和取消后都
必须精确清理。Issue 创建后还会读取结构化远端数据，复核编号、URL 和实际
附加标签；如果远端创建已发生但复核失败，错误必须返回可能已创建的 Issue
URL，供用户恢复或人工检查。

### Claim 单写者租约

`claim` 先检查 Issue 仍为 `OPEN + agent:ready`，再竞争固定远端分支
`gia/claims/<issue>`。只有远端首次创建该 ref 才获得所有权；已有 ref、
`up-to-date`、非 fast-forward 或其他不确定结果都失败关闭。任务分支也只能
首次创建，GIA 不更新已有远端分支。

标签迁移和审计评论属于同一 claim 操作。任一步失败都不会返回 `CLAIMED`，
并会尝试恢复标签、删除本次创建的任务分支和租约分支；若补偿失败，错误会
列出需人工检查的 ref。租约分支是 Issue 已被认领的审计记录，任务结束后的
治理流程应与任务分支一起删除。

## 谁提交 Issue

三种入口统一转换为同一 Issue schema：

1. 用户通过 `gia issue create --direction` 提交；
2. Agent 根据分析结果提出 Issue，但默认不自动认领高风险任务；
3. 定时审计/CI 发现问题后创建 Issue。

Issue 创建者不等于执行者。创建、认领、合并和发布是四个独立权限。

## Draft PR 申请边界

平台执行器可以通过 `workflow.RequestDraftPullRequest` 申请 Draft PR。请求必须
提供仓库、本地目标分支、任务 head、标题和 executor；正文会追加
`GIA:DRAFT-PR` 元数据块并通过 `--body-file` 提交。远端创建后，GIA 复核
PR 编号、URL、Draft 状态、base、head、标题和正文，成功结果固定为
`PENDING_USER_APPROVAL`。

该能力只创建等待用户审批的 Draft PR，不提供 approve、merge、Tag、Release
或发布接口。protected base 的决策仍由调用方配置和主线治理层执行，不在此
平台中立 API 内隐式放行。

## 验证 SHA 绑定

`validate` 不只比较本地 `HEAD`。它会 fetch 配置的 validation remote，并要求
expected SHA 等于至少一个远端 tracking ref 的当前 tip。未推送 commit、远端
PR 分支已经前移后的旧 SHA、detached HEAD 和受保护分支都会被拒绝。验证前还会
将该 SHA 与 `<remote>/<defaultBranch>` 比较；触及 `protected.paths` 时停止并
要求人工审查。
