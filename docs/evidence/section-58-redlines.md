# Section 58 — Red-line register

Living register of **rejected work** from SPEC-FOUNDRY-002 Section 58
(and the closely related rejected command/flag surface in §13.7 that agents
reintroduce under delivery pressure).

**Authority:**
[`docs/02-definitive-foundry-specification-revised-fable-5.md`](../02-definitive-foundry-specification-revised-fable-5.md)
§58, §13.7, REQ-012, REQ-030, REQ-077, REQ-130, REQ-161, REQ-180, REQ-184.

**Mechanical enforcement:** `internal/archtest` redline suite (this document’s
owning package). P1.8 (`go-foundry-cli-4hi`) **consumes** the same table —
do not open a second red-line system. Extend rows and scanners here when new
packages land.

**On violation:** tests print `redline_id`, `file:line`, match snippet
(capped), and remediation:
`rejected by Section 58; see docs/evidence/section-58-redlines.md`.
Never silent-skip.

**Process:** when a later package could reintroduce a rejected idea, add a
row (or extend Detection) and a check in `internal/archtest` before the
package merges. Reversal of any red line requires an explicitly revised
accepted specification — not a “helpful” flag.

| ID | Short name | Why rejected | Mechanical detection | Owning test package | Checkable now |
| -- | ---------- | ------------ | -------------------- | ------------------- | ------------- |
| RL-58-VERIFY-BYPASS | Verification bypass / `--verify none` | Undermines the defining quality promise (§58) | Token scan of `cmd/` + `internal/` for `--verify none`, `verify=none`, `VerifyNone`; help/flag registry forbids `none` as a verify mode value when CLI lands | `internal/archtest` | Yes (tokens); expand when `internal/cli` lands |
| RL-58-OFFLINE | `--offline` / offline mode | Unenforceable whole-process claim (FND-007, §58) | Token scan for offline flag spelling in product surface; no offline field in plan/help schemas | `internal/archtest` | Yes |
| RL-58-FORCE | `--force` / `--overwrite` | Destination replacement prohibited (§13.7, REQ-030) | Token scan for force/overwrite flags in `cmd/` + `internal/` | `internal/archtest` | Yes |
| RL-58-EXISTING-MOD | Existing-project mod / upgrade / sync | New projects only (DEC-002/DEC-003, §58) | Forbidden command-name tokens (`upgrade`, `migrate`, `sync` as commands) in CLI surface when cobra lands; static command allowlist = six commands | `internal/archtest` | Partial (token seed); full when `internal/cli` lands |
| RL-58-PLUGINS | Plugins, remote catalogs, template downloads, profile scripts | No extension platform (DEC-014, REQ-012, §58) | Forbidden package dirs; command tokens `plugin`/`template`; no network catalog fetch packages | `internal/archtest` | Partial (packages + tokens) |
| RL-58-PROFILE-FRAMEWORK | Generic profile-composition framework | Transitive requires, conflicts, capability registry, providers, helper-binary schema, provenance closure (FND-009, §58) | Forbidden dirs `internal/compose`; type/token scan for capability-DAG / provenance-chain identifiers; no `requires`/`conflicts` graph packages | `internal/archtest` | Yes (dirs + type names) |
| RL-58-TYPED-EMITTERS | Typed YAML / GoReleaser / workflow emitters + limited-substitution language | FND-015; recipes replace emitters (§58) | Forbidden dir `internal/structured`; package name tokens for typed emitters | `internal/archtest` | Yes |
| RL-58-DEMO | Generated demo features (`greet`, synthetic TUI init) | FND-014; core has no demos (§58, App. A) | Forbidden paths/symbols `internal/greet`, demo command seeds in product packages | `internal/archtest` | Yes |
| RL-58-PROVENANCE-FILE | Persistent generated provenance files | Plan is the provenance record (REQ-161, §58) | Forbidden package/type names for Foundry provenance files/chains in product code | `internal/archtest` | Yes (types/packages) |
| RL-58-WINDOWS | Windows support | DEC-013; macOS+Linux only (§58) | No `//go:build windows` / `*_windows.go` under `cmd/` or `internal/`; no product GOOS windows plan claims | `internal/archtest` | Yes |
| RL-58-CLAUDE | Claude-specific conventions | DEC-015; agent-neutral AGENTS.md only (§58) | Token scan for Claude-only scaffold conventions in product generators (expand when render lands) | `internal/archtest` | Seed only |
| RL-58-MISC-STACK | Viper, secret storage, SQLite-by-default, mutation testing, self-update, vendor agent files, generation telemetry | Explicit §58 bullet | Forbidden import paths (viper, etc.) and self-update command tokens in product graph | `internal/archtest` | Yes (imports + tokens) |
| RL-58-STAGE-DELETE | Automatic stage deletion | REQ-130/184; stage always preserved (§31.6, §58 largest rejections) | No stage-delete API symbols in production packages; expand to full no-`RemoveAll`-on-stage audit when `internal/fsx` lands | `internal/archtest` | Yes (symbol seed); expand with fsx |
| RL-58-DRY-RUN | `dry-run` command/flag | `plan` is the authoritative dry run (§13.7) | Token scan for `--dry-run` / dry-run flag in CLI surface | `internal/archtest` | Yes |

## Detection detail (mechanical)

### A. Forbidden package directories

Must not exist under the module root (REQ-180):

| Path | Redline ID |
| ---- | ---------- |
| `internal/compose` | RL-58-PROFILE-FRAMEWORK |
| `internal/structured` | RL-58-TYPED-EMITTERS |
| `internal/provenance` | RL-58-PROVENANCE-FILE |
| `internal/capability` | RL-58-PROFILE-FRAMEWORK |
| `internal/greet` | RL-58-DEMO |

### B. Forbidden tokens (product surface)

Scanned as raw file text under `cmd/` and `internal/` (all `*.go` except
paths under `testdata/`). Pattern spellings are assembled at runtime so the
scanner source does not self-match.

| Pattern class | Examples (assembled) | Redline ID |
| ------------- | -------------------- | ---------- |
| Offline flag | `--offline` | RL-58-OFFLINE |
| Force flags | `--force`, `--overwrite` | RL-58-FORCE |
| Verify bypass | `--verify none`, `verify=none` | RL-58-VERIFY-BYPASS |
| Dry-run flag | `--dry-run` | RL-58-DRY-RUN |
| Self-update | `self-update`, `selfupdate` as command-ish tokens | RL-58-MISC-STACK |

### C. Forbidden identifiers (AST / text)

| Identifier / fragment | Redline ID |
| --------------------- | ---------- |
| `ProvenanceChain`, `ProvenanceClosure` | RL-58-PROVENANCE-FILE |
| `CapabilityDAG`, `CapabilityRegistry` | RL-58-PROFILE-FRAMEWORK |
| `DeleteStage`, `RemoveStage`, `CleanStage`, `CleanupStage`, `DestroyStage`, `AutoDeleteStage` | RL-58-STAGE-DELETE |

### D. Forbidden imports (module graph)

| Import path contains / equals | Redline ID |
| ----------------------------- | ---------- |
| `github.com/spf13/viper` | RL-58-MISC-STACK |
| module path + `/internal/compose` | RL-58-PROFILE-FRAMEWORK |
| module path + `/internal/structured` | RL-58-TYPED-EMITTERS |

### E. Windows platform surface

Under `cmd/` and `internal/` only:

- Build constraint lines matching `windows` as a positive OS term
  (`//go:build windows`, `// +build windows`, `windows && …` product files).
- Source filenames `*_windows.go`.

Third-party modules (e.g. indirect `charmbracelet/x/windows`) are **not**
product Windows support and are out of scope for this check.

### F. Help / flag registry (expands with CLI)

When `internal/cli` registers Cobra flags, the same suite must assert the
registered flag set contains none of: `offline`, `force`, `overwrite`,
`dry-run`, and that `--verify` values exclude `none`. Until cobra wiring
lands, token scan (B) is the surrogate.

## Living extension checklist

When these packages first land, **extend** this suite (do not fork):

| Package | Extensions |
| ------- | ---------- |
| `internal/cli` | Flag registry scan; help golden must not contain offline/force; six-command allowlist |
| `internal/plan` | Schema forbids offline / provenance-closure / capability-list fields |
| `internal/fsx` | Full no-stage-delete audit; no `RemoveAll`/unlink API on stages |
| `internal/render` | No Claude-specific agent files; no `greet` demo templates |
| `internal/generate` | No auto stage cleanup on failure paths |
| `internal/verify` | Mode enum is exactly `default\|strict` |

## Remediation for agents

If a redline test fails:

1. Read the printed `redline_id` and this register row.
2. Remove the rejected surface — do not “gate it behind a flag”.
3. If product intent truly changed, open a specification revision; do not
   land rejected work under delivery pressure.

## Related beads

| Bead | Role |
| ---- | ---- |
| `go-foundry-cli-2y7` | Authors this register + seed mechanical negatives (this document) |
| `go-foundry-cli-4hi` | P1.8 purity/architecture suite **consumes** the redline table |
| `go-foundry-cli-5an.1` | Write-free e2e also asserts no offline token in `--help` |
| `go-foundry-cli-dlv` | Cross-cutting governance parent |
