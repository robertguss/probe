package diagnostic

import (
	"fmt"
	"sort"
)

// Entry is one immutable registry row for a domain.reason identifier.
type Entry struct {
	ID          Identifier
	ExitCode    int
	Meaning     string // short normative meaning (Appendix D)
	Remediation string // agent-actionable: what failed + how to fix (REQ-159)
}

// registry is built once at init from the fixed entry table. After init, only
// reads occur (REQ-188 — no package-level mutable state after init).
type registry struct {
	byID map[Identifier]Entry
	// ordered is Appendix D table order for deterministic dumps.
	ordered []Entry
}

// reg is the process-wide immutable registry. Never reassigned after init.
// Concurrent readers only; no writes after buildRegistry returns (REQ-188).
var reg = buildRegistry()

func buildRegistry() *registry {
	entries := []Entry{
		{
			ID:       IDSpecParseError,
			ExitCode: ExitUsage,
			Meaning:  "TOML syntax error (line/column)",
			Remediation: "Fix the TOML syntax at the reported file:line:column. " +
				"Re-run `foundry validate` after correcting the syntax.",
		},
		{
			ID:       IDSpecUnsupportedSchema,
			ExitCode: ExitUsage,
			Meaning:  "schema not in supported set (1)",
			Remediation: "Set schema = 1 in the specification (only schema version 1 is supported). " +
				"Remove or correct any other schema value, then re-run `foundry validate`.",
		},
		{
			ID:       IDSpecUnknownField,
			ExitCode: ExitUsage,
			Meaning:  "Unknown field/table (strict decoding)",
			Remediation: "Remove the unknown field or table named in the error, or correct its spelling. " +
				"Foundry uses strict TOML decoding — unknown keys are errors. See Section 14 of the specification.",
		},
		{
			ID:       IDSpecDuplicateKey,
			ExitCode: ExitUsage,
			Meaning:  "Duplicate TOML key or table (strict decoding)",
			Remediation: "Remove or rename the duplicate key/table at the reported file:line:column. " +
				"Each key path may appear only once in the Project Specification.",
		},
		{
			ID:       IDSpecTooLarge,
			ExitCode: ExitUsage,
			Meaning:  "Project Specification exceeds the 1 MiB size cap",
			Remediation: "Reduce the specification to at most 1 MiB (1048576 bytes). " +
				"Remove comments, unused keys, or oversized description text, then re-run `foundry validate`.",
		},
		{
			ID:       IDSpecInvalidEncoding,
			ExitCode: ExitUsage,
			Meaning:  "Project Specification is not UTF-8 (or has a leading BOM)",
			Remediation: "Save the specification as UTF-8 without a byte-order mark (BOM). " +
				"Remove non-UTF-8 bytes and re-run `foundry validate`.",
		},
		{
			ID:       IDSpecInvalidField,
			ExitCode: ExitUsage,
			Meaning:  "Field violates its Section 14.3 rule",
			Remediation: "Correct the field value to satisfy its Section 14 constraint (type, range, pattern, or enum). " +
				"The error names the field and the rule that failed.",
		},
		{
			ID:       IDSpecDuplicateProfile,
			ExitCode: ExitUsage,
			Meaning:  "Same profile ID listed twice",
			Remediation: "List each profile ID at most once in the specification profiles array. " +
				"Remove the duplicate entry and re-run `foundry validate`.",
		},
		{
			ID:       IDResolveUnknownProfile,
			ExitCode: ExitUsage,
			Meaning:  "Profile ID not an implemented built-in; sorted available set named",
			Remediation: "Replace the unknown profile ID with one from the available set listed in the error, " +
				"or remove it (MVP: profiles = []). Run `foundry catalog list` to see implemented profiles. " +
				"configuration and local-persistence are not schema-1 profiles — they are Section 21 recipes " +
				"(docs/recipes/configuration.md, docs/recipes/local-persistence.md). Exact IDs only; no fuzzy matching.",
		},
		{
			ID:       IDResolveUnknownArchetype,
			ExitCode: ExitUsage,
			Meaning:  "Archetype not cli/tui",
			Remediation: "Set archetype to exactly \"cli\" or \"tui\". " +
				"No other archetype values are supported.",
		},
		{
			ID:       IDResolveProfileConstraint,
			ExitCode: ExitUsage,
			Meaning:  "Direct archetype/visibility predicate failed",
			Remediation: "Remove the profile that is incompatible with the selected archetype, " +
				"or change the archetype so the profile's visibility/constraint predicates pass. " +
				"The error names the failing profile and predicate.",
		},
		{
			ID:       IDPlanFileCollision,
			ExitCode: ExitFailure,
			Meaning:  "Duplicate output ownership (all claimants named)",
			Remediation: "Resolve the file ownership collision: remove or rename one of the conflicting " +
				"contributions so each output path has a single owner. The error names every claimant.",
		},
		{
			ID:       IDFSUnsafePath,
			ExitCode: ExitUsage,
			Meaning:  "Symlink among destination parent components / unsafe destination",
			Remediation: "Choose a destination whose parent path components are real directories (no symlinks). " +
				"Foundry refuses symlink parents for custody and rename safety.",
		},
		{
			ID:       IDFSParentMissing,
			ExitCode: ExitUsage,
			Meaning:  "Destination parent absent or not a directory",
			Remediation: "Create the destination's parent directory first, or choose a destination under an existing directory. " +
				"Foundry does not create missing destination parents.",
		},
		{
			ID:       IDFSNamespaceNotPrivate,
			ExitCode: ExitUsage,
			Meaning:  "Shared-writable non-sticky destination-parent component (custody)",
			Remediation: "Use a destination under a private directory tree (not a shared-writable non-sticky component such as a world-writable non-sticky /tmp style path). " +
				"Tighten parent permissions or pick another location.",
		},
		{
			ID:       IDFSDestinationExists,
			ExitCode: ExitUsage,
			Meaning:  "Destination exists in any form (incl. commit-time EEXIST); losing stage preserved",
			Remediation: "Choose a path that does not exist, or remove/rename the existing destination after inspecting it. " +
				"Foundry never overwrites an existing destination. If a stage was preserved, its location is reported " +
				"for manual inspection/removal (RSK-310; Foundry never auto-deletes stages).",
		},
		{
			ID:       IDFSRenameUnsupported,
			ExitCode: ExitFailure,
			Meaning:  "Filesystem lacks exclusive no-replace rename, or unexpected EXDEV; stage preserved",
			Remediation: "Use a destination on a filesystem that supports exclusive no-replace rename within one mount. " +
				"The preserved stage location is reported; after identity check you may manually `mv` it into place " +
				"or remove it (RSK-310; Foundry never auto-deletes stages).",
		},
		{
			ID:       IDFSParentMoved,
			ExitCode: ExitFailure,
			Meaning:  "Pre-commit diagnostic reobservation shows the retained parent no longer at its pathname; stage preserved",
			Remediation: "Do not move or replace the destination parent while Foundry is running. " +
				"Inspect the preserved stage at the reported location and re-run against a stable parent path " +
				"(RSK-310; Foundry never auto-deletes stages).",
		},
		{
			ID:       IDFSCommitFailed,
			ExitCode: ExitFailure,
			Meaning:  "Commit rename failed (other); stage preserved",
			Remediation: "Inspect the preserved stage at the reported location and the underlying OS error. " +
				"After verifying the stage tree, manually `mv` it into place or remove it, then re-run generate " +
				"(RSK-310; Foundry never auto-deletes stages).",
		},
		{
			ID:       IDFSCommitAmbiguous,
			ExitCode: ExitFailure,
			Meaning:  "Contradictory post-syscall identity observations; all mutation stopped; stage state reported",
			Remediation: "Stop and inspect both paths named in the error before any further mutation. " +
				"Do not delete stages automatically; resolve identity manually, then decide whether to keep or remove objects " +
				"(RSK-310; Foundry never auto-deletes stages).",
		},
		{
			ID:       IDRenderFailed,
			ExitCode: ExitFailure,
			Meaning:  "Template/format failure (file and cause named)",
			Remediation: "Fix the catalog template or gomod input named in the error (syntax, missing key, or format failure). " +
				"Re-run `foundry plan` to reproduce without writing the destination.",
		},
		{
			ID:       IDToolMissing,
			ExitCode: ExitFailure,
			Meaning:  "Required go/git not found at startup",
			Remediation: "Install the missing tool and ensure it is on PATH. " +
				"`go` is always required; `git` is required when [git] init is enabled. Re-run after installation.",
		},
		{
			ID:       IDToolWrongVersion,
			ExitCode: ExitFailure,
			Meaning:  "Preflight go version differs from the exact pinned toolchain",
			Remediation: "Install or select the exact Go toolchain version Foundry pins (see catalog/versions.toml and the error detail). " +
				"Foundry does not accept nearby versions.",
		},
		{
			ID:       IDToolTimeout,
			ExitCode: ExitFailure,
			Meaning:  "External step exceeded its plan-declared timeout; stage preserved",
			Remediation: "Inspect the preserved stage and the step's bounded output. " +
				"Address the cause of the hang/slowness (network, module cache, tests) and re-run; do not delete the stage without inspection.",
		},
		{
			ID:       IDToolFailed,
			ExitCode: ExitFailure,
			Meaning:  "External step non-zero exit (step id, argv, bounded output); stage preserved",
			Remediation: "Read the step id, argv, and bounded output in the error. " +
				"Fix the underlying tool failure in the preserved stage (or inputs), then re-run generate.",
		},
		{
			ID:       IDVerifyFailed,
			ExitCode: ExitFailure,
			Meaning:  "Verification step failed in staging; stage preserved",
			Remediation: "Inspect the verification failure detail and the preserved stage. " +
				"Fix tests, vet issues, or other gate failures, then re-run.",
		},
		{
			ID:       IDVerifyModuleMutation,
			ExitCode: ExitFailure,
			Meaning:  "Tidy changed files beyond go.mod/go.sum or altered pinned requirements",
			Remediation: "Ensure the generated module's requirements match the pinned catalog versions and that tidy only adjusts go.mod/go.sum. " +
				"Remove unexpected replace/exclude/workspace directives or unplanned files tidy introduced.",
		},
		{
			ID:       IDVerifyUnplannedMutation,
			ExitCode: ExitFailure,
			Meaning:  "Staged tree deviates from frozen baseline (paths named); stage preserved",
			Remediation: "A verification step mutated the staged tree unexpectedly. " +
				"Inspect the named divergent paths in the preserved stage and correct the tool or catalog so verification is read-only.",
		},
		{
			ID:       IDGitFailed,
			ExitCode: ExitFailure,
			Meaning:  "Isolated git init or .git semantic validation failed; stage preserved",
			Remediation: "Ensure `git` is available and functional. " +
				"Inspect the preserved stage's .git layout; fix host git issues and re-run. Foundry only runs isolated `git init` with a scratch template.",
		},
		{
			ID:       IDReportFailed,
			ExitCode: ExitFailure,
			Meaning:  "Required pre-commit output stream failed (EPIPE); stage preserved",
			Remediation: "Do not close stdout/stderr before Foundry finishes pre-commit reporting. " +
				"Re-run without a broken pipe; the preserved stage is available for inspection.",
		},
		{
			ID:       IDCatalogInvalid,
			ExitCode: ExitFailure,
			Meaning:  "Embedded/dev catalog failed validation",
			Remediation: "This is a Foundry packaging/catalog defect if using the embedded catalog. " +
				"If using a dev catalog override, fix the invalid manifest or asset named in the error.",
		},
		{
			ID:       IDUsageInvalid,
			ExitCode: ExitUsage,
			Meaning:  "Unknown command/flag/argument or conflicting flags",
			Remediation: "Correct the command line: use a known command/flag, remove conflicts (e.g. --quiet with --verbose), " +
				"and run `foundry --help` or `foundry <command> --help` for usage.",
		},
		{
			ID:       IDInternalBug,
			ExitCode: ExitFailure,
			Meaning:  "Recovered orchestration panic; report requested",
			Remediation: "An internal Foundry bug was recovered (not a user input error). " +
				"Report the Foundry version, full error message, and reproduction inputs. Do not treat this as a fixable spec mistake.",
		},
	}

	r := &registry{
		byID:    make(map[Identifier]Entry, len(entries)),
		ordered: make([]Entry, len(entries)),
	}
	for i, e := range entries {
		if e.ID == "" {
			panic(fmt.Sprintf("diagnostic: empty identifier at index %d", i))
		}
		if e.ExitCode != ExitFailure && e.ExitCode != ExitUsage {
			panic(fmt.Sprintf("diagnostic: invalid exit code %d for %s", e.ExitCode, e.ID))
		}
		if e.Meaning == "" || e.Remediation == "" {
			panic(fmt.Sprintf("diagnostic: incomplete entry for %s", e.ID))
		}
		if _, dup := r.byID[e.ID]; dup {
			panic(fmt.Sprintf("diagnostic: duplicate identifier %q", e.ID))
		}
		r.byID[e.ID] = e
		r.ordered[i] = e
	}

	// Completeness vs allIdentifiers constants.
	for _, id := range allIdentifiers {
		if _, ok := r.byID[id]; !ok {
			panic(fmt.Sprintf("diagnostic: missing registry entry for %s", id))
		}
	}
	if len(r.byID) != len(allIdentifiers) {
		panic(fmt.Sprintf("diagnostic: registry size %d != identifier list %d", len(r.byID), len(allIdentifiers)))
	}
	return r
}

// Lookup returns the registry entry for id.
func Lookup(id Identifier) (Entry, bool) {
	e, ok := reg.byID[id]
	return e, ok
}

// MustLookup returns the entry or panics. Intended for internal wiring where
// the identifier is a package constant.
func MustLookup(id Identifier) Entry {
	e, ok := reg.byID[id]
	if !ok {
		panic(fmt.Sprintf("diagnostic: unknown identifier %q", id))
	}
	return e
}

// Known reports whether id is in the Appendix D registry.
func Known(id Identifier) bool {
	_, ok := reg.byID[id]
	return ok
}

// ExitCodeFor returns the Appendix D exit code for id, or ExitFailure if unknown.
func ExitCodeFor(id Identifier) int {
	if e, ok := reg.byID[id]; ok {
		return e.ExitCode
	}
	return ExitFailure
}

// RemediationFor returns the default agent-actionable remediation for id.
func RemediationFor(id Identifier) string {
	if e, ok := reg.byID[id]; ok {
		return e.Remediation
	}
	return ""
}

// Entries returns all registry entries in Appendix D table order (deterministic).
func Entries() []Entry {
	out := make([]Entry, len(reg.ordered))
	copy(out, reg.ordered)
	return out
}

// MissingIDs returns sorted identifiers from want that are not registered.
// Used by completeness tests; prints nothing — caller logs sorted missing IDs.
func MissingIDs(want []Identifier) []Identifier {
	var missing []Identifier
	for _, id := range want {
		if !Known(id) {
			missing = append(missing, id)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return missing
}

// ExtraIDs returns sorted registered identifiers not present in want.
func ExtraIDs(want []Identifier) []Identifier {
	set := make(map[Identifier]struct{}, len(want))
	for _, id := range want {
		set[id] = struct{}{}
	}
	var extra []Identifier
	for id := range reg.byID {
		if _, ok := set[id]; !ok {
			extra = append(extra, id)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	return extra
}
