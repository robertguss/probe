package generate_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/toolrun"
)

// TestOrchestrateConcurrentRace runs independent generate.Orchestrator/Machine
// lifecycles in parallel under the race detector. Each goroutine gets its own
// destination and catalog/plan inputs; the goal is to catch shared-state bugs
// in the underlying toolrun, fsx, render, verify, and gitinit packages
// (bead go-foundry-cli-wet.1.5).
func TestOrchestrateConcurrentRace(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping orchestrator race stress in short mode")
	}

	const n = 3
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()

			parent := privateParent(t)
			dest := filepath.Join(parent, "orch-cli")
			p := mustPlanCLI(t, dest, false)
			orch := &generate.Orchestrator{
				Plan:     p,
				Catalog:  mustCatalog(t),
				Host:     toolrun.HostCaptureFromPlan(hostFromPlan(p)),
				GoBinary: pinnedGo(t),
				Log:      &generate.RecordingLogger{},
			}
			defer func() { _ = orch.Close() }()

			m := generate.New(generate.Config{Stages: orch.Stages(), Log: orch.Log})
			res := m.Run(context.Background())
			if res.Exit != diagnostic.ExitSuccess {
				t.Errorf("goroutine %d: generate failed: exit=%d", idx, res.Exit)
			}
			if res.Outcome != generate.OutcomeCommitted {
				t.Errorf("goroutine %d: unexpected outcome: got %q, want %q", idx, res.Outcome, generate.OutcomeCommitted)
			}
		}(i)
	}
	wg.Wait()
}
