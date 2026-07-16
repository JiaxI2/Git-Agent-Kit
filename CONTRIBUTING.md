# 贡献指南

## 工作流

1. 先创建或选择一个带 `agent:ready` 的 Issue。
2. 使用 `gia claim` 获取单写者租约和独立任务分支。
3. 每个 Issue 只对应一个任务分支和一个 Draft PR。
4. 使用 `gia pr request` 提交 Draft PR 申请；审批、合并和 Release 由用户完成。
5. PR 必须列出已验证、未验证、风险和回滚方式。

## 本地验证

提交前至少运行：

```text
gofmt
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

涉及 GitHub、PowerShell、安全策略或用户流程时，还需执行
`docs/TEST_PLAN.md` 中对应的 T1/T2/T3/T4 测试。

## 变更边界

- 不 force push、hard reset 或自动清理用户仓库。
- 不在 Issue 正文中执行命令。
- 不加入 approve、merge 或 release 的 Agent 自动化入口。
- 行为变化必须同步测试、README/专项文档和 `CHANGELOG.md`。
