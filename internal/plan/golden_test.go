package plan_test

import (
	"path/filepath"
	"testing"

	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/testutil"
)

// TestGoldenPlans locks package-level plan JSON for Appendix B fixtures and
// MVP profiles=[] shells (acceptance: golden plans).
//
// Update: UPDATE_GOLDEN=1 go test ./internal/plan -run TestGoldenPlans
func TestGoldenPlans(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		load    func(t *testing.T) (specPath string, destAuthored string, observation plan.DestinationObservation)
	}{
		{
			name:    "mvp_minimal_cli",
			fixture: "mvp_minimal_cli",
			load: func(t *testing.T) (string, string, plan.DestinationObservation) {
				vs := minimalCLI(t)
				return "minimal-cli.toml", vs.Destination(), plan.ObservationAbsent
			},
		},
		{
			name:    "mvp_minimal_tui",
			fixture: "mvp_minimal_tui",
			load: func(t *testing.T) (string, string, plan.DestinationObservation) {
				vs := minimalTUI(t)
				return "minimal-tui.toml", vs.Destination(), plan.ObservationAbsent
			},
		},
		{
			name:    "appendix_b_private_cli",
			fixture: "appendix_b_private_cli",
			load: func(t *testing.T) (string, string, plan.DestinationObservation) {
				vs := appendixBPrivateCLI(t)
				return "private-cli.toml", vs.Destination(), plan.ObservationAbsent
			},
		},
		{
			name:    "appendix_b_private_tui",
			fixture: "appendix_b_private_tui",
			load: func(t *testing.T) (string, string, plan.DestinationObservation) {
				vs := appendixBPrivateTUI(t)
				return "private-tui.toml", vs.Destination(), plan.ObservationAbsent
			},
		},
	}

	cat := mustLoadCatalog(t)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := testutil.New(t)
			log.Phase("arrange")
			specPath, destAuthored, obs := tc.load(t)
			log.Inputs(map[string]string{
				"fixture": tc.fixture,
				"spec":    specPath,
				"verify":  "default",
			})
			log.Fixture("golden", tc.fixture+".golden")
			log.PhaseEnd("arrange", testutil.OutcomeOK)

			log.Phase("act")
			// Build through Pipeline when we have ValidatedSpecification.
			var p *plan.Plan
			var err error
			switch tc.name {
			case "mvp_minimal_cli":
				vs := minimalCLI(t)
				opts := defaultPipelineOpts(fixedDestination(destAuthored, obs))
				opts.SpecPath = specPath
				p, err = plan.Pipeline(vs, cat, opts)
			case "mvp_minimal_tui":
				vs := minimalTUI(t)
				opts := defaultPipelineOpts(fixedDestination(destAuthored, obs))
				opts.SpecPath = specPath
				p, err = plan.Pipeline(vs, cat, opts)
			case "appendix_b_private_cli":
				vs := appendixBPrivateCLI(t)
				opts := defaultPipelineOpts(fixedDestination(destAuthored, obs))
				opts.SpecPath = specPath
				// Absolute destinations from Appendix B stay as authored.
				opts.Destination = plan.DestinationInfo{
					Path:        vs.Destination(),
					Parent:      parentOf(vs.Destination()),
					Basename:    baseOf(vs.Destination()),
					Observation: obs,
				}
				p, err = plan.Pipeline(vs, cat, opts)
			case "appendix_b_private_tui":
				vs := appendixBPrivateTUI(t)
				opts := defaultPipelineOpts(fixedDestination(destAuthored, obs))
				opts.SpecPath = specPath
				opts.Destination = plan.DestinationInfo{
					Path:        vs.Destination(),
					Parent:      parentOf(vs.Destination()),
					Basename:    baseOf(vs.Destination()),
					Observation: obs,
				}
				p, err = plan.Pipeline(vs, cat, opts)
			}
			if err != nil {
				log.Fail("pipeline", err.Error())
			}
			log.Step("file_count", testutil.OutcomeOK, "n="+itoa(p.FileCount()))
			log.Step("external_step_count", testutil.OutcomeOK, "n="+itoa(p.ExternalStepCount()))
			log.Step("plan_sha256", testutil.OutcomeOK, p.PlanSHA256())
			log.PhaseEnd("act", testutil.OutcomeOK)

			log.Phase("assert")
			golden := testutil.GoldenPath(filepath.Join("testdata"), tc.fixture)
			got := p.JSON()
			testutil.CompareGolden(t, golden, got)
			if !t.Failed() {
				log.Step("golden_compare", testutil.OutcomeOK, "path="+tc.fixture+".golden")
			} else {
				log.Step("golden_compare", testutil.OutcomeFail, "mismatch path="+tc.fixture)
			}
			log.PhaseEnd("assert", testutil.OutcomeOK)
		})
	}
}

func parentOf(p string) string {
	i := lastSlash(p)
	if i < 0 {
		return "."
	}
	if i == 0 {
		return "/"
	}
	return p[:i]
}

func baseOf(p string) string {
	i := lastSlash(p)
	if i < 0 {
		return p
	}
	return p[i+1:]
}

func lastSlash(p string) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return i
		}
	}
	return -1
}
