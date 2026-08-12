// Package fixtures hosts Foundry-owned CLI golden and extension-path fixtures
// (P2.6.c / go-foundry-cli-wly; REQ-010, REQ-066).
//
// Layout:
//
//	foundry-smoke-cli/          Project Spec for the disposable smoke shell
//	extension-cli-subcommand/   Post-generate overlay proving growth recipe
//	testdata/                   Plan/tree/meta goldens
//
// Tests use internal/testutil step logging and never auto-update goldens in CI.
package fixtures
