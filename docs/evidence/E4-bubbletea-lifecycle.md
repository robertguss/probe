# E4 — Bubble Tea v2 lifecycle spike

- **Date:** 2026-07-30
- **Gated phase:** Phase 3 entry (Section 51.2, REQ-240)
- **Resolves:** FND-010 (executable confirmation of Section 18.3 / REQ-070)
- **Spike code:** [`integration/hostile/e4/`](../../integration/hostile/e4/)
- **Spike binary:** [`integration/hostile/e4/cmd/e4spike/`](../../integration/hostile/e4/cmd/e4spike/)
- **Raw logs:** [`docs/evidence/e4-logs/`](e4-logs/)

## Machine / tool versions

| Field | Value |
| ----- | ----- |
| Hostname | `dev-box` |
| OS | Ubuntu 24.04.4 LTS (`PRETTY_NAME`) |
| Kernel | `Linux 6.8.0-134-generic #134-Ubuntu SMP PREEMPT_DYNAMIC x86_64` |
| Arch | `linux/amd64` |
| Go | `go1.26.5` (`GOTOOLCHAIN=go1.26.5`; pin matches Section 12) |
| Bubble Tea | `charm.land/bubbletea/v2 v2.0.8` (Section 12.4 pin) |
| creack/pty | `github.com/creack/pty v1.1.24` |
| `golang.org/x/sys` | `v0.46.0` (pulled by bubbletea) |

Source: [`e4-logs/machine.txt`](e4-logs/machine.txt).

## Exact commands

```bash
# From repository root — full E4 matrix (buffer + PTY + real SIGINT/SIGTERM):
GOTOOLCHAIN=go1.26.5 go test ./integration/hostile/e4/ -count=1 -v -timeout 120s

# Full module (includes E1 regression after x/sys bump):
GOTOOLCHAIN=go1.26.5 go test ./... -count=1 -timeout 180s

# Spike binary (manual / subprocess harness target):
GOTOOLCHAIN=go1.26.5 go build -o /tmp/e4spike ./integration/hostile/e4/cmd/e4spike
```

Captured verbose run: [`e4-logs/go-test.txt`](e4-logs/go-test.txt) (`EXIT:0`).

## Expected behavior (specification)

| Contract | Spec | Expectation |
| -------- | ---- | ----------- |
| 1 | §18.3 #1 | Single lifecycle owner: `signal.NotifyContext(ctx, os.Interrupt, SIGTERM)` in main |
| 2 | §18.3 #2 | Program constructed with `tea.WithContext(appCtx)` + `tea.WithoutSignalHandler()`; no second signal handler |
| 3 | §18.3 #3 | Default panic recovery **enabled** (`WithoutCatchPanics` never used); framework owns terminal restore on panic |
| 4 | §18.3 #4 | Every user quit path cancels appCtx **before** `tea.Quit` / `tea.Interrupt` |
| 5 | §18.3 #5 / §18.7 | Effects are context-cooperative; tests track start/finish; zero active effects at return |
| 6 | §18.3 #6 | Exit mapping: `q`→0; Ctrl-C-as-key / SIGINT / SIGTERM / `ErrProgramKilled`→130; startup/runtime/panic/effect-error→1 |
| 7 | §18.6 | PTY/termios restored on normal quit, signal kill, and framework-recovered panic |
| 8 | FND-010 static | Source contains `WithContext` + `WithoutSignalHandler`; lacks `WithoutCatchPanics` |

**Forbidden:** dual signal owners; `WithoutCatchPanics`; non-cooperative Cmd that ignores cancel.

## Observed behavior

### Pass/fail table (run 2026-07-30T20:34:54Z)

| Probe | Path | Exit | Active effects @ return | PTY restore | Result |
| ----- | ---- | ---- | ----------------------- | ----------- | ------ |
| Static options | AST scan of spike package | n/a | n/a | n/a | **PASS** |
| `q` key (buffer) | cancel-before-quit + effect drain | 0 | 0 | n/a | **PASS** |
| Ctrl-C-as-key (buffer) | cancel + Interrupt (race→Killed ok) | 130 | 0 | n/a | **PASS** |
| SIGINT path (ctx cancel) | `WithContext` kill | 130 | 0 | n/a | **PASS** |
| SIGTERM path (ctx cancel) | identical NotifyContext cancel | 130 | 0 | n/a | **PASS** |
| Real SIGINT (subprocess+PTY) | `e4spike -mode=hold` | 130 | (process exit) | subprocess-exited | **PASS** |
| Real SIGTERM (subprocess+PTY) | `e4spike -mode=hold` | 130 | (process exit) | subprocess-exited | **PASS** |
| Real `q` (subprocess+PTY) | `e4spike -mode=idle` | 0 | (process exit) | subprocess-exited | **PASS** |
| Startup failure | short-circuit before `NewProgram` | 1 | 0 | n/a | **PASS** |
| Effect error | failing Cmd → cancel + quit | 1 | 0 | n/a | **PASS** |
| Framework panic (buffer) | default recovery + owner cancel | 1 | 0 | n/a-buffer | **PASS** |
| PTY restore on `q` | termios before==after; raw during | 0 | 0 | **restored** | **PASS** |
| PTY restore on signal | termios before==after | 130 | 0 | **restored** | **PASS** |
| PTY restore on panic | termios before==after | 1 | 0 | **restored** | **PASS** |
| MapExitCode table | q/ctrlc/signal/panic/startup/effect | — | — | — | **PASS** |
| `Run()` template helper | end-to-end owner API | 130 | 0 | unchecked | **PASS** |

All `E4PROBE` outcomes: **pass** (0 fail). Full log: [`e4-logs/go-test.txt`](e4-logs/go-test.txt).

### Cancel-before-quit / `WithContext` race (operational note)

Section 18.3 #4 requires canceling `appCtx` before returning `tea.Quit`. Because the program is also built with `tea.WithContext(appCtx)`, that cancel races the Quit/Interrupt message and frequently surfaces as:

```
program was killed: context canceled
```

**This does not contradict §18.3.** Model-reported `QuitClass` remains authoritative:

| Deliberate path | Model class | Exit after race |
| --------------- | ----------- | --------------- |
| `q` | `q` | **0** |
| Ctrl-C key | `ctrl+c-key` | **130** |
| External signal | `signal` (from `ErrProgramKilled`) | **130** |

`MapExitCode` + `ClassifyRunError` encode this for TUI templates. Documented in `lifecycle.go` and covered by `TestE4_MapExitCode_Table/q-killed-race`.

### PTY / termios matrix

On Linux PTY slaves:

| Phase | Typical flags (excerpt) | Cooked? |
| ----- | ----------------------- | ------- |
| Before `Run` | `lflag=0x8a3b` (ICANON\|ECHO) | yes |
| During raw | `lflag=0xa30` | **no** |
| After quit/signal/panic | `lflag=0x8a3b` (equal to before) | **yes** |

Primary restore proof is **termios equality** (`SnapshotTermios` / `TCGETS`). Alternate-screen exit CSI is secondary (renderer may not flush all sequences into the drain buffer under synthetic I/O); Section 18.6 is satisfied by framework `restoreTerminalState` + observed cooked-mode return.

### Raw log excerpts

Static options:

```
E4PROBE ... probe=static step=options outcome=pass
  detail="WithContext=present; WithoutSignalHandler=present; WithoutCatchPanics=absent(ok)"
```

`q` with in-flight cooperative effect:

```
E4PROBE ... probe=q step=pre_quit active_effects=1 signal_owner="main.signal.NotifyContext"
  detail="effect in-flight before q"
E4PROBE ... probe=q step=return outcome=pass active_effects=0 exit_code=0 quit_class="q"
  detail="want exit=0 class=q active=0; got exit=0 class=q active=0 run_err=program was killed: context canceled"
```

Real signals under PTY:

```
E4PROBE ... probe=subprocess-SIGINT ... exit_code=130 ... detail="signal=interrupt wait_err=exit status 130"
E4PROBE ... probe=subprocess-SIGTERM ... exit_code=130 ... detail="signal=terminated wait_err=exit status 130"
```

PTY restore on panic:

```
E4PROBE ... probe=pty-panic step=return outcome=pass pty_restore="restored" exit_code=1
  quit_class="framework-recovered-panic"
  detail="before=... cooked=true after=... cooked=true"
```

## Architecture audit (spike)

| Check | Result |
| ----- | ------ |
| Signal owner identity | **`main.signal.NotifyContext` only** — `WithoutSignalHandler` disables Bubble Tea's handler |
| `WithContext(appCtx)` | Present in `RequiredProgramOptions` / `Run` |
| `WithoutCatchPanics` | **Absent** (static AST check + runtime panic recovery observed) |
| Effect join by framework | Confirmed **not** joined; cooperation + `EffectTracker.WaitUntilIdle` required |
| Production TUI archetype | **Not created** (Phase 3 still gated; spike under `integration/hostile/e4`) |

## Template promotion path (fixtures without rewrite)

Patterns ready for generated TUI shell + permanent matrices (`1yq` / Phase 3):

| Spike path | Promotion target |
| ---------- | ---------------- |
| `NewLifecycleContext` + `RequiredProgramOptions` + `Run` | Generated `main.go` lifecycle owner |
| `SpikeModel` cancel-before-quit (`q` / `ctrl+c`) | Generated `update.go` quit paths |
| `EffectTracker` + `CooperativeSleep` | Foundry-owned fixture + `docs/ui-architecture.md` example (not empty generated `effects.go`) |
| `MapExitCode` / `ClassifyRunError` | Generated exit mapping |
| `StaticCheckOptions` | Generated CI / vet static check |
| PTY termios matrix (`pty_test.go`) | Permanent TUI lifecycle/PTY suite |
| `cmd/e4spike` | Optional fixture binary for hostile PTY CI |
| `E4PROBE` log format | Promotion into detailed probe logging |

**Do not rewrite:** single-owner option set, cancel-before-quit, zero-active-effects assertions, exit-code table, termios restore on quit/signal/panic.

## Result

**CONFIRMS** Section 18.3 (single lifecycle owner), Section 18.6 (terminal restoration), Section 18.7 (cooperative effects), FND-010, and REQ-070 on **Linux amd64** with Bubble Tea **v2.0.8**.

Proved executable matrix:

1. `tea.WithContext(appCtx)` + `tea.WithoutSignalHandler()`
2. Default panic recovery stays enabled
3. Effect-completion tracking with **zero active effects at return** on q / Ctrl-C-as-key / SIGINT / SIGTERM / startup failure / effect-error / recovered-panic paths
4. PTY termios restore on quit / signal / framework-recovered panic
5. Real process SIGINT/SIGTERM → exit 130; real PTY `q` → exit 0
6. Static presence of required options and absence of `WithoutCatchPanics`

**Operational refinement (not a contradiction):** cancel-before-quit races `WithContext` and may return `ErrProgramKilled+context.Canceled` on deliberate `q`/Ctrl-C; model `QuitClass` remains the exit-code authority. Templates MUST implement `MapExitCode` accordingly.

**FND-010 / Phase 3 entry:** architecture executable; proceed to Phase 3 TUI archetype work using this spike as the lifecycle template.
