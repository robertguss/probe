// Package e1 is the Phase-2-entry evidence spike for descriptor-relative
// transactions (Section 31, Section 34.4, OQ-400 / RSK-400 / REQ-240).
//
// This is NOT production internal/fsx. Fixtures and probe shapes are designed
// for promotion into P2.1.e (hostile filesystem suite) without rewrite.
//
// Contracts exercised:
//  1. No-follow parent component walk + namespace-custody check (31.2–31.3)
//  2. Handle-relative stage creation (0700, .foundry-<name>-<random>) (31.5)
//  3. Descriptor-bound child cwd via serialized fchdir protocol (34.4)
//  4. Exclusive no-replace commit + post-syscall identity classification (31.7–31.8)
//  5. Filesystem matrix: ext4/xfs/btrfs positive; FAT negative (rename_unsupported);
//     Darwin/APFS path is build-tagged (commit_darwin.go)
//  6. Primary sources SV-01 (os.Root), SV-02 (x/sys renames), SV-03 (openat/fchdir)
//
// Forbidden by design: pathname fallback, os.MkdirTemp staging, automatic
// stage deletion (Section 31.6).
package e1
