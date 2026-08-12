package plan_test

import (
	"sync"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// TestPipelineConcurrentRace stresses plan.Pipeline with the race detector.
// Resolve + plan construction must be safe for concurrent use with immutable
// inputs, and all concurrent plans must be byte-identical (REQ-122 /
// bead go-foundry-cli-wet.1.5).
func TestPipelineConcurrentRace(t *testing.T) {
	t.Parallel()

	cat := mustLoadCatalog(t)
	vs := minimalCLI(t)
	dest := fixedDestination("./minimal-cli", plan.ObservationAbsent)
	opts := defaultPipelineOpts(dest)

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)

	var firstSHA string
	var once sync.Once

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			p, err := plan.Pipeline(vs, cat, opts)
			if err != nil {
				t.Errorf("Pipeline: %v", err)
				return
			}
			if p == nil {
				t.Error("Pipeline returned nil result")
				return
			}
			sha := p.PlanSHA256()
			once.Do(func() { firstSHA = sha })
			if sha != firstSHA {
				t.Errorf("plan SHA256 mismatch: got %q, want %q", sha, firstSHA)
			}
		}()
	}
	wg.Wait()
}
