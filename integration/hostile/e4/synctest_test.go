//go:build unix

package e4

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// TestEffectTracker_WaitUntilIdle_Synctest proves WaitUntilIdle is driven by
// the fake clock under testing/synctest (ipk.15 readiness path) — not only
// wall-clock Sleep polls outside a bubble.
func TestEffectTracker_WaitUntilIdle_Synctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tracker := NewEffectTracker()
		tracker.Start("effect-a")
		go func() {
			// Block until bubble advances; then finish so WaitUntilIdle unblocks.
			time.Sleep(20 * time.Millisecond)
			tracker.Finish("effect-a")
		}()
		// Long wall timeout is fake-time; Finish after 20ms advances the bubble.
		if !tracker.WaitUntilIdle(time.Second) {
			t.Fatalf("WaitUntilIdle: still active=%d after Finish", tracker.Active())
		}
		active, started, finished := tracker.Snapshot()
		if active != 0 || started != 1 || finished != 1 {
			t.Fatalf("snapshot active=%d started=%d finished=%d", active, started, finished)
		}
	})
}

// TestCooperativeSleep_Cancel_Synctest proves CooperativeSleep respects ctx
// cancel under synctest without waiting for the full timer (ipk.15).
func TestCooperativeSleep_Cancel_Synctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		tracker := NewEffectTracker()
		done := make(chan effectDoneMsg, 1)
		go func() {
			msg := CooperativeSleep(ctx, tracker, "coop", time.Hour)()
			if m, ok := msg.(effectDoneMsg); ok {
				done <- m
				return
			}
			t.Errorf("unexpected msg type %T", msg)
			close(done)
		}()
		// Let the cmd reach select on timer/ctx.
		time.Sleep(1 * time.Millisecond)
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case m := <-done:
			if !m.Cancelled {
				t.Fatalf("want Cancelled=true, got %+v", m)
			}
			if m.Name != "coop" {
				t.Fatalf("name: %q", m.Name)
			}
		default:
			t.Fatal("CooperativeSleep did not return after cancel")
		}
		if tracker.Active() != 0 {
			t.Fatalf("tracker still active=%d", tracker.Active())
		}
	})
}
