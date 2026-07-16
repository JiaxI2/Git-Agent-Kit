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

## 谁提交 Issue

三种入口统一转换为同一 Issue schema：

1. 用户通过 `gia issue create --direction` 提交；
2. Agent 根据分析结果提出 Issue，但默认不自动认领高风险任务；
3. 定时审计/CI 发现问题后创建 Issue。

Issue 创建者不等于执行者。创建、认领、合并和发布是四个独立权限。
