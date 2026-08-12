// Package knownrace is a deliberate data-race fixture for E3 / REQ-217.
//
// Arm with FOUNDRY_KNOWN_RACE=1, then:
//   - `go test -race` → detector MUST report DATA RACE
//   - `go test`       → typically passes (uninstrumented)
//
// Without the env var the test skips so `go test -race ./...` stays green.
// Promote this package into Foundry CI as the known-race fixture without rewrite.
package knownrace

// EnvKnownRace arms the intentional data-race body in TestKnownDataRace.
// E3 / CI race-proof jobs set this to "1" and expect DATA RACE + non-zero exit.
const EnvKnownRace = "FOUNDRY_KNOWN_RACE"

// RacyCounter has an intentional concurrent write without synchronization.
// Do not "fix" this — it exists so race instrumentation can be proven active.
type RacyCounter struct {
	n int
}

// Inc races: two goroutines write n without a mutex or atomic.
func (c *RacyCounter) Inc() {
	done := make(chan struct{}, 2)
	go func() {
		c.n = c.n + 1
		done <- struct{}{}
	}()
	go func() {
		c.n = c.n + 1
		done <- struct{}{}
	}()
	<-done
	<-done
}

// Value returns the (racy) counter value.
func (c *RacyCounter) Value() int { return c.n }
