# CLI dogfood — repo-map (Section 52.1 step 2)

- **Date:** 2026-07-31
- **Bead:** `go-foundry-cli-vu8` (P2.7)
- **REQs:** REQ-242, REQ-245, REQ-247; FND-014; growth recipe REQ-065/067
- **Inputs:** [`dogfood/repo-map/`](../../dogfood/repo-map/)
- **Raw logs:** [`dogfood-cli-logs/`](dogfood-cli-logs/)

## Machine / tool versions

Same host as [`dogfood-cli-smoke.md`](dogfood-cli-smoke.md):

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS / arch | Ubuntu 24.04.4 LTS / `linux/amd64` |
| Go | `go1.26.5` |
| Git | `2.43.0` |
| Foundry | local dirty build `c95d6364b5c5` |
| Catalog digest | `fba7cd432c37bbe7e30fe5e4ff46034d381a14ff6e48ee7085e957ab028985d8` |

## Exact commands

```bash
export PATH="$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64/bin:$PATH"
export GOROOT="$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.5.linux-amd64"
export GOTOOLCHAIN=local
export FOUNDRY_DOGFOOD_ARTIFACT_DIR=docs/evidence/dogfood-cli-logs

# Script path (includes smoke + repo-map)
./scripts/dogfood-cli.sh --with-repo-map

# Go tests
go test -count=1 -timeout 10m -v ./integration/dogfood/ -run TestDogfoodRepoMap
```

Manual equivalent (what an agent does after reading generated docs):

```bash
PARENT=$(mktemp -d /tmp/foundry-dogfood-XXXXXX); chmod 700 "$PARENT"
foundry generate \
  --spec dogfood/repo-map/foundry.toml \
  --dest "$PARENT/repo-map" \
  --verify default

# First domain feature — manual (overlay is the reproducible form of this edit)
# 1. Add internal/inventory (no Cobra)
# 2. Add internal/cli/inventory.go constructor
# 3. Wire newInventoryCmd() in internal/cli/root.go AddCommand list
# 4. go test ./... && go build -o repo-map ./cmd/repo-map
# 5. ./repo-map inventory . --output text|json
```

## plan_sha256 samples

| Run | plan_sha256 |
| --- | ----------- |
| Script generate | `74813771b5fa4a63afdebdcd72521b3d9a260a85ed66414443ac2276884d21b8` |
| Go test generate | `f84f8925c5a366931afaf413c6f0eafda8e6c0e09742eccb4632df258d43281f` |

## Time-to-first-green

| Stage | Result |
| ----- | ------ |
| Generate (default) | exit 0; ~1474 ms (script) / ~1481 ms (Go test) |
| Orientation | Read `AGENTS.md`, `docs/architecture.md`, `docs/commands.md`, `README.md` |
| First feature edit | Add `internal/inventory` + `internal/cli/inventory.go` + one-line root wire (~1 ms automated overlay; human edit is the same shape) |
| `go test ./...` | green |
| Exercise | `inventory --output text` and `--output json` list sample files |
| Preserved stages | **none** |
| Generated non-test deletions | **0** (FND-014 target met) |

Go-test wall clock generate→green: **~37.7 s** (dominated by `go test` download/compile of generated module deps, not Foundry itself).

## First-feature diff size/shape

**Shape (additive only):**

```text
+ internal/inventory/inventory.go
+ internal/inventory/inventory_test.go
+ internal/cli/inventory.go
+ internal/cli/inventory_test.go
~ internal/cli/root.go   # +newInventoryCmd() in AddCommand(...)
```

**Deleted generated non-test files:** none.

**Package placement errors:** 0 (domain package is responsibility-named; Cobra stays in `internal/cli`).

## REQ-247 measurement seed (repo-map)

| Measurement | Observation |
| ----------- | ----------- |
| Orientation files before first correct edit | `AGENTS.md`, `docs/architecture.md`, `docs/commands.md`, `README.md` |
| Attempts / time to first correct edit | 1 automated attempt matching documented growth recipe; edit wiring ~1 ms; full green ~38 s incl. tests |
| Package-placement errors | 0 |
| Retained vs deleted non-test | retained=16 generated non-test files; deleted=0 |
| First-feature diff size/shape | 4 new files + 1-line root wire (see above) |
| Gen+verify latency | ~1.48 s default (host caches warm) |
| Median PR CI latency (FND-016) | **seeded empty** — no PR yet for independent repo-map |
| Default vs strict failure split | default-only this run |
| Bypass demand | none |
| Escaped defects (RSK-403) | none observed |
| Config/persistence (RSK-401) | none yet — inventory is stateless filesystem walk |
| Preserved-stage burden (RSK-310) | none encountered |
| Agent observations | Growth recipe is clear; constructor + domain package split works; no urge to delete Core files |

## Simplification triggers

| Trigger | Status |
| ------- | ------ |
| >1/4 generated non-test files deleted | **Not fired** (0/16) |
| Same generated file deleted in both real projects | N/A (only one real project this phase) |
| Repeated agent boundary confusion | **Not observed** |

## Independence check

- Spec + overlay live under `dogfood/repo-map/` as **Foundry-owned reproducible inputs**.
- Generated trees stay under `/tmp/foundry-dogfood-*` and are not committed.
- Foundry never modified an existing destination; each run uses a fresh parent.
- No Foundry module dependency in generated `go.mod`.

## Friction list

| Friction | Severity | Disposition |
| -------- | -------- | ----------- |
| Exact go1.26.5 required; host mise default is 1.26.4 | Medium | **Resolved** (`go-foundry-cli-0wc`): see smoke friction list + README / `dogfood/README.md` |
| Absolute dest in plan_sha256 | Low | Expected |

## Related

- Smoke dogfood: [`dogfood-cli-smoke.md`](dogfood-cli-smoke.md)
- Extension-path fixture (similar growth shape): `integration/fixtures/extension-cli-subcommand/`
- Phase 2 exit still requires full matrix + hostile (`5pr`); dogfood success does not waive them
