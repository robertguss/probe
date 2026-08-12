// Package dogfood_test exercises early Phase 2 CLI dogfood (Section 52 / 60,
// REQ-242 / REQ-245 / REQ-247, bead go-foundry-cli-vu8).
//
// Sequence:
//  1. Disposable foundry-smoke-cli: generate → build → test → help/version
//  2. Real repo-map: generate → manual inventory domain overlay → test → exercise
//
// Complements (does not replace) the full generate e2e matrix (j8h.2) and
// hostile suite (qd8). Evidence lives under docs/evidence/dogfood-cli-*.
//
// Generated trees use custody-safe private parents under sticky /tmp; Foundry
// never writes dogfood projects into this repository.
package dogfood_test
