package archtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Paths relative to repo root for the living REQ→test matrix (DoD #2 / dlv.1).
const (
	DocReqTraceJSONPath = "docs/evidence/req-traceability.json"
	DocReqTraceMDPath   = "docs/evidence/req-traceability.md"
	SpecAuthorityPath   = "docs/02-definitive-foundry-specification-revised-fable-5.md"
)

// Env keys controlling phase-exit / strict path modes.
const (
	EnvMatrixPhaseExit   = "FOUNDRY_MATRIX_PHASE_EXIT"   // P1|P2|P3|P4|S6|DoD
	EnvMatrixStrictPaths = "FOUNDRY_MATRIX_STRICT_PATHS" // 1 = fail verified rows missing on-disk test paths
)

// Allowed matrix status values. "TBD" is explicitly forbidden.
var AllowedMatrixStatuses = map[string]bool{
	"planned":     true,
	"implemented": true,
	"verified":    true,
}

// ReqTraceRow is one matrix entry (machine-readable).
type ReqTraceRow struct {
	ID               string   `json:"id"`
	Phase            string   `json:"phase"`
	OwningBeads      []string `json:"owning_beads"`
	Tests            []string `json:"tests"`
	Status           string   `json:"status"`
	LastUpdate       string   `json:"last_update"`
	Summary          string   `json:"summary,omitempty"`
	SpecVerification string   `json:"spec_verification,omitempty"`
}

// ReqTraceDoc is the seeded matrix document.
type ReqTraceDoc struct {
	Version      int           `json:"version"`
	Authority    string        `json:"authority"`
	Bead         string        `json:"bead"`
	Seeded       string        `json:"seeded"`
	LastUpdate   string        `json:"last_update"`
	StatusEnum   []string      `json:"status_enum"`
	Requirements []ReqTraceRow `json:"requirements"`
}

// ReqTraceReport is the checker outcome (for logs / tests).
type ReqTraceReport struct {
	SpecCount    int
	MatrixCount  int
	Missing      []string // in Section 53, absent from matrix
	Extra        []string // in matrix, not in Section 53
	Duplicates   []string
	BadStatus    []string // id:status
	TBDCells     []string // rows with TBD placeholder
	EmptyBeads   []string
	EmptyTests   []string
	Unverified   []string // for phase-exit: not verified in scoped phase
	MissingPaths []string // verified rows whose test paths do not exist (when strict)
	OrphanClaims []string // optional: test path claims that do not exist (info/warn)
	OK           bool
}

func (r ReqTraceReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "req-traceability: spec=%d matrix=%d ok=%v\n", r.SpecCount, r.MatrixCount, r.OK)
	writeSortedList(&b, "missing (in §53, not in matrix)", r.Missing)
	writeSortedList(&b, "extra (in matrix, not in §53)", r.Extra)
	writeSortedList(&b, "duplicates", r.Duplicates)
	writeSortedList(&b, "bad_status", r.BadStatus)
	writeSortedList(&b, "tbd_cells", r.TBDCells)
	writeSortedList(&b, "empty_owning_beads", r.EmptyBeads)
	writeSortedList(&b, "empty_tests", r.EmptyTests)
	writeSortedList(&b, "unverified (phase-exit scope)", r.Unverified)
	writeSortedList(&b, "verified_missing_paths", r.MissingPaths)
	writeSortedList(&b, "orphan_test_claims (warn)", r.OrphanClaims)
	return b.String()
}

func writeSortedList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	fmt.Fprintf(b, "  %s (%d): %s\n", title, len(sorted), strings.Join(sorted, ", "))
}

var (
	reqIDRE = regexp.MustCompile(`^REQ-\d{3}$`)
	// Section 53 table rows begin with "| REQ-### |". Requirement text may
	// contain "|" (e.g. text\|json, O_CREATE|O_EXCL), so only anchor on the ID cell.
	sec53REQLineRE = regexp.MustCompile(`(?m)^\|\s*(REQ-\d{3})\s*\|`)
)

// LoadSection53REQIDs extracts the ordered set of active REQ ids from the
// normative Section 53 tables in the specification authority document.
func LoadSection53REQIDs(root string) ([]string, error) {
	path := filepath.Join(root, SpecAuthorityPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}
	text := string(data)
	// Bound to §53 … §54 so other mentions do not inflate the set.
	start := strings.Index(text, "## 53. Normative Requirements")
	if start < 0 {
		return nil, fmt.Errorf("section 53 heading not found in %s", SpecAuthorityPath)
	}
	rest := text[start:]
	endRel := strings.Index(rest, "## 54. Requirement Traceability")
	if endRel < 0 {
		return nil, fmt.Errorf("section 54 heading not found after section 53 in %s", SpecAuthorityPath)
	}
	sec := rest[:endRel]

	seen := map[string]bool{}
	var ids []string
	for _, m := range sec53REQLineRE.FindAllStringSubmatch(sec, -1) {
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no REQ rows found in section 53 of %s", SpecAuthorityPath)
	}
	sort.Strings(ids)
	return ids, nil
}

// LoadReqTraceDoc loads the machine-readable matrix.
func LoadReqTraceDoc(root string) (*ReqTraceDoc, error) {
	path := filepath.Join(root, DocReqTraceJSONPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read matrix: %w", err)
	}
	var doc ReqTraceDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse matrix json: %w", err)
	}
	if doc.Version < 1 {
		return nil, fmt.Errorf("matrix version must be >= 1")
	}
	return &doc, nil
}

// CheckReqTraceabilityOptions controls optional phase-exit / path strictness.
type CheckReqTraceabilityOptions struct {
	// PhaseExit is empty (completeness only), or P1|P2|P3|P4|S6|DoD.
	PhaseExit string
	// StrictPaths fails verified rows whose primary test paths are missing on disk.
	// When false, missing paths for verified rows are reported as warnings (OrphanClaims).
	StrictPaths bool
	// WarnOrphans lists non-verified test path claims that do not exist (informational).
	WarnOrphans bool
}

// CheckReqTraceability validates the living matrix against Section 53.
// Completeness (exactly-once mapping) always runs. Phase-exit modes add
// verified-row requirements for the scoped phase (or whole matrix for DoD).
func CheckReqTraceability(root string, opt CheckReqTraceabilityOptions) (ReqTraceReport, error) {
	var rep ReqTraceReport

	specIDs, err := LoadSection53REQIDs(root)
	if err != nil {
		return rep, err
	}
	rep.SpecCount = len(specIDs)
	specSet := map[string]bool{}
	for _, id := range specIDs {
		specSet[id] = true
	}

	doc, err := LoadReqTraceDoc(root)
	if err != nil {
		return rep, err
	}
	rep.MatrixCount = len(doc.Requirements)

	// Markdown twin must exist (human register).
	if _, err := os.Stat(filepath.Join(root, DocReqTraceMDPath)); err != nil {
		return rep, fmt.Errorf("matrix markdown missing: %s: %w", DocReqTraceMDPath, err)
	}

	matrixSet := map[string]int{}
	for i := range doc.Requirements {
		row := &doc.Requirements[i]
		id := strings.TrimSpace(row.ID)
		if !reqIDRE.MatchString(id) {
			rep.Extra = append(rep.Extra, id+" (invalid id)")
			continue
		}
		matrixSet[id]++
		if matrixSet[id] > 1 {
			rep.Duplicates = append(rep.Duplicates, id)
		}

		st := strings.TrimSpace(strings.ToLower(row.Status))
		if strings.EqualFold(row.Status, "TBD") || st == "tbd" {
			rep.TBDCells = append(rep.TBDCells, id)
		}
		if !AllowedMatrixStatuses[st] {
			rep.BadStatus = append(rep.BadStatus, id+":"+row.Status)
		}
		// Also scan beads/tests for literal TBD placeholders.
		for _, b := range row.OwningBeads {
			if isTBDPlaceholder(b) {
				rep.TBDCells = append(rep.TBDCells, id+":bead")
			}
		}
		for _, tp := range row.Tests {
			if isTBDPlaceholder(tp) {
				rep.TBDCells = append(rep.TBDCells, id+":test")
			}
		}
		if len(row.OwningBeads) == 0 {
			rep.EmptyBeads = append(rep.EmptyBeads, id)
		}
		if len(row.Tests) == 0 {
			rep.EmptyTests = append(rep.EmptyTests, id)
		}

		// Path existence for verified (and optional orphan warn for others).
		if st == "verified" || opt.WarnOrphans {
			for _, tpath := range row.Tests {
				tpath = strings.TrimSpace(tpath)
				if tpath == "" || strings.EqualFold(tpath, "TBD") {
					continue
				}
				// Skip non-path claims (docs prose, go.mod is a path though).
				abs := filepath.Join(root, tpath)
				if _, err := os.Stat(abs); err != nil {
					claim := id + ":" + tpath
					if st == "verified" {
						rep.MissingPaths = append(rep.MissingPaths, claim)
					} else if opt.WarnOrphans {
						rep.OrphanClaims = append(rep.OrphanClaims, claim)
					}
				}
			}
		}
	}

	for _, id := range specIDs {
		if matrixSet[id] == 0 {
			rep.Missing = append(rep.Missing, id)
		}
	}
	for id := range matrixSet {
		if !specSet[id] {
			rep.Extra = append(rep.Extra, id)
		}
	}
	sort.Strings(rep.Missing)
	sort.Strings(rep.Extra)
	sort.Strings(rep.Duplicates)
	sort.Strings(rep.BadStatus)
	sort.Strings(rep.TBDCells)
	sort.Strings(rep.EmptyBeads)
	sort.Strings(rep.EmptyTests)
	sort.Strings(rep.MissingPaths)
	sort.Strings(rep.OrphanClaims)

	// Phase-exit: require verified for scope.
	phase := strings.TrimSpace(opt.PhaseExit)
	if phase != "" {
		for _, row := range doc.Requirements {
			st := strings.ToLower(strings.TrimSpace(row.Status))
			inScope := false
			switch phase {
			case "DoD", "dod", "DOD":
				inScope = true
			case "P1", "P2", "P3", "P4", "S6":
				inScope = row.Phase == phase
			default:
				return rep, fmt.Errorf("unknown %s=%q (want P1|P2|P3|P4|S6|DoD)", EnvMatrixPhaseExit, phase)
			}
			if inScope && st != "verified" {
				rep.Unverified = append(rep.Unverified, row.ID+":"+row.Status)
			}
		}
		sort.Strings(rep.Unverified)
	}

	// Completeness failures.
	fail := len(rep.Missing) > 0 ||
		len(rep.Extra) > 0 ||
		len(rep.Duplicates) > 0 ||
		len(rep.BadStatus) > 0 ||
		len(rep.TBDCells) > 0 ||
		len(rep.EmptyBeads) > 0 ||
		len(rep.EmptyTests) > 0 ||
		len(rep.Unverified) > 0

	if opt.StrictPaths && len(rep.MissingPaths) > 0 {
		fail = true
	}
	// Expected active count from the program charter (125).
	if rep.SpecCount != 125 {
		// Soft: still report but treat mismatch vs matrix as already covered by missing/extra.
		// Hard-fail if we did not get 125 from the authority doc — identity drift.
		fail = true
		if !containsString(rep.BadStatus, fmt.Sprintf("spec_count:%d", rep.SpecCount)) {
			rep.BadStatus = append(rep.BadStatus, fmt.Sprintf("spec_count_want_125_got_%d", rep.SpecCount))
		}
	}

	rep.OK = !fail
	return rep, nil
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// PhaseExitFromEnv reads FOUNDRY_MATRIX_PHASE_EXIT.
func PhaseExitFromEnv() string {
	return strings.TrimSpace(os.Getenv(EnvMatrixPhaseExit))
}

// StrictPathsFromEnv is true when FOUNDRY_MATRIX_STRICT_PATHS=1/true, or when
// a phase-exit mode is active (defaults strict for exit reviews).
func StrictPathsFromEnv(phaseExit string) bool {
	v := strings.TrimSpace(os.Getenv(EnvMatrixStrictPaths))
	if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
		return true
	}
	if v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no") {
		return false
	}
	// Default: strict when phase-exit is set.
	return phaseExit != ""
}

// isTBDPlaceholder reports forbidden placeholder cells (exact TBD tokens only).
func isTBDPlaceholder(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	return strings.EqualFold(s, "TBD") || strings.EqualFold(s, "TODO") || s == "?"
}
