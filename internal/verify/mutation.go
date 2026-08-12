package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// AllowedTidyPaths are the only relative paths go mod tidy may create or
// modify (Section 35.1 / REQ-150). Any other path change is
// verify.module_mutation.
var AllowedTidyPaths = []string{"go.mod", "go.sum"}

// Pin is one exact module path → version requirement expected after tidy.
type Pin struct {
	Path    string
	Version string
}

// MutationReport is the result of tidy mutation-set validation + pin reparse.
type MutationReport struct {
	// ChangedPaths lists relative paths that differ between pre and post tidy.
	ChangedPaths []string
	// ExtraPaths are changed paths outside AllowedTidyPaths.
	ExtraPaths []string
	// MissingPins are expected pins absent or with wrong version after tidy.
	MissingPins []string
	// ForbiddenDirectives names replace/exclude/retract/workspace hits.
	ForbiddenDirectives []string
	// ParsedOK is true when go.mod reparsed successfully.
	ParsedOK bool
	// ModulePath is the module path from reparsed go.mod.
	ModulePath string
}

// OK reports overall mutation-set + pin success.
func (r MutationReport) OK() bool {
	return r.ParsedOK &&
		len(r.ExtraPaths) == 0 &&
		len(r.MissingPins) == 0 &&
		len(r.ForbiddenDirectives) == 0
}

// ValidateTidyMutationSet compares pre/post inventories: only go.mod/go.sum
// may appear as new, removed, or content-changed paths. Directories that only
// exist as parents of those files are ignored when empty of other changes.
func ValidateTidyMutationSet(pre, post ConformanceBaseline) MutationReport {
	rep := MutationReport{}
	// Path presence + content/mode/type equality.
	all := make(map[string]struct{})
	for k := range pre.Entries {
		all[k] = struct{}{}
	}
	for k := range post.Entries {
		all[k] = struct{}{}
	}
	var changed []string
	for k := range all {
		a, aOK := pre.Entries[k]
		b, bOK := post.Entries[k]
		if !aOK || !bOK {
			changed = append(changed, k)
			continue
		}
		if a.Type != b.Type || a.Mode != b.Mode || a.Size != b.Size || a.Digest != b.Digest {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	rep.ChangedPaths = changed

	allowed := map[string]struct{}{}
	for _, p := range AllowedTidyPaths {
		allowed[p] = struct{}{}
	}
	for _, p := range changed {
		if _, ok := allowed[p]; !ok {
			rep.ExtraPaths = append(rep.ExtraPaths, p)
		}
	}
	sort.Strings(rep.ExtraPaths)
	return rep
}

// ReparsePins parses go.mod at stageRoot and asserts:
//  1. no replace, exclude, retract directives
//  2. no go.work sibling that would imply workspace mode (file presence)
//  3. every expected pin is present with the exact version
//
// Expected pins may be a subset of requires (catalog pins); tidy may add
// transitive requires, which is allowed. Expected pins that are missing or
// wrong-version fail closed.
func ReparsePins(stageRoot string, expected []Pin) MutationReport {
	rep := MutationReport{}
	gomodPath := filepath.Join(stageRoot, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		rep.MissingPins = []string{fmt.Sprintf("go.mod unreadable: %v", err)}
		return rep
	}
	parsed, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		rep.MissingPins = []string{fmt.Sprintf("go.mod parse error: %v", err)}
		return rep
	}
	rep.ParsedOK = true
	if parsed.Module != nil {
		rep.ModulePath = parsed.Module.Mod.Path
	}

	if len(parsed.Replace) > 0 {
		for _, r := range parsed.Replace {
			rep.ForbiddenDirectives = append(rep.ForbiddenDirectives,
				fmt.Sprintf("replace %s", r.Old.Path))
		}
	}
	if len(parsed.Exclude) > 0 {
		for _, e := range parsed.Exclude {
			rep.ForbiddenDirectives = append(rep.ForbiddenDirectives,
				fmt.Sprintf("exclude %s %s", e.Mod.Path, e.Mod.Version))
		}
	}
	if len(parsed.Retract) > 0 {
		rep.ForbiddenDirectives = append(rep.ForbiddenDirectives, "retract")
	}

	// Workspace: go.work beside go.mod is forbidden in staged projects.
	if _, err := os.Stat(filepath.Join(stageRoot, "go.work")); err == nil {
		rep.ForbiddenDirectives = append(rep.ForbiddenDirectives, "workspace(go.work present)")
	}
	// Defense: scan go.mod text for a literal workspace directive (invalid in
	// go.mod but fail closed if tool ever emits it).
	if strings.Contains(string(data), "\nworkspace ") || strings.HasPrefix(string(data), "workspace ") {
		rep.ForbiddenDirectives = append(rep.ForbiddenDirectives, "workspace directive in go.mod")
	}

	// Build require map (path → version). Include all require blocks.
	have := make(map[string]string, len(parsed.Require))
	for _, r := range parsed.Require {
		if r == nil {
			continue
		}
		have[r.Mod.Path] = r.Mod.Version
	}

	for _, pin := range expected {
		path := strings.TrimSpace(pin.Path)
		want := strings.TrimSpace(pin.Version)
		if path == "" {
			continue
		}
		got, ok := have[path]
		if !ok {
			rep.MissingPins = append(rep.MissingPins, path+" (absent)")
			continue
		}
		if got != want {
			rep.MissingPins = append(rep.MissingPins,
				fmt.Sprintf("%s (want %s got %s)", path, want, got))
			continue
		}
		// Exact pin shape: require valid semver (v-prefix).
		if !isExactVersion(want) {
			rep.MissingPins = append(rep.MissingPins,
				fmt.Sprintf("%s version %q is not an exact pin", path, want))
		}
	}
	sort.Strings(rep.MissingPins)
	sort.Strings(rep.ForbiddenDirectives)
	return rep
}

// ValidateMutationAndPins runs mutation-set + pin reparse and returns a
// verify.module_mutation error when either fails.
func ValidateMutationAndPins(pre, post ConformanceBaseline, stageRoot string, expected []Pin) (MutationReport, error) {
	mut := ValidateTidyMutationSet(pre, post)
	pins := ReparsePins(stageRoot, expected)
	// Merge reports.
	rep := mut
	rep.ParsedOK = pins.ParsedOK
	rep.ModulePath = pins.ModulePath
	rep.MissingPins = pins.MissingPins
	rep.ForbiddenDirectives = pins.ForbiddenDirectives

	if rep.OK() {
		return rep, nil
	}
	return rep, moduleMutationError(rep)
}

func moduleMutationError(rep MutationReport) error {
	var parts []string
	if len(rep.ExtraPaths) > 0 {
		parts = append(parts, "extra paths: "+strings.Join(rep.ExtraPaths, ", "))
	}
	if len(rep.MissingPins) > 0 {
		parts = append(parts, "pin failures: "+strings.Join(rep.MissingPins, ", "))
	}
	if len(rep.ForbiddenDirectives) > 0 {
		parts = append(parts, "forbidden: "+strings.Join(rep.ForbiddenDirectives, ", "))
	}
	if !rep.ParsedOK && len(parts) == 0 {
		parts = append(parts, "go.mod reparse failed")
	}
	detail := strings.Join(parts, "; ")
	return diagnostic.Newf(
		diagnostic.IDVerifyModuleMutation,
		diagnostic.StepLocation(CheckModuleMutation),
		"tidy mutation-set / pin reparse failed: %s", detail,
	).WithRemediation(
		"Ensure the generated module's requirements match the pinned catalog versions and that tidy only adjusts go.mod/go.sum. " +
			"Remove unexpected replace/exclude/workspace directives or unplanned files tidy introduced. " +
			"Changed paths: " + strings.Join(rep.ChangedPaths, ", "),
	)
}

func isExactVersion(v string) bool {
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "latest", "master", "main", "head":
		return false
	}
	if strings.ContainsAny(v, " \t") {
		return false
	}
	// Ranges and wildcards.
	if strings.ContainsAny(v, "<>~*") {
		return false
	}
	return semver.IsValid(v)
}
