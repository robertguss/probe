// Package e5 hosts the Phase-1-exit version/action lock evidence spike (E5).
//
// The normative lock file is catalog/versions.toml (Section 33.4 / REQ-090).
// These tests assert the lock is present, well-formed, and covers every
// Section 12 pin so the fixture can be promoted into internal/catalog
// validation without rewrite.
//
// Online primary-source re-verification is recorded in
// docs/evidence/E5-version-action-lock.md and docs/evidence/e5-logs/.
package e5
