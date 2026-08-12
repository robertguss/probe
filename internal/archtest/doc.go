// Package archtest holds static architecture checks for the Foundry module
// (REQ-180–REQ-188, Section 42, Section 58) and the Phase-1 quality super-suite
// (P1.8 / go-foundry-cli-4hi: REQ-031 purity, REQ-219 golden discipline,
// REQ-157/188 registry completeness).
//
// Rules enforced:
//   - Forbidden packages (compose/structured/provenance/capability/greet) must not exist
//   - Section 58 red-line negatives: no offline/force/verify-none/dry-run surface tokens,
//     no stage-delete API symbols, no Windows product build tags, no viper/compose imports
//   - Production code must not import internal/testutil
//   - Only cmd/foundry may call os.Exit
//   - Cobra may only be imported by cmd/foundry and internal/cli
//   - Dependency direction (Section 42.3) when packages exist
//   - No import cycles (delegated to the Go toolchain; we assert graph acyclicity
//     of the declared layer order)
//   - Write-free purity (CheckPurity): P1 packages ban net/os/exec/syscall; cli
//     allows exec.LookPath only (no Command/Start) — complements P1.8.a e2e
//   - Golden CI discipline (CheckGoldenDiscipline): UPDATE_GOLDEN only outside CI;
//     workflows set CI=true and never UPDATE_GOLDEN (REQ-219)
//   - P1 registry completeness (CheckP1RegistryCompleteness): Appendix D ids
//     registered with remediation for every P1 domain
//
// Living red-line register: docs/evidence/section-58-redlines.md
// (table Redlines + CheckRedlines). P1.8 (4hi) consumes this suite — extend
// here when later packages land; do not open a second red-line system.
// Super-suite entry: TestP1QualitySuperSuite in quality_test.go.
//
// Living REQ→test matrix (Section 59 DoD #2 / dlv.1):
// docs/evidence/req-traceability.md + req-traceability.json.
// CheckReqTraceability ensures every Section 53 REQ appears exactly once.
// Phase exits (0z4/5pr/66w/7fm) invoke FOUNDRY_MATRIX_PHASE_EXIT=P1|…|DoD;
// DoD mode refuses any planned/TBD row (7fm cannot close with gaps).
//
// Profile admission process (Section 19.4 / REQ-078 / tpa):
// docs/dev/profile-admission.md — copy-pasteable checklist and recipe holdouts.
// TestProfileAdmissionDocHeadings keeps required headings and needles present
// so vague profile proposals stay process-blocked.
//
// Section 21 recipes (REQ-073–075 / 6hp):
// docs/recipes/configuration.md and docs/recipes/local-persistence.md.
// TestSection21RecipeDocsExist / TestSection21RecipesReferencedFromRemediation
// keep content, w4n remediation pointers, and catalog exclusion green.
//
// Package directories are created only when real code exists (Section 42.1);
// rules for packages that do not yet exist are still declared so violations
// fail as soon as the package appears with a bad import.
package archtest
