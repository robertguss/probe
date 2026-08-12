# Residual risk register status (RSK-300–312, RSK-400–403)

- **Date:** 2026-08-01
- **Bead:** `go-foundry-cli-bjt` (register); refresh `go-foundry-cli-n0y.3`
- **Authority:** SPEC-FOUNDRY-002 Section 55
- **Dogfood:** CLI `docs/evidence/dogfood-cli-smoke.md` + TUI `docs/evidence/dogfood-tui-smoke.md`
- **Product E2E:** Linux `docs/evidence/product-e2e-linux.md`; macOS MBP `docs/evidence/product-e2e-macos.md` (`mpc` closed)

Legend: **L/I** = Likelihood/Impact (H/M/L). **Mit** = mitigation implemented.
**Test** = automated or evidence gate.

| ID | Summary | L/I | Mit | Test / evidence | Dogfood observation |
| -- | ------- | --- | --- | --------------- | ------------------- |
| RSK-300 | Spec drift vs implementation | M/H | Spec authority docs; REQ matrix | `docs/evidence/req-traceability.md` (dlv.1) | No MVP drift found in dogfood |
| RSK-301 | Catalog pin drift | M/M | versions.toml lock + E5 | `integration/hostile/e5`, catalog lock tests | Pins stable through TUI deps |
| RSK-302 | Agent instruction fragmentation | M/M | Single AGENTS.md + CLAUDE.md build pointer | Core AGENTS.md; CLAUDE.md links Makefile/testing.md | Agents oriented from AGENTS |
| RSK-303 | Generate non-determinism | M/H | plan_sha256 + double-gen | fixtures double-gen CLI+TUI; property tests | Content digests equal across runs |
| RSK-304 | Stage debris / failed commit | M/M | fsx custody; stage cleanup | hostile fsx; generate e2e | Happy path: no stage debris |
| RSK-305 | TUI lifecycle/terminals | M/M | E4 + 1yq PTY/lifecycle | e4, integration/tui | PTY quit=0 SIGINT=130; dogfood noted non-interactive host |
| RSK-306 | Effect non-cooperation | L/H | Section 18.7 docs; no empty effects | ui-architecture.md; absence of effects.go | Shell has no effects (by design) |
| RSK-307 | Debug-log path attacks | M/H | §18.8 exclusive-create | `TestTUIDebugLogSafetyMatrix` | Separator/hidden/exists refused |
| RSK-308 | CI under/over-gate | M/M | Foundry CI matrix + coverage floors | `.github/workflows/ci.yml` | Multi-job PR matrix; product-e2e stays local |
| RSK-309 | Cross-platform FS differences | M/H | E1/fsx; hrc e2e; Darwin CI | hostile e1/fsx; generate-e2e; `darwin-evidence.yml` | Linux dogfood green; **Darwin E1 APFS PASS** (GHA macos-latest) |
| RSK-310 | Stage debris after cancel | M/M | cancel paths in generate e2e | integration/generate cancel cells | Measured in e2e matrix (no debris on success) |
| RSK-311 | Dependency supply chain | M/H | exact pins; govulncheck strict | versions.toml; strict.yml | Weekly govulncheck scheduled |
| RSK-312 | Dogfood friction ignored | M/M | dogfood evidence + beads | dogfood-*-*.md | Friction listed in CLI+TUI dogfood docs |
| RSK-400 | Evidence gates skipped | L/H | E1–E5 phase entry | o60 children; evidence docs | E1–E5 closed; Darwin live via GHA |
| RSK-401 | Hostile FS false confidence | M/H | permanent hostile suites in CI | sentinel, hostile-fsx jobs | CI jobs present |
| RSK-402 | Golden auto-update in CI | L/H | REQ-219 CI=true refuse | archtest golden discipline | No UPDATE_GOLDEN in workflows |
| RSK-403 | Light generated CI escapes defects | M/L | Documented risk-based promotion | testing.md RSK-403; strict.yml | Promotion path documented; dogfood PR latency path filed |

## High-impact without mitigation+test

**None** at this revision for closed dogfood path.

### Darwin / macOS residual (environment, not waived product gaps)

| Item | Status | Evidence |
| ---- | ------ | -------- |
| E1 Darwin/APFS live rename | **PASS** on GHA `macos-latest` | `docs/evidence/E1-descriptor-relative-transaction.md`; `darwin-evidence.yml` |
| E3 Darwin race live | **PASS** on GHA `macos-latest` | `docs/evidence/E3-race-prerequisites.md`; `e3-logs/darwin/` |
| Full `make product-e2e` on operator MBP | **PASS** (`go-foundry-cli-mpc` closed) | Evidence `docs/evidence/product-e2e-macos.md`; still no CI job |

## Explicit measurement rows

### RSK-310 stage debris

| Probe | Result | Ref |
| ----- | ------ | --- |
| Happy-path generate | no preserved stage | dogfood-tui/cli logs |
| Cancel/pre-commit fail cells | destination not placed | integration/generate matrix |
| Concurrent same-parent cells | no `.foundry-stage*` debris | `TestGenerateConcurrentSameParentDistinctBasenames` |

### RSK-305 TUI terminals

| Probe | Result | Ref |
| ----- | ------ | --- |
| Buffer lifecycle q/ctrlc | exit map | e4 + generated lifecycle_test |
| PTY quit / SIGINT | 0 / 130 | integration/tui `-tags=tui_pty` |
| Real worktree-status interactive | begun shell only | dogfood-tui-worktree-status.md |
