package spec

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// namePattern is Section 15.1: lowercase ASCII kebab-case starting with a letter,
// 1–63 bytes: [a-z][a-z0-9]*(-[a-z0-9]+)*
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// MaxNameBytes is the name / binary / git.initial_branch length cap (Section 14.3).
const MaxNameBytes = 63

// MaxDescriptionBytes is the description length cap after trim (Section 14.3).
const MaxDescriptionBytes = 200

// SupportedSchema is the only supported schema version (REQ-039).
const SupportedSchema int64 = 1

// DefaultVisibility is applied when visibility is omitted.
const DefaultVisibility = "private"

// DefaultGitInit is applied when [git].init is omitted.
const DefaultGitInit = true

// DefaultGitInitialBranch is applied when [git].initial_branch is omitted.
const DefaultGitInitialBranch = "main"

// validName reports whether s is a Section 15.1 project/binary/branch name.
func validName(s string) bool {
	if s == "" || len(s) > MaxNameBytes {
		return false
	}
	// len(s) is bytes; pattern is ASCII-only so byte length == rune length.
	return namePattern.MatchString(s)
}

// nameRuleMessage explains a name/binary/branch character-rule failure.
func nameRuleMessage(field, s string) string {
	switch {
	case s == "":
		return field + " must be non-empty lowercase ASCII kebab-case (1–63 bytes, start with a letter)"
	case len(s) > MaxNameBytes:
		return field + " is " + itoa(len(s)) + " bytes; maximum is 63"
	case !namePattern.MatchString(s):
		return field + " must be lowercase ASCII kebab-case matching [a-z][a-z0-9]*(-[a-z0-9]+)* (got " + quote(s) + ")"
	default:
		return field + " is invalid"
	}
}

// validateDescription checks Section 14.3 description rules.
// On success, returns the trimmed value.
func validateDescription(s string) (trimmed string, errMsg string) {
	if strings.ContainsAny(s, "\n\r") {
		return "", "description must be a single line (no newline or carriage return)"
	}
	trimmed = strings.TrimSpace(s)
	if trimmed == "" {
		return "", "description must be non-empty after trim"
	}
	if !utf8.ValidString(trimmed) {
		return "", "description must be valid UTF-8"
	}
	if len(trimmed) > MaxDescriptionBytes {
		return "", "description is " + itoa(len(trimmed)) + " bytes after trim; maximum is 200"
	}
	return trimmed, ""
}

// hasSemanticImportVersionSuffix reports a final path segment that is a
// semantic import-version suffix (/v2, /v3, …) per REQ-043 / Section 14.3.
// module.CheckPath accepts these for libraries; Foundry rejects them because
// generated outputs are applications, not versioned libraries.
func hasSemanticImportVersionSuffix(modulePath string) bool {
	if modulePath == "" {
		return false
	}
	// Use path.Base (slash-separated) — module paths are always /-separated.
	base := path.Base(modulePath)
	if len(base) < 2 || base[0] != 'v' {
		return false
	}
	rest := base[1:]
	if rest == "" {
		return false
	}
	// N looks numeric: ASCII digits and dots (module.CheckPath wording).
	// Reject v2, v3, v10, etc. (and dotted forms if present).
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '.' {
			continue
		}
		return false
	}
	// Must have at least one digit.
	hasDigit := false
	for i := 0; i < len(rest); i++ {
		if rest[i] >= '0' && rest[i] <= '9' {
			hasDigit = true
			break
		}
	}
	return hasDigit
}

// moduleFinalSegment returns the last /-separated element of a module path.
func moduleFinalSegment(modulePath string) string {
	return path.Base(modulePath)
}

// destinationLexicalOK checks Section 15.3 lexical constraints that do not
// require filesystem access. On success, base is the path basename.
//
// Rejected: empty, ".", "..", components equal to ".." or containing NUL,
// "~" (alone or as a component prefix expansion form), environment markers
// "$" / "${", and any "~" character used as home expansion.
func destinationLexicalOK(dest string) (base string, errMsg string) {
	if dest == "" {
		return "", "destination must be a non-empty path"
	}
	if strings.ContainsRune(dest, 0) {
		return "", "destination must not contain NUL bytes"
	}
	// Environment markers — no expansion of any kind (Section 15.3).
	if strings.Contains(dest, "${") || strings.Contains(dest, "$") {
		return "", "destination must not contain environment-variable markers ($ or ${)"
	}
	// Tilde home expansion is prohibited.
	if strings.Contains(dest, "~") {
		return "", "destination must not contain '~' (no home-directory expansion)"
	}

	// Normalize separators for component scan; keep original for basename.
	normalized := strings.ReplaceAll(dest, "\\", "/")
	if normalized == "." || normalized == "./" {
		return "", `destination must not be "." (generation into the current directory is prohibited)`
	}

	parts := strings.Split(normalized, "/")
	// Drop empty segments from leading/trailing slashes (absolute path OK).
	var comps []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		comps = append(comps, p)
	}
	if len(comps) == 0 {
		return "", "destination path has no basename"
	}
	for _, c := range comps {
		if c == ".." {
			return "", `destination must not contain ".." components`
		}
		if c == "." {
			// Intermediate "." is useless but not a ".." escape; still reject
			// pure "." which is handled above. Allow "./foo" where first
			// meaningful component is foo (comps would be ["foo"] after split
			// of "./foo" → "", ".", "foo" wait: "./foo" → [".", "foo"]).
			// Section 15.3 forbids ".." and "~" and env — lone "." components
			// from "./foo" are normal relative form. Allow "." components that
			// are not the sole path.
			continue
		}
	}

	base = comps[len(comps)-1]
	if base == "." || base == ".." {
		return "", "destination basename is invalid"
	}
	if base == "" {
		return "", "destination path has no basename"
	}
	return base, ""
}

// validArchetype reports whether s is exactly "cli" or "tui".
func validArchetype(s string) bool {
	return s == "cli" || s == "tui"
}

// validVisibility reports whether s is exactly "private" or "public".
func validVisibility(s string) bool {
	return s == "private" || s == "public"
}

func quote(s string) string {
	return strconv.Quote(s)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
