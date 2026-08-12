package spec

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// ValidatedSpecification is the immutable post-validation Project Specification
// (Section 43). Defaults from Section 14.3 are applied; every field is within
// contract. Construct only via Validate.
//
// Values crossing the package boundary are immutable: no setters; slices returned
// by accessors are defensive copies; fields are unexported.
type ValidatedSpecification struct {
	file string

	schema      int64
	name        string
	module      string
	description string
	archetype   string
	destination string
	binary      string
	visibility  string
	profiles    []string // owned copy; never nil after construct

	gitInit          bool
	gitInitialBranch string
}

// File returns the source path/label used in diagnostics.
func (v *ValidatedSpecification) File() string {
	if v == nil {
		return ""
	}
	return v.file
}

// Schema returns the schema version (always 1 for a successful Validate).
func (v *ValidatedSpecification) Schema() int64 {
	if v == nil {
		return 0
	}
	return v.schema
}

// Name returns the project name.
func (v *ValidatedSpecification) Name() string {
	if v == nil {
		return ""
	}
	return v.name
}

// Module returns the module path.
func (v *ValidatedSpecification) Module() string {
	if v == nil {
		return ""
	}
	return v.module
}

// Description returns the trimmed single-line description.
func (v *ValidatedSpecification) Description() string {
	if v == nil {
		return ""
	}
	return v.description
}

// Archetype returns "cli" or "tui".
func (v *ValidatedSpecification) Archetype() string {
	if v == nil {
		return ""
	}
	return v.archetype
}

// Destination returns the destination path string from the specification
// (lexical form only; filesystem checks are out of this package).
func (v *ValidatedSpecification) Destination() string {
	if v == nil {
		return ""
	}
	return v.destination
}

// Binary returns the binary name (defaults to name when omitted in the source).
func (v *ValidatedSpecification) Binary() string {
	if v == nil {
		return ""
	}
	return v.binary
}

// Visibility returns "private" or "public" (defaults to "private").
func (v *ValidatedSpecification) Visibility() string {
	if v == nil {
		return ""
	}
	return v.visibility
}

// Profiles returns a copy of the profile ID list (defaults to empty non-nil).
// Order is the source order; resolution treats order as irrelevant.
func (v *ValidatedSpecification) Profiles() []string {
	if v == nil {
		return nil
	}
	out := make([]string, len(v.profiles))
	copy(out, v.profiles)
	return out
}

// GitInit reports whether isolated git init should run (default true).
func (v *ValidatedSpecification) GitInit() bool {
	if v == nil {
		return false
	}
	return v.gitInit
}

// GitInitialBranch returns the initial branch name (default "main").
func (v *ValidatedSpecification) GitInitialBranch() string {
	if v == nil {
		return ""
	}
	return v.gitInitialBranch
}

// Equal reports whether two validated specifications have identical field values
// (including file label). Used by tests for schema-position invariance.
func (v *ValidatedSpecification) Equal(o *ValidatedSpecification) bool {
	if v == nil || o == nil {
		return v == o
	}
	if v.file != o.file ||
		v.schema != o.schema ||
		v.name != o.name ||
		v.module != o.module ||
		v.description != o.description ||
		v.archetype != o.archetype ||
		v.destination != o.destination ||
		v.binary != o.binary ||
		v.visibility != o.visibility ||
		v.gitInit != o.gitInit ||
		v.gitInitialBranch != o.gitInitialBranch {
		return false
	}
	if len(v.profiles) != len(o.profiles) {
		return false
	}
	for i := range v.profiles {
		if v.profiles[i] != o.profiles[i] {
			return false
		}
	}
	return true
}

// NormalizedBytes returns a deterministic, position-independent encoding of all
// validated fields in Section 14.3 contract order (FND-013 / REQ-039).
// Schema physical position in the source never affects this encoding.
func (v *ValidatedSpecification) NormalizedBytes() []byte {
	if v == nil {
		return nil
	}
	var b bytes.Buffer
	// Fixed key order matching Section 14.3 table.
	writeKV(&b, "schema", strconv.FormatInt(v.schema, 10))
	writeKV(&b, "name", v.name)
	writeKV(&b, "module", v.module)
	writeKV(&b, "description", v.description)
	writeKV(&b, "archetype", v.archetype)
	writeKV(&b, "destination", v.destination)
	writeKV(&b, "binary", v.binary)
	writeKV(&b, "visibility", v.visibility)
	writeKV(&b, "profiles", formatProfiles(v.profiles))
	writeKV(&b, "git.init", strconv.FormatBool(v.gitInit))
	writeKV(&b, "git.initial_branch", v.gitInitialBranch)
	return b.Bytes()
}

func writeKV(b *bytes.Buffer, key, value string) {
	b.WriteString(key)
	b.WriteByte('=')
	// Escape newlines so the encoding stays single-record-per-line.
	b.WriteString(strings.ReplaceAll(value, "\n", "\\n"))
	b.WriteByte('\n')
}

func formatProfiles(profiles []string) string {
	if len(profiles) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, p := range profiles {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(p))
	}
	b.WriteByte(']')
	return b.String()
}

// String returns a short debug summary (not the normalized form).
func (v *ValidatedSpecification) String() string {
	if v == nil {
		return "ValidatedSpecification(nil)"
	}
	return fmt.Sprintf("ValidatedSpecification{name=%q archetype=%q module=%q}", v.name, v.archetype, v.module)
}
