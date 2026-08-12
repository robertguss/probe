// Package cli owns Cobra wiring, flag contracts, and output-mode selection
// for the Foundry process boundary (SPEC-FOUNDRY-002 Sections 13, 17.4, 36–38;
// REQ-030, REQ-162, REQ-181, REQ-186).
//
// Command surface (no aliases or hidden verbs):
//
//	init, validate, plan, generate, catalog list, catalog show, doctor, version
//
// Global flags: --output text|json, --quiet, --verbose, --color never|always|auto.
// JSON mode rejects human-mode flags (--quiet/--verbose/explicit --color).
// --spec is always explicit (path or "-" for stdin). init requires explicit
// --out. There is no network-isolation flag, destination-replacement flag, or
// separate dry run flag; plan is the authoritative dry run.
// generate is the only command that writes a Generated Project; init may write
// one Project Spec TOML; doctor may probe go binaries (advisory).
//
// Output formatting is delegated to internal/report (REQ-186). This package
// selects mode flags and constructs a report.Encoder per invocation; it does
// not re-implement text or JSON envelopes.
//
// Construction is fresh per NewRoot/Run (no package-level command globals,
// no init registration). Cobra is confined here; lower packages do not import it.
// SilenceErrors and SilenceUsage are set so the process boundary owns error
// presentation. Running with no subcommand prints deterministic help and exits 0.
//
// validate and plan share one pure pipeline (REQ-032/033 / Section 13.3):
// read spec (path or stdin) → Decode → Validate → catalog → non-binding
// destination observation → plan.Pipeline (resolve + construct). validate
// discards the plan; plan emits it (text summary; full schema-1 document in
// JSON). --verify default|strict is recorded on the plan. --dest overrides
// the specification destination under Section 15.3 rules; observation is
// non-binding. Write-free: no subprocess Start, no network, no writable opens.
//
// generate (Phase 2): shared pure pipeline → internal/generate lifecycle →
// report.Encoder progress/JSON (REQ-034/036/155). Stages are production
// Orchestrator funcs or Options.GenerateStages for tests. Exit codes follow
// the transaction/commit result (0/1/2/130); post-commit report failure never
// flips success. --quiet suppresses progress+summary never errors/network/
// stage_path. Cancellation uses the process NotifyContext from cmd/foundry.
//
// catalog list / catalog show (REQ-035) render only embedded catalog metadata
// (including core). JSON results carry catalog_digest matching version.
// Unknown show IDs fail closed with catalog.invalid. Write-free: no network,
// FS writes, or subprocesses on these paths.
package cli
