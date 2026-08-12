package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/robertguss/go-foundry-cli/internal/diagnostic"
)

// ANSI codes for optional diagnostic decoration (stderr only; Section 36.1).
const (
	ansiReset = "\033[0m"
	ansiRed   = "\033[31m"
	ansiBold  = "\033[1m"
)

// WriteTextError writes a human diagnostic to stderr (or w). Format:
//
//	foundry: <id>: <message> (at <location>)
//	  remediation: <sentence>
//	  stage_path: <path>          // when preserved
//
// Never emits a stack dump as the primary message. Secret-like content is
// already redacted inside FoundryError / ErrorFrom.
func WriteTextError(w io.Writer, err error, stagePath string, color bool) {
	if w == nil || err == nil {
		return
	}
	obj := ErrorFrom(err, stagePath)
	writeTextErrorObject(w, obj, color)
}

func writeTextErrorObject(w io.Writer, obj *ErrorObject, color bool) {
	if w == nil || obj == nil {
		return
	}
	var b strings.Builder
	b.WriteString("foundry: ")
	if obj.ErrorID != "" {
		b.WriteString(obj.ErrorID)
		if obj.Message != "" {
			b.WriteString(": ")
			b.WriteString(obj.Message)
		}
	} else if obj.Message != "" {
		b.WriteString(obj.Message)
	} else {
		b.WriteString("error")
	}
	if obj.Location != "" {
		b.WriteString(" (at ")
		b.WriteString(obj.Location)
		b.WriteString(")")
	} else if obj.Path != "" && obj.Line > 0 {
		b.WriteString(" (at ")
		b.WriteString(obj.Path)
		if obj.Line > 0 {
			b.WriteByte(':')
			b.WriteString(fmt.Sprintf("%d", obj.Line))
			if obj.Col > 0 {
				b.WriteByte(':')
				b.WriteString(fmt.Sprintf("%d", obj.Col))
			}
		}
		b.WriteString(")")
	}
	line := b.String()
	if color {
		fmt.Fprintf(w, "%s%s%s%s\n", ansiBold, ansiRed, line, ansiReset)
	} else {
		fmt.Fprintln(w, line)
	}
	if obj.Remediation != "" {
		fmt.Fprintf(w, "  remediation: %s\n", obj.Remediation)
	}
	// stage_path is never suppressed by quiet (Section 36.3) — callers pass
	// stagePath even in quiet mode.
	if obj.StagePath != "" {
		fmt.Fprintf(w, "  stage_path: %s\n", obj.StagePath)
	}
}

// WriteTextSuccess writes a non-empty summary line to stdout when quiet is
// false. Empty text is a no-op.
func WriteTextSuccess(w io.Writer, text string, quiet bool) error {
	if w == nil || quiet || text == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, text)
	return err
}

// WriteProgress writes a stable progress line to stdout (text mode only).
// Suppressed when quiet is true. Format: ProgressLine(id).
func WriteProgress(w io.Writer, id StageID, quiet bool) error {
	if w == nil || quiet {
		return nil
	}
	_, err := fmt.Fprintln(w, ProgressLine(id))
	return err
}

// WriteNetworkDisclosure writes network disclosure lines (Section 13.5 / 36.2).
// Never suppressed by quiet — agents and humans must see network-capable steps
// before staging regardless of verbosity.
func WriteNetworkDisclosure(w io.Writer, lines []string) error {
	if w == nil || len(lines) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "network disclosure:"); err != nil {
		return err
	}
	for _, ln := range lines {
		ln = diagnostic.Redact(strings.TrimSpace(ln))
		if ln == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s\n", ln); err != nil {
			return err
		}
	}
	return nil
}

// WriteVerbose writes a verbose diagnostic detail line to stderr.
// No-op when verbose is false.
func WriteVerbose(w io.Writer, detail string, verbose bool) error {
	if w == nil || !verbose || detail == "" {
		return nil
	}
	_, err := fmt.Fprintf(w, "verbose: %s\n", diagnostic.Redact(detail))
	return err
}

// WritePostCommitStreamNotice is the best-effort stderr notice when post-commit
// stdout reporting fails (Section 36.4). Exit code remains 0 at the CLI.
func WritePostCommitStreamNotice(w io.Writer, destination string) {
	if w == nil {
		return
	}
	dest := diagnostic.Redact(destination)
	if dest == "" {
		fmt.Fprintln(w, "foundry: generation succeeded; report stream failed after commit")
		return
	}
	fmt.Fprintf(w, "foundry: generation succeeded at %s; report stream failed after commit\n", dest)
}
