# GIA 0.1.0 验证修复计划

## 模式

计划模式（Plan Mode），执行路线已确定，无待选架构分支。

## 目标

修复首轮 T2/T3/T4 暴露的阻塞缺陷，建立 canonical Git 仓库，重新完成真实
GitHub、对抗场景和用户体验验证。只有三类验证全部通过后，才允许接入
下游 lifecycle consumer。

## 范围

- Windows 多行 GitHub 参数传递和 label bootstrap。
- claim 单写者所有权、错误传播及失败回滚。
- 远端 SHA 可达性和 protected policy 门禁。
- CLI help、workflow dispatch、PowerShell 安全和用户提示。
- canonical 仓库、真实测试证据和临时资产清理。

## 非目标

- 自动 merge、Tag 或 Release。
- 绕过 GitHub 分支保护。
- 在验证通过前修改任何下游 lifecycle consumer。

## 决策记录

- 使用三个 Git worktree 隔离并行修改。
- 先建立本地审计基线，再决定远端仓库可见性。
- GitHub 写操作失败时必须失败关闭，不能报告部分成功。
- 下游平台只消费验证后的独立 capability，不反向绑定 GIA 源码。

## 回滚

每组修复独立提交；可按提交回退。测试仓库和 worktree 均为临时资产，完成
证据回读后按已验证绝对路径移除。
