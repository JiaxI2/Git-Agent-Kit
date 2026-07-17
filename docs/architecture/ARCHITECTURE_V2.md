# Git Agent Kit Architecture V2

## Purpose

Git Agent Kit (GIA) is a Git governance kernel with multiple interfaces and
execution strategies. The CLI, MCP server, SDK, and future user interfaces are
adapters around the same domain and application behavior.

Architecture V2 separates four concerns:

1. **Domain** defines tasks, workflow transitions, policy decisions, effects,
   evidence, validation, executor identity, and executor capabilities.
2. **Application** coordinates use cases through ports without importing
   concrete Git, GitHub, process, filesystem, CLI, or MCP implementations.
3. **Adapters** translate CLI, MCP, SDK, Git, GitHub, and filesystem data at the
   system boundary.
4. **Execution** selects local or remote executors from required capabilities,
   not from the interface that initiated the request.

```text
CLI -----\
MCP ------> Application ---> Domain
SDK -----/        |
                  +-- ports --> Git / GitHub / process / filesystem
                  +-- selector --> LocalExecutor / RemoteExecutor
```

Dependencies point inward. Domain imports only the Go standard library.
Application imports Domain. Interface and infrastructure adapters may import
Application and Domain, but Domain and Application never import adapters.

## Execution boundary

Execution location is represented explicitly, but location alone is not a
selection policy. Every requested effect declares capabilities, and the
application selects an executor that provides all of them.

### Local execution

Local capabilities include:

- running commands with exact argument boundaries;
- Git and isolated worktree operations;
- builds, tests, static analysis, and hooks;
- repository-scoped filesystem access;
- locally observed evidence.

Local execution must be bound to a verified repository root. Relative working
directories may not escape that root. Command arguments are represented as an
array and are never reconstructed by splitting a shell string.

### Remote execution

Remote capabilities include:

- Issue discovery and claim operations;
- branches, commits, and pull requests;
- reviews and approval metadata;
- GitHub Actions status and remote evidence.

Remote status never substitutes for a local build or test. Evidence records its
source, observation time, and provider reference so policy can distinguish the
two paths.

## Stable contracts

`internal/domain` owns canonical internal contracts. `internal/app` owns use
cases and ports. `pkg/sdk/v1` is the external Go integration boundary and must
expose only public SDK types; exported SDK signatures must not leak any
`internal` type.

Interface adapters own transport concerns:

- CLI flag parsing, aliases, exit codes, and JSON rendering;
- MCP JSON-RPC framing, discovery, resources, prompts, and tool results;
- SDK conversion between public versioned DTOs and internal contracts.

They do not own workflow transitions or policy decisions.

## Incremental migration rules

1. Existing `gia` commands remain available until an application-backed path
   has behavior-parity tests.
2. Each migrated command translates input to an application request and maps
   the response back to its existing output contract.
3. Governance rules move once into Domain or Application; adapters may only
   translate or invoke ports.
4. Old and new paths must not mutate the same remote state twice.
5. A legacy path is removed only after local and remote evidence demonstrates
   parity on supported platforms.
6. Each layer is additive and independently revertible while migration is in
   progress.

## Validation boundary

Architecture validation includes:

- source-level dependency direction gates;
- domain invariant and transition tests;
- adapter delegation tests;
- SDK external-consumer compilation;
- MCP protocol and delegation tests;
- Windows and Linux build, test, vet, formatting, and diff checks.

Validation evidence is attached to the exact commit under review. A Draft PR
remains Draft while required evidence is missing or failing.

## Non-goals

- Rewriting the existing CLI in one change.
- Introducing a framework or network daemon for MCP.
- Moving provider-specific payloads into Domain or Application.
- Treating a user interface as the owner of governance policy.
- Treating remote CI as proof of local toolchain or filesystem behavior.
- Removing legacy production paths before parity is demonstrated.
