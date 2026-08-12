// Package generatee2e is the permanent generate end-to-end matrix
// (bead go-foundry-cli-j8h.2 / P2.5.d).
//
// It runs real `foundry generate` (via cli.Run and process-boundary testscripts)
// across the Section 45 matrix dimensions so CI failures name exact stage,
// plan step, CommitResult, and FS outcome without a debugger.
//
// Dimensions (acceptance / REQ-010 / REQ-034 / REQ-036 / REQ-133 / REQ-155):
//  1. Archetype: cli (tui waits for Phase 3)
//  2. Verify: default, strict
//  3. Git: init true/false
//  4. Commit outcomes: clean success; pre-existing destination (exit 2);
//     injected mid-stage failure (exit 1, stage preserved)
//  5. Cancellation: before stage (130, nothing on disk); after stage (130, preserve)
//  6. Network disclosure before stage create (human + JSON)
//  7. Stream failure: pre-commit blocks placement; post-commit exit 0
//  8. Process-tree plan external_steps equality (ids + FakePlanRunner shape)
//  9. Final tree digests + go test of generated project
//  10. Content scans: no timestamps/usernames/hostpaths (REQ-133)
//  11. Double-generation: identical non-git digests + plan_sha256
//  12. Quiet/progress: --quiet suppresses progress not errors; Section 29.2 names
//  13. Plan/generate equality smoke (full property owned by j8h.4)
//
// Logging: testutil step logger per case. On failure (or when
// FOUNDRY_GENERATE_E2E_ARTIFACT_DIR is set and the case intentionally fails),
// a redacted artifact names stage + cause + plan_sha256 + CommitResult +
// stage path / remediation. Never logs secret-bearing env values.
//
// Ordering: soft-related to dogfood (vu8); hard-blocks Phase 2 exit (5pr).
//
// CI: jobs linux-generate-e2e and macos-generate-e2e in .github/workflows/ci.yml.
//
//	go test -count=1 -timeout 15m ./integration/generate/
//	FOUNDRY_GENERATE_E2E_ARTIFACT_DIR=artifacts/generate-e2e go test -v ./integration/generate/
package generatee2e
