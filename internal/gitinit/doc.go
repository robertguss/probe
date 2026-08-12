// Package gitinit owns the Foundry's complete Git behavior: optional isolated
// `git init` only (SPEC-FOUNDRY-002 Section 39 / 34.2 / 29.2 stages 15–16;
// REQ-007, REQ-128, REQ-160).
//
// Exactly one Git action is permitted:
//
//	git init --initial-branch=<branch> --template=<owned-empty-scratch> .
//
// with the child's working directory bound to the stage descriptor via
// toolrun (Section 34.4). System/global Git config and host template
// directories are closed by construction (GIT_CONFIG_*=/dev/null,
// GIT_CONFIG_NOSYSTEM=1, GIT_TEMPLATE_DIR=<owned empty>).
//
// After a successful init the package:
//  1. Removes the Foundry-owned template scratch (outside the destination
//     namespace — under the Foundry temporary root, mode 0700).
//  2. Re-runs non-.git path/type/mode/byte conformance against a pre-init
//     snapshot (FND-006; full verify package may replace/extend later).
//  3. Validates `.git` semantically (directory layout, HEAD names the
//     configured branch, no hook content, empty index).
//
// NEVER (DEC-004 narrow interpretation / REQ-160):
// commits, user identity, hooks installation, remotes, push, tag, release,
// GitHub API, or any Git subcommand other than `init`.
//
// Subprocess starts are delegated exclusively to internal/toolrun (REQ-185).
// This package never imports os/exec for product starts.
//
// Failures use diagnostic id git.failed (exit 1). The stage is never deleted
// here (Section 31.6 / REQ-130) — preservation is generate/fsx authority.
//
// Logging contract: each step records argv (basename), env-allowlist hash
// and key names (never values), timeout, exit code, stdout/stderr byte
// lengths, and first/last output lines on failure only.
//
// Layering (Section 42.3): may import diagnostic, plan, toolrun. Must not
// import generate, verify, cli, or testutil in production sources.
package gitinit
