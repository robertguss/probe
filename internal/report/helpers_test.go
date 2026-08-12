package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

type capture struct {
	Stdout bytes.Buffer
	Stderr bytes.Buffer
}

func newEnc(t *testing.T, opts report.Options) (*report.Encoder, *capture) {
	t.Helper()
	c := &capture{}
	return report.New(&c.Stdout, &c.Stderr, opts), c
}

func textOpts() report.Options {
	return report.Options{Mode: report.ModeText, Color: report.ColorNever}
}

func jsonOpts() report.Options {
	return report.Options{Mode: report.ModeJSON, Color: report.ColorNever}
}

func parseEnvelope(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("json: %v\n%s", err, s)
	}
	return m
}

func errorObject(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	errObj, ok := env["error"].(map[string]any)
	if !ok || errObj == nil {
		t.Fatalf("missing error object: %v", env)
	}
	return errObj
}

func normalizeLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func compareGolden(t *testing.T, log *testutil.Logger, name string, got string) {
	t.Helper()
	path := testutil.GoldenPath("testdata", name)
	testutil.CompareGolden(t, path, []byte(normalizeLF(got)))
	log.Step("golden_"+name, testutil.OutcomeOK, path)
}
