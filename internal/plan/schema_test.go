package plan_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// requiredTopLevelKeys are Section 28.2 / Appendix C schema 1 fields that MUST
// appear in every marshaled plan.
var requiredTopLevelKeys = []string{
	"schema",
	"foundry",
	"specification",
	"project",
	"destination",
	"profiles",
	"files",
	"dependencies",
	"tools",
	"external_steps",
	"tool_outputs",
	"verification",
	"network",
	"git",
	"commit_result_model",
	"plan_sha256",
	"warnings",
}

// TestJSONSchema1Structural checks every Section 28.2 field (presence, types).
func TestJSONSchema1Structural(t *testing.T) {
	log := testutil.New(t)
	log.Phase("schema")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	raw := p.JSON()
	m := decodePlanMap(t, raw)

	for _, k := range requiredTopLevelKeys {
		_, ok := m[k]
		log.Assert("has_"+k, ok, true, ok)
	}

	// schema
	log.Assert("schema_num", asFloat(m["schema"]) == 1, 1, m["schema"])

	// foundry
	foundry, ok := m["foundry"].(map[string]any)
	log.Assert("foundry_obj", ok, true, ok)
	if ok {
		for _, k := range []string{"version", "go", "catalog_digest"} {
			_, has := foundry[k]
			log.Assert("foundry_"+k, has, true, has)
		}
	}

	// specification
	specRef, ok := m["specification"].(map[string]any)
	log.Assert("spec_obj", ok, true, ok)
	if ok {
		log.Assert("spec_source", specRef["source"] == "path", "path", specRef["source"])
		log.Assert("spec_path", specRef["path"] != nil && specRef["path"] != "", true, specRef["path"])
	}

	// project
	proj, ok := m["project"].(map[string]any)
	log.Assert("project_obj", ok, true, ok)
	if ok {
		for _, k := range []string{"name", "binary", "module", "description", "archetype", "visibility"} {
			_, has := proj[k]
			log.Assert("project_"+k, has, true, has)
		}
	}

	// destination object
	dest, ok := m["destination"].(map[string]any)
	log.Assert("dest_obj", ok, true, ok)
	if ok {
		for _, k := range []string{"path", "parent", "basename", "observation"} {
			_, has := dest[k]
			log.Assert("dest_"+k, has, true, has)
		}
		log.Assert("dest_obs", dest["observation"] == "absent", "absent", dest["observation"])
	}

	// profiles array
	_, ok = m["profiles"].([]any)
	log.Assert("profiles_arr", ok, true, ok)

	// files
	files, ok := m["files"].([]any)
	log.Assert("files_arr", ok && len(files) > 0, true, ok)
	if ok && len(files) > 0 {
		f0, ok := files[0].(map[string]any)
		log.Assert("file0_obj", ok, true, ok)
		if ok {
			for _, k := range []string{"path", "owner", "mode", "render", "source", "content_sha256"} {
				_, has := f0[k]
				log.Assert("file0_"+k, has, true, has)
			}
		}
	}

	// dependencies
	deps, ok := m["dependencies"].([]any)
	log.Assert("deps_arr", ok, true, ok)
	if ok && len(deps) > 0 {
		d0 := deps[0].(map[string]any)
		for _, k := range []string{"module", "version", "scope", "owner"} {
			_, has := d0[k]
			log.Assert("dep0_"+k, has, true, has)
		}
	}

	// tools
	tools, ok := m["tools"].([]any)
	log.Assert("tools_arr", ok && len(tools) > 0, true, ok)

	// external_steps
	steps, ok := m["external_steps"].([]any)
	log.Assert("steps_arr", ok && len(steps) > 0, true, ok)
	if ok && len(steps) > 0 {
		s0 := steps[0].(map[string]any)
		for _, k := range []string{"id", "binary", "argv", "cwd", "mutates", "network", "timeout_s", "output_cap_bytes", "env"} {
			_, has := s0[k]
			log.Assert("step0_"+k, has, true, has)
		}
		log.Assert("step0_cwd", s0["cwd"] == plan.StageDescriptorCWD, plan.StageDescriptorCWD, s0["cwd"])
	}

	// tool_outputs
	outs, ok := m["tool_outputs"].([]any)
	log.Assert("tool_outputs_arr", ok && len(outs) >= 2, true, ok)

	// verification
	ver, ok := m["verification"].(map[string]any)
	log.Assert("ver_obj", ok, true, ok)
	if ok {
		log.Assert("ver_mode", ver["mode"] == "default", "default", ver["mode"])
		checks, ok := ver["checks"].([]any)
		log.Assert("ver_checks", ok && len(checks) > 0, true, ok)
		if ok && len(checks) > 0 {
			last := checks[len(checks)-1].(string)
			log.Assert("final_conformance", last == "final-conformance", "final-conformance", last)
		}
	}

	// network
	net, ok := m["network"].(map[string]any)
	log.Assert("net_obj", ok, true, ok)
	if ok {
		log.Assert("may_be_required", net["may_be_required"] == true, true, net["may_be_required"])
	}

	// git
	git, ok := m["git"].(map[string]any)
	log.Assert("git_obj", ok, true, ok)
	if ok {
		log.Assert("git_isolated", git["isolated"] == true, true, git["isolated"])
	}

	// commit_result_model + plan_sha256
	log.Assert("commit_model", m["commit_result_model"] == plan.CommitResultModelRef,
		plan.CommitResultModelRef, m["commit_result_model"])
	sha, ok := m["plan_sha256"].(string)
	log.Assert("sha_str", ok && len(sha) == 64, 64, len(sha))

	// warnings array (may be empty)
	_, ok = m["warnings"].([]any)
	log.Assert("warnings_arr", ok, true, ok)

	log.PhaseEnd("schema", testutil.OutcomeOK)
}

// TestBannedFieldsAbsent ensures offline/capability/provenance keys never appear.
func TestBannedFieldsAbsent(t *testing.T) {
	log := testutil.New(t)
	log.Phase("banned")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	raw := string(p.JSON())

	// Walk all JSON keys recursively.
	var m any
	if err := json.Unmarshal(p.JSON(), &m); err != nil {
		log.Fail("unmarshal", err.Error())
	}
	keys := collectKeys(m, nil)
	log.Step("key_count", testutil.OutcomeOK, "n="+itoa(len(keys)))

	for _, banned := range plan.BannedJSONKeys {
		// Key exact match (not substring of values).
		found := false
		for _, k := range keys {
			if k == banned || strings.EqualFold(k, banned) {
				found = true
				break
			}
		}
		log.Assert("no_key_"+banned, !found, false, found)
		// Also ensure the banned token is not a JSON object key substring in raw
		// as `"offline"` form.
		needle := `"` + banned + `"`
		log.Assert("no_raw_"+banned, !strings.Contains(raw, needle), false, strings.Contains(raw, needle))
	}
	log.PhaseEnd("banned", testutil.OutcomeOK)
}

// TestTopLevelKeyOrder locks stable field order in compact JSON via a
// depth-1 key walk (nested "network"/"path"/… keys must not confuse order).
func TestTopLevelKeyOrder(t *testing.T) {
	log := testutil.New(t)
	log.Phase("key_order")
	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	rp := mustResolve(t, vs, cat)
	p := mustConstruct(t, defaultInputs(rp, cat, fixedDestination(vs.Destination(), plan.ObservationAbsent)))
	raw := bytes.TrimSpace(p.JSON())

	gotKeys, err := topLevelJSONKeys(raw)
	if err != nil {
		log.Fail("parse_keys", err.Error())
	}
	log.Step("keys", testutil.OutcomeOK, strings.Join(gotKeys, ","))
	log.Assert("key_count", len(gotKeys) == len(requiredTopLevelKeys),
		len(requiredTopLevelKeys), len(gotKeys))
	for i, want := range requiredTopLevelKeys {
		if i >= len(gotKeys) {
			log.Assert("key_"+want, false, want, "<missing>")
			continue
		}
		log.Assert("key_"+want, gotKeys[i] == want, want, gotKeys[i])
	}
	log.PhaseEnd("key_order", testutil.OutcomeOK)
}

// topLevelJSONKeys returns object keys at depth 1 in encounter order.
func topLevelJSONKeys(raw []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return nil, errNotObject
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errNotObject
		}
		keys = append(keys, key)
		// Skip the value (object/array/primitive).
		if err := skipJSONValue(dec); err != nil {
			return nil, err
		}
	}
	// consume closing }
	_, _ = dec.Token()
	return keys, nil
}

var errNotObject = errString("expected JSON object")

type errString string

func (e errString) Error() string { return string(e) }

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); ok {
		switch d {
		case '{', '[':
			for dec.More() {
				if d == '{' {
					// key
					if _, err := dec.Token(); err != nil {
						return err
					}
				}
				if err := skipJSONValue(dec); err != nil {
					return err
				}
			}
			_, err = dec.Token() // closing delim
			return err
		}
	}
	return nil
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return -1
	}
}

func collectKeys(v any, acc []string) []string {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			acc = append(acc, k)
			acc = collectKeys(child, acc)
		}
	case []any:
		for _, child := range x {
			acc = collectKeys(child, acc)
		}
	}
	return acc
}
