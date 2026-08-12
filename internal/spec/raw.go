package spec

import "github.com/robertguss/go-foundry-cli/internal/diagnostic"

// RawSpecification is the strict TOML decode result before field validation
// and before Section 14.3 defaults (Section 43). Absence is represented by
// nil pointers / ProfilesSet=false / GitSet=false — never by default values.
//
// Immutable after construction: slices and the Positions map are owned by the
// value and must not be mutated by callers.
type RawSpecification struct {
	// File is the source path or label used in diagnostic locations.
	File string
	// ByteLen is the raw input length in bytes (pre-decode size check).
	ByteLen int

	Schema      *int64
	Name        *string
	Module      *string
	Description *string
	Archetype   *string
	Destination *string
	Binary      *string
	Visibility  *string

	// ProfilesSet is true when the profiles key appeared (including empty []).
	ProfilesSet bool
	Profiles    []string // non-nil when ProfilesSet; copy owned by this value

	// GitSet is true when a [git] table (or git.* keys) appeared.
	GitSet           bool
	GitInit          *bool
	GitInitialBranch *string

	// Positions maps dotted key paths ("schema", "git.init") to source locations.
	// Present for every key that appeared in the document (decoded or not).
	Positions map[string]diagnostic.Location
}

// Position returns the source location for a dotted key path, or a zero
// location when the key was absent.
func (r *RawSpecification) Position(key string) diagnostic.Location {
	if r == nil || r.Positions == nil {
		return diagnostic.Location{}
	}
	return r.Positions[key]
}

// IsDefined reports whether the dotted key path appeared in the document.
func (r *RawSpecification) IsDefined(key string) bool {
	if r == nil || r.Positions == nil {
		return false
	}
	_, ok := r.Positions[key]
	return ok
}
