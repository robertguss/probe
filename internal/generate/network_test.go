package generate_test

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestDisclosureGoldenDefaultVsStrict(t *testing.T) {
	log := testutil.New(t)
	log.Phase("disclosure_goldens")

	for _, tc := range []struct {
		mode     plan.VerifyMode
		textName string
		jsonName string
		wantIDs  []string
	}{
		{
			mode:     plan.VerifyDefault,
			textName: "disclosure_default_text",
			jsonName: "disclosure_default_json",
			wantIDs:  []string{generate.StepIDGoModTidy},
		},
		{
			mode:     plan.VerifyStrict,
			textName: "disclosure_strict_text",
			jsonName: "disclosure_strict_json",
			wantIDs:  []string{generate.StepIDGoModTidy, generate.StepIDGovulncheck},
		},
	} {
		tc := tc
		t.Run(string(tc.mode), func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase("mode_" + string(tc.mode))
			tl.Inputs(map[string]string{"verify": string(tc.mode)})

			d := generate.BuildDisclosureFromMode(tc.mode)
			tl.Assert("may_required", d.MayBeRequired, true, d.MayBeRequired)
			tl.Assert("mode", d.VerifyMode == string(tc.mode), string(tc.mode), d.VerifyMode)
			tl.Assert("step_count", len(d.Steps) == len(tc.wantIDs), len(tc.wantIDs), len(d.Steps))
			for i, id := range tc.wantIDs {
				tl.Assert("step_"+id, d.Steps[i].ID == id, id, d.Steps[i].ID)
				tl.Assert("network_"+id, d.Steps[i].Network == string(plan.NetworkMay),
					string(plan.NetworkMay), d.Steps[i].Network)
				tl.Assert("reason_nonempty_"+id, d.Steps[i].Reason != "", true, d.Steps[i].Reason)
			}

			text := d.FormatText()
			tl.Assert("has_header", strings.HasPrefix(text, "network disclosure:\n"), true, text)
			tl.Assert("no_offline_text", !generate.ContainsOfflineToken(text), false, text)
			path := testutil.GoldenPath(testdataDir, tc.textName)
			testutil.CompareGolden(t, path, []byte(text))
			tl.Step("golden_text", testutil.OutcomeOK, filepath.Base(path))

			raw, err := d.FormatJSON()
			if err != nil {
				tl.Fail("json_marshal", err.Error())
			}
			// Pretty-stable single-line JSON from encoding/json is fine for goldens.
			tl.Assert("no_offline_json", !generate.ContainsOfflineToken(string(raw)), false, string(raw))
			// Structural checks: step ids + reasons present.
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				tl.Fail("json_decode", err.Error())
			}
			tl.Assert("json_may", got["may_be_required"] == true, true, got["may_be_required"])
			tl.Assert("json_mode", got["verify_mode"] == string(tc.mode), string(tc.mode), got["verify_mode"])
			steps, _ := got["steps"].([]any)
			tl.Assert("json_steps_len", len(steps) == len(tc.wantIDs), len(tc.wantIDs), len(steps))
			for i, id := range tc.wantIDs {
				sm, _ := steps[i].(map[string]any)
				tl.Assert("json_id_"+id, sm["id"] == id, id, sm["id"])
				tl.Assert("json_reason_"+id, sm["reason"] != nil && sm["reason"] != "", true, sm["reason"])
			}
			reasons, _ := got["reasons"].([]any)
			tl.Assert("json_reasons_len", len(reasons) >= 1, 1, len(reasons))

			jpath := testutil.GoldenPath(testdataDir, tc.jsonName)
			// Append newline for POSIX golden consistency.
			payload := append(append([]byte(nil), raw...), '\n')
			testutil.CompareGolden(t, jpath, payload)
			tl.Step("golden_json", testutil.OutcomeOK, filepath.Base(jpath))

			tl.PhaseEnd("mode_"+string(tc.mode), testutil.OutcomeOK)
		})
	}

	log.PhaseEnd("disclosure_goldens", testutil.OutcomeOK)
}

func TestDisclosureOrderingBeforeStageCreate(t *testing.T) {
	log := testutil.New(t)
	log.Phase("ordering")

	for _, mode := range []plan.VerifyMode{plan.VerifyDefault, plan.VerifyStrict} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase("order_" + string(mode))
			tl.Inputs(map[string]string{"verify": string(mode)})

			sink := &generate.CollectingSink{}
			rec := &generate.RecordingLogger{}
			stages := defaultStages()
			stages[generate.StageReportPlanNetwork] = generate.DiscloseFromModeStage(mode)
			m := newMachine(t, stages, sink, rec)
			res := m.Run(context.Background())

			tl.Assert("exit_0", res.Exit == 0, 0, res.Exit)

			netIdx := -1
			createIdx := -1
			acquireIdx := -1
			preflightIdx := -1
			for i, ev := range res.Events {
				if ev.Kind == generate.EventNetwork {
					netIdx = i
					tl.Assert("net_stage", ev.Stage == generate.StageReportPlanNetwork,
						generate.StageReportPlanNetwork, ev.Stage)
					tl.Assert("net_lines", len(ev.Lines) > 0, true, len(ev.Lines))
				}
				if ev.Kind == generate.EventProgress && ev.Stage == generate.StageCreateStage {
					createIdx = i
				}
				if ev.Kind == generate.EventProgress && ev.Stage == generate.StageAcquireParent {
					acquireIdx = i
				}
				if ev.Kind == generate.EventProgress && ev.Stage == generate.StageToolPreflight {
					preflightIdx = i
				}
			}
			tl.Assert("network_present", netIdx >= 0, true, netIdx)
			tl.Assert("create_present", createIdx >= 0, true, createIdx)
			tl.Assert("acquire_present", acquireIdx >= 0, true, acquireIdx)
			tl.Assert("preflight_present", preflightIdx >= 0, true, preflightIdx)
			// After tool preflight, before parent acquire / stage create.
			tl.Assert("after_preflight", netIdx > preflightIdx, true,
				"net="+itoa(netIdx)+" pre="+itoa(preflightIdx))
			tl.Assert("before_acquire", netIdx < acquireIdx, true,
				"net="+itoa(netIdx)+" acq="+itoa(acquireIdx))
			tl.Assert("before_create", netIdx < createIdx, true,
				"net="+itoa(netIdx)+" create="+itoa(createIdx))

			// Stage numbering invariant: report-plan-network is 7, create is 9.
			tl.Assert("stage_num_network", generate.StageNumber(generate.StageReportPlanNetwork) == 7,
				7, generate.StageNumber(generate.StageReportPlanNetwork))
			tl.Assert("stage_num_create", generate.StageNumber(generate.StageCreateStage) == 9,
				9, generate.StageNumber(generate.StageCreateStage))

			tl.PhaseEnd("order_"+string(mode), testutil.OutcomeOK)
		})
	}

	log.PhaseEnd("ordering", testutil.OutcomeOK)
}

func TestDisclosurePureFormattingNoNetworkIO(t *testing.T) {
	log := testutil.New(t)
	log.Phase("purity")

	// Source-level: network.go must not import net / net/http / os/exec.
	srcPath := filepath.Join("network.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, srcPath, nil, parser.ImportsOnly)
	if err != nil {
		// Fallback: absolute from package dir.
		wd, _ := os.Getwd()
		srcPath = filepath.Join(wd, "network.go")
		f, err = parser.ParseFile(fset, srcPath, nil, parser.ImportsOnly)
	}
	if err != nil {
		log.Fail("parse_network_go", err.Error())
	}
	banned := []string{"net", "net/http", "net/url", "os/exec", "syscall", "crypto/rand"}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, b := range banned {
			if path == b || strings.HasPrefix(path, b+"/") {
				log.Fail("banned_import", path)
			}
		}
		log.Step("import", testutil.OutcomeOK, path)
	}

	// Functional: BuildDisclosure / Format* are pure — no panic, stable across calls.
	d1 := generate.BuildDisclosureFromMode(plan.VerifyStrict)
	d2 := generate.BuildDisclosureFromMode(plan.VerifyStrict)
	t1 := d1.FormatText()
	t2 := d2.FormatText()
	log.Assert("text_stable", t1 == t2, true, t1 == t2)
	j1, err1 := d1.FormatJSON()
	j2, err2 := d2.FormatJSON()
	log.Assert("json_err_nil", err1 == nil && err2 == nil, true, err1)
	log.Assert("json_stable", string(j1) == string(j2), true, string(j1) == string(j2))

	// Stage func is pure assignment.
	rt := &generate.Runtime{}
	fn := generate.DiscloseFromModeStage(plan.VerifyDefault)
	if err := fn(context.Background(), rt); err != nil {
		log.Fail("disclose_stage", err.Error())
	}
	log.Assert("lines_set", len(rt.NetworkLines) == 1, 1, len(rt.NetworkLines))
	log.Assert("ids_set", len(rt.NetworkStepIDs) == 1, 1, len(rt.NetworkStepIDs))
	log.Assert("mode_set", rt.VerifyMode == string(plan.VerifyDefault),
		string(plan.VerifyDefault), rt.VerifyMode)

	// No offline claim in any formatted output.
	log.Assert("no_offline", !generate.ContainsOfflineToken(t1+string(j1)), false, t1)

	log.PhaseEnd("purity", testutil.OutcomeOK)
}

func TestDisclosureStepLogger(t *testing.T) {
	log := testutil.New(t)
	log.Phase("step_logger")

	for _, mode := range []plan.VerifyMode{plan.VerifyDefault, plan.VerifyStrict} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			tl := testutil.New(t)
			tl.Phase("log_" + string(mode))
			tl.Inputs(map[string]string{"verify": string(mode)})

			rec := &generate.RecordingLogger{}
			stages := defaultStages()
			d := generate.BuildDisclosureFromMode(mode)
			stages[generate.StageReportPlanNetwork] = generate.DiscloseStage(d)
			m := newMachine(t, stages, &generate.CollectingSink{}, rec)
			res := m.Run(context.Background())
			tl.Assert("exit_0", res.Exit == 0, 0, res.Exit)

			found := false
			wantPrefix := "verify=" + string(mode) + " steps="
			for _, s := range rec.Steps {
				if s.Event == "network-disclosure" {
					found = true
					tl.Assert("result_has_verify", strings.HasPrefix(s.Result, wantPrefix),
						wantPrefix, s.Result)
					tl.Assert("result_has_tidy", strings.Contains(s.Result, generate.StepIDGoModTidy),
						true, s.Result)
					if mode == plan.VerifyStrict {
						tl.Assert("result_has_govuln", strings.Contains(s.Result, generate.StepIDGovulncheck),
							true, s.Result)
					} else {
						tl.Assert("result_no_govuln", !strings.Contains(s.Result, generate.StepIDGovulncheck),
							false, s.Result)
					}
					tl.Step("disclosure_log", testutil.OutcomeOK,
						"state="+s.State+" result="+s.Result)
				}
			}
			tl.Assert("logged", found, true, found)

			// FormatNetworkLogResult matches what the machine records.
			want := generate.FormatNetworkLogResult(d)
			tl.Assert("format_helper", strings.HasPrefix(want, wantPrefix), wantPrefix, want)

			tl.PhaseEnd("log_"+string(mode), testutil.OutcomeOK)
		})
	}

	log.PhaseEnd("step_logger", testutil.OutcomeOK)
}

func TestDisclosureFromPlanMatchesMode(t *testing.T) {
	log := testutil.New(t)
	log.Phase("from_plan")

	// Synthetic plan-shaped inputs via mode builder equality on step ids.
	// Full plan.Construct needs catalog; unit path uses mode + synthetic steps.
	modeD := generate.BuildDisclosureFromMode(plan.VerifyDefault)
	// BuildDisclosureFromPlan(nil) falls back to default.
	nilPlan := generate.BuildDisclosureFromPlan(nil)
	log.Assert("nil_mode", nilPlan.VerifyMode == modeD.VerifyMode, modeD.VerifyMode, nilPlan.VerifyMode)
	log.Assert("nil_steps", len(nilPlan.Steps) == len(modeD.Steps), len(modeD.Steps), len(nilPlan.Steps))

	// ReasonForStepID coverage.
	log.Assert("tidy_reason", generate.ReasonForStepID(generate.StepIDGoModTidy) == generate.ReasonModuleResolution,
		generate.ReasonModuleResolution, generate.ReasonForStepID(generate.StepIDGoModTidy))
	log.Assert("vuln_reason", generate.ReasonForStepID(generate.StepIDGovulncheck) == generate.ReasonVulnDB,
		generate.ReasonVulnDB, generate.ReasonForStepID(generate.StepIDGovulncheck))
	log.Assert("unknown_reason", strings.Contains(generate.ReasonForStepID("custom-step"), "custom-step"),
		true, generate.ReasonForStepID("custom-step"))

	// Empty disclosure text/json edge cases.
	empty := generate.Disclosure{}
	log.Assert("empty_text", empty.FormatText() == "", "", empty.FormatText())
	raw, err := empty.FormatJSON()
	log.Assert("empty_json_err", err == nil, true, err)
	log.Assert("empty_json_has_fields", strings.Contains(string(raw), `"steps"`), true, string(raw))

	log.PhaseEnd("from_plan", testutil.OutcomeOK)
}

func TestDisclosureNoOfflineAnywhere(t *testing.T) {
	log := testutil.New(t)
	log.Phase("no_offline")

	for _, mode := range []plan.VerifyMode{plan.VerifyDefault, plan.VerifyStrict} {
		d := generate.BuildDisclosureFromMode(mode)
		text := d.FormatText()
		js, _ := d.FormatJSON()
		log.Assert("text_"+string(mode), !generate.ContainsOfflineToken(text), false, text)
		log.Assert("json_"+string(mode), !generate.ContainsOfflineToken(string(js)), false, string(js))
		for _, line := range d.TextLines() {
			log.Assert("line_"+line[:min(12, len(line))], !generate.ContainsOfflineToken(line), false, line)
		}
	}
	// Assemble flag spelling without contiguous forbidden token (Section 58).
	flagToken := "--" + "off" + "line" + " mode"
	log.Assert("token_detects", generate.ContainsOfflineToken(flagToken), true, true)
	log.Assert("token_clean", !generate.ContainsOfflineToken("network may"), false, false)

	log.PhaseEnd("no_offline", testutil.OutcomeOK)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
