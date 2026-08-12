package testutil

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// RedactPlanJSONForDiff rewrites destination.path (and parent) to basenames
// so failure dumps never embed absolute host homes (Section 13.3 logging).
// Input must be a plan object (schema 1) or an envelope with result=plan.
// Returns pretty multi-line JSON for line-oriented unified diffs.
func RedactPlanJSONForDiff(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	redactDestFields(v)
	// Prefer result payload when envelope-shaped.
	if m, ok := v.(map[string]any); ok {
		if res, ok := m["result"]; ok && res != nil {
			if rm, ok := res.(map[string]any); ok {
				if _, hasSchema := rm["schema"]; hasSchema {
					v = res
				}
			}
		}
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out) + "\n"
}

func redactDestFields(v any) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	if dest, ok := m["destination"].(map[string]any); ok {
		if p, ok := dest["path"].(string); ok && p != "" {
			dest["path"] = path.Base(strings.ReplaceAll(p, "\\", "/"))
		}
		if p, ok := dest["parent"].(string); ok && p != "" {
			// Parent is the directory containing dest — keep basename only.
			dest["parent"] = path.Base(strings.ReplaceAll(p, "\\", "/"))
		}
	}
	if res, ok := m["result"]; ok {
		redactDestFields(res)
	}
}

// UnifiedDiff returns a compact unified-style line diff of a vs b.
// Context is limited so agent failure dumps stay actionable.
func UnifiedDiff(labelA, a, labelB, b string) string {
	la := strings.Split(strings.ReplaceAll(a, "\r\n", "\n"), "\n")
	lb := strings.Split(strings.ReplaceAll(b, "\r\n", "\n"), "\n")
	// Drop trailing empty from final newline.
	if n := len(la); n > 0 && la[n-1] == "" {
		la = la[:n-1]
	}
	if n := len(lb); n > 0 && lb[n-1] == "" {
		lb = lb[:n-1]
	}

	var bld strings.Builder
	fmt.Fprintf(&bld, "--- %s\n+++ %s\n", labelA, labelB)

	// LCS-free greedy: walk with a simple Myers-ish window (O(n*m) capped).
	const maxLines = 400
	if len(la) > maxLines {
		la = append(la[:maxLines], fmt.Sprintf("… (%d more lines truncated)", len(la)-maxLines))
	}
	if len(lb) > maxLines {
		lb = append(lb[:maxLines], fmt.Sprintf("… (%d more lines truncated)", len(lb)-maxLines))
	}

	i, j := 0, 0
	const look = 32
	for i < len(la) || j < len(lb) {
		if i < len(la) && j < len(lb) && la[i] == lb[j] {
			fmt.Fprintf(&bld, " %s\n", la[i])
			i++
			j++
			continue
		}
		// Find next match within look-ahead.
		mi, mj, ok := nextMatch(la, lb, i, j, look)
		if !ok {
			// Drain rest as delete/add.
			for i < len(la) {
				fmt.Fprintf(&bld, "-%s\n", la[i])
				i++
			}
			for j < len(lb) {
				fmt.Fprintf(&bld, "+%s\n", lb[j])
				j++
			}
			break
		}
		for i < mi {
			fmt.Fprintf(&bld, "-%s\n", la[i])
			i++
		}
		for j < mj {
			fmt.Fprintf(&bld, "+%s\n", lb[j])
			j++
		}
	}
	return bld.String()
}

func nextMatch(a, b []string, i, j, look int) (mi, mj int, ok bool) {
	limA := i + look
	if limA > len(a) {
		limA = len(a)
	}
	limB := j + look
	if limB > len(b) {
		limB = len(b)
	}
	best := look*2 + 1
	mi, mj = -1, -1
	for x := i; x < limA; x++ {
		for y := j; y < limB; y++ {
			if a[x] == b[y] {
				cost := (x - i) + (y - j)
				if cost < best {
					best = cost
					mi, mj = x, y
					ok = true
					if cost == 0 {
						return mi, mj, true
					}
				}
			}
		}
	}
	return mi, mj, ok
}

// AssertPlanJSONEqual fails t with a redacted unified diff when plan JSON
// bodies differ (Section 13.3 defect-class logging).
func AssertPlanJSONEqual(t testingT, name string, want, got []byte) {
	t.Helper()
	if string(want) == string(got) {
		return
	}
	// Also accept equal after redaction (absolute dest path variance).
	rw := RedactPlanJSONForDiff(want)
	rg := RedactPlanJSONForDiff(got)
	if rw == rg {
		return
	}
	diff := UnifiedDiff(name+"/want", rw, name+"/got", rg)
	t.Errorf("%s: plan JSON byte inequality (Section 13.3)\n%s", name, diff)
}

// testingT is the subset of *testing.T used by AssertPlanJSONEqual.
type testingT interface {
	Helper()
	Errorf(format string, args ...any)
}
