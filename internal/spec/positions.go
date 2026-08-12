package spec

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// indexKeyPositions scans TOML source for key and table header positions.
// Paths are dotted (e.g. "schema", "git.init"). Nested tables update the
// current prefix. This is a best-effort locator for diagnostics — BurntSushi
// MetaData does not export per-key positions.
//
// Limitations (acceptable for Foundry schema surface):
//   - Inline tables: records the parent key only
//   - Dotted keys a.b = v under [t] become t.a.b
//   - Array-of-tables [[x]] treated like [x] for prefix purposes
func indexKeyPositions(file string, data []byte) map[string]diagnostic.Location {
	out := make(map[string]diagnostic.Location)
	lines := strings.Split(string(data), "\n")
	var table []string // current table path pieces

	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Table header: [git] / [a.b] / [[array]]
		if strings.HasPrefix(trimmed, "[") {
			end := strings.Index(trimmed, "]")
			if end < 0 {
				continue
			}
			inner := trimmed[1:end]
			if strings.HasPrefix(inner, "[") {
				// Array-of-tables header [[foo]] — strip extra brackets.
				inner = strings.TrimPrefix(inner, "[")
				inner = strings.TrimSuffix(inner, "]")
			}
			inner = strings.TrimSpace(inner)
			if inner == "" {
				continue
			}
			table = splitDottedKey(inner)
			path := strings.Join(table, ".")
			col := columnOf(line, strings.Index(line, "["))
			// Prefer first sighting for table path (duplicate tables fail earlier).
			if _, exists := out[path]; !exists {
				out[path] = diagnostic.SpecLocation(file, lineNo, col)
			}
			continue
		}

		// Key = value (possibly dotted key)
		eq := indexUnquotedEquals(trimmed)
		if eq < 0 {
			continue
		}
		keyPart := strings.TrimSpace(trimmed[:eq])
		if keyPart == "" {
			continue
		}
		// Strip trailing comment-only lines already handled; keys may be quoted.
		pieces := splitDottedKey(keyPart)
		if len(pieces) == 0 {
			continue
		}
		full := append(append([]string{}, table...), pieces...)
		path := strings.Join(full, ".")
		// Column of key start in original line (1-based).
		keyStartInTrimmed := strings.Index(trimmed, keyPart)
		// Map trimmed offset back to original line.
		lead := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
		col := lead + keyStartInTrimmed + 1
		if col < 1 {
			col = 1
		}
		if _, exists := out[path]; !exists {
			out[path] = diagnostic.SpecLocation(file, lineNo, col)
		}
	}
	return out
}

// splitDottedKey splits a.b.c or "a.b".c into pieces, unquoting simple keys.
func splitDottedKey(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var parts []string
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		if s[0] == '"' || s[0] == '\'' {
			q := s[0]
			// Find closing quote (no escape handling for basic bare paths).
			end := 1
			for end < len(s) {
				if s[end] == q {
					break
				}
				if s[end] == '\\' && end+1 < len(s) {
					end += 2
					continue
				}
				end++
			}
			if end >= len(s) {
				parts = append(parts, s)
				break
			}
			parts = append(parts, s[1:end])
			s = s[end+1:]
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, ".") {
				s = s[1:]
			}
			continue
		}
		// Bare segment until '.'
		dot := strings.IndexByte(s, '.')
		if dot < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:dot])
		s = s[dot+1:]
	}
	return parts
}

// indexUnquotedEquals finds '=' outside of quotes in a single logical line.
func indexUnquotedEquals(s string) int {
	inQuote := byte(0)
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if inQuote != 0 {
			if c == '\\' && inQuote == '"' {
				escape = true
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			continue
		}
		if c == '=' {
			return i
		}
		if c == '#' {
			return -1
		}
	}
	return -1
}

func columnOf(line string, byteIdx int) int {
	if byteIdx < 0 {
		return 1
	}
	// Column is 1-based rune count up to byteIdx.
	col := utf8.RuneCountInString(line[:byteIdx]) + 1
	if col < 1 {
		return 1
	}
	return col
}

// offsetToLineCol converts a byte offset into 1-based line/column.
func offsetToLineCol(data []byte, offset int) (line, col int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(data) {
		offset = len(data)
	}
	line = 1
	col = 1
	for i := 0; i < offset; i++ {
		if data[i] == '\n' {
			line++
			col = 1
			continue
		}
		// Count runes for column when possible.
		if data[i] < utf8.RuneSelf {
			col++
			continue
		}
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			col++
			continue
		}
		col++
		i += size - 1
	}
	return line, col
}

// snippetAround returns a single-line snippet ≤ max bytes around offset,
// with control characters replaced so logs never dump binary blobs.
func snippetAround(data []byte, offset, max int) string {
	if max <= 0 {
		max = 80
	}
	if len(data) == 0 {
		return ""
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(data) {
		offset = len(data)
	}
	// Expand to line bounds.
	start := offset
	for start > 0 && data[start-1] != '\n' {
		start--
	}
	end := offset
	for end < len(data) && data[end] != '\n' {
		end++
	}
	line := data[start:end]
	if len(line) > max {
		// Prefer window around offset within the line.
		rel := offset - start
		half := max / 2
		a := rel - half
		if a < 0 {
			a = 0
		}
		b := a + max
		if b > len(line) {
			b = len(line)
			a = b - max
			if a < 0 {
				a = 0
			}
		}
		line = line[a:b]
	}
	return sanitizeSnippet(string(line))
}

func sanitizeSnippet(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
