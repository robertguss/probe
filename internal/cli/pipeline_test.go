package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/catalog"
	"github.com/robertguss/go-foundry-cli/internal/cli"
	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/spec"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
	"github.com/robertguss/go-foundry-cli/internal/version"
)

// planPackageOpts matches internal/plan golden conventions so CLI plan_sha256
// equals package-level Construct for the same fixture (single pure path).
func planPackageOpts() cli.Options {
	return cli.Options{
		Version: version.Info{
			Version: plan.DefaultFoundryVersion,
			Commit:  "testcommit000000000000000000000000000001",
			Go:      "go1.26.5",
		},
		PipelineHost:       fixedPipelineHost(),
		ObserveDestination: fixedObserve,
	}
}

func TestValidateExamplesExit0(t *testing.T) {
	log := testutil.New(t)
	log.Phase("validate_examples")

	valid := []string{
		"minimal-cli.toml",
		"minimal-tui.toml",
		"appendix-b-private-cli.toml",
		"appendix-b-private-tui.toml",
		// public-cli + distribution is selectable in Phase 4 (predicates hold).
		"appendix-b-public-cli.toml",
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			sub := testutil.New(t)
			path := examplesPath(t, name)
			sub.Fixture("spec", name)
			res := runCLI(t, "validate", "--spec", path)
			sub.Assert("exit_0", res.Code == 0, 0, res.Code)
			sub.Assert("stdout_ok", strings.Contains(res.Stdout, "validate: ok"), true, res.Stdout)
			sub.Assert("has_plan_sha", strings.Contains(res.Stdout, "plan_sha256="), true, res.Stdout)
			sub.Step("exit", testutil.OutcomeOK, "code=0")
		})
	}
	// Private + distribution → profile_constraint (visibility predicate).
	// Use a synthetic path via plan with private visibility if needed; private
	// appendix examples use profiles=[].
	log.PhaseEnd("validate_examples", testutil.OutcomeOK)
}

func TestPlanJSONSchema1AndGoldens(t *testing.T) {
	log := testutil.New(t)
	log.Phase("plan_json_schema")

	// Use package-aligned opts so plan_sha256 matches internal/plan goldens.
	opts := planPackageOpts()
	cases := []struct {
		name   string
		file   string
		golden string // internal/plan testdata name without .golden
	}{
		{"mvp_minimal_cli", "minimal-cli.toml", "mvp_minimal_cli"},
		{"mvp_minimal_tui", "minimal-tui.toml", "mvp_minimal_tui"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			sub.Fixture("spec", tc.file)
			abs := examplesPath(t, tc.file)
			res := runCLIOpts(t, nil, nil, opts, "plan", "--spec", abs, "--output", "json")
			sub.Assert("exit_0", res.Code == 0, 0, res.Code)
			sub.Assert("stderr_empty", res.Stderr == "", "", res.Stderr)

			env := parseEnvelope(t, res.Stdout)
			sub.Assert("ok", env["ok"] == true, true, env["ok"])
			sub.Assert("command", env["command"] == "plan", "plan", env["command"])
			sub.Assert("envelope_schema", env["schema"] == float64(1), 1, env["schema"])

			result, ok := env["result"].(map[string]any)
			if !ok || result == nil {
				sub.Fail("result", "missing plan result object")
			}
			sub.Assert("plan_schema", result["schema"] == float64(1), 1, result["schema"])
			sha, _ := result["plan_sha256"].(string)
			sub.Assert("sha_len", len(sha) == 64, 64, len(sha))
			sub.Step("plan_sha256", testutil.OutcomeOK, sha)

			proj, _ := result["project"].(map[string]any)
			if proj == nil {
				sub.Fail("project", "missing")
			}
			sub.Assert("project_name", proj["name"] != "", true, proj["name"])

			files, _ := result["files"].([]any)
			sub.Assert("files_ge_1", len(files) >= 1, 1, len(files))

			ver, _ := result["verification"].(map[string]any)
			sub.Assert("verify_default", ver["mode"] == "default", "default", ver["mode"])

			// CLI SpecPath is the flag value (abs); package Pipeline with same path
			// must yield identical plan_sha256 (validate≡plan pure path / goldens).
			wantSHA := packagePlanSHAWithSpecPath(t, abs, abs, plan.VerifyDefault)
			sub.Assert("sha_matches_package", sha == wantSHA, wantSHA, sha)
			sub.Step("golden_path", testutil.OutcomeOK, "package_sha_equality="+tc.golden)
		})
	}
	log.PhaseEnd("plan_json_schema", testutil.OutcomeOK)
}

func TestPlanVerifyDefaultVsStrict(t *testing.T) {
	log := testutil.New(t)
	log.Phase("verify_modes")
	spec := examplesPath(t, "minimal-cli.toml")

	def := runCLI(t, "plan", "--spec", spec, "--verify", "default", "--output", "json")
	strict := runCLI(t, "plan", "--spec", spec, "--verify", "strict", "--output", "json")
	log.Assert("default_0", def.Code == 0, 0, def.Code)
	log.Assert("strict_0", strict.Code == 0, 0, strict.Code)

	defEnv := parseEnvelope(t, def.Stdout)
	strictEnv := parseEnvelope(t, strict.Stdout)
	defPlan := defEnv["result"].(map[string]any)
	strictPlan := strictEnv["result"].(map[string]any)

	defSHA, _ := defPlan["plan_sha256"].(string)
	strictSHA, _ := strictPlan["plan_sha256"].(string)
	log.Assert("sha_differ", defSHA != strictSHA, true, defSHA+"=="+strictSHA)
	log.Step("default_sha", testutil.OutcomeOK, defSHA)
	log.Step("strict_sha", testutil.OutcomeOK, strictSHA)

	// external_steps: strict adds staticcheck + govulncheck
	defSteps := stepIDsFromPlan(defPlan)
	strictSteps := stepIDsFromPlan(strictPlan)
	log.Assert("default_no_staticcheck", !containsStr(defSteps, "go-staticcheck"), false, defSteps)
	log.Assert("strict_has_staticcheck", containsStr(strictSteps, "go-staticcheck"), true, strictSteps)
	log.Assert("strict_has_govulncheck", containsStr(strictSteps, "go-govulncheck"), true, strictSteps)

	defChecks := checksFromPlan(defPlan)
	strictChecks := checksFromPlan(strictPlan)
	log.Assert("strict_check_staticcheck", containsStr(strictChecks, "go-staticcheck"), true, strictChecks)
	log.Assert("default_no_check_staticcheck", !containsStr(defChecks, "go-staticcheck"), false, defChecks)
	log.PhaseEnd("verify_modes", testutil.OutcomeOK)
}

func TestPlanSHA256StableCount2(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")
	spec := examplesPath(t, "minimal-cli.toml")
	var shas []string
	for i := 0; i < 2; i++ {
		res := runCLI(t, "plan", "--spec", spec, "--output", "json")
		log.Assert("exit_"+itoa(i), res.Code == 0, 0, res.Code)
		env := parseEnvelope(t, res.Stdout)
		result := env["result"].(map[string]any)
		sha, _ := result["plan_sha256"].(string)
		shas = append(shas, sha)
		log.Step("plan_sha256_"+itoa(i), testutil.OutcomeOK, sha)
	}
	log.Assert("sha_stable", shas[0] == shas[1], shas[0], shas[1])
	log.Assert("json_byte_equal",
		runCLI(t, "plan", "--spec", spec, "--output", "json").Stdout ==
			runCLI(t, "plan", "--spec", spec, "--output", "json").Stdout,
		true, false)
	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

func TestDestOverrideRecorded(t *testing.T) {
	log := testutil.New(t)
	log.Phase("dest_override")
	spec := examplesPath(t, "minimal-cli.toml")
	// Basename must equal name "minimal-cli".
	dest := "/tmp/foundry-dest-override/minimal-cli"
	res := runCLI(t, "plan", "--spec", spec, "--dest", dest, "--output", "json")
	log.Assert("exit_0", res.Code == 0, 0, res.Code)
	env := parseEnvelope(t, res.Stdout)
	result := env["result"].(map[string]any)
	d, _ := result["destination"].(map[string]any)
	if d == nil {
		log.Fail("destination", "missing")
	}
	// fixedObserve maps abs paths as-is.
	log.Assert("path", d["path"] == dest, dest, d["path"])
	log.Assert("basename", d["basename"] == "minimal-cli", "minimal-cli", d["basename"])
	log.Assert("observation", d["observation"] == "absent", "absent", d["observation"])
	log.Step("dest_path", testutil.OutcomeOK, dest)

	// Bad basename rejected
	res = runCLI(t, "plan", "--spec", spec, "--dest", "/tmp/wrong-name")
	log.Assert("bad_basename_exit", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.Assert("bad_basename_id", strings.Contains(res.Stderr, "spec.invalid_field"), true, res.Stderr)

	// Hostile --dest with ..
	res = runCLI(t, "plan", "--spec", spec, "--dest", "../minimal-cli")
	log.Assert("dotdot_exit", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.PhaseEnd("dest_override", testutil.OutcomeOK)
}

func TestStdinSpecFixture(t *testing.T) {
	log := testutil.New(t)
	log.Phase("stdin_spec")
	body := readExample(t, "minimal-cli.toml")
	log.Inputs(map[string]string{
		"fixture":   "minimal-cli.toml",
		"byte_len":  itoa(len(body)),
		"spec_flag": "-",
	})

	// validate
	res := runCLICtx(t, nil, bytes.NewReader(body), "validate", "--spec", "-")
	log.Assert("validate_0", res.Code == 0, 0, res.Code)
	log.Assert("validate_stdin", strings.Contains(res.Stdout, "stdin") || strings.Contains(res.Stdout, `"spec":"-"`),
		true, res.Stdout)

	// plan JSON
	res = runCLICtx(t, nil, bytes.NewReader(body), "plan", "--spec", "-", "--output", "json")
	log.Assert("plan_0", res.Code == 0, 0, res.Code)
	env := parseEnvelope(t, res.Stdout)
	result := env["result"].(map[string]any)
	specRef, _ := result["specification"].(map[string]any)
	log.Assert("source_stdin", specRef["source"] == "stdin", "stdin", specRef["source"])
	sha, _ := result["plan_sha256"].(string)
	log.Step("plan_sha256", testutil.OutcomeOK, sha)
	log.Assert("sha_len", len(sha) == 64, 64, len(sha))

	// empty stdin → parse/field error (not usage for missing flag)
	res = runCLICtx(t, nil, bytes.NewReader(nil), "validate", "--spec", "-")
	log.Assert("empty_stdin_fail", res.Code != 0, true, res.Code)
	log.PhaseEnd("stdin_spec", testutil.OutcomeOK)
}

func TestValidatePlanSingleCodePath(t *testing.T) {
	log := testutil.New(t)
	log.Phase("single_path")
	spec := examplesPath(t, "minimal-cli.toml")

	// validate success includes plan_sha256 from the same pipeline.
	vRes := runCLI(t, "validate", "--spec", spec, "--output", "json")
	log.Assert("validate_0", vRes.Code == 0, 0, vRes.Code)
	vEnv := parseEnvelope(t, vRes.Stdout)
	vResult := vEnv["result"].(map[string]any)
	vSHA, _ := vResult["plan_sha256"].(string)
	log.Step("validate_plan_sha256", testutil.OutcomeOK, vSHA)

	pRes := runCLI(t, "plan", "--spec", spec, "--output", "json")
	log.Assert("plan_0", pRes.Code == 0, 0, pRes.Code)
	pEnv := parseEnvelope(t, pRes.Stdout)
	pResult := pEnv["result"].(map[string]any)
	pSHA, _ := pResult["plan_sha256"].(string)
	log.Step("plan_plan_sha256", testutil.OutcomeOK, pSHA)

	log.Assert("sha_equal", vSHA == pSHA, vSHA, pSHA)
	log.Assert("sha_nonempty", len(vSHA) == 64, 64, len(vSHA))

	// Same error path: unknown field fails both the same way.
	bad := examplesPath(t, "invalid-unknown-field.toml")
	vBad := runCLI(t, "validate", "--spec", bad)
	pBad := runCLI(t, "plan", "--spec", bad)
	log.Assert("validate_bad_exit", vBad.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, vBad.Code)
	log.Assert("plan_bad_exit", pBad.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, pBad.Code)
	log.Assert("validate_id", strings.Contains(vBad.Stderr, "spec.unknown_field"), true, vBad.Stderr)
	log.Assert("plan_id", strings.Contains(pBad.Stderr, "spec.unknown_field"), true, pBad.Stderr)
	log.PhaseEnd("single_path", testutil.OutcomeOK)
}

func TestAggregatedFieldErrorsAndIDs(t *testing.T) {
	log := testutil.New(t)
	log.Phase("error_ids")

	cases := []struct {
		name   string
		file   string
		wantID string
	}{
		{"unknown_field", "invalid-unknown-field.toml", "spec.unknown_field"},
		{"bad_name", "invalid-bad-name.toml", "spec.invalid_field"},
		{"bad_profile", "invalid-bad-profile.toml", "resolve.unknown_profile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := testutil.New(t)
			sub.Fixture("spec", tc.file)
			path := examplesPath(t, tc.file)
			for _, cmd := range []string{"validate", "plan"} {
				res := runCLI(t, cmd, "--spec", path)
				sub.Assert(cmd+"_exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
				sub.Assert(cmd+"_id", strings.Contains(res.Stderr, tc.wantID), true, res.Stderr)
				sub.Step(cmd+"_error_id", testutil.OutcomeOK, tc.wantID)
			}
			// JSON form
			res := runCLI(t, "validate", "--spec", path, "--output", "json")
			sub.Assert("json_exit", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
			sub.Assert("json_stderr_empty", res.Stderr == "", "", res.Stderr)
			env := parseEnvelope(t, res.Stdout)
			errObj, _ := env["error"].(map[string]any)
			if errObj == nil {
				sub.Fail("error", "missing error object")
			}
			sub.Assert("json_error_id", errObj["error_id"] == tc.wantID, tc.wantID, errObj["error_id"])
			sub.Assert("json_remediation", errObj["remediation"] != nil && errObj["remediation"] != "", true, errObj["remediation"])
		})
	}
	log.PhaseEnd("error_ids", testutil.OutcomeOK)
}

func TestMissingSpecFile(t *testing.T) {
	log := testutil.New(t)
	res := runCLI(t, "validate", "--spec", filepath.Join(t.TempDir(), "no-such-foundry.toml"))
	log.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.Assert("usage_or_not_found",
		strings.Contains(res.Stderr, "usage.invalid") || strings.Contains(res.Stderr, "not found"),
		true, res.Stderr)
}

func TestWriteFreeNoNewFiles(t *testing.T) {
	log := testutil.New(t)
	log.Phase("write_free")
	// Run validate+plan with --dest under a fresh temp parent; ensure no files
	// are created under the temp tree (observation is Lstat-only).
	tmp := t.TempDir()
	before, err := listRel(tmp)
	if err != nil {
		log.Fail("list_before", err.Error())
	}
	spec := examplesPath(t, "minimal-cli.toml")
	dest := filepath.Join(tmp, "minimal-cli")
	// Use real ObserveDestination (not fixed) so we exercise Lstat on tmp.
	opts := testOptions()
	opts.ObserveDestination = nil
	opts.WorkingDir = tmp

	res := runCLIOpts(t, nil, nil, opts, "validate", "--spec", spec, "--dest", dest)
	log.Assert("validate_0", res.Code == 0, 0, res.Code)
	res = runCLIOpts(t, nil, nil, opts, "plan", "--spec", spec, "--dest", dest, "--output", "json")
	log.Assert("plan_0", res.Code == 0, 0, res.Code)

	after, err := listRel(tmp)
	if err != nil {
		log.Fail("list_after", err.Error())
	}
	log.Assert("no_new_files", len(after) == len(before), len(before), len(after))
	// Destination must still not exist (we never mkdir).
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		log.Fail("dest_created", "destination was created: "+dest)
	}
	log.Step("tmp_entries", testutil.OutcomeOK, "n="+itoa(len(after)))
	log.PhaseEnd("write_free", testutil.OutcomeOK)
}

func TestSpecTooLargeStdin(t *testing.T) {
	log := testutil.New(t)
	// Just over 1 MiB of padding.
	header := []byte("schema = 1\n")
	over := make([]byte, spec.MaxSpecBytes+1)
	copy(over, header)
	for i := len(header); i < len(over); i++ {
		over[i] = '#'
	}
	res := runCLICtx(t, nil, bytes.NewReader(over), "validate", "--spec", "-")
	log.Assert("exit_2", res.Code == diagnostic.ExitUsage, diagnostic.ExitUsage, res.Code)
	log.Assert("too_large", strings.Contains(res.Stderr, "spec.too_large"), true, res.Stderr)
	log.Step("stdin_bytes", testutil.OutcomeOK, "n="+itoa(len(over)))
}

// --- helpers ---

func parseEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("json envelope: %v\n%s", err, truncate(raw, 300))
	}
	return env
}

func packagePlanSHAWithSpecPath(t *testing.T, absSpec, specPath string, verify plan.VerifyMode) string {
	t.Helper()
	data, err := os.ReadFile(absSpec)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw, err := spec.Decode(filepath.Base(absSpec), data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	dest, err := fixedObserve(vs.Destination())
	if err != nil {
		t.Fatal(err)
	}
	host := fixedPipelineHost()
	p, err := plan.Pipeline(vs, cat, plan.PipelineOptions{
		FoundryVersion: plan.DefaultFoundryVersion,
		FoundryCommit:  "testcommit000000000000000000000000000001",
		GoVersion:      "go1.26.5",
		SpecSource:     plan.SpecSourcePath,
		SpecPath:       specPath,
		Destination:    dest,
		Verify:         verify,
		GoBinary:       host.GoBinary,
		GitBinary:      host.GitBinary,
		GitTemplateDir: host.GitTemplateDir,
		Host:           host.Host,
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	return p.PlanSHA256()
}

func stepIDsFromPlan(m map[string]any) []string {
	steps, _ := m["external_steps"].([]any)
	var ids []string
	for _, s := range steps {
		sm, _ := s.(map[string]any)
		if sm == nil {
			continue
		}
		if id, ok := sm["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func checksFromPlan(m map[string]any) []string {
	ver, _ := m["verification"].(map[string]any)
	if ver == nil {
		return nil
	}
	raw, _ := ver["checks"].([]any)
	var out []string
	for _, c := range raw {
		if s, ok := c.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func listRel(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel != "." {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
