package generate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// StageFunc executes one Section 29.2 stage. It must not encode user output.
// Returning nil advances the machine; returning an error triggers the stage's
// failure policy. Cancel is observed via ctx (and re-checked by the machine
// before/after each stage, except commit where the commit result dominates).
type StageFunc func(ctx context.Context, rt *Runtime) error

// Runtime is the mutable per-run workspace shared with stage functions.
// Stage functions set StagePath when they create a stage and CommitOutcome
// when they classify commit.
type Runtime struct {
	// StageExists is true once stage 9 has successfully created a stage
	// (or a failure after partial create marks preserve).
	StageExists bool
	// StagePath is the preserved stage location (basename-safe in tests).
	StagePath string
	// Destination is the committed destination basename/path for reports.
	Destination string
	// CommitOutcome is set by the commit stage (Section 31.9).
	CommitOutcome CommitOutcome
	// NetworkLines may be set by report-plan-network for EventNetwork.
	NetworkLines []string
	// NetworkStepIDs are the disclosed network:may step ids (host-independent).
	NetworkStepIDs []string
	// VerifyMode is the plan verify mode used for disclosure ("default"/"strict").
	VerifyMode string
	// Meta is an opaque bag for orchestrators (not used by the machine core).
	Meta map[string]any
}

// Config configures a Machine. Missing stage funcs default to no-ops that
// simulate success (unit tests inject failures/cancel via funcs and ctx).
type Config struct {
	// Stages maps StageID → implementation. Unset stages use NopStage.
	Stages map[StageID]StageFunc
	// Sink receives GenerationEvents (nil → NopSink).
	Sink EventSink
	// Log receives step logs (nil → NopLogger).
	Log StepLogger
	// InitialState is the lifecycle state before stage 1 (default: empty,
	// then planned after stage 7). Prefer leaving zero for full runs.
	InitialState LifecycleState
}

// Machine is the total Section 29 generate state machine.
//
// It is not safe for concurrent use. One Run per Machine.
type Machine struct {
	cfg    Config
	state  LifecycleState
	rt     *Runtime
	events []GenerationEvent
	// inCommit is true while stage 18 is executing.
	inCommit bool
	// finished is true after Run returns (prevents double-run).
	finished bool
	// lastStage is the most recently entered stage.
	lastStage StageID
}

// New constructs a Machine. Does not start execution.
func New(cfg Config) *Machine {
	if cfg.Sink == nil {
		cfg.Sink = NopSink{}
	}
	if cfg.Log == nil {
		cfg.Log = NopLogger{}
	}
	if cfg.Stages == nil {
		cfg.Stages = map[StageID]StageFunc{}
	}
	return &Machine{
		cfg:   cfg,
		state: cfg.InitialState,
		rt: &Runtime{
			Meta: map[string]any{},
		},
	}
}

// State returns the current lifecycle state.
func (m *Machine) State() LifecycleState {
	if m == nil {
		return ""
	}
	return m.state
}

// Runtime returns the shared runtime (for tests/orchestration inspection).
func (m *Machine) Runtime() *Runtime {
	if m == nil {
		return nil
	}
	return m.rt
}

// Events returns a copy of emitted events so far.
func (m *Machine) Events() []GenerationEvent {
	if m == nil {
		return nil
	}
	out := make([]GenerationEvent, len(m.events))
	copy(out, m.events)
	return out
}

// Transition attempts a lifecycle state change. Rejects skip/reorder (REQ-123).
// On success, emits a state GenerationEvent and updates m.state.
func (m *Machine) Transition(to LifecycleState) error {
	if m == nil {
		return errors.New("generate: nil machine")
	}
	from := m.state
	// First transition may establish planned from empty.
	if from == "" && to == LifePlanned {
		m.setState(to, "bootstrap")
		return nil
	}
	if err := MustTransition(from, to); err != nil {
		logStep(m.cfg.Log, string(from), "transition:"+string(to), "reject", 0)
		return err
	}
	m.setState(to, "transition")
	return nil
}

// Run executes all Section 29.2 stages in order. It is the sole entry for a
// full generate lifecycle. Stage functions are invoked with ctx; cancel is
// classified per REQ-036.
//
// GenerationEvent shape invariants consumed by report are enforced by
// TestGenerationEventShapeContract in contract_test.go.
func (m *Machine) Run(ctx context.Context) Result {
	if m == nil {
		return Result{
			State:   LifeFailedPreserved,
			Outcome: OutcomeFailedPreserved,
			Exit:    diagnostic.ExitFailure,
			Err:     errors.New("generate: nil machine"),
		}
	}
	if m.finished {
		return Result{
			State:   m.state,
			Outcome: OutcomeFromLifecycle(m.state),
			Exit:    diagnostic.ExitFailure,
			Err:     errors.New("generate: machine already finished"),
			Events:  m.Events(),
		}
	}
	defer func() { m.finished = true }()

	if ctx == nil {
		ctx = context.Background()
	}

	stages := AllStageIDs()
	for _, id := range stages {
		// Pre-stage cancel check (not during commit — handled inside commit).
		if !StageIsCommit(id) {
			if err := ctx.Err(); err != nil {
				return m.finishCancel(id, false)
			}
		}

		if res, done := m.runStage(ctx, id); done {
			return res
		}
	}

	// Full success through report: state should be a commit terminal.
	return m.finishSuccess()
}

// runStage executes one stage. done=true means Run should return res.
func (m *Machine) runStage(ctx context.Context, id StageID) (res Result, done bool) {
	start := time.Now()
	m.lastStage = id

	// Enter committing state immediately before commit work (Section 29.1).
	if StageIsCommit(id) {
		m.inCommit = true
		if m.state != LifeCommitting {
			// Legal predecessors: git-initialized or conformed (git optional path).
			if err := m.Transition(LifeCommitting); err != nil {
				// Force path if tests left state unexpected — still attempt commit
				// only when legal; otherwise fail-preserve.
				logStep(m.cfg.Log, string(m.state), string(id), "illegal_pre_commit", 0)
				return m.finishFailure(id, &StageError{
					Exit:     diagnostic.ExitFailure,
					Preserve: m.rt.StageExists,
					Err:      err,
					Detail:   "illegal state before commit",
				}, start), true
			}
		}
	}

	m.emit(GenerationEvent{Kind: EventProgress, Stage: id, State: m.state})
	logStep(m.cfg.Log, string(m.state), "progress:"+string(id), "start", 0)

	fn := m.cfg.Stages[id]
	if fn == nil {
		fn = NopStage
	}

	// Commit stage: do not abandon for cancel — commit result dominates.
	var runCtx context.Context
	if StageIsCommit(id) {
		runCtx = context.WithoutCancel(ctx)
	} else {
		runCtx = ctx
	}

	err := fn(runCtx, m.rt)
	dur := time.Since(start).Milliseconds()

	if StageIsCommit(id) {
		m.inCommit = false
		// Parent ctx may be cancelled while commit ran under WithoutCancel.
		raced := ctx.Err() != nil
		return m.finishCommit(err, dur, raced)
	}

	if err != nil {
		// Cancel errors from stage funcs (cooperative stop).
		if diagnostic.IsCancelled(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logStep(m.cfg.Log, string(m.state), string(id), "cancel", dur)
			return m.finishCancel(id, true), true
		}
		logStep(m.cfg.Log, string(m.state), string(id), "fail", dur)
		return m.finishFailure(id, err, start), true
	}

	// Success path: apply lifecycle completion for this stage.
	if next := stageCompletesState(id); next != "" && next != m.state {
		if m.state == "" && next == LifePlanned {
			_ = m.Transition(LifePlanned)
		} else if err := m.Transition(next); err != nil {
			// Completing a stage into an illegal state is an internal bug.
			logStep(m.cfg.Log, string(m.state), string(id), "illegal_transition", dur)
			return m.finishFailure(id, err, start), true
		}
	}

	// Network disclosure event after report-plan-network when lines present.
	// Ordering: stage 7 (this) completes before acquire-parent / create-stage.
	if id == StageReportPlanNetwork && len(m.rt.NetworkLines) > 0 {
		m.emit(GenerationEvent{
			Kind:   EventNetwork,
			Stage:  id,
			State:  m.state,
			Lines:  append([]string(nil), m.rt.NetworkLines...),
			Detail: "network-disclosure",
		})
		// Step logger: disclosed step ids + verify mode (REQ-034 / Section 13.5).
		mode := m.rt.VerifyMode
		if mode == "" {
			mode = "default"
		}
		ids := strings.Join(m.rt.NetworkStepIDs, ",")
		logStep(m.cfg.Log, string(m.state), "network-disclosure",
			fmt.Sprintf("verify=%s steps=%s", mode, ids), 0)
	}

	// Mark stage existence after successful create-stage.
	if id == StageCreateStage {
		m.rt.StageExists = true
		if m.rt.StagePath == "" {
			m.rt.StagePath = DefaultStagePath
		}
	}

	logStep(m.cfg.Log, string(m.state), string(id), "ok", dur)

	// Post-stage cancel check (before next stage). Commit already handled.
	if err := ctx.Err(); err != nil {
		return m.finishCancel(id, true), true
	}
	return Result{}, false
}

func (m *Machine) finishCommit(err error, dur int64, cancelRaced bool) (Result, bool) {
	// Determine outcome from runtime or error. Cancel concurrent with stage 18
	// is classified solely by the commit result (REQ-036 / Section 29.2).
	outcome := m.rt.CommitOutcome
	if se, ok := AsStageError(err); ok && se.Outcome != "" {
		outcome = se.Outcome
	}
	if outcome == "" {
		if err == nil {
			outcome = OutcomeCommitted
		} else {
			outcome = OutcomeFailedPreserved
		}
	}

	term := LifecycleFromCommitOutcome(outcome)
	if trErr := MustTransition(LifeCommitting, term); trErr != nil {
		// Should not happen for the four Section 31.9 terminals; force preserve.
		term = LifeFailedPreserved
		outcome = OutcomeFailedPreserved
	}
	m.setState(term, "commit")

	exit := commitExit(outcome)
	preserve := outcome != OutcomeCommitted
	stagePath := ""
	dest := ""
	if preserve {
		stagePath = m.rt.StagePath
		if stagePath == "" {
			stagePath = DefaultStagePath
		}
		m.rt.StageExists = true
	} else {
		dest = m.rt.Destination
		if dest == "" {
			dest = "destination"
		}
	}

	res := Result{
		State:           term,
		Outcome:         outcome,
		Exit:            exit,
		StagePreserved:  preserve,
		StagePath:       stagePath,
		Destination:     dest,
		FailedStage:     StageCommit,
		Err:             err,
		CommitDominated: cancelRaced,
	}
	if outcome == OutcomeCommitted {
		res.FailedStage = ""
		res.Err = nil
	}

	m.emitTerminal(res)
	// Stage 19: best-effort report after commit. Post-commit report failure
	// never changes the exit code (FND-012 / Sections 31.9, 36.5).
	_ = m.runReportBestEffort(context.Background(), res)
	logStep(m.cfg.Log, string(m.state), string(StageCommit), resultLabel(outcome), dur)
	res.Events = m.Events()
	return res, true
}

func (m *Machine) runReportBestEffort(ctx context.Context, prior Result) int64 {
	start := time.Now()
	id := StageReport
	m.lastStage = id
	m.emit(GenerationEvent{Kind: EventProgress, Stage: id, State: m.state})
	fn := m.cfg.Stages[id]
	if fn == nil {
		fn = NopStage
	}
	// Report must not change terminal outcome; ignore cancel for emission.
	err := fn(context.WithoutCancel(ctx), m.rt)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		// Post-commit stream/report failure: keep prior exit (FND-012 / 36.5).
		// Classify and log phase/stream/errno/exit so operators can see the
		// absorbed decision without a debugger (REQ-158 acceptance).
		sf := ClassifyStreamFailure(
			StreamPostCommit,
			false,
			prior.Outcome,
			StreamStdout,
			"",
			err,
		)
		LogStreamDecision(m.cfg.Log, sf)
		logStep(m.cfg.Log, string(m.state), string(id), "report_fail_absorbed", dur)
		m.emit(GenerationEvent{
			Kind:    EventSummary,
			Stage:   id,
			State:   m.state,
			Detail:  "report-failed-absorbed",
			Outcome: prior.Outcome,
		})
	} else {
		logStep(m.cfg.Log, string(m.state), string(id), "ok", dur)
	}
	return dur
}

func (m *Machine) finishFailure(id StageID, err error, start time.Time) Result {
	_ = start
	// Preservation is solely about stage existence (Section 31.6 / REQ-130).
	// Stages 1–8 leave nothing on disk; 9+ preserve when create-stage succeeded.
	preserve := m.rt.StageExists

	exit := DefaultFailureExit(id)
	outcome := OutcomeNotStarted
	if preserve {
		outcome = OutcomeFailedPreserved
		if m.state != "" && !IsTerminal(m.state) {
			if LegalTransition(m.state, LifeFailedPreserved) {
				m.setState(LifeFailedPreserved, "failure")
			} else {
				// Direct state mutation bypassing LegalTransition: error recovery
				// must reach a terminal state even when the current state lacks a
				// legal edge to LifeFailedPreserved. REQ-123 (no skip/reorder)
				// governs the happy path; failure recovery is a forced terminal.
				m.state = LifeFailedPreserved
				m.emit(GenerationEvent{Kind: EventState, State: LifeFailedPreserved, Detail: "failure"})
			}
		} else if m.state == "" {
			m.state = LifeFailedPreserved
			m.emit(GenerationEvent{Kind: EventState, State: LifeFailedPreserved, Detail: "failure"})
		}
	} else {
		// No stage: keep last non-terminal state (or planned) with not-started.
		if m.state == "" {
			m.state = LifePlanned
		}
		outcome = OutcomeNotStarted
	}

	if se, ok := AsStageError(err); ok {
		if se.Exit != 0 {
			exit = se.Exit
		}
		if se.Outcome != "" {
			outcome = se.Outcome
		}
		if se.Preserve && m.rt.StageExists {
			preserve = true
			outcome = OutcomeFailedPreserved
			if !IsTerminal(m.state) {
				// Forced terminal — see comment above re: REQ-123.
				m.state = LifeFailedPreserved
				m.emit(GenerationEvent{Kind: EventState, State: LifeFailedPreserved, Detail: "failure"})
			}
		}
	}
	if fe, ok := diagnostic.AsFoundryError(err); ok {
		exit = fe.ExitCode()
	}

	stagePath := ""
	if preserve {
		stagePath = m.rt.StagePath
		if stagePath == "" {
			stagePath = DefaultStagePath
		}
	}

	res := Result{
		State:          m.state,
		Outcome:        outcome,
		Exit:           exit,
		StagePreserved: preserve,
		StagePath:      stagePath,
		FailedStage:    id,
		Err:            err,
	}
	m.emitTerminal(res)
	res.Events = m.Events()
	return res
}

func (m *Machine) finishCancel(at StageID, afterStageBody bool) Result {
	_ = afterStageBody
	class := ClassifyCancel(m.rt.StageExists, m.inCommit)
	if class == CancelRacingCommit {
		// Should not reach here for commit (commit path handles domination).
		// If we do, still prefer preserve semantics without inventing commit.
		class = CancelAfterStage
		if !m.rt.StageExists {
			class = CancelBeforeStage
		}
	}

	// Transition to cancelled when legal.
	if m.state != "" && LegalTransition(m.state, LifeCancelled) {
		m.setState(LifeCancelled, "cancel")
	} else {
		m.state = LifeCancelled
		m.emit(GenerationEvent{Kind: EventState, State: LifeCancelled, Detail: "cancel"})
	}

	res := ApplyCancel(class, m.rt.StagePath, at, nil)
	if res.StagePreserved && res.StagePath == "" {
		res.StagePath = DefaultStagePath
	}
	m.emitTerminal(res)
	res.Events = m.Events()
	logStep(m.cfg.Log, string(m.state), "cancel:"+string(at), string(class), 0)
	return res
}

func (m *Machine) finishSuccess() Result {
	// If commit+report already finished via finishCommit, Run should have
	// returned. finishSuccess covers the path where all stages returned
	// without done — e.g. if commit stage was a no-op that set outcome.
	outcome := m.rt.CommitOutcome
	if outcome == "" {
		outcome = OutcomeCommitted
	}
	term := LifecycleFromCommitOutcome(outcome)
	if !IsTerminal(m.state) {
		if m.state == LifeCommitting || m.state == LifeGitInitialized || m.state == LifeConformed {
			_ = m.Transition(LifeCommitting)
			if err := MustTransition(LifeCommitting, term); err == nil {
				m.setState(term, "success")
			} else {
				m.state = term
				m.emit(GenerationEvent{Kind: EventState, State: term, Detail: "success"})
			}
		} else if !IsTerminal(m.state) {
			m.state = term
			m.emit(GenerationEvent{Kind: EventState, State: term, Detail: "success"})
		}
	}
	res := Result{
		State:       m.state,
		Outcome:     outcome,
		Exit:        commitExit(outcome),
		Destination: m.rt.Destination,
	}
	if outcome == OutcomeCommitted && res.Destination == "" {
		res.Destination = "destination"
	}
	if outcome != OutcomeCommitted {
		res.StagePreserved = true
		res.StagePath = m.rt.StagePath
	}
	// Avoid double terminal if already emitted.
	if !m.hasTerminal() {
		m.emitTerminal(res)
	}
	res.Events = m.Events()
	return res
}

func (m *Machine) hasTerminal() bool {
	for _, ev := range m.events {
		if ev.Kind == EventTerminal {
			return true
		}
	}
	return false
}

func (m *Machine) setState(to LifecycleState, detail string) {
	m.state = to
	m.emit(GenerationEvent{
		Kind:   EventState,
		State:  to,
		Detail: detail,
	})
	logStep(m.cfg.Log, string(to), "state:"+string(to), "ok", 0)
}

func (m *Machine) emit(ev GenerationEvent) {
	m.events = append(m.events, ev)
	if m.cfg.Sink != nil {
		m.cfg.Sink.OnEvent(ev)
	}
}

func (m *Machine) emitTerminal(res Result) {
	m.emit(GenerationEvent{
		Kind:        EventTerminal,
		State:       res.State,
		Outcome:     res.Outcome,
		StagePath:   res.StagePath,
		Destination: res.Destination,
		Detail:      "terminal",
	})
}

// NopStage is a successful no-op stage function.
func NopStage(ctx context.Context, rt *Runtime) error {
	_ = ctx
	_ = rt
	return nil
}

// FailStage returns a StageFunc that always fails with se (or a default error).
func FailStage(se *StageError) StageFunc {
	return func(ctx context.Context, rt *Runtime) error {
		_ = ctx
		_ = rt
		if se == nil {
			return &StageError{Exit: diagnostic.ExitFailure, Detail: "injected failure"}
		}
		return se
	}
}

// CommitStage returns a StageFunc that sets CommitOutcome and optional error.
func CommitStage(outcome CommitOutcome, dest, stagePath string, err error) StageFunc {
	return func(ctx context.Context, rt *Runtime) error {
		_ = ctx
		rt.CommitOutcome = outcome
		if dest != "" {
			rt.Destination = dest
		}
		if stagePath != "" {
			rt.StagePath = stagePath
			rt.StageExists = true
		}
		if outcome != OutcomeCommitted {
			rt.StageExists = true
			if rt.StagePath == "" {
				rt.StagePath = stagePath
				if rt.StagePath == "" {
					rt.StagePath = DefaultStagePath
				}
			}
		}
		return err
	}
}

// CreateStageFunc marks the runtime as having a stage at path.
func CreateStageFunc(path string) StageFunc {
	return func(ctx context.Context, rt *Runtime) error {
		_ = ctx
		rt.StageExists = true
		rt.StagePath = path
		if rt.StagePath == "" {
			rt.StagePath = DefaultStagePath
		}
		return nil
	}
}

func commitExit(o CommitOutcome) int {
	switch o {
	case OutcomeCommitted:
		return diagnostic.ExitSuccess
	case OutcomeConflicted:
		return diagnostic.ExitUsage // exit 2 per Section 31.9
	case OutcomeCancelled:
		return diagnostic.ExitCancelled
	default:
		return diagnostic.ExitFailure
	}
}

func resultLabel(o CommitOutcome) string {
	if o == "" {
		return "ok"
	}
	return string(o)
}

// AssertNoEncoding is a compile-time / test helper surface: the generate
// package must not own text/JSON report encoding. Kept as documentation
// for arch/static checks in tests.
func AssertNoEncoding() string {
	return "report owns encoding"
}

// IllegalTransitionProbe attempts from→to and returns the error (for tests).
func IllegalTransitionProbe(from, to LifecycleState) error {
	m := New(Config{InitialState: from})
	return m.Transition(to)
}

// FormatMachineDebug returns a host-independent debug summary (tests/logs).
func FormatMachineDebug(m *Machine) string {
	if m == nil {
		return "machine=nil"
	}
	return fmt.Sprintf("state=%s stage_exists=%v last=%s events=%d",
		m.state, m.rt.StageExists, m.lastStage, len(m.events))
}
