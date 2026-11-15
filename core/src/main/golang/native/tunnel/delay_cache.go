package tunnel

import (
	"sync"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

// invalidDelay is the value reported by mihomo for proxies without a usable delay.
const invalidDelay = 0xffff

const defaultTestURL = "https://www.gstatic.com/generate_204"

type delayRecord struct {
	delay int
	at    time.Time
}

// delayCache keeps the latest delay test result of every proxy so it survives
// config reloads (e.g. switching the tunnel mode), which recreate every proxy
// object and drop its live delay history, including the test URL it was
// measured with. Hence entries are keyed by proxy name only.
type delayCache struct {
	mutex   sync.Mutex
	profile string
	entries map[string]delayRecord
}

var delays = &delayCache{entries: map[string]delayRecord{}}

func (c *delayCache) store(name string, record delayRecord) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if old, ok := c.entries[name]; ok && old.at.After(record.at) {
		return
	}

	c.entries[name] = record
}

func (c *delayCache) load(name string) (delayRecord, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	record, ok := c.entries[name]

	return record, ok
}

func (c *delayCache) switchProfile(profile string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.profile == profile {
		return
	}

	c.profile = profile
	c.entries = map[string]delayRecord{}
}

// SwitchDelayProfile drops cached delays when a different profile is loaded.
// Reloading the same profile (mode switch, override change) keeps them.
func SwitchDelayProfile(profile string) {
	delays.switchProfile(profile)
}

// delayTestURL returns the URL a proxy was most recently tested with.
func delayTestURL(p C.Proxy) string {
	url := defaultTestURL

	var latest time.Time

	for k, state := range p.ExtraDelayHistories() {
		if len(k) == 0 || len(state.History) == 0 {
			continue
		}

		if at := state.History[len(state.History)-1].Time; at.After(latest) {
			latest = at
			url = k
		}
	}

	return url
}

// latestRecord returns the newest delay test result of a proxy across every
// test URL, so the value does not depend on which group started the test.
func latestRecord(p C.Proxy) (delayRecord, bool) {
	var (
		best  delayRecord
		found bool
	)

	for k, state := range p.ExtraDelayHistories() {
		if len(k) == 0 || len(state.History) == 0 {
			continue
		}

		last := state.History[len(state.History)-1]
		if found && !last.Time.After(best.at) {
			continue
		}

		delay := int(last.Delay)
		if delay == 0 || !state.Alive {
			delay = invalidDelay
		}

		best = delayRecord{delay: delay, at: last.Time}
		found = true
	}

	return best, found
}

// currentDelay returns the newest known delay of a proxy: the live result when
// it is more recent than the cached one, otherwise the cached one. Proxies that
// were never tested report invalidDelay.
func currentDelay(p C.Proxy) int {
	live, hasLive := latestRecord(p)
	cached, hasCached := delays.load(p.Name())

	switch {
	case hasLive && (!hasCached || !cached.at.After(live.at)):
		delays.store(p.Name(), live)

		return live.delay
	case hasCached:
		return cached.delay
	default:
		return invalidDelay
	}
}

// recordDelays snapshots the results of a finished delay test, failures included.
func recordDelays(proxies []C.Proxy) {
	for _, p := range proxies {
		if record, ok := latestRecord(p); ok {
			delays.store(p.Name(), record)
		}
	}
}
