package sampler

import (
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}

		time.Sleep(2 * time.Millisecond)
	}
}

func TestPollsPeriodicallyWithoutTriggers(t *testing.T) {
	var calls atomic.Int32

	s := New(func() { calls.Add(1) }, 10*time.Millisecond, time.Millisecond)
	defer s.Stop()

	waitFor(t, func() bool { return calls.Load() >= 3 })
}

func TestTriggerSamplesPromptly(t *testing.T) {
	var calls atomic.Int32

	s := New(func() { calls.Add(1) }, time.Hour, time.Millisecond)
	defer s.Stop()

	s.Trigger()

	waitFor(t, func() bool { return calls.Load() >= 1 })
}

func TestBurstOfTriggersIsCoalesced(t *testing.T) {
	var calls atomic.Int32

	s := New(func() { calls.Add(1) }, time.Hour, 50*time.Millisecond)
	defer s.Stop()

	for i := 0; i < 1000; i++ {
		s.Trigger()
	}

	time.Sleep(200 * time.Millisecond)

	if got := calls.Load(); got > 12 {
		t.Fatalf("1000 triggers produced %d samples, want them coalesced", got)
	}

	if calls.Load() == 0 {
		t.Fatal("triggers produced no sample")
	}
}

func TestTriggerAfterBurstStillSamples(t *testing.T) {
	var calls atomic.Int32

	s := New(func() { calls.Add(1) }, time.Hour, 5*time.Millisecond)
	defer s.Stop()

	s.Trigger()
	waitFor(t, func() bool { return calls.Load() >= 2 })

	settled := calls.Load()

	s.Trigger()
	waitFor(t, func() bool { return calls.Load() > settled })
}

func TestTriggerNeverBlocks(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 1)

	s := New(func() {
		select {
		case started <- struct{}{}:
		default:
		}

		<-block
	}, time.Hour, time.Millisecond)

	s.Trigger()
	<-started

	done := make(chan struct{})

	go func() {
		for i := 0; i < 10000; i++ {
			s.Trigger()
		}

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Trigger blocked while a sample was running")
	}

	close(block)
	s.Stop()
}

func TestStopEndsSampling(t *testing.T) {
	var calls atomic.Int32

	s := New(func() { calls.Add(1) }, 5*time.Millisecond, time.Millisecond)

	waitFor(t, func() bool { return calls.Load() >= 1 })
	s.Stop()

	before := calls.Load()

	time.Sleep(60 * time.Millisecond)

	if calls.Load() > before+1 {
		t.Fatalf("sampling continued after Stop: %d -> %d", before, calls.Load())
	}
}

func TestStopIsIdempotentAndTriggerAfterStopIsSafe(t *testing.T) {
	s := New(func() {}, time.Hour, time.Millisecond)

	s.Stop()
	s.Stop()
	s.Trigger()
}

func TestPanickingSampleDoesNotKillTheLoop(t *testing.T) {
	var calls atomic.Int32

	s := New(func() {
		if calls.Add(1) == 1 {
			panic("boom")
		}
	}, 5*time.Millisecond, time.Millisecond)
	defer s.Stop()

	waitFor(t, func() bool { return calls.Load() >= 3 })
}

func TestTriggerSamplesAgainAfterTheGap(t *testing.T) {
	var calls atomic.Int32

	s := New(func() { calls.Add(1) }, time.Hour, 20*time.Millisecond)
	defer s.Stop()

	s.Trigger()

	// The first sample may run before the event that caused the trigger has
	// finished taking effect, so a second one follows after the gap.
	waitFor(t, func() bool { return calls.Load() == 2 })

	time.Sleep(80 * time.Millisecond)

	if got := calls.Load(); got != 2 {
		t.Fatalf("a single trigger caused %d samples, want exactly 2", got)
	}
}
