// Package e2 is the Phase-2-entry Go/Git environment isolation spike (evidence gate E2).
//
// It proves FND-005 / Section 34.2 / REQ-135 / REQ-154 / REQ-214:
//
//  1. Subprocess environments are constructed from an empty base plus an exact
//     allowlist — never inherited-minus-denylist.
//  2. Hostile sentinel values in host GOENV, GOFLAGS, GOCACHEPROG, GOAUTH,
//     GOVCS, GOPRIVATE, GONOPROXY, GONOSUMDB, GOINSECURE, GODEBUG,
//     GOEXPERIMENT, and other undeclared keys do not appear in the constructed
//     child environment and do not affect tool behavior.
//  3. git init runs with GIT_CONFIG_GLOBAL=/dev/null, GIT_CONFIG_SYSTEM=/dev/null,
//     GIT_CONFIG_NOSYSTEM=1, and an empty Foundry-owned template directory —
//     host template files and hook scripts are never copied.
//  4. Observed process trees equal the plan's external_steps (no extras, no shell).
//
// Patterns promote into the permanent REQ-214 sentinel suite without rewrite:
// integration/hostile/sentinel exercises production internal/toolrun constructors
// against these plants; CI job linux-hostile-sentinel (go-foundry-cli-bhn).
//
// Bound: ≤2 days. Spike evidence gate; production path is toolrun + sentinel.
package e2
