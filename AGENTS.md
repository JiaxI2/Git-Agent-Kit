# GIA Kit Agent Instructions

## Scope

This repository owns the standalone, platform-neutral Git Isolated Agent Kit.
Do not add dependencies on AiCoding or another upper-layer platform.

## Change Rules

- Keep Git and GitHub operations fail-closed.
- Never force-push, hard-reset, or recursively clean user repositories.
- Use isolated Git worktrees for concurrent agent changes.
- Remote test repositories must be temporary and removed after evidence capture.
- Any behavior change requires focused tests and documentation updates.
- Preserve Windows PowerShell and Linux shell support.

## Required Verification

Before a change is complete, run:

```text
gofmt
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

Also run the relevant local integration, GitHub, adversarial, or UX test tier
described in `docs/TEST_PLAN.md`.
