package verify

import (
	"strings"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/plan"
)

// Check and step identifiers (plan.verification.checks + Appendix E).
const (
	CheckGofmt            = "gofmt"
	CheckModuleMutation   = "module-mutation"
	CheckGoModVerify      = "go-mod-verify"
	CheckGoTest           = "go-test"
	CheckGoVet            = "go-vet"
	CheckGoStaticcheck    = "go-staticcheck"
	CheckGoGovulncheck    = "go-govulncheck"
	CheckFinalConformance = "final-conformance"

	// StepGoModTidy is the Appendix E tidy step (mutates go.mod/go.sum).
	StepGoModTidy = "go-mod-tidy"
)

// Kind classifies a verification step.
type Kind string

const (
	// KindInProc is an in-process check (no subprocess).
	KindInProc Kind = "inproc"
	// KindExternal is a toolrun subprocess step.
	KindExternal Kind = "external"
)

// Step is one ordered verification unit (Section 35 / Appendix E).
//
// Race is intentionally absent: IsRace is always false for generation-gate
// steps and ContainsRace audits the list (FND-004 / Section 35.4).
type Step struct {
	// ID is the check or Appendix E step id.
	ID string
	// Kind is inproc or external.
	Kind Kind
	// Argv is the planned argv for external steps (basename at [0]), or a
	// human description for in-process checks.
	Argv []string
	// Description is a short log label.
	Description string
	// When is "Always" or "Strict".
	When string
	// Timeout is the plan-declared timeout for external steps; 0 for in-proc.
	Timeout time.Duration
	// NetworkMay is true when the step may require network (disclosure).
	NetworkMay bool
	// Mutates lists relative paths the step is allowed to change (tidy only).
	Mutates []string
	// IsRace must be false for all generation-gate steps.
	IsRace bool
	// AfterToolConform is true when non-.git conformance must re-run after
	// this external step succeeds (FND-006).
	AfterToolConform bool
}

// DefaultSteps is Section 35.1 default verification including tidy and
// in-process checks, in execution order.
func DefaultSteps() []Step {
	return []Step{
		{
			ID: StepGoModTidy, Kind: KindExternal,
			// No -mod flag: go mod tidy rejects -mod=… (Go 1.16+). See plan.buildExternalSteps.
			Argv:        []string{"go", "mod", "tidy"},
			Description: "go mod tidy (network-may; mutates go.mod/go.sum only)",
			When:        "Always", Timeout: 600 * time.Second, NetworkMay: true,
			Mutates: []string{"go.mod", "go.sum"},
		},
		{
			ID: CheckModuleMutation, Kind: KindInProc,
			Argv:        []string{"in-process", "tidy-mutation-set+pin-reparse"},
			Description: "validate go.mod/go.sum only + exact pins; no replace/exclude",
			When:        "Always",
		},
		{
			ID: CheckGofmt, Kind: KindInProc,
			Argv:        []string{"in-process", "go/format-idempotence"},
			Description: "gofmt conformance of every staged .go file",
			When:        "Always",
		},
		{
			ID: CheckGoModVerify, Kind: KindExternal,
			Argv:        []string{"go", "mod", "verify"},
			Description: "go mod verify",
			When:        "Always", Timeout: 120 * time.Second,
			AfterToolConform: true,
		},
		{
			ID: CheckGoTest, Kind: KindExternal,
			Argv:        []string{"go", "test", "-count=1", "-buildvcs=false", "-mod=readonly", "./..."},
			Description: "go test uncached (-count=1)",
			When:        "Always", Timeout: 300 * time.Second,
			AfterToolConform: true,
		},
		{
			ID: CheckGoVet, Kind: KindExternal,
			Argv:        []string{"go", "vet", "-buildvcs=false", "-mod=readonly", "./..."},
			Description: "go vet",
			When:        "Always", Timeout: 300 * time.Second,
			AfterToolConform: true,
		},
		{
			ID: CheckFinalConformance, Kind: KindInProc,
			Argv:        []string{"in-process", "non-.git-tree-byte-compare"},
			Description: "final non-.git path/type/mode/byte conformance",
			When:        "Always",
		},
	}
}

// StrictSteps is Section 35.2: default plus staticcheck and govulncheck,
// still excluding race. Strict tools run before final-conformance.
func StrictSteps() []Step {
	base := DefaultSteps()
	// Insert strict tools before final-conformance.
	out := make([]Step, 0, len(base)+2)
	for _, s := range base {
		if s.ID == CheckFinalConformance {
			out = append(out,
				Step{
					ID: CheckGoStaticcheck, Kind: KindExternal,
					Argv:        []string{"go", "tool", "staticcheck", "./..."},
					Description: "go tool staticcheck",
					When:        "Strict", Timeout: 300 * time.Second,
					AfterToolConform: true,
				},
				Step{
					ID: CheckGoGovulncheck, Kind: KindExternal,
					Argv:        []string{"go", "tool", "govulncheck", "./..."},
					Description: "go tool govulncheck (network-may)",
					When:        "Strict", Timeout: 600 * time.Second, NetworkMay: true,
					AfterToolConform: true,
				},
			)
		}
		out = append(out, s)
	}
	return out
}

// StepsFor returns the ordered generation-gate step list for mode.
func StepsFor(mode Mode) []Step {
	switch mode.Normalize() {
	case ModeStrict:
		return StrictSteps()
	default:
		return DefaultSteps()
	}
}

// CheckNames returns plan.verification.checks-compatible ordered names for mode.
// Matches plan.buildVerification (gofmt first, final-conformance last; tidy is
// not a named check — it is an external_steps entry).
func CheckNames(mode Mode) []string {
	checks := []string{
		CheckGofmt,
		CheckModuleMutation,
		CheckGoModVerify,
		CheckGoTest,
		CheckGoVet,
	}
	if mode.Normalize() == ModeStrict {
		checks = append(checks, CheckGoStaticcheck, CheckGoGovulncheck)
	}
	checks = append(checks, CheckFinalConformance)
	return checks
}

// ExternalArgv returns the planned full argv for a known external step id,
// or nil when unknown / in-process.
func ExternalArgv(stepID string) []string {
	for _, s := range StrictSteps() {
		if s.ID == stepID && s.Kind == KindExternal {
			out := make([]string, len(s.Argv))
			copy(out, s.Argv)
			return out
		}
	}
	return nil
}

// ExternalArgs returns argv[1:] for toolrun.StepRequest (binary basename separate).
func ExternalArgs(stepID string) []string {
	argv := ExternalArgv(stepID)
	if len(argv) <= 1 {
		return nil
	}
	out := make([]string, len(argv)-1)
	copy(out, argv[1:])
	return out
}

// TimeoutFor returns the plan-declared timeout for stepID, or 0 if unknown.
func TimeoutFor(stepID string) time.Duration {
	for _, s := range StrictSteps() {
		if s.ID == stepID {
			return s.Timeout
		}
	}
	return 0
}

// ContainsRace reports whether any step is a race job or mentions -race in
// argv/description. Spec requires this to be false for generation gates
// (Section 35.4 / FND-004).
func ContainsRace(steps []Step) bool {
	for _, s := range steps {
		if s.IsRace {
			return true
		}
		if s.ID == "go-test-race" || s.ID == "go-race" || s.ID == "race" {
			return true
		}
		joined := strings.ToLower(strings.Join(s.Argv, " ") + " " + s.Description)
		if strings.Contains(joined, "-race") || strings.Contains(joined, "race detector") {
			return true
		}
	}
	return false
}

// HasCountOne reports whether the go-test step includes -count=1 (FND-006
// cached-test bypass defense).
func HasCountOne(steps []Step) bool {
	for _, s := range steps {
		if s.ID != CheckGoTest {
			continue
		}
		for _, a := range s.Argv {
			if a == "-count=1" {
				return true
			}
		}
	}
	return false
}

// PlanExternalSteps builds plan.ExternalStep values for the external verify
// steps (including tidy) matching plan.buildExternalSteps shape. Env and
// Binary must be filled by the caller (generate/plan).
func PlanExternalSteps(mode Mode) []plan.ExternalStep {
	var out []plan.ExternalStep
	for _, s := range StepsFor(mode) {
		if s.Kind != KindExternal {
			continue
		}
		net := plan.NetworkNo
		if s.NetworkMay {
			net = plan.NetworkMay
		}
		mut := append([]string(nil), s.Mutates...)
		if mut == nil {
			mut = []string{}
		}
		out = append(out, plan.ExternalStep{
			ID:             s.ID,
			Binary:         "go",
			Argv:           append([]string(nil), s.Argv...),
			Cwd:            plan.StageDescriptorCWD,
			Mutates:        mut,
			Network:        net,
			TimeoutS:       int(s.Timeout / time.Second),
			OutputCapBytes: plan.OutputCapBytes,
		})
	}
	return out
}
