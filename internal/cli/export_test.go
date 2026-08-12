package cli

import (
	"context"
	"io"
	"os"

	"github.com/robertguss/go-foundry-cli/internal/generate"
	"github.com/robertguss/go-foundry-cli/internal/plan"
	"github.com/robertguss/go-foundry-cli/internal/report"
	"github.com/spf13/cobra"
)

// ExitErrorString exposes exitError.Error for unit tests (nil + non-nil).
func ExitErrorString(code int, nilRecv bool) string {
	if nilRecv {
		var e *exitError
		return e.Error()
	}
	return (&exitError{code: code}).Error()
}

// AsExitErrorForTest exposes asExitError.
func AsExitErrorForTest(err error) (code int, ok bool) {
	ee, ok := asExitError(err)
	if !ok || ee == nil {
		return 0, ok
	}
	return ee.code, true
}

// ExitWithForTest exposes exitWith.
func ExitWithForTest(code int) error {
	return exitWith(code)
}

// IsTerminalForTest exposes isTerminal.
func IsTerminalForTest(f *os.File) bool {
	return isTerminal(f)
}

// NewEncoderForTest exposes newEncoder.
func NewEncoderForTest(stdout, stderr io.Writer, g *globalFlags) *report.Encoder {
	return newEncoder(stdout, stderr, g)
}

// NewGlobalFlagsForTest builds globalFlags for encoder edge tests.
func NewGlobalFlagsForTest(output, color string, quiet, verbose bool) *globalFlags {
	return &globalFlags{Output: output, Color: color, Quiet: quiet, Verbose: verbose}
}

// GeneratePipelineHostForTest exposes generatePipelineHost on a root built from opts.
func GeneratePipelineHostForTest(opts Options, gitInit bool) PipelineHost {
	r := &root{opts: opts}
	return r.generatePipelineHost(gitInit)
}

// PipelineHostForTest exposes pipelineHost.
func PipelineHostForTest(opts Options, gitInit bool) PipelineHost {
	r := &root{opts: opts}
	return r.pipelineHost(gitInit)
}

// RedactDestForLogForTest exposes redactDestForLog.
func RedactDestForLogForTest(path string) string {
	return redactDestForLog(path)
}

// NetworkStepIDsNilForTest hits the nil-plan branch of networkStepIDs.
func NetworkStepIDsNilForTest() []string {
	return networkStepIDs(nil)
}

// GenerateNextStepsNilForTest hits the nil-plan branch of generateNextSteps.
func GenerateNextStepsNilForTest() []string {
	return generateNextSteps(nil)
}

// GenerateNextStepsForTest exposes generateNextSteps for a constructed plan.
func GenerateNextStepsForTest(p *plan.Plan) []string {
	return generateNextSteps(p)
}

// FormatGenerateSuccessForTest exposes formatGenerateSuccess.
func FormatGenerateSuccessForTest(r report.GenerateResult) string {
	return formatGenerateSuccess(r, true)
}

// ReportSinkOnEventForTest exercises reportSink.OnEvent including nil receiver.
func ReportSinkOnEventForTest(enc *report.Encoder, ev generate.GenerationEvent) {
	var s *reportSink
	s.OnEvent(ev) // nil receiver
	s = &reportSink{enc: enc}
	s.OnEvent(ev)
	s2 := &reportSink{enc: nil}
	s2.OnEvent(ev)
}

// WorkingDirForTest exposes root.workingDir.
func WorkingDirForTest(opts Options) (string, error) {
	r := &root{opts: opts}
	return r.workingDir()
}

// ReadFileForTest exposes root.readFile.
func ReadFileForTest(opts Options, path string) ([]byte, error) {
	r := &root{opts: opts}
	return r.readFile(path)
}

// ObserveDestinationAtForTest exposes observeDestinationAt.
func ObserveDestinationAtForTest(authored, wd string) (plan.DestinationInfo, error) {
	return observeDestinationAt(authored, wd)
}

// DestLexicalBaseForTest exposes destLexicalBase.
func DestLexicalBaseForTest(dest string) (base, errMsg string) {
	return destLexicalBase(dest)
}

// FlagChangedForTest exposes flagChanged.
func FlagChangedForTest(cmd *cobra.Command, name string) bool {
	return flagChanged(cmd, name)
}

// NormalizeCLIErrorForTest exposes normalizeCLIError.
func NormalizeCLIErrorForTest(err error) error {
	return normalizeCLIError(err)
}

// CheckCancelledForTest exposes checkCancelled.
func CheckCancelledForTest(ctx context.Context) error {
	return checkCancelled(ctx)
}

// RequireSpecForTest exposes requireSpec.
func RequireSpecForTest(spec string) error {
	return requireSpec(spec)
}

// LoadCatalogForTest exposes root.loadCatalog.
func LoadCatalogForTest(opts Options) error {
	r := newRootState(opts)
	_, err := r.loadCatalog()
	return err
}

// VersionInfoForTest exposes root.versionInfo.
func VersionInfoForTest(opts Options) (version string, goVer string, digest string) {
	r := newRootState(opts)
	info := r.versionInfo()
	return info.Version, info.Go, info.CatalogDigest
}

// EmptyDashForTest exposes emptyDash.
func EmptyDashForTest(s string) string {
	return emptyDash(s)
}
