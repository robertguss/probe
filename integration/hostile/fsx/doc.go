// Package hostilefsx is the permanent REQ-213 / REQ-220 hostile filesystem suite
// for production internal/fsx (bead go-foundry-cli-qd8, P2.1.e).
//
// It promotes E1 spike fixtures (integration/hostile/e1) onto the production
// Transaction / Commit / custody surface without rewriting the Section 31.8
// identity classification matrix, two-process EEXIST race shape, parent-swap
// reobserve, custody owner/mode/sticky cases, EXDEV/rename_unsupported
// fail-closed paths, case-sensitivity native matrix, cancellation leftovers,
// sentinel survival, or the static no-stage-delete audit.
//
// Production surface under test:
//
//	internal/fsx.Begin / Transaction.Commit / Close
//	internal/fsx.AcquireParent / CreateStage / Commit
//	internal/fsx.ClassifyCommit / SetExclusiveRenameForTest (injection only)
//
// Probes (Section 45.1 / REQ-213):
//  1. Barrier-controlled parent-swap races
//  2. Namespace-custody owner/mode/sticky matrices
//  3. Stage-entry swaps before commit
//  4. Two-process EEXIST commit races with loser preservation
//  5. Injected ambiguous rename classification
//  6. EXDEV / rename-unsupported (FAT/exFAT negative) fail-closed
//  7. Case-sensitivity native matrix
//  8. Cancellation and crash leftovers preserve stage
//  9. Unrelated sentinel files always survive
//  10. Static no-stage-delete audit with surviving sentinels
//
// Logging: per probe OS, FS type, kernel, syscall, args summary, errno,
// pass/fail, stage identity. On failure, step log dumps land in
// FOUNDRY_HOSTILE_FSX_ARTIFACT_DIR when set (CI artifact directory).
//
// Build: //go:build hostile (see suite files). Default `go test ./...` stays
// green without the tag. CI job linux-hostile-fsx runs:
//
//	go test -tags=hostile -count=1 ./integration/hostile/fsx/
//
// Multi-FS matrix: FOUNDRY_FSX_ROOTS or E1_FS_ROOTS="ext4:/path,xfs:/path,..."
// (see scripts/fsx-mount-matrix.sh). Host FS only is the default CI path.
//
// Bound: Linux required in CI; macOS when runners available.
package hostilefsx
