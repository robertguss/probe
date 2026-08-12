// Package fsx owns descriptor-relative destination filesystem operations
// (SPEC-FOUNDRY-002 Section 31, Sections 15.3–15.4; REQ-003/044/124–131/184).
//
// This is the ONLY package that mutates destination or staging filesystem
// state. It acquires the destination parent through a no-follow component
// walk with an explicit namespace-custody check, retains the parent directory
// descriptor for the transaction's life, and performs all existence checks,
// stage creation, and commit relative to that handle.
//
// Implemented (P2.1.a–d + Transaction surface):
//
//   - Transaction (Section 43): Begin → Writer/RootedWriter / DuplicateStageHandle /
//     Commit() CommitResult / Close(); holds retained parent+stage handles and
//     recorded stage identity; exposes no deletion method (REQ-124–131/184)
//   - No-follow parent component walk (Section 31.2)
//   - Namespace custody: reject shared-writable non-sticky → fs.namespace_not_private (31.3)
//   - Destination must not exist in any form (15.3, 31.4, REQ-003)
//   - Pre-commit parent reobservation → fs.parent_moved (REQ-124)
//   - Exclusive stage create relative to parent fd: 0700, `.foundry-<name>-<random>`,
//     bounded EEXIST retries (16) (Section 31.5 / REQ-125)
//   - RootedWriter bound to stage object (os.Root / openat family); identity
//     verified before writes; exact modes via fchmod (REQ-125/184)
//   - DuplicateStageHandle: CLOEXEC dup of stage directory FD for Section 34.4
//     fchdir child start (toolrun never re-opens by path)
//   - Exclusive no-replace commit relative to parent: Linux Renameat2(RENAME_NOREPLACE),
//     Darwin RenameatxNp(RENAME_EXCL|RENAME_NOFOLLOW_ANY) (Section 31.7 / REQ-129)
//   - Post-syscall identity classification + Section 31.9 CommitResult matrix
//     (REQ-131); EEXIST → fs.destination_exists (exit 2, stage preserved);
//     unsupported/EXDEV fail closed; never guess on ambiguity
//   - Stage preservation policy + reporting hooks (Section 31.6 / REQ-130, RSK-310):
//     no unlink/remove/scavenge API; every uncommitted CommitResult carries
//     StagePath + RSK-310 manual inspect/remove remediation; PreserveReport
//     for human/JSON; static audit forbids production stage deletion
//   - Permanent hostile FS suite (REQ-213 / P2.1.e): integration/hostile/fsx
//     (//go:build hostile) + internal/fsx/hostile_test.go injection matrix;
//     CI job linux-hostile-fsx; E1 fixtures remain the promotion source
//
// Explicitly forbidden:
//
//   - Path-based MkdirTemp / RemoveAll for transactional steps
//   - Path-only check followed by path-based mutation
//   - Automatic stage deletion (Section 31.6) — Stage has no Delete/Remove API
//   - Following symbolic links among parent components
//   - Exposing raw host paths for mutation through RootedWriter
//   - Link/unlink or check-then-rename fallback when exclusive rename unsupported
//
// Errors use internal/diagnostic identifiers (Appendix D). Logging records
// component basenames, open flags, custody verdicts, refusal classes, stage
// basenames/paths, attempt counts, identities (dev/ino), rename syscall
// name/errno, CommitResult class, preserved stage basenames, and RSK-310 —
// never secrets or host home paths in golden streams.
//
// Layering (Section 42.3): may import diagnostic only among product packages.
// Must not import plan, render, toolrun, generate, cli, or testutil.
package fsx
