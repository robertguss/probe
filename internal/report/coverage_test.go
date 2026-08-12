package report_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// seqFailWriter fails on the Nth Write call (1-based).
type seqFailWriter struct {
	n      int
	failOn int
	err    error
	buf    bytes.Buffer
}

func (w *seqFailWriter) Write(p []byte) (int, error) {
	w.n++
	if w.n >= w.failOn {
		return 0, w.err
	}
	return w.buf.Write(p)
}

func TestApplyEventMatrix(t *testing.T) {
	log := testutil.New(t)
	log.Phase("apply_event_matrix")

	var nilEnc *report.Encoder
	if err := nilEnc.ApplyEvent(report.GenerationEvent{Kind: "progress", Stage: report.StageRender}); err != nil {
		t.Fatalf("nil ApplyEvent: %v", err)
	}
	log.Step("nil_encoder", testutil.OutcomeOK, "")

	cases := []struct {
		name    string
		opts    report.Options
		ev      report.GenerationEvent
		wantOut string
		wantAbs bool
	}{
		{
			name:    "progress_empty_stage",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "progress"},
			wantAbs: true,
		},
		{
			name:    "progress_stage",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "progress", Stage: report.StageRender},
			wantOut: report.ProgressLine(report.StageRender),
		},
		{
			name:    "network_lines",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "network", Lines: []string{"proxy may contact network"}},
			wantOut: "network disclosure:",
		},
		{
			name:    "summary_detail",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "summary", Detail: "files=3"},
			wantOut: "files=3",
		},
		{
			name:    "state_text",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "state", State: report.LifeCommitted},
			wantOut: "state: " + string(report.LifeCommitted),
		},
		{
			name:    "state_quiet_suppressed",
			opts:    report.Options{Mode: report.ModeText, Quiet: true, Color: report.ColorNever},
			ev:      report.GenerationEvent{Kind: "state", State: report.LifeCommitted},
			wantAbs: true,
		},
		{
			name:    "state_json_suppressed",
			opts:    jsonOpts(),
			ev:      report.GenerationEvent{Kind: "state", State: report.LifeCommitted},
			wantAbs: true,
		},
		{
			name:    "state_empty",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "state"},
			wantAbs: true,
		},
		{
			name:    "terminal_stage_path",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "terminal", StagePath: "/tmp/.foundry-stage"},
			wantOut: "stage_path: /tmp/.foundry-stage",
		},
		{
			name:    "terminal_stage_path_quiet_still_emits",
			opts:    report.Options{Mode: report.ModeText, Quiet: true, Color: report.ColorNever},
			ev:      report.GenerationEvent{Kind: "terminal", StagePath: "/tmp/.foundry-stage"},
			wantOut: "stage_path: /tmp/.foundry-stage",
		},
		{
			name:    "terminal_json_no_stage_emit",
			opts:    jsonOpts(),
			ev:      report.GenerationEvent{Kind: "terminal", StagePath: "/tmp/.foundry-stage"},
			wantAbs: true,
		},
		{
			name:    "terminal_no_stage",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "terminal", Outcome: report.OutcomeCommitted},
			wantAbs: true,
		},
		{
			name:    "unknown_kind",
			opts:    textOpts(),
			ev:      report.GenerationEvent{Kind: "mystery"},
			wantAbs: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)
			enc, cap := newEnc(t, tc.opts)
			if err := enc.ApplyEvent(tc.ev); err != nil {
				sub.Fail("apply", err.Error())
			}
			out := cap.Stdout.String() + cap.Stderr.String()
			if tc.wantOut != "" {
				sub.Assert("has_out", strings.Contains(out, tc.wantOut), true, out)
			}
			if tc.wantAbs {
				switch {
				case tc.ev.Kind == "progress" && tc.ev.Stage == "":
					sub.Assert("empty_stdout", out == "", "", out)
				case tc.ev.Kind == "state":
					sub.Assert("state_absent", !strings.Contains(out, "state:"), true, out)
				case tc.ev.Kind == "terminal":
					sub.Assert("terminal_absent", !strings.Contains(out, "stage_path:"), true, out)
				case tc.ev.Kind == "mystery":
					sub.Assert("unknown_empty", out == "", "", out)
				}
			}
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}

	failOut := &failWriter{err: errors.New("EPIPE")}
	var stderr bytes.Buffer
	enc := report.New(failOut, &stderr, textOpts())
	enc.MarkCommitted()
	err := enc.ApplyEvent(report.GenerationEvent{Kind: "terminal", StagePath: "/stage"})
	log.Assert("terminal_absorb", err == nil, true, err)
	log.Assert("stream_failed", enc.StreamFailed(), true, false)

	log.PhaseEnd("apply_event_matrix", testutil.OutcomeOK)
}

func TestDestinationFromResultViaPostCommit(t *testing.T) {
	log := testutil.New(t)
	log.Phase("destination_from_result")

	rows := []struct {
		name   string
		result any
		want   string
	}{
		{
			name:   "generate_value",
			result: report.GenerateResult{Destination: "/tmp/val-dest", CommitOutcome: report.OutcomeCommitted},
			want:   "/tmp/val-dest",
		},
		{
			name:   "generate_pointer",
			result: &report.GenerateResult{Destination: "/tmp/ptr-dest", CommitOutcome: report.OutcomeCommitted},
			want:   "/tmp/ptr-dest",
		},
		{
			name:   "generate_nil_pointer",
			result: (*report.GenerateResult)(nil),
			want:   "",
		},
		{
			name:   "map_destination",
			result: map[string]any{"destination": "/tmp/map-dest", "ok": true},
			want:   "/tmp/map-dest",
		},
		{
			name:   "map_wrong_type",
			result: map[string]any{"destination": 42},
			want:   "",
		},
		{
			name:   "unknown_shape",
			result: "just-a-string",
			want:   "",
		},
	}

	for _, tc := range rows {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)
			failOut := &failWriter{err: errors.New("broken pipe")}
			var stderr bytes.Buffer
			enc := report.New(failOut, &stderr, textOpts())
			enc.MarkCommitted()
			err := enc.Success("generate", tc.result, "summary line")
			sub.Assert("absorbed", err == nil, true, err)
			sub.Assert("stream_failed", enc.StreamFailed(), true, false)
			notice := stderr.String()
			sub.Assert("notice_present", strings.Contains(notice, "report stream failed after commit"), true, notice)
			if tc.want != "" {
				sub.Assert("dest_in_notice", strings.Contains(notice, tc.want), true, notice)
			} else {
				sub.Assert("generic_notice",
					strings.Contains(notice, "generation succeeded; report stream failed"),
					true, notice)
			}

			failOut2 := &failWriter{err: errors.New("EPIPE")}
			var stderr2 bytes.Buffer
			enc2 := report.New(failOut2, &stderr2, jsonOpts())
			enc2.MarkCommitted()
			_ = enc2.Success("generate", tc.result, "")
			if tc.want != "" {
				sub.Assert("json_dest", strings.Contains(stderr2.String(), tc.want), true, stderr2.String())
			}
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}
	log.PhaseEnd("destination_from_result", testutil.OutcomeOK)
}

func TestColorEnabledModes(t *testing.T) {
	log := testutil.New(t)
	log.Phase("color_modes")
	err := diagnostic.New(diagnostic.IDUsageInvalid, "bad", diagnostic.Location{})

	cases := []struct {
		name     string
		opts     report.Options
		wantANSI bool
	}{
		{
			name:     "always",
			opts:     report.Options{Mode: report.ModeText, Color: report.ColorAlways},
			wantANSI: true,
		},
		{
			name:     "never",
			opts:     report.Options{Mode: report.ModeText, Color: report.ColorNever, StderrIsTerminal: true},
			wantANSI: false,
		},
		{
			name:     "auto_terminal",
			opts:     report.Options{Mode: report.ModeText, Color: report.ColorAuto, StderrIsTerminal: true},
			wantANSI: true,
		},
		{
			name:     "auto_non_terminal",
			opts:     report.Options{Mode: report.ModeText, Color: report.ColorAuto, StderrIsTerminal: false},
			wantANSI: false,
		},
		{
			name:     "auto_default_empty_color",
			opts:     report.Options{Mode: report.ModeText, StderrIsTerminal: true},
			wantANSI: true,
		},
		{
			name:     "nocolor_overrides_always",
			opts:     report.Options{Mode: report.ModeText, Color: report.ColorAlways, NOColor: true, StderrIsTerminal: true},
			wantANSI: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sub := testutil.New(t)
			sub.Phase(tc.name)
			enc, cap := newEnc(t, tc.opts)
			_, _ = enc.Failure("version", err, "")
			has := strings.Contains(cap.Stderr.String(), "\033[")
			sub.Assert("ansi", has == tc.wantANSI, tc.wantANSI, has)
			sub.PhaseEnd(tc.name, testutil.OutcomeOK)
		})
	}

	// JSON mode: colorEnabled false; Failure writes JSON to stdout (no ANSI on stderr).
	encJSON, capJSON := newEnc(t, report.Options{Mode: report.ModeJSON, Color: report.ColorAlways, StderrIsTerminal: true})
	_, _ = encJSON.Failure("version", err, "")
	log.Assert("json_no_ansi_stderr", !strings.Contains(capJSON.Stderr.String(), "\033["), true, capJSON.Stderr.String())
	log.Assert("json_stdout", strings.Contains(capJSON.Stdout.String(), `"ok":false`) || strings.Contains(capJSON.Stdout.String(), `"ok": false`), true, capJSON.Stdout.String())

	// Empty Mode/Color defaults via New/normalized.
	enc, _ := newEnc(t, report.Options{})
	log.Assert("default_mode_text", enc.Options().Mode == report.ModeText, report.ModeText, enc.Options().Mode)
	log.Assert("default_color_auto", enc.Options().Color == report.ColorAuto, report.ColorAuto, enc.Options().Color)

	log.PhaseEnd("color_modes", testutil.OutcomeOK)
}

func TestWriteNetworkDisclosureAndVerboseEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("network_verbose_edges")

	if err := report.WriteNetworkDisclosure(nil, []string{"x"}); err != nil {
		t.Fatalf("nil w: %v", err)
	}
	if err := report.WriteVerbose(nil, "x", true); err != nil {
		t.Fatalf("verbose nil: %v", err)
	}
	log.Step("nil_writers", testutil.OutcomeOK, "")

	var buf bytes.Buffer
	if err := report.WriteNetworkDisclosure(&buf, nil); err != nil || buf.Len() != 0 {
		t.Fatalf("empty lines: err=%v buf=%q", err, buf.String())
	}
	if err := report.WriteNetworkDisclosure(&buf, []string{}); err != nil || buf.Len() != 0 {
		t.Fatalf("empty slice: err=%v", err)
	}
	if err := report.WriteVerbose(&buf, "x", false); err != nil || buf.Len() != 0 {
		t.Fatalf("verbose off: %v", err)
	}
	if err := report.WriteVerbose(&buf, "", true); err != nil || buf.Len() != 0 {
		t.Fatalf("empty detail: %v", err)
	}
	log.Step("noops", testutil.OutcomeOK, "")

	buf.Reset()
	if err := report.WriteNetworkDisclosure(&buf, []string{"  ", "token=supersecret", "ok step"}); err != nil {
		t.Fatalf("network: %v", err)
	}
	out := buf.String()
	log.Assert("header", strings.Contains(out, "network disclosure:"), true, out)
	log.Assert("blank_skipped_has_ok", strings.Contains(out, "ok step"), true, out)
	log.Assert("secret_redacted", !strings.Contains(out, "supersecret"), true, out)
	log.Assert("sentinel", strings.Contains(out, diagnostic.RedactedSentinel), true, out)

	buf.Reset()
	if err := report.WriteVerbose(&buf, "password=hunter2 detail", true); err != nil {
		t.Fatalf("verbose: %v", err)
	}
	vout := buf.String()
	log.Assert("verbose_prefix", strings.HasPrefix(vout, "verbose: "), true, vout)
	log.Assert("verbose_redact", !strings.Contains(vout, "hunter2"), true, vout)

	fw := &failWriter{err: errors.New("disk full")}
	if err := report.WriteNetworkDisclosure(fw, []string{"line"}); err == nil {
		t.Fatal("expected header write error")
	}
	sw := &seqFailWriter{failOn: 2, err: errors.New("line2")}
	if err := report.WriteNetworkDisclosure(sw, []string{"a", "b"}); err == nil {
		t.Fatal("expected line write error")
	}
	log.Step("write_failures", testutil.OutcomeOK, "")

	enc, cap := newEnc(t, jsonOpts())
	if err := enc.NetworkDisclosure([]string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err := enc.Verbose("y"); err != nil {
		t.Fatal(err)
	}
	if err := enc.Summary("z"); err != nil {
		t.Fatal(err)
	}
	if err := enc.Progress(report.StageRender); err != nil {
		t.Fatal(err)
	}
	log.Assert("json_silent", cap.Stdout.Len() == 0 && cap.Stderr.Len() == 0, true, cap.Stdout.String())

	var nilEnc *report.Encoder
	if err := nilEnc.NetworkDisclosure([]string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err := nilEnc.Verbose("x"); err != nil {
		t.Fatal(err)
	}
	if err := nilEnc.Summary("x"); err != nil {
		t.Fatal(err)
	}
	if err := nilEnc.Progress(report.StageRender); err != nil {
		t.Fatal(err)
	}
	if err := nilEnc.Success("c", nil, "s"); err != nil {
		t.Fatal(err)
	}
	code, werr := nilEnc.Failure("c", errors.New("e"), "")
	log.Assert("nil_failure_code", code == diagnostic.ExitFailure, diagnostic.ExitFailure, code)
	log.Assert("nil_failure_werr", werr == nil, true, werr)
	log.Assert("nil_options", nilEnc.Options().Mode == "", true, nilEnc.Options().Mode)
	log.Assert("nil_committed", !nilEnc.Committed(), true, false)
	log.Assert("nil_stream", !nilEnc.StreamFailed(), true, false)
	log.Assert("nil_last_err", nilEnc.LastStreamError() == nil, true, false)
	nilEnc.MarkCommitted()

	report.WritePostCommitStreamNotice(nil, "/x")
	var nb bytes.Buffer
	report.WritePostCommitStreamNotice(&nb, "")
	log.Assert("notice_generic", strings.Contains(nb.String(), "generation succeeded; report stream failed"), true, nb.String())
	nb.Reset()
	report.WritePostCommitStreamNotice(&nb, "/dest/path")
	log.Assert("notice_dest", strings.Contains(nb.String(), "/dest/path"), true, nb.String())

	report.WriteTextError(nil, errors.New("x"), "", false)
	report.WriteTextError(io.Discard, nil, "", false)
	var eb bytes.Buffer
	fe := diagnostic.New(diagnostic.IDFSDestinationExists, "exists", diagnostic.PathLocation("/tmp/out"))
	report.WriteTextError(&eb, fe, "/stage", false)
	log.Assert("text_err_path", strings.Contains(eb.String(), "/tmp/out"), true, eb.String())
	log.Assert("text_err_stage", strings.Contains(eb.String(), "stage_path: /stage"), true, eb.String())

	eb.Reset()
	fe2 := diagnostic.New(diagnostic.IDSpecInvalidField, "bad name", diagnostic.SpecLocation("f.toml", 3, 4))
	report.WriteTextError(&eb, fe2, "", true)
	log.Assert("ansi_err", strings.Contains(eb.String(), "\033["), true, eb.String())
	log.Assert("spec_loc", strings.Contains(eb.String(), "f.toml:3:4"), true, eb.String())

	eb.Reset()
	report.WriteTextError(&eb, errors.New("plain boom"), "", false)
	log.Assert("plain", strings.Contains(eb.String(), "plain boom") || strings.Contains(eb.String(), "internal"), true, eb.String())

	log.PhaseEnd("network_verbose_edges", testutil.OutcomeOK)
}

func TestFormatVersionTextEdges(t *testing.T) {
	log := testutil.New(t)
	log.Phase("format_version")
	got := report.FormatVersionText("1.2.3", "", "go1.26.5", "")
	log.Assert("commit_dash", strings.Contains(got, "commit: -"), true, got)
	log.Assert("digest_dash", strings.Contains(got, "catalog_digest: -"), true, got)
	log.Assert("version", strings.Contains(got, "foundry 1.2.3"), true, got)
	full := report.FormatVersionText("1.0.0", "abc", "go1.26.5", "deadbeef")
	log.Assert("full_commit", strings.Contains(full, "commit: abc"), true, full)
	log.Assert("full_digest", strings.Contains(full, "catalog_digest: deadbeef"), true, full)
	log.PhaseEnd("format_version", testutil.OutcomeOK)
}

func TestTextErrorObjectBranches(t *testing.T) {
	log := testutil.New(t)
	log.Phase("text_error_object")

	var buf bytes.Buffer
	enc, cap := newEnc(t, report.Options{Mode: report.ModeText, Color: report.ColorAlways})
	fe := diagnostic.New(diagnostic.IDSpecUnknownField, "unknown field x", diagnostic.SpecLocation("p.toml", 2, 0))
	_, _ = enc.Failure("validate", fe, "")
	log.Assert("line_no_col", strings.Contains(cap.Stderr.String(), "p.toml:2"), true, cap.Stderr.String())

	buf.Reset()
	report.WriteTextError(&buf, errors.New(""), "stg", false)
	log.Assert("empty_msg_encoded", buf.Len() > 0, true, buf.String())

	enc2, cap2 := newEnc(t, textOpts())
	if err := enc2.Success("validate", map[string]string{"ok": "1"}, ""); err != nil {
		t.Fatal(err)
	}
	log.Assert("empty_summary", cap2.Stdout.Len() == 0, true, cap2.Stdout.String())

	enc3, cap3 := newEnc(t, report.Options{Mode: report.ModeText, Quiet: true, Color: report.ColorNever})
	if err := enc3.Success("validate", nil, "should hide"); err != nil {
		t.Fatal(err)
	}
	log.Assert("quiet_success", cap3.Stdout.Len() == 0, true, cap3.Stdout.String())

	fw := &failWriter{err: errors.New("first-epipe")}
	enc4 := report.New(fw, io.Discard, textOpts())
	enc4.MarkCommitted()
	_ = enc4.Success("generate", report.GenerateResult{Destination: "/d"}, "sum")
	first := enc4.LastStreamError()
	_ = enc4.Progress(report.StageCommit)
	log.Assert("first_err_kept", first != nil && enc4.LastStreamError() == first, true, enc4.LastStreamError())

	// Committed text Failure still returns exit 0.
	enc5, _ := newEnc(t, textOpts())
	enc5.MarkCommitted()
	code, werr := enc5.Failure("generate", diagnostic.New(diagnostic.IDReportFailed, "x", diagnostic.Location{}), "/stage")
	log.Assert("committed_failure_exit0", code == diagnostic.ExitSuccess, diagnostic.ExitSuccess, code)
	log.Assert("committed_failure_werr", werr == nil, true, werr)

	log.PhaseEnd("text_error_object", testutil.OutcomeOK)
}
