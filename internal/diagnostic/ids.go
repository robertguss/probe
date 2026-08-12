package diagnostic

// Identifier is a stable machine-readable domain.reason string from Appendix D.
// Identifiers are append-only across releases (REQ-157).
type Identifier string

// Appendix D identifiers (normative inventory). Constants exist for every
// domain even before owning packages implement emission.
const (
	// spec.* — specification parse/validate (exit 2)
	IDSpecParseError        Identifier = "spec.parse_error"
	IDSpecUnsupportedSchema Identifier = "spec.unsupported_schema"
	IDSpecUnknownField      Identifier = "spec.unknown_field"
	IDSpecDuplicateKey      Identifier = "spec.duplicate_key"
	IDSpecTooLarge          Identifier = "spec.too_large"
	IDSpecInvalidEncoding   Identifier = "spec.invalid_encoding"
	IDSpecInvalidField      Identifier = "spec.invalid_field"
	IDSpecDuplicateProfile  Identifier = "spec.duplicate_profile"

	// resolve.* — flat resolution (exit 2)
	IDResolveUnknownProfile    Identifier = "resolve.unknown_profile"
	IDResolveUnknownArchetype  Identifier = "resolve.unknown_archetype"
	IDResolveProfileConstraint Identifier = "resolve.profile_constraint"

	// plan.* — plan construction
	IDPlanFileCollision Identifier = "plan.file_collision"

	// fs.* — descriptor-relative transaction / destination
	IDFSUnsafePath          Identifier = "fs.unsafe_path"
	IDFSParentMissing       Identifier = "fs.parent_missing"
	IDFSNamespaceNotPrivate Identifier = "fs.namespace_not_private"
	IDFSDestinationExists   Identifier = "fs.destination_exists"
	IDFSRenameUnsupported   Identifier = "fs.rename_unsupported"
	IDFSParentMoved         Identifier = "fs.parent_moved"
	IDFSCommitFailed        Identifier = "fs.commit_failed"
	IDFSCommitAmbiguous     Identifier = "fs.commit_ambiguous"

	// render.* — pure render
	IDRenderFailed Identifier = "render.failed"

	// tool.* — preflight and external steps
	IDToolMissing      Identifier = "tool.missing"
	IDToolWrongVersion Identifier = "tool.wrong_version"
	IDToolTimeout      Identifier = "tool.timeout"
	IDToolFailed       Identifier = "tool.failed"

	// verify.* — staging verification
	IDVerifyFailed            Identifier = "verify.failed"
	IDVerifyModuleMutation    Identifier = "verify.module_mutation"
	IDVerifyUnplannedMutation Identifier = "verify.unplanned_mutation"

	// git.* — isolated git init
	IDGitFailed Identifier = "git.failed"

	// report.* — required pre-commit output stream
	IDReportFailed Identifier = "report.failed"

	// catalog.* — embedded/dev catalog
	IDCatalogInvalid Identifier = "catalog.invalid"

	// usage.* — CLI usage
	IDUsageInvalid Identifier = "usage.invalid"

	// internal.* — recovered orchestration panic
	IDInternalBug Identifier = "internal.bug"
)

// allIdentifiers is the complete Appendix D domain.reason set, in Appendix D
// table order. Used for registry completeness and dump determinism.
// Cancellation (exit 130) is not a domain.reason identifier — see ExitCancelled.
var allIdentifiers = []Identifier{
	IDSpecParseError,
	IDSpecUnsupportedSchema,
	IDSpecUnknownField,
	IDSpecDuplicateKey,
	IDSpecTooLarge,
	IDSpecInvalidEncoding,
	IDSpecInvalidField,
	IDSpecDuplicateProfile,
	IDResolveUnknownProfile,
	IDResolveUnknownArchetype,
	IDResolveProfileConstraint,
	IDPlanFileCollision,
	IDFSUnsafePath,
	IDFSParentMissing,
	IDFSNamespaceNotPrivate,
	IDFSDestinationExists,
	IDFSRenameUnsupported,
	IDFSParentMoved,
	IDFSCommitFailed,
	IDFSCommitAmbiguous,
	IDRenderFailed,
	IDToolMissing,
	IDToolWrongVersion,
	IDToolTimeout,
	IDToolFailed,
	IDVerifyFailed,
	IDVerifyModuleMutation,
	IDVerifyUnplannedMutation,
	IDGitFailed,
	IDReportFailed,
	IDCatalogInvalid,
	IDUsageInvalid,
	IDInternalBug,
}

// AllIdentifiers returns a copy of the Appendix D identifier inventory in
// stable table order.
func AllIdentifiers() []Identifier {
	out := make([]Identifier, len(allIdentifiers))
	copy(out, allIdentifiers)
	return out
}

// String returns the domain.reason text.
func (id Identifier) String() string { return string(id) }
