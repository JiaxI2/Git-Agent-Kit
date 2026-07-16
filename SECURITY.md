# 安全策略

## 报告漏洞

请不要通过公开 Issue 报告凭据泄漏、权限绕过、任意命令执行、远端 ref
覆盖或 worktree 数据破坏问题。

优先使用 GitHub 的
[Private vulnerability reporting](https://github.com/JiaxI2/git-isolated-agent-kit/security/advisories/new)。
如果该入口不可用，请联系仓库维护者，并仅提供复现所需的最小日志；删除 Token、
Webhook URL、用户目录和私有仓库地址。

## 支持范围

安全修复优先应用于最新 Release。报告应包含版本、操作系统、Git/GitHub CLI
版本、最小复现步骤、预期/实际结果，以及是否涉及远端写入。

## 权限边界

GIA 的 `permissions` 是工作流策略，不是身份认证系统。生产部署应让 Agent
使用独立 GitHub App installation token，并用 GitHub ruleset/branch protection
限制审批、合并、Tag 和 Release。不要把用户级 `gh` 登录态当作用户与 Agent
之间的硬边界。
