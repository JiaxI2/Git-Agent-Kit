# GIA Kit 0.1.0 Release Notes / 发布说明

## 摘要 / Summary

GIA Kit 0.1.0 提供平台中立的多 Agent Git 隔离工作流：以 GitHub Issue
承载任务真源，以原子 claim ref 维护单写者租约，以独立任务分支和
`git worktree` 隔离执行面，并以 Draft PR、handoff metadata 和远端
upstream tip 绑定的 SHA 验证形成可审计交付链。

最终 T0、T1、T2、T3、T4 全部通过，未发现 release-blocking 缺陷。

## 发布说明 / Release Notes

本版本是 GIA Kit 的首个 canonical standalone release。源码权威由
`v0.1.0` Tag 固定；Windows/Linux amd64 资产由 Tag workflow 在 clean
checkout 中使用 Go 1.22.12 构建，版本号通过 `VERSION` 注入。

## 变更内容 / What's Changed

- 修复 Windows 多行参数、label bootstrap、分类幂等和完整错误传播。
- 以远端 claim ref 实现并发认领单写者租约与失败补偿。
- 将 validation 绑定当前 branch upstream tip，拒绝旧 SHA 和歧义 ref。
- 增加 JSON/YAML/YML 严格配置、executor policy 和 Draft PR 申请命令。
- 完成 single-JSON 错误输出、stdout/stderr 分离和 HTML 字符可读输出。
- 增加固定工具链、精确 Tag 构建与 GitHub Release 自动发布流程。

## 主要变化 / Highlights

- 支持 JSON、YAML、YML 单一配置源和严格 unknown-field 校验。
- Issue 创建自动 bootstrap lifecycle、risk 和 executor labels。
- Windows 多行 Issue、claim、handoff 和 Draft PR 正文通过安全 body file
  传递。
- `gia/claims/<issue>` 远端 ref 提供原子 claim 租约和失败补偿。
- `gia pr request` 只创建 Draft PR，并返回 `PENDING_USER_APPROVAL`。
- validation 绑定当前分支 upstream；旧 SHA、歧义 remote refs、未推送
  commit 和错误 ref 均失败关闭。
- `protected.branches`、`protected.paths`、force-push policy 和 executor
  permissions 进入运行时门禁。
- CLI 提供 help、Issue 空结果 guidance、初始化 next steps 和机器可读
  单 JSON 输出。

## 验证 / Validation

| 层级 | 最终结果 | 关键证据 |
|---|---|---|
| T0 | PASS | `gofmt`、unit、race、vet、Windows/Linux amd64 build |
| T1 | PASS | bare remote、隔离 worktree、dirty refusal、PowerShell safety |
| T2 | PASS | 多行 Issue、幂等分类、原子 claim、Draft PR、handoff、SHA 与双平台 CI |
| T3 | PASS | 全量黑盒 35/35；发布候选 targeted 4/4 |
| T4 | ACCEPTED / PASS | 核心目标 3/3、成功率 100%、0 次意外恢复 |

T2 历史 Actions run IDs：

- Issue classification：`29503281411`、`29503281957`、`29503282321`、
  `29503282454`；
- manual ready dispatch：`29503348525`；
- final task push CI：`29503541374`；
- final Draft PR CI：`29503545631`。

T3 targeted 4/4：

1. upstream tip 精确匹配时验证通过；
2. upstream 前移后，旧 SHA 即使仍是 archive ref tip 也被拒绝；
3. 无 upstream 且多个 remote refs 指向同一 SHA 时按歧义拒绝；
4. `gia worktree create` 自动配置 upstream 后，smoke/full 均通过。

T4 首次完整路径约 4 分 15.6 秒，执行 17 条 GIA 命令，产生 9 个用户决策。

## 可追溯性 / Traceability

- Source Tag：`v0.1.0`。
- Canonical repository：`github.com/JiaxI2/git-isolated-agent-kit`。
- Release workflow 复核每个 binary 的 `vcs.revision` 等于 Tag commit 且
  `vcs.modified=false`。
- T2/T3 主验证候选 SHA-256：
  `CFB9153DABA85EB71D654860E33E6CABB1C0099EFD03EF0635A9BB0B4C65FE0E`；
  该值仅标识发布前黑盒候选，不是 Release asset checksum。

## 发布资产 / Assets and Checksums

- `gia-linux-amd64`
- `gia-windows-amd64.exe`
- `SHA256SUMS`

`SHA256SUMS` 使用顶层 basename，可在下载目录直接复核。Release workflow
会在发布前重新计算并验证两个二进制的 SHA-256。

## 兼容性 / Compatibility

- Windows amd64 和 Linux amd64 构建通过。
- Windows PowerShell 5.1/PowerShell 7 路径和 Linux/macOS shell 快速开始已统一。
- GitHub CLI `gh` 负责认证和远端 Issue/PR/Actions 操作。
- 默认验证 profile 以 Go 项目为示例，其他技术栈需在 `.gia/config.*`
  中替换命令。

## 安全边界 / Security

- GIA CLI/runtime 不提供 approve、merge、Tag 或 Release 命令；canonical
  release workflow 只响应用户已创建并推送的版本 Tag。
- workflow permissions 不是硬身份隔离；生产仓库必须使用独立 GitHub
  App/token 和 ruleset 建立审批、merge 和 release 边界。
- 禁止 force push、hard reset、破坏性清理和未授权跨仓库写入。

## 弃用 / Deprecations

None。

## 新贡献者 / New Contributors

None identified in this candidate validation record。

## 已知问题 / Known Issues

没有已知 release-blocking 缺陷。GitHub Actions 对
`actions/github-script@v7` 可能显示 Node.js 20 deprecation annotation，
当前 hosted runner 会强制使用 Node.js 24，最终 workflow 运行成功。

## 完整变更 / Full Changelog

首发版本完整历史：
`https://github.com/JiaxI2/git-isolated-agent-kit/commits/v0.1.0`。

## 清理 / Cleanup

所有 T2、T3、T4 私有远程测试仓库、本地 bare remote、认证隔离目录、
worktree、临时 binary 和测试缓存均已删除并复核不存在。测试期间未 merge、
未创建 Tag、未发布 Release。

## 回滚 / Rollback

- 仓库变更通过 revert 对应发布 commit 回滚，不重写共享历史。
- 安装侧删除 GIA binary 和由安装流程拥有的链接；不删除用户仓库、
  `.gia` 配置或未知 Codex/GitHub 配置。
