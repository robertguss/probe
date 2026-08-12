package archtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Perf evidence paths (Section 49 / REQ-165 / go-foundry-cli-j8h.1).
const (
	DocPerfDir          = "docs/evidence/perf"
	DocPerfLatestJSON   = "docs/evidence/perf/baselines-latest.json"
	DocPerfSchemaJSON   = "docs/evidence/perf/schema.json"
	DocPerfSummaryMD    = "docs/evidence/perf/summary.md"
	DocPerfP2ExitLinkMD = "docs/evidence/perf/P2.8-citation.md"
)

// PerfBaselineDoc is the stable capture schema (schema_version 1).
// Absolute thresholds stay null until multi-machine history exists.
type PerfBaselineDoc struct {
	SchemaVersion int                        `json:"schema_version"`
	Purpose       string                     `json:"purpose"`
	Bead          string                     `json:"bead"`
	CapturedAtUTC string                     `json:"captured_at_utc"`
	Host          PerfHost                   `json:"host"`
	Config        json.RawMessage            `json:"config"`
	WriteFree     map[string]json.RawMessage `json:"write_free"`
	Generate      PerfGenerate               `json:"generate"`
	Gates         PerfGates                  `json:"gates"`
}

// PerfHost is machine metadata recorded with every capture.
type PerfHost struct {
	Hostname       string `json:"hostname"`
	MachineClass   string `json:"machine_class"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	Kernel         string `json:"kernel"`
	Go             string `json:"go"`
	Git            string `json:"git"`
	FoundryVersion string `json:"foundry_version"`
}

// PerfGenerate holds samples + aggregates for generate timings.
type PerfGenerate struct {
	Samples    []PerfSample               `json:"samples"`
	Aggregates map[string]json.RawMessage `json:"aggregates"`
}

// PerfSample is one generate run.
type PerfSample struct {
	TimestampUTC string         `json:"timestamp_utc"`
	Label        string         `json:"label"`
	SampleID     string         `json:"sample_id"`
	CacheState   PerfCacheState `json:"cache_state"`
	Command      string         `json:"command"`
	ExitCode     int            `json:"exit_code"`
	ElapsedMs    int            `json:"elapsed_ms"`
	Stages       []PerfStage    `json:"stages"`
}

// PerfCacheState labels GOMODCACHE/GOCACHE/parent conditions.
type PerfCacheState struct {
	Gomodcache string `json:"gomodcache"`
	Gocache    string `json:"gocache"`
	Parent     string `json:"parent"`
}

// PerfStage is one progress-event boundary timing.
type PerfStage struct {
	ID        string `json:"id"`
	ElapsedMs int    `json:"elapsed_ms"`
}

// PerfGates records that absolute thresholds are intentionally absent.
type PerfGates struct {
	AbsoluteThresholds json.RawMessage `json:"absolute_thresholds"`
	Note               string          `json:"note"`
}

// PerfBaselineCheck is one schema/format finding.
type PerfBaselineCheck struct {
	Path    string
	Message string
}

// CheckPerfBaselineSchema validates committed Section 49 capture files.
//
// Format stability only — never asserts absolute millisecond gates (REQ-165).
// Requires at least one warm and one cold generate sample in baselines-latest.json.
func CheckPerfBaselineSchema(root string) []PerfBaselineCheck {
	var out []PerfBaselineCheck
	latest := filepath.Join(root, DocPerfLatestJSON)
	schema := filepath.Join(root, DocPerfSchemaJSON)
	summary := filepath.Join(root, DocPerfSummaryMD)
	citation := filepath.Join(root, DocPerfP2ExitLinkMD)

	for _, p := range []string{latest, schema, summary, citation} {
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			out = append(out, PerfBaselineCheck{Path: p, Message: "required evidence file missing"})
		}
	}
	if len(out) > 0 {
		return out
	}

	raw, err := os.ReadFile(latest)
	if err != nil {
		return []PerfBaselineCheck{{Path: latest, Message: err.Error()}}
	}
	var doc PerfBaselineDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return []PerfBaselineCheck{{Path: latest, Message: "json decode: " + err.Error()}}
	}

	if doc.SchemaVersion != 1 {
		out = append(out, PerfBaselineCheck{Path: latest, Message: fmt.Sprintf("schema_version want 1 got %d", doc.SchemaVersion)})
	}
	if doc.Bead == "" {
		out = append(out, PerfBaselineCheck{Path: latest, Message: "bead empty"})
	}
	if doc.CapturedAtUTC == "" {
		out = append(out, PerfBaselineCheck{Path: latest, Message: "captured_at_utc empty"})
	}
	for _, field := range []struct {
		name, val string
	}{
		{"host.hostname", doc.Host.Hostname},
		{"host.machine_class", doc.Host.MachineClass},
		{"host.os", doc.Host.OS},
		{"host.arch", doc.Host.Arch},
		{"host.go", doc.Host.Go},
		{"host.git", doc.Host.Git},
		{"host.foundry_version", doc.Host.FoundryVersion},
	} {
		if strings.TrimSpace(field.val) == "" {
			out = append(out, PerfBaselineCheck{Path: latest, Message: field.name + " empty"})
		}
	}

	// Gates: absolute_thresholds MUST be JSON null (no premature gates).
	if string(doc.Gates.AbsoluteThresholds) != "null" {
		out = append(out, PerfBaselineCheck{
			Path:    latest,
			Message: "gates.absolute_thresholds must be null (no premature absolute gates); got " + string(doc.Gates.AbsoluteThresholds),
		})
	}

	// Write-free commands present.
	for _, cmd := range []string{"version", "catalog_list", "validate", "plan"} {
		if _, ok := doc.WriteFree[cmd]; !ok {
			out = append(out, PerfBaselineCheck{Path: latest, Message: "write_free missing command " + cmd})
		}
	}

	var hasWarm, hasCold bool
	for i, s := range doc.Generate.Samples {
		prefix := fmt.Sprintf("generate.samples[%d]", i)
		if s.TimestampUTC == "" {
			out = append(out, PerfBaselineCheck{Path: latest, Message: prefix + ".timestamp_utc empty"})
		}
		if s.Label == "" {
			out = append(out, PerfBaselineCheck{Path: latest, Message: prefix + ".label empty"})
		}
		if s.Command == "" {
			out = append(out, PerfBaselineCheck{Path: latest, Message: prefix + ".command empty"})
		}
		if s.CacheState.Gomodcache == "" || s.CacheState.Gocache == "" {
			out = append(out, PerfBaselineCheck{Path: latest, Message: prefix + ".cache_state incomplete"})
		}
		if s.Stages == nil {
			out = append(out, PerfBaselineCheck{Path: latest, Message: prefix + ".stages null (want array)"})
		}
		switch s.Label {
		case "warm_host_cache":
			hasWarm = true
		case "cold_gomodcache_cleared", "cold_parent_warm_cache":
			// True cleared-cache OR cold-parent seed both satisfy "cold" for acceptance.
			if s.Label == "cold_gomodcache_cleared" {
				hasCold = true
			}
		}
	}
	// Prefer true cold_gomodcache_cleared; if only cold_parent present, still note.
	if !hasWarm {
		out = append(out, PerfBaselineCheck{Path: latest, Message: "need at least one generate sample with label=warm_host_cache"})
	}
	if !hasCold {
		// Accept cold_parent_warm_cache only when cleared sample absent is a hard fail —
		// acceptance requires a cold capture; prefer cleared. Fall back: any cold_* label.
		for _, s := range doc.Generate.Samples {
			if strings.HasPrefix(s.Label, "cold_") {
				hasCold = true
				break
			}
		}
	}
	if !hasCold {
		out = append(out, PerfBaselineCheck{
			Path:    latest,
			Message: "need at least one cold generate sample (cold_gomodcache_cleared or cold_*)",
		})
	}

	// Summary must disclaim absolute gates and cite P2.8.
	sum, err := os.ReadFile(summary)
	if err != nil {
		out = append(out, PerfBaselineCheck{Path: summary, Message: err.Error()})
	} else {
		text := string(sum)
		if !strings.Contains(text, "None") && !strings.Contains(strings.ToLower(text), "no absolute") {
			out = append(out, PerfBaselineCheck{Path: summary, Message: "summary must disclaim absolute gates"})
		}
		if !strings.Contains(text, "P2.8") && !strings.Contains(text, "5pr") {
			out = append(out, PerfBaselineCheck{Path: summary, Message: "summary should link P2.8 / 5pr exit review"})
		}
	}

	cite, err := os.ReadFile(citation)
	if err != nil {
		out = append(out, PerfBaselineCheck{Path: citation, Message: err.Error()})
	} else {
		text := string(cite)
		if !strings.Contains(text, "go-foundry-cli-5pr") && !strings.Contains(text, "P2.8") {
			out = append(out, PerfBaselineCheck{Path: citation, Message: "P2.8 citation must name 5pr or P2.8"})
		}
		if !strings.Contains(text, "baselines-latest.json") {
			out = append(out, PerfBaselineCheck{Path: citation, Message: "P2.8 citation must point at baselines-latest.json"})
		}
	}

	return out
}
