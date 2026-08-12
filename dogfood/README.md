# Foundry CLI dogfood (Section 52 / REQ-242 / REQ-245 / REQ-247)

Early Phase 2 dogfood begins the moment one CLI fixture passes real-platform
transaction + default verification. This directory holds **Foundry-owned inputs**
for that exercise — not the independent dogfood repositories themselves.

**Authority:** [SPEC-FOUNDRY-002](../docs/02-definitive-foundry-specification-revised-fable-5.md)
Sections 49, 52, 60; REQ-242, REQ-245, REQ-247.

## Sequence

| Step | Artifact                                  | Role                                                                                |
| ---- | ----------------------------------------- | ----------------------------------------------------------------------------------- |
| 1    | `integration/fixtures/foundry-smoke-cli/` | Disposable generate → build → test → help/version                                   |
| 2    | [`repo-map/`](repo-map/)                  | Real project: inventory CLI; first domain command added **manually** after generate |

## Independence (normative)

- Dogfood repositories stay **independent** and evolve manually after first generate.
- Foundry **never** modifies an existing project; comparisons use freshly generated siblings.
- Generated trees are written to a **custody-safe temp parent** (`0700` under sticky `/tmp`), not into this repository.
- Spec + post-generate overlay sources live here for reproducibility; they are not catalog content.

## Toolchain pin (`go1.26.5`)

Foundry preflight requires **exact** `go1.26.5` (`catalog/versions.toml`). Host
version managers often expose a nearby patch (e.g. mise `go@latest` → 1.26.4)
while `cmd/go` under ambient `GOTOOLCHAIN=auto` may _report_ 1.26.5 by
auto-downloading the module-cache toolchain — Foundry probes with
`GOTOOLCHAIN=local` and rejects the nearby base.

| Mechanism           | Purpose                                                                                                              |
| ------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `FOUNDRY_GO_BIN`    | Absolute path override; highest priority for the Foundry binary and dogfood/perf scripts                             |
| Auto-discovery      | `internal/toolrun.FindPinnedGoBinary`: module-cache toolchain, mise `1.26.5`, `/usr/local/go`, then matching `PATH`  |
| Script `resolve_go` | `scripts/dogfood-cli.sh` / `scripts/perf/capture-baselines.sh` — same candidate order with `GOTOOLCHAIN=local` probe |

```bash
# Explicit (always works when the pin exists under the module cache)
export FOUNDRY_GO_BIN="${FOUNDRY_GO_BIN:-$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.$(uname -s | tr A-Z a-z)-$(uname -m | sed 's/x86_64/amd64;s/aarch64/arm64/')/bin/go}"
# linux-amd64 example when uname mapping is awkward:
# export FOUNDRY_GO_BIN="$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64/bin/go"

export PATH="$(dirname "$FOUNDRY_GO_BIN"):$PATH"
export GOROOT="$(cd "$(dirname "$FOUNDRY_GO_BIN")/.." && pwd)"
export GOTOOLCHAIN=local

# Optional: make go1.26.5 the mise default on a dogfood machine
mise install go@1.26.5 && mise use -g go@1.26.5
```

Bare `foundry generate` after `go build` should succeed without hand-hunting when
the pin is present in the module cache or via `FOUNDRY_GO_BIN`. If not, the
`tool.wrong_version` remediation names the env var and any nearby matching path.

## Automated support

```bash
# Script: generate smoke-cli into a temp parent, run its tests, exercise help/version
# (resolves go1.26.5 via FOUNDRY_GO_BIN / module-cache / PATH)
./scripts/dogfood-cli.sh

# Strict verify adds go-staticcheck and go-govulncheck; govulncheck requires
# outbound network access to fetch the vulnerability database.
./scripts/dogfood-cli.sh --verify strict

# Go tests with step logger (complements j8h.2; does not replace it)
go test -count=1 -timeout 10m ./integration/dogfood/
```

Evidence lands under [`docs/evidence/dogfood-cli-*.md`](../docs/evidence/) and
[`docs/evidence/dogfood-cli-logs/`](../docs/evidence/dogfood-cli-logs/).

## Non-goals

- Not a substitute for the full generate e2e matrix (`j8h.2`) or hostile suite (`qd8`).
- Not TUI dogfood (`foundry-smoke-tui` / `worktree-status` — Phase 3).
- Not absolute performance gates (Section 49 baselines are **seeded** here; full capture scripts are `j8h.1`).
- Not agent acceptance scenarios (Section 52.3 / `k0o`).
