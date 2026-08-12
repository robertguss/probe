package fsx

// BeginOptions configures Begin (Section 31 pipeline + Section 42.2/43).
type BeginOptions struct {
	// Log receives structured walk/custody/stage/commit steps. Nil is a no-op.
	Log StepLogger
	// Project is the project name embedded in the stage basename
	// (.foundry-<project>-<random>). Empty uses the destination basename
	// (Section 15.3 / 31.5).
	Project string
}

// Options is an alias for BeginOptions (historical / convenience name).
type Options = BeginOptions

// Transaction holds retained parent and stage handles plus the recorded stage
// identity for one destination generation (SPEC-FOUNDRY-002 Section 31 +
// package table 42.2).
//
// Public surface (normative):
//
//   - RootedWriter() / Writer() — descriptor-relative writes into the stage
//   - DuplicateStageHandle() — CLOEXEC FD for descriptor-bound child start (34.4)
//   - Commit() CommitResult — exclusive no-replace rename + Section 31.9 matrix
//   - Close() — release handles only
//
// There is deliberately no Delete/Remove/Unlink/Scavenge method (Section 31.6 /
// REQ-130). Stages are always preserved for owner inspection. Raw host paths
// are never exposed for mutation.
//
// Begin is the only production constructor: it runs the full Section 31
// pipeline through stage creation (parent walk → custody → destination
// preflight → exclusive stage create). E1 primitives are the underlying
// implementation (openat/mkdirat/os.Root/Renameat2|RenameatxNp).
type Transaction struct {
	parent    *ParentHandle
	stage     *Stage
	log       StepLogger
	committed bool
	closed    bool
}
