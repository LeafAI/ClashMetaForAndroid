// Package sampler runs a function periodically and on demand.
//
// It lets the connection recorder sample the core's live connections right when
// a connection is reported, instead of waiting for the next tick, so short lived
// connections are not missed. Bursts of triggers are coalesced so a flood of new
// connections costs a bounded number of samples.
package sampler

import (
	"sync"
	"time"
)

// Sampler calls a function every interval, and shortly after Trigger.
type Sampler struct {
	fn       func()
	interval time.Duration
	gap      time.Duration

	trigger  chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
}

// New starts a Sampler. fn runs every interval. A Trigger runs it immediately and
// once more after gap, because the event that caused the trigger often becomes
// visible just after it. Triggers arriving meanwhile are coalesced. fn is never
// run concurrently with itself.
func New(fn func(), interval, gap time.Duration) *Sampler {
	s := &Sampler{
		fn:       fn,
		interval: interval,
		gap:      gap,
		trigger:  make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}

	go s.loop()

	return s
}

// Trigger asks for a sample soon. It never blocks and may be called after Stop.
func (s *Sampler) Trigger() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Stop ends the sampling loop. It is safe to call more than once.
func (s *Sampler) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

func (s *Sampler) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.run()
		case <-s.trigger:
			s.run()

			if !s.pause() {
				return
			}

			// The caller usually triggers just before its change becomes visible,
			// so sample once more after the gap.
			s.run()
		}
	}
}

// pause waits out the minimum gap so a burst of triggers is coalesced into one
// sample per gap. It reports false if the sampler was stopped meanwhile.
func (s *Sampler) pause() bool {
	timer := time.NewTimer(s.gap)
	defer timer.Stop()

	select {
	case <-s.stop:
		return false
	case <-timer.C:
		return true
	}
}

func (s *Sampler) run() {
	defer func() { _ = recover() }()

	s.fn()
}
