// Package render owns pure rendering mechanisms for Foundry (SPEC-FOUNDRY-002
// Section 26; REQ-092–REQ-097, REQ-184, REQ-212).
//
// Exactly three mechanisms exist (REQ-096):
//  1. static  — catalog source bytes copied verbatim (this package's Static path)
//  2. template — restricted complete-file text/template (this package; P1.5.b)
//  3. gomod   — typed go.mod generator via golang.org/x/mod/modfile (P1.5.c)
//
// Dispatch (package root / P1.5):
//   - Job + Render / RenderAll route by Mechanism with fail-closed payload checks.
//   - Merge combines pure inventories (path collisions fatal).
//   - Mechanisms() is the exhaustive closed set for strategy enum tests.
//   - ValidateTemplateSource supports hostile-template catalog validation.
//
// Phase 1 purity (Section 42.2 / REQ-184):
//   - Render never mutates the real destination or staging filesystem.
//   - Content is produced into pure buffers (MemoryWriter) for plan digests.
//   - Phase 2 wires the same Writer interface to fsx RootedWriter.
//
// Static pipeline (REQ-092–094):
//  1. Resolve catalog source by path from a CatalogReader
//  2. Compute SHA-256 content digest of source bytes
//  3. Copy bytes unchanged into a buffer (no substitution/evaluation)
//  4. Record an inventory entry: path, mode, mechanism=static, source id, digests
//
// Template pipeline (REQ-097 / Section 26.2):
//  1. Resolve catalog source; refuse unsafe paths
//  2. Parse with delimiters [[ / ]], Option missingkey=error
//  3. FuncMap exactly join + quote; reject define/include/partial
//  4. Execute over frozen TemplateData; fail closed on unknown fields
//  5. LF-normalize; go/format for .go outputs (Section 26.3)
//  6. Record inventory: mechanism=template, source digest, content digest
//
// Gomod pipeline (REQ-095 / Section 26.4):
//  1. Validate module path (module.CheckPath) and exact go/toolchain pins
//  2. Aggregate exact require + tool pins; version conflicts fatal
//  3. Emit via modfile (never replace/exclude/retract); SortBlocks for stability
//  4. Record inventory: path=go.mod, mechanism=gomod, source=typed, content digest
//
// Path safety (REQ-094): output paths must be relative and free of ".." /
// absolute forms; escape attempts fail closed at the render planning layer.
//
// Invariants:
//   - No package-level mutable state after init (REQ-188).
//   - Values crossing package boundaries are immutable after construction.
//   - Production sources import only stdlib + golang.org/x/mod + internal/catalog
//   - internal/diagnostic (no resolve/plan/fsx; no OS side effects).
package render
