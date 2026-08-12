package report_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestRealPipePreCommitFailure closes the reader end of a real OS pipe before
// encoding and asserts the write error surfaces (not absorbed) — Section 36.5
// pre-commit path. Injected failWriter coverage lives in encoder_test.go;
// this file owns real-pipe evidence for fy9.
func TestRealPipePreCommitFailure(t *testing.T) {
	log := testutil.New(t)
	log.Phase("real_pipe_pre_commit")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close() // break before write

	var stderr bytes.Buffer
	enc := report.New(w, &stderr, jsonOpts())
	err = enc.Success("validate", map[string]string{"status": "ok"}, "")
	_ = w.Close()

	log.Assert("write_err", err != nil, true, err)
	log.Assert("not_stream_failed_flag", !enc.StreamFailed(), false, true)
	log.Assert("not_committed", !enc.Committed(), false, true)
	log.Assert("epipe_or_closed", isPipeErr(err), true, err)
	log.Step("pre_commit_pipe", testutil.OutcomeOK, "err="+errString(err))

	log.PhaseEnd("real_pipe_pre_commit", testutil.OutcomeOK)
}

// TestRealPipePostCommitAbsorbed closes the reader after MarkCommitted and
// asserts exit stays success-shaped: write error absorbed, StreamFailed set,
// best-effort stderr notice, no ok=false JSON (FND-012).
func TestRealPipePostCommitAbsorbed(t *testing.T) {
	log := testutil.New(t)
	log.Phase("real_pipe_post_commit")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close()

	var stderr bytes.Buffer
	enc := report.New(w, &stderr, jsonOpts())
	enc.MarkCommitted()
	log.Assert("committed", enc.Committed(), true, false)

	result := report.GenerateResult{
		CommitOutcome: report.OutcomeCommitted,
		Destination:   "demo-cli",
	}
	err = enc.Success("generate", result, "")
	_ = w.Close()

	log.Assert("absorbed_nil", err == nil, true, err)
	log.Assert("stream_failed", enc.StreamFailed(), true, false)
	log.Assert("notice", strings.Contains(stderr.String(), "report stream failed after commit"), true, stderr.String())
	log.Assert("notice_dest", strings.Contains(stderr.String(), "demo-cli"), true, stderr.String())
	// No JSON body on the broken stdout — and stderr must not claim ok=false.
	log.Assert("no_ok_false_stderr", !strings.Contains(stderr.String(), `"ok":false`), false, stderr.String())
	log.Step("post_commit_pipe", testutil.OutcomeOK,
		"stream_failed=true exit_contract=0 stderr_notice=true")

	log.PhaseEnd("real_pipe_post_commit", testutil.OutcomeOK)
}

// TestJSONEncoderFailureAfterCommitDoesNotFlipExit0 is the FND-012 matrix row:
// after MarkCommitted, Failure refuses ok=false and returns exit 0.
func TestJSONEncoderFailureAfterCommitDoesNotFlipExit0(t *testing.T) {
	log := testutil.New(t)
	log.Phase("json_post_commit_no_flip")

	// Broken real pipe as stdout.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close()

	var stderr bytes.Buffer
	enc := report.New(w, &stderr, jsonOpts())
	enc.MarkCommitted()

	code, werr := enc.Failure("generate", diagnostic.New(
		diagnostic.IDReportFailed, "stdout closed", diagnostic.Location{},
	), "")
	_ = w.Close()

	log.Assert("exit_0", code == diagnostic.ExitSuccess, diagnostic.ExitSuccess, code)
	log.Assert("write_nil", werr == nil, true, werr)
	log.Assert("notice", stderr.Len() > 0, true, stderr.String())
	log.Assert("no_ok_false", !strings.Contains(stderr.String(), `"ok":false`), false, stderr.String())

	// Success path on a second encoder with failing pipe still exit-contract 0.
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe2: %v", err)
	}
	_ = r2.Close()
	var stderr2 bytes.Buffer
	enc2 := report.New(w2, &stderr2, jsonOpts())
	enc2.MarkCommitted()
	err = enc2.Success("generate", report.GenerateResult{
		CommitOutcome: report.OutcomeCommitted,
		Destination:   "x",
	}, "")
	_ = w2.Close()
	log.Assert("success_absorbed", err == nil, true, err)
	log.Assert("success_stream_failed", enc2.StreamFailed(), true, false)

	log.PhaseEnd("json_post_commit_no_flip", testutil.OutcomeOK)
}

// TestNoCommittedErrorJSONOnSuccessPath encodes a committed GenerateResult and
// asserts the document is ok=true with commit_outcome=committed (never the lie).
func TestNoCommittedErrorJSONOnSuccessPath(t *testing.T) {
	log := testutil.New(t)
	log.Phase("no_committed_error_json")

	var stdout, stderr bytes.Buffer
	enc := report.New(&stdout, &stderr, jsonOpts())
	enc.MarkCommitted()
	err := enc.Success("generate", report.GenerateResult{
		CommitOutcome: report.OutcomeCommitted,
		Destination:   "demo",
	}, "")
	log.Assert("err_nil", err == nil, true, err)
	log.Assert("stderr_empty", stderr.Len() == 0, 0, stderr.Len())

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	log.Assert("ok_true", env["ok"] == true, true, env["ok"])
	res, _ := env["result"].(map[string]any)
	log.Assert("outcome_committed", res["commit_outcome"] == "committed", "committed", res["commit_outcome"])
	log.Assert("error_null", env["error"] == nil, true, env["error"])
	// Forbidden lie probe.
	log.Assert("not_lie", !(env["ok"] == false && res["commit_outcome"] == "committed"), false, true)

	log.PhaseEnd("no_committed_error_json", testutil.OutcomeOK)
}

func isPipeErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, os.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "epipe")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
