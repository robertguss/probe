// Package e3 is the Phase-2-entry race prerequisite probe (evidence gate E3).
//
// It proves FND-004 / Section 44.3 / Section 35.4 / REQ-004 / REQ-217:
//
//  1. Compiler preflight: race jobs require CGO_ENABLED=1 and a working host
//     C compiler; preflight discovers and validates the compiler.
//  2. Skip notice: missing compiler produces an explicit, stable skip notice
//     (never a silent pass).
//  3. Known-race fixture: with preflight OK, `go test -race` on the fixture
//     reports DATA RACE (instrumentation is active).
//  4. Generation gate exclusion: default and strict generation verification
//     steps (Section 35.1–35.2 / Appendix E) never include `go test -race`.
//  5. Product CGO ban: product/analysis/release stay CGO_ENABLED=0; race is
//     the sole, explicit CGO exception (DEC-013 / Section 44.3).
//
// Patterns here promote into Foundry CI race jobs and generated strict.yml
// compiler-preflighted race steps without rewrite.
//
// Bound: ≤1 day. Does not implement production CI templates (Phase 2/3).
package e3
