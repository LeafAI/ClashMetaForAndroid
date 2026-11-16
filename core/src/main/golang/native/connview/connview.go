// Package connview turns a snapshot of the core's tracked connections into the
// flat, filterable list shown on the Connections page.
//
// The package has no mihomo dependency so it can be unit tested on the host.
package connview

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// SortMode selects the order of Query results.
type SortMode int

const (
	// SortNewest puts the most recently started connection first.
	SortNewest SortMode = iota
	// SortDownload puts the connection with the most downloaded bytes first.
	SortDownload
	// SortUpload puts the connection with the most uploaded bytes first.
	SortUpload
)

// Entry is one tracked connection.
type Entry struct {
	ID            string    `json:"id"`
	Network       string    `json:"network"`
	Host          string    `json:"host"`
	DestinationIP string    `json:"destinationIP"`
	Port          uint16    `json:"port"`
	Process       string    `json:"process"`
	Rule          string    `json:"rule"`
	RulePayload   string    `json:"rulePayload"`
	Chains        []string  `json:"chains"`
	Upload        int64     `json:"upload"`
	Download      int64     `json:"download"`
	Start         time.Time `json:"start"`
	// Active is false once the core no longer tracks the connection.
	Active bool `json:"active"`
	// End is when the connection was first seen closed; zero while active.
	End time.Time `json:"end"`
}

// Target is the host name if known, otherwise the destination IP.
func (e Entry) Target() string {
	if e.Host != "" {
		return e.Host
	}

	return e.DestinationIP
}

// RuleLabel renders the matched rule as TYPE(PAYLOAD), or TYPE when the rule has no payload.
func (e Entry) RuleLabel() string {
	if e.Rule == "" || e.RulePayload == "" {
		return e.Rule
	}

	return e.Rule + "(" + e.RulePayload + ")"
}

// Outbound is the proxy that finally carries the connection. mihomo lists chains
// from the outermost proxy back to the group that was selected, so it is the first element.
func (e Entry) Outbound() string {
	if len(e.Chains) == 0 {
		return ""
	}

	return e.Chains[0]
}

// Options controls Query.
type Options struct {
	// Filter keeps entries whose host, IP, rule, chain or process contains it,
	// ignoring case. Blank means no filtering.
	Filter string
	Sort   SortMode
	// ActiveOnly drops connections that have already closed.
	ActiveOnly bool
	// Limit caps the result length; zero or negative means no limit.
	Limit int
}

// Query filters and sorts entries. The input slice is left untouched.
func Query(entries []Entry, options Options) []Entry {
	needle := strings.ToLower(strings.TrimSpace(options.Filter))

	result := make([]Entry, 0, len(entries))

	for _, entry := range entries {
		if options.ActiveOnly && !entry.Active {
			continue
		}

		if needle == "" || matches(entry, needle) {
			result = append(result, entry)
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		return less(result[i], result[j], options.Sort)
	})

	if options.Limit > 0 && len(result) > options.Limit {
		result = result[:options.Limit]
	}

	return result
}

func less(a, b Entry, mode SortMode) bool {
	switch mode {
	case SortDownload:
		return a.Download > b.Download
	case SortUpload:
		return a.Upload > b.Upload
	default:
		return a.Start.After(b.Start)
	}
}

func matches(entry Entry, needle string) bool {
	fields := []string{entry.Host, entry.DestinationIP, entry.Process, entry.Rule, entry.RulePayload, entry.Network}
	fields = append(fields, entry.Chains...)

	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}

	return false
}

// DefaultHistoryCapacity is used when NewHistory gets a non-positive capacity.
const DefaultHistoryCapacity = 1000

// History remembers every connection the core has tracked since it was created,
// including those that already closed. The core only lists live connections, so
// History is fed periodic snapshots: a connection that was in an earlier snapshot
// but is missing from the latest one is marked closed and its last seen traffic
// is kept.
//
// History is safe for concurrent use.
type History struct {
	capacity int

	mu    sync.Mutex
	order []string
	items map[string]Entry
}

// NewHistory returns an empty History that keeps at most capacity connections.
// Only closed connections are ever evicted, oldest first; live ones are always kept.
func NewHistory(capacity int) *History {
	if capacity <= 0 {
		capacity = DefaultHistoryCapacity
	}

	return &History{capacity: capacity, items: map[string]Entry{}}
}

// Observe merges a snapshot of the currently live connections taken at now.
func (h *History) Observe(live []Entry, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	seen := make(map[string]struct{}, len(live))

	for _, entry := range live {
		seen[entry.ID] = struct{}{}

		entry.Active = true
		entry.End = time.Time{}

		if _, known := h.items[entry.ID]; !known {
			h.order = append(h.order, entry.ID)
		}

		h.items[entry.ID] = entry
	}

	for id, entry := range h.items {
		if _, ok := seen[id]; ok || !entry.Active {
			continue
		}

		entry.Active = false
		entry.End = now
		h.items[id] = entry
	}

	h.evictLocked()
}

func (h *History) evictLocked() {
	excess := len(h.items) - h.capacity
	if excess <= 0 {
		return
	}

	kept := h.order[:0]

	for _, id := range h.order {
		if excess > 0 && !h.items[id].Active {
			delete(h.items, id)

			excess--

			continue
		}

		kept = append(kept, id)
	}

	h.order = kept
}

// Entries returns a copy of everything remembered, in the order first seen.
func (h *History) Entries() []Entry {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]Entry, 0, len(h.order))

	for _, id := range h.order {
		entry := h.items[id]
		entry.Chains = append([]string(nil), entry.Chains...)

		out = append(out, entry)
	}

	return out
}

// ClearClosed forgets connections that have already closed.
func (h *History) ClearClosed() {
	h.mu.Lock()
	defer h.mu.Unlock()

	kept := h.order[:0]

	for _, id := range h.order {
		if h.items[id].Active {
			kept = append(kept, id)

			continue
		}

		delete(h.items, id)
	}

	h.order = kept
}

// Reset forgets everything, live connections included.
func (h *History) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.order = nil
	h.items = map[string]Entry{}
}
