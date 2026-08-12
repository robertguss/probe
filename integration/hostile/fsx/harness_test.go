//go:build hostile && unix

package hostilefsx

import (
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/fsx"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// NewHarness builds a probe bridge + testutil step logger for one FS-labeled case.
// Lives in _test.go so internal/testutil is never imported from production sources.
func NewHarness(t *testing.T, fsLabel string) (fsx.StepLogger, *testutil.Logger, *ProbeLog) {
	t.Helper()
	pl := NewProbeLog()
	tl := testutil.New(t)
	kv := KernelVersion()
	recordHarnessMeta(pl, fsLabel, kv)
	bridge := newProbeBridge(pl, fsLabel, kv, func(name, outcome, detail string) {
		o := testutil.OutcomeOK
		switch outcome {
		case "fail":
			o = testutil.OutcomeFail
		case "info":
			o = testutil.OutcomeInfo
		}
		tl.Step(name, o, detail)
	})
	attachHarnessCleanup(t, pl)
	return bridge, tl, pl
}
