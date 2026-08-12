// Package sentinel is the permanent REQ-214 hostile env/config/template
// sentinel suite for production toolrun constructors (bead go-foundry-cli-bhn).
//
// It promotes E2 spike fixtures (integration/hostile/e2) into CI without
// rewrite of allowlist key sets, fixed normative values, empty-template
// protocol, process-tree equality rules, or sentinel key matrix names.
//
// Production surface under test:
//
//	internal/toolrun.ConstructGoEnv / ConstructGitEnv / EmptyTemplateDir
//	internal/toolrun.CompareProcessTree / IsolationDump
//
// Fixture plants (helpers, hostile template/config) come from package e2 so
// the E2 evidence matrix and this suite stay aligned.
//
// Logging: step logger per case (sentinel name, observation, argv, exit).
// On failure, redacted isolation dumps (env key names only + argv) are written
// to FOUNDRY_SENTINEL_ARTIFACT_DIR when set (CI artifact directory).
//
// Bound: Linux required in CI; macOS when available (//go:build unix).
package sentinel
