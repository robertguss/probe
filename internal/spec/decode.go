package spec

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// MaxSpecBytes is the Project Specification size cap (REQ-038): 1 MiB.
const MaxSpecBytes = 1 << 20 // 1048576

// UTF-8 BOM — rejected (strict UTF-8 text, no BOM).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// wireSpec is the TOML decode target. Pointers preserve absence; no defaults.
type wireSpec struct {
	Schema      *int64   `toml:"schema"`
	Name        *string  `toml:"name"`
	Module      *string  `toml:"module"`
	Description *string  `toml:"description"`
	Archetype   *string  `toml:"archetype"`
	Destination *string  `toml:"destination"`
	Binary      *string  `toml:"binary"`
	Visibility  *string  `toml:"visibility"`
	Profiles    []string `toml:"profiles"`
	Git         *wireGit `toml:"git"`
}

type wireGit struct {
	Init          *bool   `toml:"init"`
	InitialBranch *string `toml:"initial_branch"`
}

// Decode strictly decodes a Project Specification from data.
//
// Stages (Section 14.5 partial — parse subset only):
//  1. Size cap (1 MiB)
//  2. Encoding (UTF-8, no BOM)
//  3. TOML syntax + duplicate keys (BurntSushi/toml)
//  4. Unknown fields/tables (MetaData.Undecoded)
//
// No Section 14.3 field business rules or defaults are applied; call Validate
// on the result for ValidatedSpecification (defaults + field contract).
// filename is used only for diagnostic locations (may be a basename or path).
func Decode(filename string, data []byte) (*RawSpecification, error) {
	if filename == "" {
		filename = "foundry.toml"
	}
	byteLen := len(data)

	if byteLen > MaxSpecBytes {
		err := diagnostic.Newf(
			diagnostic.IDSpecTooLarge,
			diagnostic.SpecLocation(filename, 0, 0),
			"specification is %d bytes; maximum is %d (1 MiB)",
			byteLen, MaxSpecBytes,
		)
		logDecodeFailure(byteLen, err, snippetAround(data, 0, 80))
		return nil, err
	}

	if len(data) >= 3 && bytes.Equal(data[:3], utf8BOM) {
		err := diagnostic.New(
			diagnostic.IDSpecInvalidEncoding,
			"specification begins with a UTF-8 BOM; save as UTF-8 without BOM",
			diagnostic.SpecLocation(filename, 1, 1),
		)
		logDecodeFailure(byteLen, err, snippetAround(data, 0, 80))
		return nil, err
	}

	if !utf8.Valid(data) {
		off := firstInvalidUTF8(data)
		line, col := offsetToLineCol(data, off)
		err := diagnostic.Newf(
			diagnostic.IDSpecInvalidEncoding,
			diagnostic.SpecLocation(filename, line, col),
			"specification is not valid UTF-8 (first bad byte at offset %d)",
			off,
		)
		logDecodeFailure(byteLen, err, snippetAround(data, off, 80))
		return nil, err
	}

	positions := indexKeyPositions(filename, data)

	var wire wireSpec
	md, decErr := toml.Decode(string(data), &wire)
	if decErr != nil {
		fe := mapTOMLError(filename, data, decErr)
		logDecodeFailure(byteLen, fe, snippetAround(data, 0, 80))
		return nil, fe
	}

	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		key := undecoded[0]
		path := key.String()
		loc := positions[path]
		if loc.IsZero() {
			// Fall back to parent table or file-only.
			loc = diagnostic.SpecLocation(filename, 0, 0)
			if len(key) > 0 {
				// Try progressive parents.
				for i := len(key); i >= 1; i-- {
					if p, ok := positions[toml.Key(key[:i]).String()]; ok {
						loc = p
						break
					}
				}
			}
		}
		err := diagnostic.Newf(
			diagnostic.IDSpecUnknownField,
			loc,
			"unknown field or table %q (strict decoding; not in Section 14.3)",
			path,
		)
		logDecodeFailure(byteLen, err, snippetAround(data, 0, 80))
		return nil, err
	}

	raw := buildRaw(filename, byteLen, &wire, &md, positions)
	return raw, nil
}

func buildRaw(filename string, byteLen int, wire *wireSpec, md *toml.MetaData, positions map[string]diagnostic.Location) *RawSpecification {
	raw := &RawSpecification{
		File:      filename,
		ByteLen:   byteLen,
		Positions: positions,
	}

	raw.Schema = cloneInt64(wire.Schema)
	raw.Name = cloneString(wire.Name)
	raw.Module = cloneString(wire.Module)
	raw.Description = cloneString(wire.Description)
	raw.Archetype = cloneString(wire.Archetype)
	raw.Destination = cloneString(wire.Destination)
	raw.Binary = cloneString(wire.Binary)
	raw.Visibility = cloneString(wire.Visibility)

	if md.IsDefined("profiles") {
		raw.ProfilesSet = true
		if wire.Profiles == nil {
			raw.Profiles = []string{}
		} else {
			raw.Profiles = append([]string(nil), wire.Profiles...)
		}
	}

	if md.IsDefined("git") {
		raw.GitSet = true
		if wire.Git != nil {
			raw.GitInit = cloneBool(wire.Git.Init)
			raw.GitInitialBranch = cloneString(wire.Git.InitialBranch)
		}
	}

	return raw
}

func mapTOMLError(filename string, data []byte, err error) *diagnostic.FoundryError {
	var pe toml.ParseError
	if errors.As(err, &pe) {
		line := pe.Position.Line
		col := pe.Position.Col
		if line <= 0 {
			line = pe.Line
		}
		if col <= 0 {
			col = 1
		}
		loc := diagnostic.SpecLocation(filename, line, col)
		msg := pe.Message
		if msg == "" {
			msg = pe.Error()
		}
		if isDuplicateKeyMessage(msg, pe.LastKey) {
			key := pe.LastKey
			if key == "" {
				key = extractQuotedKey(msg)
			}
			return diagnostic.Newf(
				diagnostic.IDSpecDuplicateKey,
				loc,
				"duplicate key %q",
				key,
			)
		}
		if isInvalidUTF8Message(msg) {
			return diagnostic.Newf(
				diagnostic.IDSpecInvalidEncoding,
				loc,
				"%s",
				msg,
			)
		}
		return diagnostic.Newf(
			diagnostic.IDSpecParseError,
			loc,
			"%s",
			msg,
		)
	}

	// Type unify and other decode errors often look like:
	// "toml: line N (last key \"x\"): incompatible types: ..."
	line, col, key, msg := parseGenericTOMLError(err.Error())
	loc := diagnostic.SpecLocation(filename, line, col)
	if key != "" {
		if p, ok := indexKeyPositions(filename, data)[key]; ok {
			loc = p
		}
	}
	if msg == "" {
		msg = err.Error()
	}
	return diagnostic.Newf(diagnostic.IDSpecParseError, loc, "%s", msg)
}

func isDuplicateKeyMessage(msg, lastKey string) bool {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "already been defined") ||
		strings.Contains(lower, "already been created") ||
		strings.Contains(lower, "already defined") {
		return true
	}
	_ = lastKey
	return false
}

func isInvalidUTF8Message(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "invalid utf-8") || strings.Contains(lower, "utf-8 byte")
}

func extractQuotedKey(msg string) string {
	// e.g. Key 'a' has already been defined.
	for _, q := range []struct{ a, b string }{
		{"'", "'"},
		{"\"", "\""},
		{"`", "`"},
	} {
		i := strings.Index(msg, q.a)
		if i < 0 {
			continue
		}
		rest := msg[i+len(q.a):]
		j := strings.Index(rest, q.b)
		if j < 0 {
			continue
		}
		return rest[:j]
	}
	return ""
}

func parseGenericTOMLError(s string) (line, col int, key, msg string) {
	// toml: line 1 (last key "schema"): incompatible types: ...
	// toml: line 2: ...
	col = 1
	const prefix = "toml: line "
	if !strings.HasPrefix(s, prefix) {
		return 0, 0, "", s
	}
	rest := s[len(prefix):]
	// line number
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		line = line*10 + int(rest[n]-'0')
		n++
	}
	rest = rest[n:]
	if strings.HasPrefix(rest, " (last key \"") {
		rest = rest[len(" (last key \""):]
		end := strings.Index(rest, "\")")
		if end >= 0 {
			key = rest[:end]
			rest = rest[end+2:]
		}
	}
	if strings.HasPrefix(rest, ": ") {
		msg = rest[2:]
	} else {
		msg = strings.TrimPrefix(rest, ":")
		msg = strings.TrimSpace(msg)
	}
	return line, col, key, msg
}

func firstInvalidUTF8(data []byte) int {
	for i := 0; i < len(data); {
		if data[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return 0
}

func logDecodeFailure(byteLen int, err *diagnostic.FoundryError, snippet string) {
	if err == nil {
		return
	}
	loc := err.Location()
	slog.Debug("spec.decode_failure",
		"byte_length", byteLen,
		"error_id", string(err.ID()),
		"line", loc.Line,
		"column", loc.Column,
		"snippet", truncateRunes(snippet, 80),
	)
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i] + "…"
		}
		n++
	}
	return s
}

func cloneString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneInt64(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// FormatDecodeFailure returns a one-line summary for tests and step logs.
func FormatDecodeFailure(byteLen int, err error) string {
	fe, ok := diagnostic.AsFoundryError(err)
	if !ok {
		return fmt.Sprintf("byte_length=%d error=%v", byteLen, err)
	}
	loc := fe.Location()
	return fmt.Sprintf("byte_length=%d error_id=%s line=%d col=%d",
		byteLen, fe.ID(), loc.Line, loc.Column)
}
