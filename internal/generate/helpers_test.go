package generate_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

const testdataDir = "testdata"

// defaultStages wires create-stage + successful commit so full runs land committed.
func defaultStages() map[generate.StageID]generate.StageFunc {
	return map[generate.StageID]generate.StageFunc{
		generate.StageCreateStage: generate.CreateStageFunc(".foundry-demo-deadbeef"),
		generate.StageCommit: generate.CommitStage(
			generate.OutcomeCommitted,
			"demo",
			".foundry-demo-deadbeef",
			nil,
		),
	}
}

// stagesThrough builds a stage map that succeeds through `lastOK` (inclusive)
// using default create/commit helpers when those stages are included, and fails
// at `failAt` when non-empty.
func stagesThrough(lastOK generate.StageID, failAt generate.StageID, fail *generate.StageError) map[generate.StageID]generate.StageFunc {
	m := map[generate.StageID]generate.StageFunc{}
	lastN := generate.StageNumber(lastOK)
	failN := generate.StageNumber(failAt)

	for _, id := range generate.AllStageIDs() {
		n := generate.StageNumber(id)
		id := id
		switch {
		case failN > 0 && n == failN:
			m[id] = generate.FailStage(fail)
		case n <= lastN:
			switch id {
			case generate.StageCreateStage:
				m[id] = generate.CreateStageFunc(".foundry-demo-deadbeef")
			case generate.StageCommit:
				m[id] = generate.CommitStage(generate.OutcomeCommitted, "demo", ".foundry-demo-deadbeef", nil)
			case generate.StageReportPlanNetwork:
				m[id] = func(ctx context.Context, rt *generate.Runtime) error {
					_ = ctx
					rt.NetworkLines = []string{"go mod tidy may use the network"}
					return nil
				}
			default:
				// Nop via omission
			}
		default:
			// Stages after lastOK should not run when we fail earlier; if they
			// do, mark unexpected.
			if failN == 0 {
				continue
			}
		}
	}
	return m
}

func newMachine(t *testing.T, stages map[generate.StageID]generate.StageFunc, sink *generate.CollectingSink, log *generate.RecordingLogger) *generate.Machine {
	t.Helper()
	cfg := generate.Config{
		Stages: stages,
	}
	if sink != nil {
		cfg.Sink = sink
	}
	if log != nil {
		cfg.Log = log
	}
	return generate.New(cfg)
}

func compareEventGolden(t *testing.T, log *testutil.Logger, name string, events []generate.GenerationEvent) {
	t.Helper()
	got := generate.EventSequence(events)
	path := testutil.GoldenPath(testdataDir, name)
	testutil.CompareGolden(t, path, []byte(got))
	log.Step("golden", testutil.OutcomeOK, filepath.Base(path))
}

// cancelBefore returns a context already cancelled.
func cancelBefore() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// cancelAfterStage returns stages that cancel the context when `at` begins,
// after ensuring create-stage has run when at is post-stage.
func cancelAfterStage(at generate.StageID, parent context.Context, cancel context.CancelFunc) map[generate.StageID]generate.StageFunc {
	m := defaultStages()
	orig := m[at]
	m[at] = func(ctx context.Context, rt *generate.Runtime) error {
		cancel()
		if orig != nil {
			return orig(ctx, rt)
		}
		// Cooperative stages observe ctx; commit uses WithoutCancel so this
		// cancel becomes a race classified by commit result.
		if err := ctx.Err(); err != nil && !generate.StageIsCommit(at) {
			return err
		}
		return nil
	}
	_ = parent
	return m
}
