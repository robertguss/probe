package generate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// StreamPhase locates a stream failure relative to the exclusive rename
// (Section 36.5 / FND-012 / REQ-158).
type StreamPhase string

const (
	// StreamPreCommit — required output stream failed before commit; blocks
	// placement, preserves any created stage, exit 1 (report.failed).
	StreamPreCommit StreamPhase = "pre-commit"
	// StreamPostCommit — report stream failed after successful or classified
	// commit; absorbed so the commit exit dominates (Section 31.9 / 36.4).
	StreamPostCommit StreamPhase = "post-commit"
)

// StreamName identifies which user-facing stream failed.
type StreamName string

const (
	StreamStdout StreamName = "stdout"
	StreamStderr StreamName = "stderr"
)

// StreamFailure is the Section 36.5 classification of a broken output stream.
//
// Pre-commit: BlockPlacement, exit 1, preserve when a stage exists.
// Post-commit: Absorb, exit = DominatedExit(commit outcome), never flips
// a success (or any commit-class exit) based on the report channel.
type StreamFailure struct {
	Phase          StreamPhase
	Stream         StreamName
	Errno          string // host-independent token, e.g. "EPIPE"
	Exit           int
	Absorb         bool // true when post-commit: do not change process exit
	Preserve       bool // true when pre-commit and a stage exists
	BlockPlacement bool // true when pre-commit: exclusive rename must not run
	Detail         string
}

// ClassifyStreamFailure implements Section 36.5 / FND-012 stream policy.
//
//	phase=pre-commit  → exit 1, block placement, preserve iff stageExists
//	phase=post-commit → absorb; exit = DominatedExit(commitOutcome, false)
//
// errno should be a stable token (EPIPE, EIO, ENOSPC, unknown). Empty errno
// defaults to "EPIPE" when err looks like a broken pipe, else "unknown".
func ClassifyStreamFailure(phase StreamPhase, stageExists bool, commitOutcome CommitOutcome, stream StreamName, errno string, err error) StreamFailure {
	if stream == "" {
		stream = StreamStdout
	}
	if errno == "" {
		errno = ErrnoToken(err)
	}
	sf := StreamFailure{
		Phase:  phase,
		Stream: stream,
		Errno:  errno,
	}
	switch phase {
	case StreamPostCommit:
		// Commit result always dominates reporting (Section 31.9).
		sf.Absorb = true
		sf.BlockPlacement = false
		sf.Preserve = false // destination already placed or stage already preserved by commit class
		sf.Exit = DominatedExit(commitOutcome, false)
		sf.Detail = fmt.Sprintf("phase=%s stream=%s errno=%s exit=%d absorbed=true",
			phase, stream, errno, sf.Exit)
	default:
		// Pre-commit (and unknown) — treat as report.failed class.
		sf.Absorb = false
		sf.BlockPlacement = true
		sf.Preserve = stageExists
		sf.Exit = diagnostic.ExitFailure
		sf.Detail = fmt.Sprintf("phase=%s stream=%s errno=%s exit=%d absorbed=false preserve=%v block=true",
			phase, stream, errno, sf.Exit, stageExists)
	}
	return sf
}

// DominatedExit returns the process exit code for a commit outcome when the
// report stream may have succeeded or failed. Report failure NEVER changes
// the exit (FND-012 / Section 31.9): the commit result alone decides.
//
// reportOK is accepted for matrix completeness and logging; it is intentionally
// unused in the exit computation.
func DominatedExit(outcome CommitOutcome, reportOK bool) int {
	_ = reportOK
	return commitExit(outcome)
}

// StreamFailStageError builds a StageError for a pre-commit stream failure
// (report.failed / exit 1). Post-commit callers should ClassifyStreamFailure
// and absorb rather than return this as a terminal machine failure.
func StreamFailStageError(stream StreamName, err error) *StageError {
	errno := ErrnoToken(err)
	msg := fmt.Sprintf("required output stream %s failed (%s)", stream, errno)
	fe := diagnostic.New(
		diagnostic.IDReportFailed,
		msg,
		diagnostic.Location{},
	)
	if err != nil {
		fe = diagnostic.Wrap(
			diagnostic.IDReportFailed,
			msg,
			diagnostic.Location{},
			err,
		)
	}
	return &StageError{
		Exit:     diagnostic.ExitFailure,
		Preserve: true, // honor when runtime.StageExists
		Err:      fe,
		Detail:   fmt.Sprintf("stream=%s errno=%s phase=%s", stream, errno, StreamPreCommit),
	}
}

// ErrnoToken maps a write error to a stable, host-independent errno token for
// step logs and goldens (never raw platform numbers alone).
func ErrnoToken(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, os.ErrClosed) {
		return "EPIPE"
	}
	// Some stdlib paths wrap broken-pipe as path errors / plain text.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "broken pipe"):
		return "EPIPE"
	case strings.Contains(msg, "epipe"):
		return "EPIPE"
	case errors.Is(err, syscall.EIO) || strings.Contains(msg, "input/output"):
		return "EIO"
	case errors.Is(err, syscall.ENOSPC) || strings.Contains(msg, "no space"):
		return "ENOSPC"
	case errors.Is(err, io.ErrClosedPipe):
		return "EPIPE"
	default:
		return "unknown"
	}
}

// IsBrokenPipe reports whether err is a closed/broken pipe class failure.
func IsBrokenPipe(err error) bool {
	return ErrnoToken(err) == "EPIPE"
}

// LogStreamDecision records phase, stream, errno, and chosen exit on the
// step logger (acceptance: step logs phase/stream/exit).
func LogStreamDecision(log StepLogger, sf StreamFailure) {
	if log == nil {
		return
	}
	result := fmt.Sprintf("errno=%s exit=%d absorbed=%v preserve=%v",
		sf.Errno, sf.Exit, sf.Absorb, sf.Preserve)
	logStep(log, string(sf.Phase), "stream:"+string(sf.Stream), result, 0)
}

// NeverCommittedErrorJSON is a documentation/test sentinel: JSON must never
// encode ok=false together with commit_outcome=committed (FND-012).
// Callers that would emit that lie must rewrite to success + stderr notice.
func NeverCommittedErrorJSON(outcome CommitOutcome, ok bool) bool {
	// Returns true when the combination is the forbidden lie.
	return !ok && outcome == OutcomeCommitted
}
