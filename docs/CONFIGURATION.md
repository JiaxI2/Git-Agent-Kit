# 配置说明

配置位于目标仓库 `.gia/config.json`、`.gia/config.yaml` 或
`.gia/config.yml`。三者必须恰好存在一个；同时存在多个格式时失败关闭，避免
不同执行器读取到不同策略。

```text
gia init --repo <path> --format json
gia init --repo <path> --format yaml
gia init --repo <path> --format yml
```

默认格式是 JSON。`--force` 只覆盖当前已存在的同格式单一配置；若存在另一格式
或多个配置，GIA 拒绝处理并要求用户显式保留一个文件，不会自动删除或猜测优先级。

需要临时或集中配置时，下列远程/安全命令支持 `--config <path>`：

```text
doctor
issue create
issue list
scan
claim
pr request
validate
notify
```

显式路径优先于 `.gia` 自动发现，且相对路径按 `--repo` 指定的仓库解析。自动
发现不合并不同格式。JSON 使用 `DisallowUnknownFields`，YAML/YML 使用
`KnownFields(true)`，三种格式都拒绝 unknown fields 和多文档内容。

YAML 解析使用 YAML 官方组织维护的
[`go.yaml.in/yaml/v3`](https://pkg.go.dev/go.yaml.in/yaml/v3) v3.0.4，
保持 v3 API 兼容；不使用已停止维护的旧 `go-yaml/yaml` 模块路径。

## validation.profiles

每个 profile 是按顺序执行的 shell 命令数组。命令失败立即停止。

`validation.remote` 指定 Claim 创建分支和 validation fetch/远端 SHA 门禁使用的
Git remote，默认是 `origin`。为兼容 0.1.0 配置，缺失或空值也按 `origin` 处理。
expected SHA 必须等于该 remote 某个已 fetch tracking ref 的当前 tip；仅存在于
本地或只是远端分支祖先都不满足门禁。当前分支存在 upstream 时，expected SHA
还必须精确等于该 upstream 的 tip；不能借用另一个远端分支上相同 SHA 冒充当前
PR/ref。没有 upstream 时只接受唯一匹配的远端 ref，多 ref 歧义会失败关闭。

C/C++ 示例：

```json
{
  "smoke": ["cmake -S . -B build", "cmake --build build"],
  "full": ["cmake -S . -B build", "cmake --build build", "ctest --test-dir build --output-on-failure"],
  "release": ["cmake -S . -B build-release -DCMAKE_BUILD_TYPE=Release", "cmake --build build-release", "ctest --test-dir build-release --output-on-failure"]
}
```

Node.js 示例：

```json
{
  "smoke": ["npm ci", "npm run lint"],
  "full": ["npm ci", "npm test", "npm run build"],
  "release": ["npm ci", "npm test", "npm run build", "npm pack --dry-run"]
}
```

## notifications

```json
{
  "console": true,
  "webhookUrl": "",
  "command": ["powershell", "-File", "scripts/notify.ps1", "{event}", "{message}"]
}
```

Webhook 接收 JSON：`event`、`message`、`time`。

## permissions

`permissions` 是平台中立的 workflow policy：

```yaml
permissions:
  allowedExecutors: ["*"]
  approvalPrincipals: [user]
  mergePrincipals: [user]
  releasePrincipals: [user]
```

- `allowedExecutors`：允许执行任务并提交 Draft PR 申请的执行器；`"*"` 表示任意已接入执行器。
- `approvalPrincipals`：可批准从 Draft 进入后续评审阶段的主体，默认仅 `user`。
- `mergePrincipals`：可执行合并的主体，默认仅 `user`。
- `releasePrincipals`：可执行 Tag/Release 的主体，默认仅 `user`。

显式空数组表示 deny-all；旧配置遗漏字段时应用上述安全默认值。这些字段只表达
GIA workflow policy，不能替代 GitHub 的硬权限隔离。生产接入必须为 Agent 使用
独立 GitHub App 或最小权限 token，并通过 branch protection、environments 和
required reviewers 实际限制批准、合并与发布权限。

## Draft PR 申请

```text
gia pr request --repo <path> --issue <number> \
  [--config <path>] [--title <text>] [--body-file <path>] \
  [--executor <name>]
```

- base 来自 `defaultBranch`，head 来自当前分支；
- 当前分支不得是 detached、default 或 `protected.branches`；
- 仓库必须干净，本地 HEAD 必须等于 `<validation.remote>/<current-branch>` tip；
- executor 必须匹配 `allowedExecutors`；
- Issue 必须包含 `issue.claimedLabel`；已有 `executor:*` 标签必须匹配请求者；
- 未提供标题或正文文件时读取 Issue 标题和正文；
- `--body-file` 的真实路径必须位于目标仓库内，避免读取仓库外凭据或秘密；
- 远端始终创建 Draft PR，成功状态固定为 `PENDING_USER_APPROVAL`。

GIA 不提供 approve、merge 或 release 子命令。

## 安全默认值

- `requireExpectedHead=true`：验证必须指定预期 SHA；
- `requireClean=true`：验证前后工作区必须干净；
- `refuseDirtyRemoval=true`：拒绝删除脏 worktree；
- `protected.branches`：Claim 不得创建匹配分支，validation 不得在匹配分支运行；
- `protected.paths`：相对 `<validation.remote>/<defaultBranch>` 的变更命中 glob 时停止 validation；
- `rejectForcePush=true`：Claim 的强制安全门；设为 `false` 时 Claim 失败关闭，GIA 不提供 force-push 路径。

Claim 使用 `gia/claims/<issue>` 远端租约分支实现单写者所有权。租约和任务分支
都必须是首次创建；重复或并发 Claim 不会更新已有 ref。
