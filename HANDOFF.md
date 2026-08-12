# Session handoff

**Date:** 2026-08-01  
**Branch:** `main` (must be clean and up to date with `origin/main`)  
**Last work:** macOS `make product-e2e` green; residual beads closed (`mpc` tree)  
**Issue tracker:** `bd` (Beads). Run `bd prime` first. Do **not** use TodoWrite / markdown TODO lists for work tracking.

This file is the on-ramp for the next agent or human session. Prefer **SPEC-FOUNDRY-002** and `bd` over inventing scope.

---

## 1. One-line state

**Product E2E residual is closed.** Linux + macOS MBP both green for `make product-e2e`. No open beads from the test/hardening epic chain. Prefer **product/SPEC feature work** next (or inspect CI for post-n0y Windows/coverage floors).

---

## 2. Open beads

```bash
bd ready
bd list --status=open
```

Expect **no open residual operator beads** after mpc closeout. If Dolt and JSONL diverge again, recover with `bd bootstrap` or `bd init --from-jsonl` per `bd help init-safety` (JSONL had the open residuals when local Dolt was stale).

---

## 3. What just closed (this session)

| ID | Title | Result |
| -- | ----- | ------ |
| `go-foundry-cli-mpc` | Verify `make product-e2e` on macOS (MBP) | **PASS** ~3.0m; evidence `docs/evidence/product-e2e-macos.md` |
| `go-foundry-cli-874.9.2` | [H2] macOS MBP product-e2e | closed via mpc |
| `go-foundry-cli-874.9` | §H Platforms (H1–H2) | H1+H2 complete |
| `go-foundry-cli-874` | Product E2E inventory checklist epic | all children closed |
| `go-foundry-cli-q8b` | Product E2E residual / operator verification | complete |

Host: Darwin arm64, go1.26.5, commit `a27eda5` at run time.

---

## 4. Platforms (current truth)

| Platform | Product | CI / tests |
| -------- | ------- | ---------- |
| **Linux** | Full support | Primary unit, race, hostile, generate-e2e; product-e2e green |
| **macOS** | Full support | unit + generate-e2e + darwin-evidence; **product-e2e green on MBP** |
| **Windows** | **Not a product platform** | Hard-gated unit job for stubs/build tags only |

---

## 5. Project rules (non-negotiable)

1. **SPEC-FOUNDRY-002** is sole product authority:  
   [`docs/02-definitive-foundry-specification-revised-fable-5.md`](docs/02-definitive-foundry-specification-revised-fable-5.md)
2. **`bd` for all task tracking** — claim before code; close with reasons; `bd dolt push` on session end  
3. **Session complete only after push:**

   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status   # must show up to date with origin
   ```

4. Testing guide: [`docs/dev/testing.md`](docs/dev/testing.md)  
5. Product E2E plan: [`docs/dev/product-e2e-plan.md`](docs/dev/product-e2e-plan.md)  
6. Agent instructions: [`AGENTS.md`](AGENTS.md), [`CLAUDE.md`](CLAUDE.md)

---

## 6. Commands (suite budget)

```bash
# Fast (default agent loop)
go test -count=1 ./internal/...
go test ./cmd/foundry -run TestWriteFree -count=1
go vet ./...
gofmt -l .

# Medium — generate path changes
go test -count=1 -timeout 15m ./integration/generate/

# Full product bar (local/agent only — NOT CI)
make product-e2e

# Hostile / sentinel
make hostile
make sentinel

# Docs / testscripts
make docs-validate
make lint-testscripts

# Build product binary
CGO_ENABLED=0 go build -o /tmp/foundry ./cmd/foundry
```

---

## 7. Architecture (one screen)

```
spec → catalog/resolve → plan → render → fsx (stage/commit)
     → toolrun (go/git) → verify → report/cli
```

Embedded catalog under `catalog/`. Product entry: `cmd/foundry` → `internal/cli`.  
Generate is the only mutating command; validate/plan are write-free.

---

## 8. What NOT to start next

- Another large coverage / wet-style test epic  
- More residual/coverage line-chasing (`coverage_test.go` filler)  
- Wiring product-e2e into CI  
- Mutation testing (low priority)  
- Re-softening Windows CI without a bead + policy decision  

Prefer **product/SPEC feature work**. Optional: inspect latest GitHub Actions on `main` for Windows hard gate or package coverage floors after n0y.

---

## 9. Key paths

| Path | Role |
| ---- | ---- |
| `docs/02-definitive-foundry-specification-revised-fable-5.md` | Spec authority |
| `docs/dev/testing.md` | Test layers, CI, tags, suite budget |
| `docs/dev/product-e2e-plan.md` | Product E2E inventory (H2 closed) |
| `docs/evidence/product-e2e-linux.md` | Linux product-e2e evidence |
| `docs/evidence/product-e2e-macos.md` | macOS MBP product-e2e evidence |
| `docs/evidence/req-traceability.md` / `.json` | REQ→test matrix |
| `docs/evidence/risk-register-status.md` | Residual risks (Darwin + MBP PASS) |
| `Makefile` | `product-e2e`, `hostile`, `sentinel`, … |

---

## 10. Session close protocol

Work is **not** done until:

1. Issues updated/closed in `bd`  
2. Quality gates green for any code change  
3. `git pull --rebase && bd dolt push && git push`  
4. `git status` shows **up to date with origin**  

Never leave commits only local. Never say “ready to push when you are.”

---

## 11. Suggested first message for next agent

```text
Read HANDOFF.md. Run bd prime and bd ready.

Product E2E residual (mpc / 874 / q8b) is closed. Prefer SPEC feature
work or CI health (windows-unit hard gate + coverage floors after n0y).
Do not invent another test epic. Use bd for all task tracking. Push
before ending.
```
