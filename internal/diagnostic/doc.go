// Package diagnostic owns Foundry's stable error-identifier registry, the
// FoundryError type, source locations, and redaction of known-sensitive
// environment/proxy/credential values (SPEC-FOUNDRY-002 Sections 38, 42.2, 43;
// Appendix D; REQ-157, REQ-159, REQ-188).
//
// Identifiers are append-only across releases. Every product failure carries a
// domain.reason identifier from the registry plus an agent-actionable
// remediation. The CLI maps FoundryError to a process exit code once at the
// process boundary.
//
// Invariants:
//   - No package-level mutable state after init (REQ-188).
//   - FoundryError does not store context.Context (REQ-188).
//   - Values crossing package boundaries are immutable after construction.
//   - Stack dumps are never the primary user/agent message.
package diagnostic
