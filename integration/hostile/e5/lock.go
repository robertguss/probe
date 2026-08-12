package e5

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ExpectedSection12 is the human-readable Section 12 pin table that the lock
// MUST cover. IDs match catalog/versions.toml entry ids.
var ExpectedSection12 = []struct {
	ID      string
	Kind    string // module | tool | toolchain
	Path    string // module path or tool name
	Version string
}{
	{ID: "toolchain_go", Kind: "toolchain", Path: "go", Version: "1.26.5"},
	{ID: "cobra", Kind: "module", Path: "github.com/spf13/cobra", Version: "v1.10.2"},
	{ID: "toml", Kind: "module", Path: "github.com/BurntSushi/toml", Version: "v1.6.0"},
	{ID: "x_mod", Kind: "module", Path: "golang.org/x/mod", Version: "v0.38.0"},
	{ID: "x_sys", Kind: "module", Path: "golang.org/x/sys", Version: "v0.47.0"},
	{ID: "go_cmp", Kind: "module", Path: "github.com/google/go-cmp", Version: "v0.7.0"},
	{ID: "testscript", Kind: "module", Path: "github.com/rogpeppe/go-internal", Version: "v1.15.0"},
	{ID: "bubbletea", Kind: "module", Path: "charm.land/bubbletea/v2", Version: "v2.0.8"},
	{ID: "bubbles", Kind: "module", Path: "charm.land/bubbles/v2", Version: "v2.1.1"},
	{ID: "lipgloss", Kind: "module", Path: "charm.land/lipgloss/v2", Version: "v2.0.5"},
	{ID: "staticcheck", Kind: "tool", Path: "honnef.co/go/tools", Version: "v0.7.0"},
	{ID: "govulncheck", Kind: "tool", Path: "golang.org/x/vuln", Version: "v1.6.0"},
	{ID: "goreleaser", Kind: "tool", Path: "goreleaser", Version: "v2.17.1"},
	{ID: "syft", Kind: "tool", Path: "syft", Version: "v1.44.0"},
}

// RequiredActionIDs are the GitHub Actions that Foundry + generated workflows
// are expected to pin via full SHA in the lock (Sections 16.6, 20, 47).
var RequiredActionIDs = []string{
	"checkout",
	"setup_go",
	"upload_artifact",
	"download_artifact",
	"dependency_review",
	"attest_build_provenance",
	"goreleaser_action",
	"sbom_action",
}

var (
	reSHA40    = regexp.MustCompile(`(?m)^\s*sha\s*=\s*"([0-9a-f]{40})"`)
	reID       = regexp.MustCompile(`(?m)^\s*id\s*=\s*"([^"]+)"`)
	rePath     = regexp.MustCompile(`(?m)^\s*path\s*=\s*"([^"]+)"`)
	reVersion  = regexp.MustCompile(`(?m)^\s*version\s*=\s*"([^"]+)"`)
	reModule   = regexp.MustCompile(`(?m)^\s*module\s*=\s*"([^"]+)"`)
	reUses     = regexp.MustCompile(`(?m)^\s*uses\s*=\s*"([^"]+)"`)
	reTag      = regexp.MustCompile(`(?m)^\s*tag\s*=\s*"([^"]+)"`)
	reGo       = regexp.MustCompile(`(?m)^\s*go\s*=\s*"([^"]+)"`)
	reSchema   = regexp.MustCompile(`(?m)^\s*schema\s*=\s*(\d+)`)
	reTableHdr = regexp.MustCompile(`(?m)^\[\[(modules|tools|actions)\]\]\s*$`)
)

// FindVersionsTOML walks up from this source file (or cwd) to locate
// catalog/versions.toml at the repository root.
func FindVersionsTOML() (string, error) {
	// Prefer relative to this file so tests work from any cwd.
	_, thisFile, _, ok := runtime.Caller(0)
	candidates := []string{}
	if ok {
		dir := filepath.Dir(thisFile)
		// integration/hostile/e5 -> repo root is ../../..
		candidates = append(candidates,
			filepath.Join(dir, "..", "..", "..", "catalog", "versions.toml"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "catalog", "versions.toml"),
			filepath.Join(wd, "..", "catalog", "versions.toml"),
			filepath.Join(wd, "..", "..", "catalog", "versions.toml"),
			filepath.Join(wd, "..", "..", "..", "catalog", "versions.toml"),
		)
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err == nil && st.Mode().IsRegular() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("catalog/versions.toml not found from e5 package or cwd")
}

// LockFile is a minimal parse of catalog/versions.toml for spike validation.
type LockFile struct {
	Path    string
	Raw     string
	Schema  string
	Go      string
	Modules []LockEntry
	Tools   []LockEntry
	Actions []LockEntry
}

// LockEntry is one [[modules]] / [[tools]] / [[actions]] table.
type LockEntry struct {
	ID      string
	Path    string // module path or tool module
	Version string
	Module  string // tools.module
	Uses    string // actions.uses
	Tag     string
	SHA     string
}

// ParseLock parses versions.toml with line-oriented extraction (no TOML lib).
// Good enough for lock integrity; catalog package will use BurntSushi/toml.
func ParseLock(path string) (*LockFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := string(b)
	lf := &LockFile{Path: path, Raw: raw}

	if m := reSchema.FindStringSubmatch(raw); m != nil {
		lf.Schema = m[1]
	}
	if m := reGo.FindStringSubmatch(raw); m != nil {
		lf.Go = m[1]
	}

	// Split into table sections by [[kind]] headers.
	type section struct {
		kind string
		body string
	}
	var sections []section
	idxs := reTableHdr.FindAllStringSubmatchIndex(raw, -1)
	for i, loc := range idxs {
		kind := raw[loc[2]:loc[3]]
		start := loc[1]
		end := len(raw)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		sections = append(sections, section{kind: kind, body: raw[start:end]})
	}

	for _, sec := range sections {
		e := LockEntry{}
		if m := reID.FindStringSubmatch(sec.body); m != nil {
			e.ID = m[1]
		}
		if m := rePath.FindStringSubmatch(sec.body); m != nil {
			e.Path = m[1]
		}
		if m := reVersion.FindStringSubmatch(sec.body); m != nil {
			e.Version = m[1]
		}
		if m := reModule.FindStringSubmatch(sec.body); m != nil {
			e.Module = m[1]
		}
		if m := reUses.FindStringSubmatch(sec.body); m != nil {
			e.Uses = m[1]
		}
		if m := reTag.FindStringSubmatch(sec.body); m != nil {
			e.Tag = m[1]
		}
		if m := reSHA40.FindStringSubmatch(sec.body); m != nil {
			e.SHA = m[1]
		}
		switch sec.kind {
		case "modules":
			lf.Modules = append(lf.Modules, e)
		case "tools":
			lf.Tools = append(lf.Tools, e)
		case "actions":
			lf.Actions = append(lf.Actions, e)
		}
	}
	return lf, nil
}

// ValidateSection12 returns issues if the lock does not cover Section 12 pins.
func ValidateSection12(lf *LockFile) []string {
	var issues []string
	if lf.Schema != "1" {
		issues = append(issues, fmt.Sprintf("schema want 1 got %q", lf.Schema))
	}
	if lf.Go != "1.26.5" {
		issues = append(issues, fmt.Sprintf("toolchain.go want 1.26.5 got %q", lf.Go))
	}

	modByID := map[string]LockEntry{}
	for _, m := range lf.Modules {
		modByID[m.ID] = m
	}
	toolByID := map[string]LockEntry{}
	for _, t := range lf.Tools {
		toolByID[t.ID] = t
	}

	for _, exp := range ExpectedSection12 {
		switch exp.Kind {
		case "toolchain":
			// already checked lf.Go
		case "module":
			m, ok := modByID[exp.ID]
			if !ok {
				issues = append(issues, fmt.Sprintf("missing module id=%s", exp.ID))
				continue
			}
			if m.Path != exp.Path {
				issues = append(issues, fmt.Sprintf("module %s path want %s got %s", exp.ID, exp.Path, m.Path))
			}
			if m.Version != exp.Version {
				issues = append(issues, fmt.Sprintf("module %s version want %s got %s", exp.ID, exp.Version, m.Version))
			}
		case "tool":
			t, ok := toolByID[exp.ID]
			if !ok {
				issues = append(issues, fmt.Sprintf("missing tool id=%s", exp.ID))
				continue
			}
			if t.Version != exp.Version {
				issues = append(issues, fmt.Sprintf("tool %s version want %s got %s", exp.ID, exp.Version, t.Version))
			}
			// staticcheck/govulncheck must record module path
			if exp.Path != "" && (t.Module != "" || t.Path != "") {
				got := t.Module
				if got == "" {
					got = t.Path
				}
				// goreleaser/syft are not Go module pins in the same way
				if exp.ID == "staticcheck" || exp.ID == "govulncheck" {
					if got != exp.Path {
						issues = append(issues, fmt.Sprintf("tool %s module want %s got %s", exp.ID, exp.Path, got))
					}
				}
			}
		}
	}
	return issues
}

// ValidateActions returns issues if required actions lack full SHA + tag.
func ValidateActions(lf *LockFile) []string {
	var issues []string
	byID := map[string]LockEntry{}
	for _, a := range lf.Actions {
		byID[a.ID] = a
		if a.Uses == "" {
			issues = append(issues, fmt.Sprintf("action %s missing uses", a.ID))
		}
		if a.Tag == "" {
			issues = append(issues, fmt.Sprintf("action %s missing tag", a.ID))
		}
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(a.SHA) {
			issues = append(issues, fmt.Sprintf("action %s sha not 40-hex: %q", a.ID, a.SHA))
		}
		// rendered form must never be floating
		if strings.Contains(a.Tag, "latest") {
			issues = append(issues, fmt.Sprintf("action %s floating tag %q", a.ID, a.Tag))
		}
	}
	for _, id := range RequiredActionIDs {
		if _, ok := byID[id]; !ok {
			issues = append(issues, fmt.Sprintf("missing required action id=%s", id))
		}
	}
	return issues
}

// ValidateUniqueIDs ensures lock entry ids are unique across the file.
func ValidateUniqueIDs(lf *LockFile) []string {
	seen := map[string]string{}
	var issues []string
	check := func(kind string, e LockEntry) {
		if e.ID == "" {
			issues = append(issues, fmt.Sprintf("%s entry missing id", kind))
			return
		}
		key := e.ID
		if prev, ok := seen[key]; ok {
			issues = append(issues, fmt.Sprintf("duplicate id %q (%s and %s)", key, prev, kind))
		}
		seen[key] = kind
	}
	for _, e := range lf.Modules {
		check("modules", e)
	}
	for _, e := range lf.Tools {
		check("tools", e)
	}
	for _, e := range lf.Actions {
		check("actions", e)
	}
	return issues
}
