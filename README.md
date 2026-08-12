# go-foundry-cli (Foundry)

Foundry generates **minimal, production-grade Go CLI and TUI projects** from a
strict TOML specification and an embedded catalog — with descriptor-relative
filesystem transactions, closed tool environments, and commit-dominant exit
semantics.

**New to Foundry? Read the [User Guide](docs/user-guide.md)** for
installation, writing a Project Specification, every command, and the full
field/error reference.

## Specification authority

**SPEC-FOUNDRY-002** is the sole implementation authority:

- [`docs/02-definitive-foundry-specification-revised-fable-5.md`](docs/02-definitive-foundry-specification-revised-fable-5.md)

Do not invent behavior outside that document. Agent workflow for this repo:
[`AGENTS.md`](AGENTS.md).

## Quickstart

Prefer a **dry-run** workflow — validate and plan before any filesystem write:

```bash
CGO_ENABLED=0 go build -o foundry ./cmd/foundry
./foundry doctor   # catalog Go pin + FOUNDRY_GO_BIN guidance

# 0. Scaffold a Project Spec (or copy examples/minimal-cli.toml)
./foundry init --out ./project.toml --name my-cli --module github.com/you/my-cli

# 1. Validate the specification (no project writes)
./foundry validate --spec ./project.toml

# 2. Plan: show the Generation Plan (no writes; --output json for full plan)
./foundry plan --spec ./project.toml

# 3. Generate: the only command that writes a Generated Project
./foundry generate --spec ./project.toml
# git init only — commit yourself: cd <dest> && git add . && git commit
```

See the [User Guide](docs/user-guide.md) for a complete walkthrough,
including the Project Specification field reference, archetypes (`cli`/`tui`),
the `distribution` profile, and troubleshooting.

## Examples

Copy-paste Project Specs and intentional invalids live under
[`examples/`](examples/):

| Path                                                                         | Purpose                                                  |
| ---------------------------------------------------------------------------- | -------------------------------------------------------- |
| [`examples/minimal-cli.toml`](examples/minimal-cli.toml)                     | Valid CLI — `validate` + `plan` + `generate` all succeed |
| [`examples/minimal-tui.toml`](examples/minimal-tui.toml)                     | Valid TUI — `validate` + `plan` + `generate` all succeed |
| [`examples/appendix-b-public-cli.toml`](examples/appendix-b-public-cli.toml) | Public CLI with the `distribution` profile               |
| [`examples/invalid-*.toml`](examples/)                                       | Negative fixtures with exact Appendix D error ids        |
| [`examples/README.md`](examples/README.md)                                   | Agent workflow: copy → edit → validate → plan → generate |

Do not invent fields or profile IDs; `distribution` is currently the only
selectable profile (see the [User Guide](docs/user-guide.md#profiles-distribution)).

## Development

| Doc                                                                    | Role                                                 |
| ---------------------------------------------------------------------- | ---------------------------------------------------- |
| [docs/user-guide.md](docs/user-guide.md)                               | End-user guide: install, commands, fields, errors    |
| [docs/dev/testing.md](docs/dev/testing.md)                             | Unit vs e2e, step logger, golden updates, build tags |
| [docs/dev/profile-admission.md](docs/dev/profile-admission.md)         | §19.4 profile admission process                      |
| [docs/recipes/configuration.md](docs/recipes/configuration.md)         | §21.1 config recipe (not a profile)                  |
| [docs/recipes/local-persistence.md](docs/recipes/local-persistence.md) | §21.2 persistence recipe (not a profile)             |
| [catalog/versions.toml](catalog/versions.toml)                         | Single lock for modules, tools, action SHAs          |
| [docs/evidence/](docs/evidence/)                                       | Phase-entry evidence (E1–E5)                         |

```bash
# Fast path
go test -count=1 ./internal/...
go test ./cmd/foundry -run TestWriteFree -count=1
go vet ./...
gofmt -l .

# Full product bar (local/agent only — not CI; see docs/dev/product-e2e-plan.md)
make product-e2e
```

Golden updates (local only; refused when `CI=true`):

```bash
UPDATE_GOLDEN=1 go test ./internal/foo -run TestName
```

Maintainer testing guide: [`docs/dev/testing.md`](docs/dev/testing.md).

## Module & toolchain

- Module: `github.com/robertguss/go-foundry-cli`
- Go toolchain: **1.26.5** exact (`catalog/versions.toml`)
- **Supported product platforms: macOS and Linux** (`CGO_ENABLED=0` product builds)
- **Windows is not a product platform.** CI may run a hard-gated Windows unit
  job for compile/vet hygiene of stubs and build tags only — not for product
  generate/dogfood claims.

### Exact `go1.26.5` for generate / dogfood

Foundry tool preflight requires the **exact** catalog pin (`go1.26.5` under
`GOTOOLCHAIN=local`). Nearby versions (for example mise `go@latest` → 1.26.4)
fail closed with `tool.wrong_version`.

**Resolution order** at startup:

1. `FOUNDRY_GO_BIN` — absolute path to a `go` that reports `go1.26.5`
2. Auto-discovery — module-cache toolchain, mise install of 1.26.5, `/usr/local/go`, then `PATH` (only if version matches)
3. `PATH` lookup — still used so failures name a concrete binary and remediation

```bash
# Option A — point Foundry at a known pin (module-cache toolchain from cmd/go)
export FOUNDRY_GO_BIN="$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.$(go env GOOS)-$(go env GOARCH)/bin/go"

# Option B — put the pin first on PATH for this shell
export PATH="$(dirname "$FOUNDRY_GO_BIN"):$PATH"
export GOTOOLCHAIN=local

# Option C — install the pin via mise (optional dogfood-machine default)
mise install go@1.26.5
mise use -g go@1.26.5   # or project-local: mise use go@1.26.5

# Build and generate (auto-discovery usually enough when the pin exists on disk)
CGO_ENABLED=0 go build -o foundry ./cmd/foundry
./foundry generate --spec examples/minimal-cli.toml --dest /tmp/minimal-cli
```

When preflight still fails, the remediation names `FOUNDRY_GO_BIN` and, when a
matching binary is nearby, its **absolute path**. Dogfood details:
[`dogfood/README.md`](dogfood/README.md).

## License

See repository license when published. Install via local build or
`go install` until tagged binary releases are published.
