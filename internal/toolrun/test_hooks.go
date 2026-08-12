package toolrun

// NewStepResultForTest constructs a StepResult with an injected error for
// cross-package unit tests (e.g. gitinit mapStepFailure cause branches).
// Production code must use Executor.Run; this hook exists so tests do not
// poke unexported fields via reflect/unsafe (go-foundry-cli-ipk.9).
func NewStepResultForTest(exit int, class string, stdout, stderr []byte, err error) StepResult {
	return StepResult{
		ExitCode:  exit,
		FailClass: class,
		Stdout:    stdout,
		Stderr:    stderr,
		err:       err,
	}
}
