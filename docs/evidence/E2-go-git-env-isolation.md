# E2 — Exact Go/Git environment isolation spike

- **Date:** 2026-07-30
- **Gated phase:** Phase 2 entry (Section 51.2, REQ-240)
- **Resolves:** FND-005 (executable confirmation of Section 34.2 / REQ-135 / REQ-154 / REQ-214 seed)
- **Spike code:** [`integration/hostile/e2/`](../../integration/hostile/e2/)
- **Raw logs:** [`docs/evidence/e2-logs/`](e2-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS (`PRETTY_NAME`) |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=go1.26.5`; pin matches Section 12) |
| Git | `git version 2.43.0` (`/usr/bin/git`) |
| Host `GOENV` path (ambient) | `/home/rob/.config/go/env` (must be ignored under construction) |

Source: [`e2-logs/machine.txt`](e2-logs/machine.txt).

### macOS / Darwin

| Item | Status |
| ---- | ------ |
| Package is `//go:build unix` (Linux + Darwin) | Present |
| `GOOS=darwin GOARCH=arm64 go test -c` of `e2` | **PASS** ([`e2-logs/darwin-crosscompile.txt`](e2-logs/darwin-crosscompile.txt)) |
| Live Darwin isolation run on owner macOS | **Not executed** on this Linux host — fixtures ready for CI matrix (REQ-214 / bhn) |

Portable construction and process-tree logic are identical on both supported platforms.

## Exact commands

```bash
# From repository root — full E2 sentinel matrix + process-tree:
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e2/ -count=1 -v -timeout 180s

# Full module regression:
GOTOOLCHAIN=go1.26.5 go test ./... -count=1 -timeout 180s

# Darwin compile of spike (no live run without macOS host):
GOTOOLCHAIN=go1.26.5 GOOS=darwin GOARCH=arm64 \
  go test -c -o /tmp/e2-darwin.test ./integration/hostile/e2/
```

Captured verbose run: [`e2-logs/go-test.txt`](e2-logs/go-test.txt) (`EXIT:0`).  
Module regression: [`e2-logs/module-regression.txt`](e2-logs/module-regression.txt) (`EXIT:0`).

## Expected behavior (specification)

| Contract | Spec | Expectation |
| -------- | ---- | ----------- |
| 1 | §34.2 / FND-005 | Subprocess env = **empty base + exact allowlist**, never inherited-minus-denylist |
| 2 | §34.2 | Go fixed keys: `GOENV=off`, empty `GOFLAGS`/`GOCACHEPROG`, `GOTOOLCHAIN=local`, `GOWORK=off`, `GOVCS=*:off`, `GOAUTH=off`, empty private-module vars, `CGO_ENABLED=0`, `LC_ALL`/`LANG=C`, `TERM=dumb` |
| 3 | §34.2 / FND-005 | Hostile sentinels for `GOENV`, `GOFLAGS`, `GOCACHEPROG`, `GOAUTH`, `GOVCS`, `GOPRIVATE`/`GONOPROXY`/`GONOSUMDB`/`GOINSECURE` do **not** leak into child env or tool behavior |
| 4 | §34.2 | Undeclared host vars (`GODEBUG`, `GOEXPERIMENT`, `CC`, …) absent from construction |
| 5 | §34.2 | `git init` env: only `PATH`, `LC_ALL=C`, `LANG=C`, `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_TEMPLATE_DIR=<owned empty>`; no `HOME`/`GIT_DIR`/`XDG_CONFIG_HOME` |
| 6 | §34.2 | Empty owned template: no host template files or hooks copied into `.git` |
| 7 | §34.1 / REQ-120 / REQ-214 | Observed process tree equals plan `external_steps` (no extras, no shell) |
| 8 | §34.1 | No step uses a shell; argv/env/timeout recorded in plan |

**Forbidden:** host GO* inheritance, host git template/hooks, shell wrappers, denylist-only filtering.

## Observed behavior

### Pass/fail table (run 2026-07-30T20:54:58Z)

| Probe | Path | Result |
| ----- | ---- | ------ |
| Construct go env allowlist only | `ConstructGoEnv` key set vs `GoAllowlistKeys` | **PASS** |
| Fixed normative values | all `FixedGoEnv` keys | **PASS** (leak=false each) |
| Sentinel matrix go-fixed | GOENV, GOFLAGS, GOCACHEPROG, GOAUTH, GOVCS, GOPRIVATE, GONOPROXY, GONOSUMDB, GOINSECURE | **PASS** (leak=false) |
| Sentinel matrix go-absent | GODEBUG, GOEXPERIMENT, GCCGO, CC, CXX, CGO_CFLAGS, CGO_LDFLAGS | **PASS** (absent) |
| Live `go env` under construction | 12 closed surfaces | **PASS** |
| GOFLAGS `-toolexec` control (threat) | inherited host GOFLAGS | **INFO** (helper **ran** — threat real) |
| GOFLAGS `-toolexec` isolated | constructed env + `go test -a -c` | **PASS** (helper **did not** run) |
| GOCACHEPROG control (threat) | inherited | **INFO** (helper ran) |
| GOCACHEPROG isolated | constructed | **PASS** (helper did not run) |
| GOENV file control (threat) | `GOENV=<hostile-file>` | **INFO** (injects GOFLAGS with toolexec) |
| GOENV file isolated | child `GOENV=off`; tool GOFLAGS empty | **PASS** |
| Construct git env allowlist | 7 keys only | **PASS** |
| Git sentinel keys absent | HOME, GIT_DIR, GIT_WORK_TREE, XDG_CONFIG_HOME, … | **PASS** (13 keys) |
| Git template contrast (threat) | `--template=<hostile>` | **INFO** (SENTINEL_TEMPLATE_FILE copied) |
| Git template isolated | empty owned template + null configs | **PASS** (no sentinel file, no hostile hooks) |
| Git config origins | `git config --list --show-origin` | **PASS** (no hostile-gitconfig) |
| Process tree = default plan | FakeRunner | **PASS** |
| Detect extra shell step | negative FakeRunner | **PASS** (count + shell + extra) |
| Detect env pollution | MutateEnv GOFLAGS/EVIL_EXTRA | **PASS** |
| Live go-env step tree | RealRunner one-step plan | **PASS** |
| Full e2e matrix | all of the above + marker clean | **PASS** (43/43 pass, 0 fail) |
| Plan closed world (no shell) | default + strict step tables | **PASS** |
| Darwin cross-compile | `GOOS=darwin GOARCH=arm64 go test -c` | **PASS** |
| Module regression | `go test ./...` | **PASS** |

All required Linux contracts: **pass** (0 fail). Full log: [`e2-logs/go-test.txt`](e2-logs/go-test.txt).

### Sentinel matrix (per-key leakage boolean)

Spike logging format: `E2PROBE … key=<NAME> leak=true|false …` (key names only; no secret values).

| Key | Kind | Observed leak | Notes |
| --- | ---- | ------------- | ----- |
| `GOENV` | go-fixed | **false** | Child env `off`; `go env GOENV` reports empty when disabled |
| `GOFLAGS` | go-fixed | **false** | Empty; toolexec sentinel does not execute |
| `GOCACHEPROG` | go-fixed | **false** | Empty; cache helper does not execute |
| `GOAUTH` | go-fixed | **false** | `off` |
| `GOVCS` | go-fixed | **false** | `*:off` |
| `GOPRIVATE` | go-fixed | **false** | empty |
| `GONOPROXY` | go-fixed | **false** | empty |
| `GONOSUMDB` | go-fixed | **false** | empty |
| `GOINSECURE` | go-fixed | **false** | empty |
| `GODEBUG` | go-absent | **false** | absent from child |
| `GOEXPERIMENT` | go-absent | **false** | absent |
| `GCCGO` / `CC` / `CXX` / `CGO_*` | go-absent | **false** | absent |
| Git `HOME` / `GIT_DIR` / … | git-absent | **false** | 13 keys absent |
| `GIT_TEMPLATE` | git-template | **false** | no host file/hook copy |
| Helper marker | helper-prog | **false** | no toolexec/cacheprog/goauth/hook lines |

### Process-tree vs plan `external_steps`

Default plan (Section 34.1 closed world, default verify):

```
go-mod-tidy: go mod tidy -mod=mod
go-mod-verify: go mod verify
go-test: go test -count=1 -buildvcs=false -mod=readonly ./...
go-vet: go vet -buildvcs=false -mod=readonly ./...
```

With optional git (when `git.init=true`):

```
git-init: git init --initial-branch=main --template=<owned-empty> .
```

| Audit | Result |
| ----- | ------ |
| FakeRunner observations == plan (id, argv, binary, env keys+values) | **PASS** |
| Injected `sh -c` extra step | **Detected** (`count` + `shell` + `extra`) |
| Injected `GOFLAGS` / `EVIL_EXTRA` env | **Detected** (`env` diffs) |
| Live single-step `go env` RealRunner | **PASS** |
| E2E plan includes go steps + git-init | **PASS** — `process-tree equals plan external_steps` |

Env allowlist for go steps is the Section 34.2 map recorded verbatim on each `ExternalStep.Env` (promotion source for plan JSON `external_steps[].env`).

### Raw log excerpts

Construction + fixed keys:

```
E2PROBE	os=linux arch=amd64 probe=construct_go step=allowlist_keys
  args="CGO_ENABLED,GOAUTH,GOCACHE,…" outcome=pass detail="key count=22"
E2PROBE	… probe=construct_go step=fixed_value key=GOENV leak=false outcome=pass
E2PROBE	… probe=construct_go step=fixed_value key=GOFLAGS leak=false outcome=pass
E2PROBE	… probe=construct_go step=fixed_value key=GOCACHEPROG leak=false outcome=pass
```

Threat model (control) vs isolation:

```
E2PROBE	… probe=toolexec step=control_inherit key=GOFLAGS leak=true outcome=info
  detail="threat model: inherited GOFLAGS -toolexec ran=true"
E2PROBE	… probe=toolexec step=isolated_construct key=GOFLAGS leak=false outcome=pass
  args="go test -a -c" detail="constructed env must not execute host toolexec"

E2PROBE	… probe=gocacheprog step=control_inherit key=GOCACHEPROG leak=true outcome=info
E2PROBE	… probe=gocacheprog step=isolated_construct key=GOCACHEPROG leak=false outcome=pass

E2PROBE	… probe=goenv_file step=control_read key=GOENV leak=true outcome=info
  detail="threat: GOENV file injects GOFLAGS containing toolexec=true"
E2PROBE	… probe=goenv_file step=isolated_off key=GOENV leak=false outcome=pass
  detail="child_GOENV=off tool_GOENV= tool_GOFLAGS_empty=true"
```

Git template:

```
E2PROBE	… probe=git_template step=contrast_hostile key=GIT_TEMPLATE leak=true outcome=info
  detail="threat model: host template copies SENTINEL_TEMPLATE_FILE"
E2PROBE	… probe=git_template step=isolated_empty key=GIT_TEMPLATE leak=false outcome=pass
  args="git init --initial-branch=main --template=<owned-empty>"
  detail="sentinel_file=false hostile_hooks= hooks_entries="
```

Process tree:

```
E2PROBE	… probe=process_tree step=default_plan outcome=pass
  detail="process-tree equals plan external_steps"
E2PROBE	… probe=process_tree step=detect_extra_shell outcome=pass
  detail="… shell step=evil-shell unplanned step launched via shell; extra …"
E2PROBE	… probe=e2e_matrix step=process_tree outcome=pass
  detail="process-tree equals plan external_steps"
```

Full matrix:

```
E2 full matrix: pass=43 fail=0 os=linux/amd64
PASS
ok  	github.com/robertguss/go-foundry-cli/integration/hostile/e2
```

## Architecture audit (spike)

| Check | Result |
| ----- | ------ |
| Empty base + allowlist (not denylist) | **Present** — `ConstructGoEnv` / `ConstructGitEnv` build maps from scratch |
| Host pollution ignored for fixed keys | **Proven** — ambient sentinels + live `go env` |
| Helper execution under isolation | **Absent** — toolexec / GOCACHEPROG / goauth / template hooks |
| Git system/global config | **Null** — `GIT_CONFIG_*=/dev/null`, `NOSYSTEM=1` |
| Empty owned template | **Present** — `EmptyTemplateDir` mode 0700; removed after use |
| Plan records exact env | **Present** — `ExternalStep.Env` + `CompareProcessTree` |
| Shell in closed world | **Forbidden** — `TestE2_PlanClosedWorld_NoShell` |
| Production toolrun package | **Not created** (Phase 2); fixtures are the promotion source for REQ-214 / bhn |

### Operational note: `go env GOENV` reporting

When the child process environment sets `GOENV=off`, `cmd/go env GOENV` reports an **empty** string (file disabled), not the literal `off`. The isolation contract is:

1. Child env map contains `GOENV=off` (construction).
2. Tool does not read a host/persisted env file (proven: hostile file injects GOFLAGS only when inherited; isolated GOFLAGS stays empty).
3. `go env GOENV` is not a filesystem path.

This is **not** a contradiction of Section 34.2 (which mandates the process env value `off`).

## P2 / CI promotion path (fixtures without rewrite)

| Spike path | Promotion target |
| ---------- | ---------------- |
| `integration/hostile/e2/env.go` (`ConstructGoEnv`, `ConstructGitEnv`, allowlists) | `internal/toolrun` env construction (P2) — REQ-154 |
| `integration/hostile/e2/sentinels.go` (`FullSentinelMatrix`, `WriteSentinelHelpers`, `HostileHostEnv`) | Permanent REQ-214 hostile suite (bead **go-foundry-cli-bhn**) |
| `integration/hostile/e2/plan.go` (`ExternalStep`, `CompareProcessTree`, default/strict steps) | Plan `external_steps` type + process-tree audit helper |
| `integration/hostile/e2/runner.go` (`FakeRunner`, `RealRunner`) | Fake-runner unit tests + live sentinel integration (REQ-214) |
| `integration/hostile/e2/git.go` (`EmptyTemplateDir`, `IsolatedGitInit`) | Git init step in toolrun (REQ-128 / §34.2) |
| `E2PROBE` log format (`key=` + `leak=`) | CI isolation failure artifacts (key names only; redacted values) |
| `TestE2_FullMatrix_EndToEnd` | CI required job seed for bhn |

**Do not rewrite:** allowlist key sets, fixed normative values, empty-template protocol, process-tree equality rules, sentinel key matrix names.

## Result

**CONFIRMS** Section 34.2 (exact environments), FND-005 (external-tool boundary / undeclared helpers and Git templates), and seeds REQ-135 / REQ-154 / REQ-214 on **Linux** (`linux/amd64`) with real `go1.26.5` and `git 2.43.0`:

1. Environments are constructed from an empty base plus the exact Section 34.2 allowlist.
2. Full sentinel matrix (GOENV / GOFLAGS / GOCACHEPROG / GOAUTH / GOVCS / private-module vars / undeclared GO* / git config & template) shows **leak=false** under construction; threat-model controls confirm helpers **would** run if inherited.
3. Isolated `git init` uses an empty owned template and null system/global config; host template files and hooks are not copied.
4. Process-tree comparison against plan `external_steps` passes; extras, shells, and env pollution are detected.
5. Fixtures under `integration/hostile/e2/` are promotable into the permanent REQ-214 / **bhn** CI suite without rewrite.

**Operational refinements recorded (not contradictions of §34.2):**

1. **`go env GOENV`** prints empty when process env is `GOENV=off` (disabled file); construction still sets `off`.
2. **Live owner-macOS** isolation run awaits Darwin CI; Darwin cross-compile of the spike already succeeds.

**FND-005:** executable allowlisting without configuration isolation is false — this spike proves configuration isolation closes the surfaces. Proceed to Phase 2 toolrun using these constructors and the sentinel suite as the REQ-214 template.
