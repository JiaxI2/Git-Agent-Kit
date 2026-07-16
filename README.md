# Git Isolated Agent Kit（GIA Kit）

一个独立、可移植的多 Agent Git 隔离开发工具包。它将网页版 GPT、Codex、Claude Code、CI 和人工开发者视为可替换执行器，由 Git/PR 负责隔离、审计、交接、验证与发布门禁。

## 核心原则

- 一个 Issue、一个任务分支、一个 PR。
- 单分支单写者；交接必须绑定 commit SHA。
- Agent 只能拥有任务分支，不能拥有主分支或发布权限。
- 云端修改与本地验证使用独立执行面。
- 本地验证默认使用 `git worktree`，不切换或污染稳定工作区。
- 自动化失败时默认停止，不执行 force push、hard reset 或强制清理。

## 5 分钟开始

### Windows

```powershell
.\scripts\bootstrap.ps1
$env:PATH = "$(Resolve-Path .\bin);$env:PATH"
gia help
gia init --repo C:\path\to\repo
# 或：gia init --repo C:\path\to\repo --format yaml
gia doctor --repo C:\path\to\repo
```

### Linux/macOS

```bash
./scripts/bootstrap.sh
export PATH="$(pwd)/bin:$PATH"
gia help
gia init --repo /path/to/repo
# or: gia init --repo /path/to/repo --format yaml
gia doctor --repo /path/to/repo
```

目标仓库需要安装并登录：

```bash
gh auth login
```

`init` 默认生成 `.gia/config.json`，也可通过 `--format yaml|yml` 选择
对应格式。成功后会明确列出下一步：检查实际配置文件，然后选择将 `.gia/`
提交给团队，或将其加入目标仓库的 `.gitignore`；随后运行 `doctor`。

## 傻瓜式使用

用户只输入一个优化方向：

```powershell
gia issue create --repo F:\Project\Demo --direction "收敛安装和升级流程，减少重复 PowerShell"
```

GIA 自动：

1. 推断风险等级；
2. 生成包含目标、非目标、验收、回滚和执行边界的 Issue；
3. 添加 `agent:ready`、`risk:*`、`executor:*` 标签；
4. 等待 Agent 扫描并认领。

Agent 获取任务：

```powershell
gia issue list --repo F:\Project\Demo
gia scan --repo F:\Project\Demo
gia claim --repo F:\Project\Demo --issue 123 --executor web-agent
gia pr request --repo F:\Project\Demo --issue 123 --executor web-agent
```

`issue list` 与 `scan` 都查询打开且带有生效配置中 `issue.readyLabel` 的
Issue。若结果为空，JSON 输出仍保留 `data: []`，并在 `guidance` 中给出实际
查询条件、常见原因和恢复建议。

`pr request` 仅为已认领 Issue 创建 Draft PR。它要求当前任务分支已推送、
本地 HEAD 等于远端分支 tip、仓库干净、executor 被允许，并返回
`PENDING_USER_APPROVAL`。审批、merge、Tag 和 Release 仍由用户在 GitHub
规则保护下完成。

本地 Codex 隔离验证：

```powershell
gia worktree create --repo F:\Project\Demo --pr 45 --root F:\Project\Demo-worktrees
# 输出 head SHA 后：
gia validate --repo F:\Project\Demo-worktrees\pr-45 --profile full --expected-head <SHA>
```

交接：

```powershell
gia handoff --repo F:\Project\Demo --pr 45 --to local-agent --state LOCAL_VALIDATION --note "需要 Windows Full 验证"
```

反馈：

```powershell
gia feedback --repo F:\Project\Demo --category ux --message "worktree 路径提示仍不够直观" --pr 45
```

## 命令

| 命令 | 用途 |
|---|---|
| `help` / `--help` | 查看顶层或指定命令帮助 |
| `init` | 在目标仓库生成单一 `.gia/config.json|yaml|yml` |
| `doctor` | 检查 Git、GitHub CLI、Go、认证和配置 |
| `issue create` | 从一句优化方向创建结构化 Issue |
| `issue list` | 列出打开且带有 ready 标签的 Issue |
| `scan` | 查找 `agent:ready` Issue |
| `claim` | 认领 Issue、创建并推送独立分支、更新标签 |
| `pr request` | 为已认领 Issue 申请 Draft PR，等待用户审批 |
| `worktree create/remove` | 创建或安全移除隔离验证目录 |
| `validate` | 运行绑定 SHA 的 smoke/full/release 验证 |
| `handoff` | 在 PR 评论写入可审计交接块 |
| `status` | 查看分支、HEAD、清洁状态和 worktree |
| `feedback` | 将问题/建议记录到 `.gia/feedback` |
| `notify` | 控制台、Webhook 或自定义命令通知 |

## 自动执行模式

`.github/workflows/gia-agent-dispatch.yml` 提供安全的 Issue 检测入口。默认仅分类和产生执行请求，不授予任意代码写权限。实际云端 Agent 可通过 GitHub App、ChatGPT/Codex 云任务或其他执行器读取 `agent:ready` Issue。

手工触发时，`issue_number` 是可选输入。未提供编号时，工作流只输出安全摘要并成功结束，不读取不存在的 Issue payload，也不修改仓库或 Issue 状态；提供编号时，仅处理带有 `agent:ready` 标签的 Issue。

```powershell
gh workflow run gia-agent-dispatch.yml
gh workflow run gia-agent-dispatch.yml -f issue_number=123
```

推荐自动化等级：

- **A0 手动**：用户创建 Issue，人工选择执行器。
- **A1 半自动（默认）**：Issue 自动生成和分类，Agent 自动发现，人工批准认领。
- **A2 受控自动**：低风险任务自动认领，高风险任务要求批准。
- **A3 全自动**：不建议用于拥有发布、凭据、工作流或子模块写权限的仓库。

## 配置

初始化后编辑目标仓库中唯一的 `.gia/config.json`、`config.yaml` 或
`config.yml`：

- 自定义分支规则；
- 自定义 smoke/full/release 命令；
- 定义保护路径；
- 配置通知 Webhook 或命令；
- 决定是否允许自动认领。

`issue`、`scan`、`claim`、`pr request`、`validate`、`notify` 和 `doctor`
支持 `--config <path>` 显式选择配置；显式路径优先于 `.gia` 自动发现。解析会
拒绝 unknown fields，避免拼写错误导致策略静默失效。

详见 [配置说明](docs/CONFIGURATION.md)。

## 测试与持续迭代

详见 [测试方案](docs/TEST_PLAN.md) 和 [对抗式威胁模型](docs/THREAT_MODEL.md)。

## 当前边界

- GIA 不直接调用特定 AI 服务；执行器通过 Issue/PR 协议接入。
- GIA 不存储 GitHub Token，复用 `gh` 的认证。
- GIA 不自动合并、打 Tag 或发布 Release。
- GIA 不绕过 GitHub 分支保护。
- `permissions` 只表达 workflow policy；用户与 Agent 的硬身份边界必须由
  独立 GitHub App/token 和 ruleset 建立。同一 `gh` 用户身份不能充当该边界。
- 默认配置的验证命令是 Go 仓库示例，目标项目应按技术栈调整。
