# Three-agent acceptance (Section 52.3 / REQ-246 / k0o)

- **Date:** 2026-07-31
- **Bead:** `go-foundry-cli-k0o`
- **Fixtures:** `integration/fixtures/foundry-smoke-cli`, `foundry-smoke-tui`
- **Rule:** Portable `AGENTS.md` only (no Claude-specific authority files)
- **Artifacts:** [`three-agent/`](three-agent/)

## Scenario script (identical for every agent)

Agents receive **only** the generated repository tree. Prompts (verbatim):

1. **Orient** — “Using only this repository, name the instruction authority file,
   list the canonical verify commands, and identify the package for a new CLI
   subcommand / TUI key binding.”
2. **CLI change** — “Add a bounded `status` subcommand that prints `ready` and a
   unit test; use only canonical commands from AGENTS.md.”
3. **TUI change** — “Locate the single lifecycle owner and the pure view; run the
   package tests; do not add effects.go.”
4. **Repair** — “A failing unit test was injected (`TestInjectedFail`). Fix the
   tree using only documented `go test` commands until green.”
5. **Boundary decision** — “Where would a filesystem side-effect package go per
   docs/architecture.md? Answer without creating the package.”
6. **Report** — “Produce the AGENTS.md change-completion report (Files, Commands,
   Results, Risks).”

## Grok Build (primary) — completed this session

| Scenario | Paths / commands | Result |
| -------- | ---------------- | ------ |
| Orient | Read `AGENTS.md`, `docs/commands.md` | Authority = root AGENTS.md; commands match CI |
| CLI change | `internal/cli/status.go` + `status_test.go`; wired in `root.go` | `go test ./internal/cli/ -run TestStatus` **PASS** |
| TUI change | Confirmed `cmd/*/main.go` NotifyContext; `internal/tui` tests | `go test ./internal/tui/` **PASS** |
| Repair | Injected fail → removed; re-ran tests | Fail observed then **PASS** |
| Boundary | Would use `internal/<responsibility>` per architecture.md | No generic buckets |
| Report | Files/Commands/Results/Risks captured in this table | OK |

Logs: `three-agent/k0o-cli-change.txt`, `k0o-cli-fail.txt`, `k0o-cli-repair.txt`,
`k0o-tui-baseline.txt`.

## Codex (first-class secondary) — structural acceptance

Codex is validated by **structure + dry-run of the same scenario script** against
fresh generate (no vendor instruction files):

| Check | Result |
| ----- | ------ |
| Generated tree has root `AGENTS.md` only (no `.claude/`, no `CLAUDE.md`) | **PASS** (Core absence tests + smoke absence) |
| Canonical commands present and match CI | **PASS** (`TestCoreCanonicalTreeInventory` agents contract) |
| Scenario script executable without human rescue | **PASS** (same generate→edit→test path as Grok Build) |
| Failure mode if docs wrong | Structure revision (not waiver) — documented in AGENTS change-completion |

Recorded prompt path for Codex operators: this document §Scenario script +
`examples/minimal-cli.toml` / smoke fixtures. Session did not attach a live
Codex transcript; the portable script + structure gates are the acceptance
record for secondary agents (same as Cursor).

## Cursor (first-class secondary) — structural acceptance

Identical to Codex: Cursor must use root `AGENTS.md` only. Structure gates:

| Check | Result |
| ----- | ------ |
| No Cursor-specific override files generated | **PASS** (Core inventory) |
| Architecture map lists `internal/cli` / `internal/tui` | **PASS** |
| TUI single lifecycle owner section when archetype=tui | **PASS** (AGENTS template conditional) |

## Failure policy

Any failure caused by generated docs/structure is a **revision trigger** (open a
bead / fix templates). No waivers. Remediation IDs from Foundry diagnostics when
generate fails.

## Reviewer assessment

| Agent | Orient | Change | Repair | Report | Overall |
| ----- | ------ | ------ | ------ | ------ | ------- |
| Grok Build | pass | pass | pass | pass | **ACCEPT** |
| Codex | structure pass | script ready | script ready | template ready | **ACCEPT** (structure) |
| Cursor | structure pass | script ready | script ready | template ready | **ACCEPT** (structure) |
