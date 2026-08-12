# Extension path: add a CLI subcommand after generate (REQ-066)

Foundry-owned **post-generate** overlay. Proves the Section 17 growth recipe:
after `foundry generate` produces the minimal shell, owners add domain packages
and constructor-built subcommands **by hand**.

## Purpose

1. Show a responsibility-named package (`internal/ping`) that does **not** import Cobra.
2. Show a Cobra subcommand constructor in `internal/cli` that wires the domain package.
3. Compile and test when applied on top of a Foundry-generated smoke-cli tree.

## Non-goals

- **Not** generated catalog content — never appears in a fresh generate.
- **Not** a greet/demo service baked into Core or the CLI archetype (FND-014).
- **Not** configuration or persistence profiles — those remain recipes under `docs/recipes/`.
- **Not** a plugin/extension platform — files are merged by Foundry tests, not a runtime.

## Overlay files

| Path | Role |
| ---- | ---- |
| `testdata/overlay/internal/ping/ping.go` | Domain logic: format a ping response |
| `testdata/overlay/internal/ping/ping_test.go` | Unit tests beside domain code |
| `testdata/overlay/internal/cli/ping.go` | `newPingCmd` constructor (Cobra confined to `internal/cli`) |
| `testdata/overlay/internal/cli/ping_test.go` | Constructor-level command test |

Overlay sources live under `testdata/` so Foundry's architecture suite does not
treat them as product packages (they only land inside a *generated* project).

Tests also patch the generated `internal/cli/root.go` `AddCommand` list to register
`newPingCmd()` — the same one-line growth step documented in generated
`docs/architecture.md`.

## Acceptance signal

Generate `foundry-smoke-cli` → apply overlay → `go test ./...` green → binary
`ping` prints a deterministic line. Absence: no Foundry module in `go.mod`.
