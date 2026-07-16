# GIA Kit 测试方案

## 目标

验证功能正确性、Git 隔离、安全拒绝、跨平台行为、Agent 交接效率和用户体验。

## 测试层级

### T0 静态与单元测试

```bash
go fmt ./...
go vet ./...
go test ./...
```

覆盖配置初始化、风险推断、模板生成、路径净化和校验逻辑。

GitHub I/O 聚焦测试还必须覆盖：

1. Issue 正文和 claim/handoff 评论的多行内容不经命令行参数传递；
2. 临时 body file 在成功、失败和 context 取消后三种路径均被删除；
3. 缺失标签可被补齐，权限不足时在创建 Issue 前失败关闭；
4. `gh` 空输出、异常 JSON、无效 Issue URL 或缺失附加标签不得返回成功；
5. `List` 和 `ListReady` 对状态、标签、数量及结构化输出执行严格校验。
6. Draft PR 申请始终包含 `--draft`，完整正文绑定 executor，远端非 Draft 或
   字段不匹配时返回包含 PR URL 的部分成功错误，且不暴露审批、合并或发布能力。

配置加载需覆盖 JSON、YAML、YML 三种单一来源，以及多格式并存失败关闭；权限
默认值需确认 Agent 只能申请 Draft PR，批准、合并和发布主体仍为 `user`。

### T1 本地沙箱集成

运行：

```bash
./scripts/test-local.sh
```

Windows：

```powershell
.\scripts\test-local.ps1
```

场景：

1. 创建临时 bare remote；
2. 创建稳定主仓库；
3. 初始化 GIA；
4. 创建任务分支；
5. 创建隔离 worktree；
6. 验证稳定工作区未切换、未污染；
7. 在 worktree 制造脏文件，确认删除被拒绝；
8. 使用错误 SHA 验证，确认失败关闭；
9. 使用正确 SHA 执行 smoke；
10. 删除操作仅接受已验证位于测试临时根内的绝对路径，并经过 `ShouldProcess`；
11. 清理全部临时资源。

### T2 GitHub 测试仓库

在专用测试仓库执行：

- 一句话创建 Issue；
- `issue list` 和 `scan` 检测；
- `claim` 标签迁移和分支创建；
- 云端 Agent 创建 Draft PR；
- `handoff` 评论生成；
- 本地 worktree 验证；
- PR HEAD 更新后旧 SHA 验证失败；
- required checks 后 squash merge；
- 删除远程任务分支。

不得首次在生产仓库测试自动认领。

### T3 对抗测试

- Issue 正文包含 `rm -rf`、PowerShell 删除命令或提示注入；确认 GIA 不执行正文。
- 从两个独立 clone 并发 `claim`；确认只有一个能首次创建
  `gia/claims/<issue>`，第二个失败且不覆盖租约或任务分支。
- 模拟 `gh issue edit` 部分生效后失败、评论失败；确认标签和本次新建 ref 被
  补偿，命令不报告 `CLAIMED`，补偿失败信息包含人工恢复 ref。
- 模拟断网、过期认证、远端分支已存在、PR 被更新。
- 尝试删除脏 worktree。
- 修改 protected path，确认风险升级流程和人工门禁。
- 子模块 dirty、未推送 commit、远端 PR head 前移后的旧 SHA、detached HEAD。

### T4 用户体验测试

邀请不了解内部实现的用户只完成三项操作：

1. 输入优化方向；
2. 查看任务状态；
3. 收到完成通知。

记录：成功率、操作次数、失败信息可理解性、从 Issue 到 Draft PR 的人工干预次数。

必须同时验证：

1. `gia help`、`gia --help`、`gia <command> --help` 和嵌套命令帮助返回 0；
2. `init` 输出明确提示检查 `.gia/config.json`，并选择提交或忽略 `.gia/`；
3. 空 `issue list` / `scan` 保持 `data: []`，同时报告 `state`、ready label、limit、可能原因和恢复建议；
4. `workflow_dispatch` 无 `issue_number` 时成功结束且不修改状态；有编号时只处理 `agent:ready` Issue；
5. Windows 快速开始中的命令在同一 PowerShell 会话可直接复制执行。

## 反馈闭环

每次异常执行：

```bash
gia feedback --category bug|ux|security|improvement --message "..." --pr N
```

反馈文件可随复现日志打包。迭代优先级：

1. 数据损坏或权限越界；
2. 隔离失效；
3. 错误成功/验证误报；
4. 无法恢复的流程阻塞；
5. 用户操作复杂度；
6. 性能和体验改进。

## 版本迭代准入

新版本必须满足：

- 所有 T0/T1 通过；
- 无已知高危隔离缺陷；
- 配置向后兼容或提供迁移；
- Release Notes 列出安全行为变化；
- 至少一次真实 GitHub T2 回归。
