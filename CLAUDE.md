# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:970c3bf2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->


## Build & Test

Authority: [`docs/dev/testing.md`](docs/dev/testing.md), Makefile targets.

```bash
# Build (product: CGO_ENABLED=0)
CGO_ENABLED=0 go build -o /tmp/foundry ./cmd/foundry

# Fast feedback (default agent loop)
go test -count=1 ./internal/...
go test ./cmd/foundry -run TestWriteFree -count=1
go vet ./...
gofmt -l .

# Medium: generate integration
go test -count=1 -timeout 15m ./integration/generate/

# Full product E2E (local/agent only — NOT a CI job; ~15–40 min warm)
make product-e2e

# Hostile / sentinel
make hostile
make sentinel

# Docs + testscript lint
make docs-validate
make lint-testscripts
```

Platforms: **Linux + macOS** product; Windows CI is compile hygiene only.

## Architecture Overview

Foundry is a CLI that generates Go CLI/TUI projects from a TOML Project Spec
and an embedded catalog. Pipeline stages:

`spec` → `catalog`/`resolve` → `plan` → `render` → `fsx` (stage/commit) →
`toolrun` (go/git) → `verify` → `report`/`cli`

Specification authority: **SPEC-FOUNDRY-002**
(`docs/02-definitive-foundry-specification-revised-fable-5.md`).

## Conventions & Patterns

- Use `bd` for task tracking; `bd prime` for session context.
- Multi-step tests use `internal/testutil` step logger.
- Goldens: `UPDATE_GOLDEN=1` locally only; never in CI.
- Do not invent product behavior outside the specification.
- Prefer extending existing integration/generate, dogfood, writefree suites
  over new one-off e2e harnesses.
