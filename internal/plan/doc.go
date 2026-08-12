// Package plan owns Generation Plan construction and JSON schema 1
// (SPEC-FOUNDRY-002 Section 28; REQ-120–REQ-122, REQ-032/033).
//
// The Generation Plan is the immutable, inspectable contract between pure
// interpretation and side effects. plan prints it without writes; generate
// executes exactly it. validate runs the identical pure pipeline and discards
// the plan (Section 30).
//
// Construction (Construct):
//   - Sole shared entrypoint for validate / plan / generate (REQ-120)
//   - Pure given Inputs: no FS, env, subprocess, clock, or network
//   - Host-dependent values (tool binaries, destination observation, host
//     env allowlist captures) are injected by the CLI layer so package tests
//     and goldens stay host-independent
//   - Renders planned content via internal/render for digests (Phase 1 pure)
//
// Serialization (REQ-122):
//   - Stable field order (struct order) and sorted collections
//   - Byte-identical JSON for identical Inputs
//   - plan_sha256 is SHA-256 hex of the canonical JSON excluding that field
//
// Schema 1 fields (Section 28.2 / Appendix C naming):
//
//	schema, foundry, specification, project, destination, profiles, files,
//	dependencies, tools, external_steps, tool_outputs, verification, network,
//	git, commit_result_model, plan_sha256, warnings
//
// Banned (FND-007/009 / Section 58): offline fields, capability lists,
// provenance closures, helper-binary implications.
//
// Layering (Section 42.3): may import diagnostic, spec, catalog, resolve,
// render. Must not import fsx, toolrun, generate, cli, or testutil.
package plan
