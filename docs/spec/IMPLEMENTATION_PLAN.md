# 实施计划

1. 固化当前 0.1.0 验证快照为本地 Git 基线。
2. 创建三个隔离 worktree：
   - T2：GitHub I/O、完整正文、labels 和 issue list。
   - T3：claim 原子性、错误传播、远端 SHA 和 protected policy。
   - T4：CLI help、workflow dispatch、PowerShell safety 和用户提示。
3. 合并三个提交并执行 T0/T1、本地安全和跨平台构建门禁。
4. 建立 canonical GitHub 仓库并替换 module 占位身份。
5. 由三个独立智能体重新执行 T2/T3/T4，记录可复核证据并清理临时资产。
6. 三类验证全部通过后，才允许下游平台实现和验证 lifecycle 接入。
