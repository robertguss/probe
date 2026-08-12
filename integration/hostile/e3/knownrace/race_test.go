package knownrace

import (
	"os"
	"testing"
)

// TestKnownDataRace is the permanent known-race fixture (REQ-217).
//
// Expected outcomes when FOUNDRY_KNOWN_RACE=1:
//   - `go test -race`  → fails with "DATA RACE" (instrumentation active)
//   - `go test`        → typically passes (no instrumentation)
//
// Parent E3 probes run this package as a subprocess with the env armed.
func TestKnownDataRace(t *testing.T) {
	if os.Getenv(EnvKnownRace) != "1" {
		t.Skip("intentional data-race fixture; set FOUNDRY_KNOWN_RACE=1 to arm (E3/CI only)")
	}
	// Many iterations raise the probability of an uninstrumented race
	// surviving, and guarantee the detector sees concurrent writes under -race.
	for i := 0; i < 100; i++ {
		var c RacyCounter
		c.Inc()
		_ = c.Value()
	}
}

// TestRacyCounterSmoke exercises Inc/Value without arming the race detector
// so default coverage runs include this fixture package.
func TestRacyCounterSmoke(t *testing.T) {
	var c RacyCounter
	c.Inc()
	_ = c.Value()
}
