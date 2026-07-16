# 对抗式威胁模型

| 威胁 | 默认防御 | 仍需平台配置 |
|---|---|---|
| Agent 直接推 main | 分支协议、Hook 模板 | GitHub branch protection |
| 两个 Agent 同写分支 | claim/handoff 单写者协议 | 执行器遵守 ownership 标签 |
| 验证旧 commit | `--expected-head` 强制 SHA 匹配 | PR 更新后重新验证 |
| 本地稳定目录污染 | worktree 验证 | 为 Agent 指定独立目录 |
| 脏 worktree 被删除 | 默认拒绝 remove | 人工明确 `--force` |
| 云端谎报本机验证 | PR 模板区分已验证/未验证 | 人工审查与 required checks |
| 子模块或依赖供应链污染 | protected paths + 高风险分类 | CODEOWNERS、签名、依赖审查 |
| 恶意 Issue 注入命令 | Issue 只是规格，不直接执行正文 | 执行器禁止把 Issue 文本当 shell |
| Token 泄漏 | 不存储 Token，复用 gh 凭据 | 最小权限 GitHub App/token |
| PR 合并后难回滚 | squash/revert 策略 | 禁止共享历史 force push |
| 通知 Webhook 外泄 | 默认仅 console | 用户自行保护 URL/内容 |

## 关键拒绝策略

任何不确定情况优先失败关闭（fail closed）：

- HEAD 不匹配：拒绝验证；
- 仓库脏：拒绝验证；
- worktree 脏：拒绝删除；
- 缺少 `gh` 认证：拒绝远程写入；
- 高风险任务：不应默认自动认领；
- 分支或远端冲突：不自动 force push。
