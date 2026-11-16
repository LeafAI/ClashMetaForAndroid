package tunnel

import (
	"errors"
	"os"
	"time"

	C "github.com/metacubex/mihomo/constant"

	"cfa/native/delaycache"
)

const defaultTestURL = "https://www.gstatic.com/generate_204"

const delayCacheFile = "delays.json"

const delayCacheDebounce = 2 * time.Second

var delays = delaycache.New(func() string {
	return C.Path.Resolve(delayCacheFile)
}, delayCacheDebounce)

// SwitchDelayProfile makes profile the active one for cached delays. Reloading
// the same profile (mode switch, override change) keeps them, loading another
// profile replaces them with the ones saved for it, if any.
func SwitchDelayProfile(profile string) error {
	return delays.SwitchProfile(profile)
}

// PruneDelays forgets the cached delays of profiles whose directory no longer
// exists, such as deleted profiles. The profile that is loaded is always kept.
func PruneDelays() {
	delays.Prune(func(profile string) bool {
		_, err := os.Stat(profile)

		return err == nil || !errors.Is(err, os.ErrNotExist)
	})
}

// FlushDelays writes cached delays to disk immediately.
func FlushDelays() error {
	return delays.Flush()
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
func latestRecord(p C.Proxy) (delaycache.Record, bool) {
	var (
		best  delaycache.Record
		found bool
	)

	for k, state := range p.ExtraDelayHistories() {
		if len(k) == 0 || len(state.History) == 0 {
			continue
		}

		last := state.History[len(state.History)-1]
		if found && !last.Time.After(best.At) {
			continue
		}

		delay := int(last.Delay)
		if delay == 0 || !state.Alive {
			delay = delaycache.Invalid
		}

		best = delaycache.Record{Delay: delay, At: last.Time}
		found = true
	}

	return best, found
}

// currentDelay returns the delay to display for a proxy and whether a test of
// it is still in progress.
func currentDelay(p C.Proxy) (delay int, pending bool) {
	live, hasLive := latestRecord(p)

	return delays.Resolve(p.Name(), live, hasLive)
}

// recordDelays snapshots the results of a finished delay test, failures included.
func recordDelays(proxies []C.Proxy) {
	for _, p := range proxies {
		if record, ok := latestRecord(p); ok {
			delays.Store(p.Name(), record)
		}
	}
}

func proxyNames(proxies []C.Proxy) []string {
	names := make([]string, 0, len(proxies))

	for _, p := range proxies {
		names = append(names, p.Name())
	}

	return names
}
