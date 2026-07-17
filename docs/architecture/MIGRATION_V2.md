# Architecture V2 Migration Map

Architecture V2 is additive. Existing command output and safety behavior remain
the compatibility contract until parity tests cover an application-backed path.

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

The shared legacy adapter lives in `internal/adapters/legacy`. It translates
existing production types into Domain contracts and is composed by both the MCP
server and migrated CLI commands. It does not duplicate claim, worktree, PR,
validation, or repository inspection rules.

Remaining legacy CLI paths are intentionally retained. They may be switched to
the application adapter one at a time after focused behavior-parity tests prove
identical validation, mutation, JSON output, and failure semantics.
