# 追踪矩阵

| 缺陷 | 实现区域 | 验证 |
|---|---|---|
| 多行正文和评论截断 | `internal/issue`、`internal/workflow` | T2 完整正文与 handoff 评论 |
| 缺少 labels | GitHub bootstrap | T2 创建后立即可 scan |
| 重复 claim | claim ownership | T3 并发/重复认领 |
| GitHub 错误被忽略 | workflow 错误传播 | T3 认证与 API 失败 |
| 未推送 SHA 通过 | `internal/validate` | T3 本地-only commit |
| protected 配置未执行 | claim/validation policy | T3 protected path/branch |
| workflow dispatch skipped | GitHub Actions | T2 手工 dispatch |
| CLI 和 PowerShell 体验 | `cmd/gia`、`scripts`、README | T1 safety 与 T4 用户旅程 |
