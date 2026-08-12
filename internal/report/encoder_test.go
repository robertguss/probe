package report_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

func TestQuietMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("quiet_matrix")

	// Progress + summary suppressed; network disclosure + errors + stage_path remain.
	opts := report.Options{Mode: report.ModeText, Quiet: true, Color: report.ColorNever}
	enc, cap := newEnc(t, opts)

	log.Step("progress", testutil.OutcomeStart, "quiet")
	if err := enc.Progress(report.StageCreateStage); err != nil {
		log.Fail("progress", err.Error())
	}
	log.Assert("progress_absent", cap.Stdout.String() == "", "", cap.Stdout.String())

	log.Step("summary", testutil.OutcomeStart, "quiet")
	if err := enc.Summary("plan summary: files=3"); err != nil {
		log.Fail("summary", err.Error())
	}
	log.Assert("summary_absent", cap.Stdout.String() == "", "", cap.Stdout.String())

	log.Step("network", testutil.OutcomeStart, "always")
	if err := enc.NetworkDisclosure([]string{
		"go mod tidy may contact the module proxy when caches are cold",
	}); err != nil {
		log.Fail("network", err.Error())
	}
	out := cap.Stdout.String()
	log.Assert("network_present", strings.Contains(out, "network disclosure:"), true, out)
	log.Assert("network_line", strings.Contains(out, "go mod tidy"), true, out)

	log.Step("error_stage", testutil.OutcomeStart, "always")
	err := diagnostic.New(
		diagnostic.IDRenderFailed,
		"template render failed",
		diagnostic.PathLocation("cmd/app/main.go"),
	)
	stage := "/tmp/parent/.foundry-demo-xyz"
	code, werr := enc.Failure("generate", err, stage)
	log.Assert("write_nil", werr == nil, true, werr)
	log.Assert("exit_1", code == diagnostic.ExitFailure, diagnostic.ExitFailure, code)
	stderr := cap.Stderr.String()
	log.Assert("error_id", strings.Contains(stderr, "render.failed"), true, stderr)
	log.Assert("remediation", strings.Contains(stderr, "remediation:"), true, stderr)
	log.Assert("stage_path", strings.Contains(stderr, "stage_path: "+stage), true, stderr)
	log.Assert("stage_not_on_stdout_only", strings.Contains(stderr, stage), true, stderr)

	log.PhaseEnd("quiet_matrix", testutil.OutcomeOK)
}

func TestProgressStreamStepLog(t *testing.T) {
	log := testutil.New(t)
	log.Phase("multi_event_stream")

	enc, cap := newEnc(t, textOpts())
	stages := report.AllStageIDs()
	for i, id := range stages {
		log.Step(string(id), testutil.OutcomeStart, "idx="+itoa(i+1))
		if err := enc.Progress(id); err != nil {
			log.Fail("progress_"+string(id), err.Error())
		}
		log.Step(string(id), testutil.OutcomeOK, report.ProgressLine(id))
	}

	// Network disclosure mid-stream
	ev := report.GenerationEvent{
		Kind:  "network",
		Lines: []string{"govulncheck may fetch vulnerability data (strict verify)"},
	}
	log.Step("network_event", testutil.OutcomeStart, "")
	if err := enc.ApplyEvent(ev); err != nil {
		log.Fail("network", err.Error())
	}
	log.Step("network_event", testutil.OutcomeOK, "")

	// Fake progress events via ApplyEvent
	for _, id := range []report.StageID{report.StageRender, report.StageCommit} {
		if err := enc.ApplyEvent(report.GenerationEvent{Kind: "progress", Stage: id}); err != nil {
			log.Fail("apply_"+string(id), err.Error())
		}
	}

	out := cap.Stdout.String()
	for _, id := range stages {
		want := report.ProgressLine(id)
		log.Assert("has_"+string(id), strings.Contains(out, want), true, want)
	}
	log.Assert("has_network", strings.Contains(out, "network disclosure:"), true, out)
	// render + commit appear again via ApplyEvent (at least once each — already from loop)
	log.Assert("stderr_empty", cap.Stderr.String() == "", "", cap.Stderr.String())

	log.PhaseEnd("multi_event_stream", testutil.OutcomeOK)
}

func TestJSONDeterminism(t *testing.T) {
	log := testutil.New(t)
	log.Phase("determinism")

	err := diagnostic.New(
		diagnostic.IDSpecInvalidField,
		`field "name" must match Section 15.1`,
		diagnostic.SpecLocation("foundry.toml", 2, 8),
	)

	var prev string
	for i := 0; i < 2; i++ {
		enc, cap := newEnc(t, jsonOpts())
		_, _ = enc.Failure("validate", err, "")
		got := cap.Stdout.String()
		if i == 0 {
			prev = got
		} else {
			log.Assert("byte_equal", got == prev, prev, got)
		}
		// Key order: structural via struct — re-parse and check required keys.
		env := parseEnvelope(t, got)
		for _, k := range []string{"schema", "command", "ok", "result", "error", "warnings"} {
			_, ok := env[k]
			log.Assert("key_"+k, ok, true, ok)
		}
		obj := errorObject(t, env)
		for _, k := range []string{"error_id", "message", "remediation", "exit_code"} {
			_, ok := obj[k]
			log.Assert("err_key_"+k, ok, true, ok)
		}
	}

	// Success path also deterministic (-count=2 friendly).
	result := map[string]any{"status": "ok", "n": 1}
	enc1, cap1 := newEnc(t, jsonOpts())
	enc2, cap2 := newEnc(t, jsonOpts())
	_ = enc1.Success("validate", result, "")
	_ = enc2.Success("validate", result, "")
	log.Assert("success_equal", cap1.Stdout.String() == cap2.Stdout.String(),
		cap1.Stdout.String(), cap2.Stdout.String())

	log.PhaseEnd("determinism", testutil.OutcomeOK)
}

func TestRedaction(t *testing.T) {
	log := testutil.New(t)
	log.Phase("redaction")

	// Message/remediation with secret-like content must not appear raw.
	err := diagnostic.New(
		diagnostic.IDToolFailed,
		"step failed: token=super-secret-value-xyz",
		diagnostic.StepLocation("go-test"),
	).WithRemediation("Unset API_KEY=should-not-leak and retry")

	enc, cap := newEnc(t, textOpts())
	_, _ = enc.Failure("generate", err, "")
	stderr := cap.Stderr.String()
	log.Assert("no_raw_token", !strings.Contains(stderr, "super-secret-value-xyz"), false, true)
	log.Assert("has_redacted", strings.Contains(stderr, diagnostic.RedactedSentinel), true, stderr)
	log.Assert("no_raw_api_key_value", !strings.Contains(stderr, "should-not-leak"), false, true)

	enc, cap = newEnc(t, jsonOpts())
	_, _ = enc.Failure("generate", err, "")
	stdout := cap.Stdout.String()
	log.Assert("json_no_raw_token", !strings.Contains(stdout, "super-secret-value-xyz"), false, true)
	log.Assert("json_redacted", strings.Contains(stdout, diagnostic.RedactedSentinel), true, stdout)
	log.Assert("json_no_api_key_value", !strings.Contains(stdout, "should-not-leak"), false, true)

	log.PhaseEnd("redaction", testutil.OutcomeOK)
}

func TestRemediationForAppendixDIds(t *testing.T) {
	log := testutil.New(t)
	log.Phase("remediation_all_ids")

	// Exercise every Appendix D id at the report layer; remediation must be
	// non-empty in both text and JSON projections (bead 41p).
	ids := diagnostic.AllIdentifiers()
	log.Inputs(map[string]string{"count": itoa(len(ids))})

	for _, id := range ids {
		id := id
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			fe := diagnostic.New(id, "test failure for "+string(id), diagnostic.Location{})
			// JSON
			enc, cap := newEnc(t, jsonOpts())
			_, _ = enc.Failure("test", fe, "")
			env := parseEnvelope(t, cap.Stdout.String())
			obj := errorObject(t, env)
			sub.Assert("error_id", obj["error_id"] == string(id), string(id), obj["error_id"])
			rem, _ := obj["remediation"].(string)
			sub.Assert("remediation_nonempty", rem != "", "nonempty", rem)
			// Text
			enc, cap = newEnc(t, textOpts())
			_, _ = enc.Failure("test", fe, "/stage/path")
			stderr := cap.Stderr.String()
			sub.Assert("text_id", strings.Contains(stderr, string(id)), true, stderr)
			sub.Assert("text_remediation", strings.Contains(stderr, "remediation:"), true, stderr)
			sub.Assert("text_stage", strings.Contains(stderr, "stage_path: /stage/path"), true, stderr)
		})
	}

	log.PhaseEnd("remediation_all_ids", testutil.OutcomeOK)
}

func TestPostCommitReportFailureExit0(t *testing.T) {
	log := testutil.New(t)
	log.Phase("post_commit_stream")

	// Success after MarkCommitted with a failing stdout still "succeeds"
	// (absorbed); StreamFailed is true; best-effort stderr notice.
	failOut := &failWriter{err: errors.New("EPIPE")}
	var stderr bytes.Buffer
	enc := report.New(failOut, &stderr, jsonOpts())
	enc.MarkCommitted()
	log.Assert("committed", enc.Committed(), true, false)

	result := report.GenerateResult{
		CommitOutcome: report.OutcomeCommitted,
		Destination:   "/tmp/demo-cli",
	}
	err := enc.Success("generate", result, "")
	log.Assert("write_absorbed", err == nil, true, err)
	log.Assert("stream_failed", enc.StreamFailed(), true, false)
	log.Assert("notice", strings.Contains(stderr.String(), "report stream failed after commit"), true, stderr.String())
	log.Assert("notice_dest", strings.Contains(stderr.String(), "/tmp/demo-cli"), true, stderr.String())

	// Failure after commit must not return non-zero exit (FND-012 guard).
	enc2 := report.New(io.Discard, &stderr, jsonOpts())
	enc2.MarkCommitted()
	code, werr := enc2.Failure("generate", diagnostic.New(
		diagnostic.IDReportFailed, "stdout closed", diagnostic.Location{},
	), "")
	log.Assert("failure_exit_0", code == diagnostic.ExitSuccess, diagnostic.ExitSuccess, code)
	log.Assert("failure_write_nil", werr == nil, true, werr)

	// Pre-commit stream failure is NOT absorbed.
	failOut2 := &failWriter{err: errors.New("EPIPE")}
	enc3 := report.New(failOut2, io.Discard, jsonOpts())
	err = enc3.Success("validate", map[string]string{"status": "ok"}, "")
	log.Assert("precommit_fails", err != nil, true, err)
	log.Assert("precommit_not_marked", !enc3.StreamFailed(), false, true)

	log.PhaseEnd("post_commit_stream", testutil.OutcomeOK)
}

func TestJSONNeverErrorWithCommittedLie(t *testing.T) {
	log := testutil.New(t)
	log.Phase("fnd012")

	// FailureEnvelope itself is fine for uncommitted failures; Success for
	// committed. Guard: after MarkCommitted, Failure does not write ok=false.
	var stdout, stderr bytes.Buffer
	enc := report.New(&stdout, &stderr, jsonOpts())
	enc.MarkCommitted()
	_, _ = enc.Failure("generate", errors.New("boom"), "/stage")
	log.Assert("no_ok_false", !strings.Contains(stdout.String(), `"ok":false`), false, stdout.String())
	log.Assert("stderr_notice", stderr.Len() > 0, true, stderr.String())

	log.PhaseEnd("fnd012", testutil.OutcomeOK)
}

func TestCancelledEncoding(t *testing.T) {
	log := testutil.New(t)
	log.Phase("cancelled")

	stage := "/tmp/parent/.foundry-demo-abc"
	enc, cap := newEnc(t, jsonOpts())
	code, _ := enc.Failure("generate", diagnostic.ErrCancelled, stage)
	log.Assert("exit_130", code == diagnostic.ExitCancelled, diagnostic.ExitCancelled, code)
	env := parseEnvelope(t, cap.Stdout.String())
	obj := errorObject(t, env)
	log.Assert("error_id", obj["error_id"] == "cancelled", "cancelled", obj["error_id"])
	log.Assert("stage_path", obj["stage_path"] == stage, stage, obj["stage_path"])
	log.Assert("exit_code", obj["exit_code"] == float64(130), 130, obj["exit_code"])

	enc, cap = newEnc(t, textOpts())
	_, _ = enc.Failure("generate", diagnostic.ErrCancelled, stage)
	log.Assert("text_stage", strings.Contains(cap.Stderr.String(), stage), true, cap.Stderr.String())

	log.PhaseEnd("cancelled", testutil.OutcomeOK)
}

func TestColorRespectsNOColor(t *testing.T) {
	log := testutil.New(t)
	log.Phase("nocolor")

	err := diagnostic.New(diagnostic.IDUsageInvalid, "bad flag", diagnostic.Location{})

	// ColorAlways + terminal but NOColor set → no ANSI.
	opts := report.Options{
		Mode:             report.ModeText,
		Color:            report.ColorAlways,
		NOColor:          true,
		StderrIsTerminal: true,
	}
	enc, cap := newEnc(t, opts)
	_, _ = enc.Failure("version", err, "")
	log.Assert("no_ansi", !strings.Contains(cap.Stderr.String(), "\033["), false, cap.Stderr.String())

	// ColorAlways without NOColor → ANSI present.
	opts.NOColor = false
	enc, cap = newEnc(t, opts)
	_, _ = enc.Failure("version", err, "")
	log.Assert("has_ansi", strings.Contains(cap.Stderr.String(), "\033["), true, cap.Stderr.String())

	log.PhaseEnd("nocolor", testutil.OutcomeOK)
}

func TestErrorObjectJSONKeysStable(t *testing.T) {
	log := testutil.New(t)
	log.Phase("error_keys")

	raw, err := json.Marshal(report.ErrorFrom(
		diagnostic.New(diagnostic.IDFSDestinationExists, "dest exists", diagnostic.PathLocation("/tmp/out")),
		"/tmp/.foundry-x",
	))
	if err != nil {
		log.Fail("marshal", err.Error())
	}
	// Struct field order: error_id, message, remediation, path, line, col,
	// stage_path, location, exit_code — verify error_id appears before message.
	s := string(raw)
	iID := strings.Index(s, `"error_id"`)
	iMsg := strings.Index(s, `"message"`)
	iRem := strings.Index(s, `"remediation"`)
	log.Assert("order_id_msg", iID >= 0 && iID < iMsg, true, s)
	log.Assert("order_msg_rem", iMsg < iRem, true, s)
	log.Assert("has_stage", strings.Contains(s, `"stage_path"`), true, s)

	log.PhaseEnd("error_keys", testutil.OutcomeOK)
}

// failWriter always returns an error on Write.
type failWriter struct{ err error }

func (f *failWriter) Write(p []byte) (int, error) {
	return 0, f.err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestVerboseAndOptionsAccessors(t *testing.T) {
	log := testutil.New(t)
	log.Phase("verbose")
	opts := textOpts()
	opts.Verbose = true
	enc, cap := newEnc(t, opts)
	got := enc.Options()
	log.Assert("verbose_opt", got.Verbose, true, got.Verbose)
	if err := enc.Verbose("detail=host go=/usr/local/go/bin/go"); err != nil {
		log.Fail("verbose", err.Error())
	}
	out := cap.Stdout.String() + cap.Stderr.String()
	log.Assert("verbose_emitted", strings.Contains(out, "detail=host") || strings.Contains(out, "go="), true, out)
	if enc.LastStreamError() != nil {
		t.Fatalf("unexpected stream err: %v", enc.LastStreamError())
	}
	// JSON SuccessWithWarnings path
	cap2 := &capture{}
	enc2 := report.New(&cap2.Stdout, &cap2.Stderr, jsonOpts())
	if err := enc2.SuccessWithWarnings("validate", map[string]any{"ok": true}, "validate: ok", []string{"warn-a"}); err != nil {
		log.Fail("success_warn", err.Error())
	}
	js := cap2.Stdout.String()
	log.Assert("json_ok", strings.Contains(js, `"ok":true`) || strings.Contains(js, `"ok": true`), true, js)
	// destinationFromResult via post-commit notice: MarkCommitted + broken writer not needed;
	// GenerateResult path exercised through Format helpers if any.
	log.Assert("format_validate", report.FormatValidateText("x.toml") != "", true, false)
	log.Assert("format_plan", report.FormatPlanText("proj", "./proj", 3) != "", true, false)
	log.Assert("format_version", report.FormatVersionText("0.1.0", "deadbeef", "go1.26.5", "digest") != "", true, false)
	log.PhaseEnd("verbose", testutil.OutcomeOK)
}
