package archtest

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// productScanRoots are the trees scanned for Section 58 surface reintroduction.
// Only testdata/ directories are allowlisted (skipped).
var productScanRoots = []string{"cmd", "internal"}

// joinFlag assembles a flag spelling so scanner source does not contain the
// contiguous forbidden token (allowlist is testdata only).
func joinFlag(parts ...string) string {
	return strings.Join(parts, "")
}

// forbiddenTokenRules maps assembled surface tokens to red-line IDs.
// Patterns are built without writing the full spelling as one literal or comment.
func forbiddenTokenRules() []struct {
	ID      string
	Pattern string
} {
	return []struct {
		ID      string
		Pattern string
	}{
		{ID: "RL-58-OFFLINE", Pattern: joinFlag("--", "off", "line")},
		{ID: "RL-58-FORCE", Pattern: joinFlag("--", "force")},
		{ID: "RL-58-FORCE", Pattern: joinFlag("--", "over", "write")},
		{ID: "RL-58-VERIFY-BYPASS", Pattern: joinFlag("--", "verify", " ", "none")},
		{ID: "RL-58-VERIFY-BYPASS", Pattern: joinFlag("verify", "=", "none")},
		{ID: "RL-58-DRY-RUN", Pattern: joinFlag("--", "dry", "-", "run")},
		{ID: "RL-58-MISC-STACK", Pattern: joinFlag("self", "-", "update")},
		{ID: "RL-58-MISC-STACK", Pattern: joinFlag("self", "update")},
	}
}

// forbiddenIdentifierRules are product API names that reintroduce rejected design.
// Package-path bans live in ForbiddenRedlinePackageDirs (not here) so this
// file never needs contiguous path spellings for those packages.
func forbiddenIdentifierRules() []struct {
	ID   string
	Name string
} {
	return []struct {
		ID   string
		Name string
	}{
		{"RL-58-PROVENANCE-FILE", joinFlag("Provenance", "Chain")},
		{"RL-58-PROVENANCE-FILE", joinFlag("Provenance", "Closure")},
		{"RL-58-PROFILE-FRAMEWORK", joinFlag("Capability", "DAG")},
		{"RL-58-PROFILE-FRAMEWORK", joinFlag("Capability", "Registry")},
		{"RL-58-STAGE-DELETE", joinFlag("Delete", "Stage")},
		{"RL-58-STAGE-DELETE", joinFlag("Remove", "Stage")},
		{"RL-58-STAGE-DELETE", joinFlag("Clean", "Stage")},
		{"RL-58-STAGE-DELETE", joinFlag("Cleanup", "Stage")},
		{"RL-58-STAGE-DELETE", joinFlag("Destroy", "Stage")},
		{"RL-58-STAGE-DELETE", joinFlag("Auto", "Delete", "Stage")},
	}
}

// pkg joins internal/<name> without a single literal path spelling in source.
func pkg(name string) string {
	return "internal" + "/" + name
}

// forbiddenImportExact are import paths that must not appear in product code.
func forbiddenImportExact() map[string]string {
	return map[string]string{
		"github.com/spf13/" + "viper":        "RL-58-MISC-STACK",
		modulePath + "/" + pkg("compose"):    "RL-58-PROFILE-FRAMEWORK",
		modulePath + "/" + pkg("structured"): "RL-58-TYPED-EMITTERS",
		modulePath + "/" + pkg("provenance"): "RL-58-PROVENANCE-FILE",
		modulePath + "/" + pkg("capability"): "RL-58-PROFILE-FRAMEWORK",
		modulePath + "/" + pkg("greet"):      "RL-58-DEMO",
	}
}

func isDir(p string) (bool, error) {
	st, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return st.IsDir(), nil
}

// walkProductGoFiles visits *.go under cmd/ and internal/, skipping testdata.
func walkProductGoFiles(root string, fn func(absPath, relSlash string) error) error {
	for _, sub := range productScanRoots {
		base := filepath.Join(root, sub)
		if ok, err := isDir(base); err != nil {
			return err
		} else if !ok {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if name == "testdata" {
					return filepath.SkipDir
				}
				if strings.HasPrefix(name, ".") && path != base {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(name, ".go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			return fn(path, filepath.ToSlash(rel))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func scanForbiddenTokens(root string) ([]RedlineViolation, error) {
	rules := forbiddenTokenRules()
	var out []RedlineViolation
	err := walkProductGoFiles(root, func(abs, rel string) error {
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		content := string(data)
		lines := splitLines(content)
		for _, rule := range rules {
			if rule.Pattern == "" {
				continue
			}
			// Find all occurrences with line numbers.
			searchFrom := 0
			for {
				idx := strings.Index(content[searchFrom:], rule.Pattern)
				if idx < 0 {
					break
				}
				absIdx := searchFrom + idx
				line := 1 + strings.Count(content[:absIdx], "\n")
				snip := ""
				if line-1 < len(lines) {
					snip = capSnippet(lines[line-1])
				}
				out = append(out, RedlineViolation{
					ID:      rule.ID,
					File:    rel,
					Line:    line,
					Snippet: snip,
					Detail:  "forbidden surface token " + quotePat(rule.Pattern),
				})
				searchFrom = absIdx + len(rule.Pattern)
			}
		}
		return nil
	})
	return out, err
}

func quotePat(p string) string {
	return `"` + p + `"`
}

func splitLines(s string) []string {
	sc := bufio.NewScanner(strings.NewReader(s))
	// Allow long lines (generated blobs unlikely in product).
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) == 0 && s != "" {
		return strings.Split(s, "\n")
	}
	return lines
}

func scanForbiddenIdentifiers(root string) ([]RedlineViolation, error) {
	rules := forbiddenIdentifierRules()
	var out []RedlineViolation
	err := walkProductGoFiles(root, func(abs, rel string) error {
		// Production stage-delete symbols: non-test files only for stage-delete
		// API surface. Identifier text scan still applies to all product .go
		// (including tests that might reintroduce bad APIs as exported helpers).
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		content := string(data)
		lines := splitLines(content)
		for _, rule := range rules {
			if rule.Name == "" {
				continue
			}
			// Word-boundary-ish: identifier characters around the name.
			searchFrom := 0
			for {
				idx := strings.Index(content[searchFrom:], rule.Name)
				if idx < 0 {
					break
				}
				absIdx := searchFrom + idx
				// Skip if embedded in a larger identifier (letter/digit/_ adjacent).
				if !isIdentBoundary(content, absIdx, absIdx+len(rule.Name)) {
					searchFrom = absIdx + len(rule.Name)
					continue
				}
				// Stage-delete and type-name bans apply to all product .go under
				// cmd/ and internal/ (tests must not define the rejected APIs).
				line := 1 + strings.Count(content[:absIdx], "\n")
				snip := ""
				if line-1 < len(lines) {
					snip = capSnippet(lines[line-1])
				}
				out = append(out, RedlineViolation{
					ID:      rule.ID,
					File:    rel,
					Line:    line,
					Snippet: snip,
					Detail:  "forbidden identifier " + rule.Name,
				})
				searchFrom = absIdx + len(rule.Name)
			}
		}
		return nil
	})
	return out, err
}

func isIdentBoundary(s string, start, end int) bool {
	if start > 0 {
		r := rune(s[start-1])
		// Only ASCII-aware is enough for our banned CamelCase names.
		if isIdentByte(byte(r)) {
			return false
		}
	}
	if end < len(s) {
		if isIdentByte(s[end]) {
			return false
		}
	}
	return true
}

func isIdentByte(b byte) bool {
	return b == '_' || unicode.IsLetter(rune(b)) || unicode.IsDigit(rune(b))
}

func scanForbiddenImports(root string) ([]RedlineViolation, error) {
	forbidden := forbiddenImportExact()
	var out []RedlineViolation
	err := walkProductGoFiles(root, func(abs, rel string) error {
		// Skip test files for import bans? No — tests must not pull viper either
		// into the product module graph via test files in cmd/internal.
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, abs, nil, parser.ImportsOnly)
		if err != nil {
			// Unparseable build-tagged files still usually parse with ImportsOnly.
			return nil
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if id, ok := forbidden[path]; ok {
				pos := fset.Position(imp.Path.Pos())
				out = append(out, RedlineViolation{
					ID:      id,
					File:    rel,
					Line:    pos.Line,
					Snippet: capSnippet(path),
					Detail:  "forbidden import",
				})
			}
		}
		return nil
	})
	return out, err
}

// Build-constraint line matchers for the Windows product surface ban.
// Negative constraints (!windows) are allowed (unix-only files).
// Regexes are assembled so source lines are not themselves constraints.
var (
	goBuildLine   = regexp.MustCompile(`(?m)^` + `//go` + `:build[ \t]+(.+)$`)
	plusBuildLine = regexp.MustCompile(`(?m)^//\s*` + `\+build[ \t]+(.+)$`)
)

func scanWindowsSurface(root string) ([]RedlineViolation, error) {
	var out []RedlineViolation
	err := walkProductGoFiles(root, func(abs, rel string) error {
		base := filepath.Base(abs)
		// *_windows.go product files claim Windows support.
		if strings.HasSuffix(base, "_windows.go") {
			out = append(out, RedlineViolation{
				ID:      "RL-58-WINDOWS",
				File:    rel,
				Line:    1,
				Snippet: base,
				Detail:  "Windows-specific product source file is forbidden (DEC-013)",
			})
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		content := string(data)
		for _, re := range []*regexp.Regexp{goBuildLine, plusBuildLine} {
			matches := re.FindAllStringSubmatchIndex(content, -1)
			for _, m := range matches {
				expr := content[m[2]:m[3]]
				if buildExprSelectsWindows(expr) {
					line := 1 + strings.Count(content[:m[0]], "\n")
					snip := capSnippet(content[m[0]:m[1]])
					out = append(out, RedlineViolation{
						ID:      "RL-58-WINDOWS",
						File:    rel,
						Line:    line,
						Snippet: snip,
						Detail:  "windows build constraint in product package (DEC-013)",
					})
				}
			}
		}
		return nil
	})
	return out, err
}

// buildExprSelectsWindows reports whether a build-constraint expression
// positively requires GOOS=windows (not merely mentions !windows).
func buildExprSelectsWindows(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	// Fast path: expression is exactly the windows OS tag.
	if expr == "windows" {
		return true
	}
	// Tokenize on &|!() space and look for a bare windows OS term that is not negated.
	// Fail (selects windows): "windows", "windows && amd64", "windows || linux"
	// Pass: "!windows", "unix", "linux || darwin", "tools"
	//
	// Policy: any positive windows term means the file is part of a Windows
	// product build surface. OR with windows still compiles on Windows.
	// Only !windows alone or conjunctions that exclude windows are OK.
	return positiveBuildTag(expr, "windows")
}

func positiveBuildTag(expr, tag string) bool {
	// Normalize legacy comma-OR lists (OR of space-AND lists) into modern form.
	// "windows,amd64" means windows OR amd64 — still positive windows.
	// "windows amd64" means windows AND amd64.
	// Modern expressions use || / &&; legacy lines are converted by the caller.
	if strings.Contains(expr, ",") && !strings.Contains(expr, "||") && !strings.Contains(expr, "&&") {
		// Legacy comma-OR form without modern operators.
		parts := strings.Split(expr, ",")
		for _, p := range parts {
			if positiveBuildTagAndList(strings.TrimSpace(p), tag) {
				return true
			}
		}
		return false
	}
	// Split on || at top level (no nested parens handling beyond simple strip).
	orParts := splitTopLevel(expr, "||")
	for _, part := range orParts {
		if positiveBuildTagAndList(strings.TrimSpace(part), tag) {
			return true
		}
	}
	return false
}

func positiveBuildTagAndList(expr, tag string) bool {
	expr = strings.TrimSpace(expr)
	// Strip one layer of parens.
	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	andParts := splitTopLevel(expr, "&&")
	// Also split on whitespace for +build "windows amd64" style already converted,
	// or raw go:build without && (single tag).
	if len(andParts) == 1 && strings.ContainsAny(andParts[0], " \t") &&
		!strings.Contains(andParts[0], "||") {
		fields := strings.Fields(andParts[0])
		andParts = fields
	}
	sawPositive := false
	for _, p := range andParts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "()")
		if p == "" {
			continue
		}
		if p == "!"+tag {
			return false // conjunction excludes tag
		}
		if p == tag {
			sawPositive = true
		}
	}
	return sawPositive
}

func splitTopLevel(expr, sep string) []string {
	// sep is || or && — no nested-paren awareness beyond counting.
	if sep == "" {
		return []string{expr}
	}
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(expr[i:], sep) {
			parts = append(parts, expr[start:i])
			i += len(sep) - 1
			start = i + 1
		}
	}
	parts = append(parts, expr[start:])
	return parts
}
