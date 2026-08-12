package archtest

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// RedlineRemediation is printed on every red-line violation (never silent skip).
const RedlineRemediation = "rejected by Section 58; see docs/evidence/section-58-redlines.md"

// Redline is one Section 58 (or closely related §13.7) rejected-work entry.
// The table is the mechanical source of truth consumed by P1.8 (4hi).
// Extend this table and scanners in-place — do not open a second red-line system.
type Redline struct {
	// ID is the stable red-line identifier (e.g. RL-58-OFFLINE).
	ID string
	// ShortName is a human label for logs and the evidence doc.
	ShortName string
	// Why is the rejection rationale (spec summary).
	Why string
	// Detection describes how the suite finds reintroduction.
	Detection string
	// DocPath is the living register document.
	DocPath string
}

// DocRedlinesPath is the living Section 58 register relative to repo root.
const DocRedlinesPath = "docs/evidence/section-58-redlines.md"

// Redlines is the living mechanical register. Keep in sync with
// docs/evidence/section-58-redlines.md.
// Register copy avoids contiguous forbidden CLI spellings in this file so the
// product token scanner (testdata allowlist only) does not self-match.
var Redlines = []Redline{
	{
		ID:        "RL-58-VERIFY-BYPASS",
		ShortName: "verification bypass / verify mode none",
		Why:       "Undermines the defining quality promise (Section 58)",
		Detection: "token scan cmd/+internal/ for verify-none spellings; flag registry when cli lands",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-OFFLINE",
		ShortName: "network-isolation mode flag",
		Why:       "Unenforceable whole-process claim (FND-007, Section 58)",
		Detection: "token scan for network-isolation flag spelling in product surface",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-FORCE",
		ShortName: "destination replacement flags",
		Why:       "Destination replacement prohibited (Section 13.7, REQ-030)",
		Detection: "token scan for destination-replacement flags in cmd/+internal/",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-EXISTING-MOD",
		ShortName: "existing-project modification",
		Why:       "New projects only (DEC-002/DEC-003, Section 58)",
		Detection: "command-token seed; full six-command allowlist when cli lands",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-PLUGINS",
		ShortName: "plugins / remote catalogs / templates",
		Why:       "No extension platform (DEC-014, REQ-012, Section 58)",
		Detection: "forbidden packages + plugin/template command tokens",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-PROFILE-FRAMEWORK",
		ShortName: "profile composition framework",
		Why:       "Transitive requires/conflicts/capability graph/provenance closure (FND-009)",
		Detection: "forbidden compose package + capability-graph identifiers",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-TYPED-EMITTERS",
		ShortName: "typed workflow emitters",
		Why:       "FND-015; recipes replace emitters (Section 58)",
		Detection: "forbidden structured package directory",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-DEMO",
		ShortName: "generated demo features",
		Why:       "FND-014; core has no demos (Section 58, Appendix A)",
		Detection: "forbidden greet package path and demo seeds",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-PROVENANCE-FILE",
		ShortName: "persistent provenance files",
		Why:       "Plan is the provenance record (REQ-161, Section 58)",
		Detection: "forbidden provenance package/type names in product code",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-WINDOWS",
		ShortName: "Windows support",
		Why:       "DEC-013; macOS+Linux only (Section 58)",
		Detection: "no windows build tags or windows-suffixed go files under cmd/ or internal/",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-CLAUDE",
		ShortName: "Claude-specific conventions",
		Why:       "DEC-015; agent-neutral AGENTS.md only (Section 58)",
		Detection: "seed; expand when render lands",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-MISC-STACK",
		ShortName: "viper / secrets / sqlite-default / auto-upgrade / telemetry",
		Why:       "Explicit Section 58 rejected stack choices",
		Detection: "forbidden imports (viper) + auto-upgrade tokens",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-STAGE-DELETE",
		ShortName: "automatic stage deletion",
		Why:       "REQ-130/184; stage always preserved (Section 31.6)",
		Detection: "no stage-delete API symbols in production packages; expand with fsx",
		DocPath:   DocRedlinesPath,
	},
	{
		ID:        "RL-58-DRY-RUN",
		ShortName: "separate dry run flag",
		Why:       "plan is the authoritative dry run (Section 13.7)",
		Detection: "token scan for dry run flag spelling in CLI surface",
		DocPath:   DocRedlinesPath,
	},
}

// RedlineByID returns the register entry or false.
func RedlineByID(id string) (Redline, bool) {
	for _, r := range Redlines {
		if r.ID == id {
			return r, true
		}
	}
	return Redline{}, false
}

// RedlineIDs returns sorted red-line identifiers (determinism for -count=2).
func RedlineIDs() []string {
	ids := make([]string, len(Redlines))
	for i, r := range Redlines {
		ids[i] = r.ID
	}
	sort.Strings(ids)
	return ids
}

// RedlineViolation is one mechanical red-line hit.
type RedlineViolation struct {
	ID      string
	File    string
	Line    int
	Snippet string
	Detail  string
}

func (v RedlineViolation) String() string {
	loc := v.File
	if v.Line > 0 {
		loc = fmt.Sprintf("%s:%d", v.File, v.Line)
	}
	snip := v.Snippet
	if snip != "" {
		return fmt.Sprintf("%s: %s snippet=%q detail=%s; %s",
			v.ID, loc, snip, v.Detail, RedlineRemediation)
	}
	return fmt.Sprintf("%s: %s detail=%s; %s", v.ID, loc, v.Detail, RedlineRemediation)
}

// CheckRedlines runs all Section 58 mechanical negative assertions.
func CheckRedlines(root string) ([]RedlineViolation, error) {
	var all []RedlineViolation

	all = append(all, checkForbiddenRedlineDirs(root)...)

	tokenHits, err := scanForbiddenTokens(root)
	if err != nil {
		return all, err
	}
	all = append(all, tokenHits...)

	identHits, err := scanForbiddenIdentifiers(root)
	if err != nil {
		return all, err
	}
	all = append(all, identHits...)

	importHits, err := scanForbiddenImports(root)
	if err != nil {
		return all, err
	}
	all = append(all, importHits...)

	winHits, err := scanWindowsSurface(root)
	if err != nil {
		return all, err
	}
	all = append(all, winHits...)

	// Stable order for determinism.
	sort.Slice(all, func(i, j int) bool {
		if all[i].ID != all[j].ID {
			return all[i].ID < all[j].ID
		}
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all, nil
}

// ForbiddenRedlinePackageDirs maps relative package dirs to red-line IDs.
// Superset of ForbiddenPackageDirs with IDs for failure naming.
// Keys are assembled via pkg() so source has no contiguous banned path for greet.
var ForbiddenRedlinePackageDirs = map[string]string{
	pkg("compose"):    "RL-58-PROFILE-FRAMEWORK",
	pkg("structured"): "RL-58-TYPED-EMITTERS",
	pkg("provenance"): "RL-58-PROVENANCE-FILE",
	pkg("capability"): "RL-58-PROFILE-FRAMEWORK",
	pkg("greet"):      "RL-58-DEMO",
}

func checkForbiddenRedlineDirs(root string) []RedlineViolation {
	// Deterministic key order.
	keys := make([]string, 0, len(ForbiddenRedlinePackageDirs))
	for k := range ForbiddenRedlinePackageDirs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []RedlineViolation
	for _, rel := range keys {
		id := ForbiddenRedlinePackageDirs[rel]
		p := filepath.Join(root, filepath.FromSlash(rel))
		if st, err := statDir(p); err == nil && st {
			out = append(out, RedlineViolation{
				ID:      id,
				File:    rel,
				Detail:  "forbidden package directory exists (REQ-180 / Section 58)",
				Snippet: rel,
			})
		}
	}
	return out
}

func statDir(p string) (bool, error) {
	// thin wrapper so tests can reason about path join without importing os in call sites.
	return isDir(p)
}

// snippetCap is the max rune length of match snippets in violation logs.
const snippetCap = 80

func capSnippet(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\t", " ")
	if strings.Contains(s, "\n") {
		s = strings.SplitN(s, "\n", 2)[0]
	}
	r := []rune(s)
	if len(r) > snippetCap {
		return string(r[:snippetCap]) + "…"
	}
	return s
}
