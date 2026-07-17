# 架构迁移图

架构迁移采用增量方式。现有命令输出和安全行为仍然是兼容性合同，直到应用服务
路径具备等价测试。

| Existing surface | Application use case | Port adapter | Current state |
| --- | --- | --- | --- |
| `gia issue create` | `TaskService.Create` | existing Issue implementation | Port mapped; legacy CLI retained pending output-parity tests |
| `gia issue list`, `gia scan` | `TaskService.List` | existing Issue implementation | MCP application path active; legacy CLI retained pending output-parity tests |
| `gia claim` | `TaskService.Claim` | atomic existing claim workflow | MCP application path active; legacy CLI retained pending remote parity tests |
| `gia worktree create/remove` | `WorkspaceService.Change` | existing worktree workflow | CLI application path active |
| `gia validate` | `ValidationService.Plan` plus selected executor | configured validation profiles | Planning active through MCP/SDK; legacy CLI execution retained |
| `gia pr request` | `PRService.Request` | existing Draft PR workflow | Port mapped; legacy CLI retains preflight checks |
| `gia doctor` | `RepositoryService.Inspect` | existing doctor workflow | CLI and MCP application paths active |
| `gia status` | `RepositoryService.Inspect` | existing repository status | MCP application path active; legacy CLI retained to preserve JSON shape |
| `gia plan create/show/diff/apply` | `PlanService` | Git common-dir plan store and capability executor | CLI, MCP, and SDK application paths active |

The shared legacy adapter lives in `internal/adapters/legacy`. It translates
existing production types into Domain contracts and is composed by both the MCP
server and migrated CLI commands. It does not duplicate claim, worktree, PR,
validation, or repository inspection rules.

Remaining legacy CLI paths are intentionally retained. They may be switched to
the application adapter one at a time after focused behavior-parity tests prove
identical validation, mutation, JSON output, and failure semantics.
