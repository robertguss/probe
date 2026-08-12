package report

// CommitOutcome is the Section 31.9 commit result enum for generate JSON.
// Agents must not parse prose to learn transaction state (REQ-156).
type CommitOutcome string

const (
	// OutcomeCommitted — exclusive rename succeeded (or classified committed).
	OutcomeCommitted CommitOutcome = "committed"
	// OutcomeConflicted — EEXIST; losing stage preserved (exit 2).
	OutcomeConflicted CommitOutcome = "conflicted"
	// OutcomeFailedPreserved — commit failed; verified stage preserved (exit 1).
	OutcomeFailedPreserved CommitOutcome = "failed-preserved"
	// OutcomeAmbiguous — contradictory identities; mutation stopped (exit 1).
	OutcomeAmbiguous CommitOutcome = "ambiguous"
	// OutcomeCancelled — cancelled before commit; stage preserved when created.
	OutcomeCancelled CommitOutcome = "cancelled"
	// OutcomeNotStarted — generate refused before staging (Phase 1 hard-stop).
	OutcomeNotStarted CommitOutcome = "not-started"
)

// GenerateResult is the stable JSON result payload for generate success/partial
// reports (Section 37 / REQ-156). Failure cases use Envelope.Error instead and
// MUST NOT set CommitOutcome to committed together with ok=false (FND-012).
type GenerateResult struct {
	CommitOutcome             CommitOutcome `json:"commit_outcome"`
	Destination               string        `json:"destination,omitempty"`
	StagePath                 string        `json:"stage_path,omitempty"`
	PlanSHA256                string        `json:"plan_sha256,omitempty"`
	ConformanceBaselineDigest string        `json:"conformance_baseline_digest,omitempty"`
	NetworkDisclosure         []string      `json:"network_disclosure,omitempty"`
	NextSteps                 []string      `json:"next_steps,omitempty"`
}

// GenerationEvent is a typed lifecycle unit report can encode without
// performing generation work (Section 29.1 / 43). generate (Phase 2) emits
// equivalent values; report tests use fixtures.
//
// Kind distinguishes progress, state transition, network disclosure, and
// terminal outcomes. Stage and State use the stable name tables in stages.go.
type GenerationEvent struct {
	// Kind is "progress", "state", "network", "summary", or "terminal".
	Kind string `json:"kind"`
	// Stage is a Section 29.2 StageID when Kind is progress.
	Stage StageID `json:"stage,omitempty"`
	// State is a Section 29.1 LifecycleState when Kind is state.
	State LifecycleState `json:"state,omitempty"`
	// Detail is optional human text (redacted by Encoder when emitted).
	Detail string `json:"detail,omitempty"`
	// Lines carries network disclosure lines when Kind is network.
	Lines []string `json:"lines,omitempty"`
	// Outcome is set for terminal events.
	Outcome CommitOutcome `json:"outcome,omitempty"`
	// StagePath is set when a stage is preserved.
	StagePath string `json:"stage_path,omitempty"`
	// Destination is set on committed terminal events.
	Destination string `json:"destination,omitempty"`
}

// ApplyEvent encodes one GenerationEvent according to mode/quiet rules.
// Multi-event streams are the generate progress path; step-logged in tests.
func (e *Encoder) ApplyEvent(ev GenerationEvent) error {
	if e == nil {
		return nil
	}
	switch ev.Kind {
	case "progress":
		if ev.Stage != "" {
			return e.Progress(ev.Stage)
		}
		return nil
	case "network":
		return e.NetworkDisclosure(ev.Lines)
	case "summary":
		return e.Summary(ev.Detail)
	case "state":
		// State transitions surface as progress-style lines using the state name
		// when text mode and not quiet (diffable logs per Section 36.2).
		if e.opts.Mode != ModeText || e.opts.Quiet || ev.State == "" {
			return nil
		}
		return e.Summary("state: " + string(ev.State))
	case "terminal":
		// Terminal encoding is handled by Success/Failure at the CLI boundary;
		// ApplyEvent only records progressive output.
		if ev.StagePath != "" && e.opts.Mode == ModeText {
			// stage_path on terminal failure is Failure's job; for progressive
			// preserve notices before Failure, always emit stage path (quiet
			// must not hide it — pass quiet=false).
			err := WriteTextSuccess(e.stdout, "stage_path: "+ev.StagePath, false)
			return e.absorb(err)
		}
		return nil
	default:
		return nil
	}
}
