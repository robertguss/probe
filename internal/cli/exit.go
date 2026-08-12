package cli

import (
	"fmt"
)

// exitError carries a process exit code when the command has already encoded
// its report (success or failure). cli.Run returns Code without re-encoding.
//
// Used by generate so transaction/commit exit (0/1/2/130) and stage_path
// reporting stay under generate's control while still flowing through cobra.
type exitError struct {
	code int
}

func (e *exitError) Error() string {
	if e == nil {
		return "exit"
	}
	return fmt.Sprintf("exit %d", e.code)
}

// exitWith returns an error that makes cli.Run exit with code without
// writing another report document.
func exitWith(code int) error {
	return &exitError{code: code}
}

// asExitError extracts *exitError from err.
func asExitError(err error) (*exitError, bool) {
	if err == nil {
		return nil, false
	}
	if ee, ok := err.(*exitError); ok {
		return ee, true
	}
	return nil, false
}
