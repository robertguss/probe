//go:build unix

package toolrun_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs goleak after the package suite so toolrun cancel-kill and
// bound-starter goroutines must join (short KillGrace in tests; ipk.10).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
