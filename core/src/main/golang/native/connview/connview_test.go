package connview

import (
	"reflect"
	"testing"
	"time"
)

func sample() []Entry {
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	return []Entry{
		{ID: "a", Host: "www.example.com", Port: 443, Network: "tcp", Rule: "DomainSuffix", RulePayload: "example.com",
			Chains: []string{"node-b", "🚀 节点选择"}, Upload: 10, Download: 1000, Start: base.Add(3 * time.Second)},
		{ID: "b", Host: "api.github.com", Port: 443, Network: "tcp", Rule: "Match", RulePayload: "",
			Chains: []string{"DIRECT"}, Upload: 5, Download: 50, Start: base.Add(1 * time.Second), Process: "com.android.chrome"},
		{ID: "c", Host: "", DestinationIP: "1.1.1.1", Port: 53, Network: "udp", Rule: "GeoIP", RulePayload: "CN",
			Chains: []string{"DIRECT"}, Upload: 1, Download: 2, Start: base.Add(2 * time.Second)},
	}
}

func ids(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}

	return out
}

func TestSortNewestFirstByDefault(t *testing.T) {
	got := ids(Query(sample(), Options{}))

	if want := []string{"a", "c", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSortByDownload(t *testing.T) {
	got := ids(Query(sample(), Options{Sort: SortDownload}))

	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSortIsStableForEqualKeys(t *testing.T) {
	entries := []Entry{
		{ID: "x", Download: 1}, {ID: "y", Download: 1}, {ID: "z", Download: 1},
	}

	got := ids(Query(entries, Options{Sort: SortDownload}))

	if want := []string{"x", "y", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestFilterMatchesHostRuleChainProcessAndIP(t *testing.T) {
	cases := map[string][]string{
		"example":      {"a"},
		"EXAMPLE":      {"a"},
		"geoip":        {"c"},
		"node-b":       {"a"},
		"节点":           {"a"},
		"chrome":       {"b"},
		"1.1.1.1":      {"c"},
		"direct":       {"c", "b"},
		"  github  ":   {"b"},
		"cn":           {"c"},
		"nothing-here": {},
	}

	for query, want := range cases {
		got := ids(Query(sample(), Options{Filter: query}))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("filter %q = %v, want %v", query, got, want)
		}
	}
}

func TestBlankFilterKeepsEverything(t *testing.T) {
	if got := len(Query(sample(), Options{Filter: "   "})); got != 3 {
		t.Fatalf("got %d entries, want 3", got)
	}
}

func TestQueryDoesNotMutateInput(t *testing.T) {
	in := sample()
	before := ids(in)

	Query(in, Options{Sort: SortDownload})

	if !reflect.DeepEqual(ids(in), before) {
		t.Fatalf("input reordered: %v", ids(in))
	}
}

func TestLimit(t *testing.T) {
	got := Query(sample(), Options{Limit: 2})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestDisplayHostFallsBackToIP(t *testing.T) {
	if got := (Entry{Host: "a.com", DestinationIP: "1.1.1.1"}).Target(); got != "a.com" {
		t.Fatalf("Target = %q", got)
	}

	if got := (Entry{DestinationIP: "1.1.1.1"}).Target(); got != "1.1.1.1" {
		t.Fatalf("Target = %q", got)
	}
}

func TestRuleLabel(t *testing.T) {
	cases := []struct {
		rule, payload, want string
	}{
		{"DomainSuffix", "example.com", "DomainSuffix(example.com)"},
		{"Match", "", "Match"},
		{"", "", ""},
	}

	for _, c := range cases {
		if got := (Entry{Rule: c.rule, RulePayload: c.payload}).RuleLabel(); got != c.want {
			t.Errorf("RuleLabel(%q,%q) = %q, want %q", c.rule, c.payload, got, c.want)
		}
	}
}

func TestOutboundIsFirstChainElement(t *testing.T) {
	if got := (Entry{Chains: []string{"node-b", "group"}}).Outbound(); got != "node-b" {
		t.Fatalf("Outbound = %q, want node-b", got)
	}

	if got := (Entry{}).Outbound(); got != "" {
		t.Fatalf("Outbound = %q, want empty", got)
	}
}

func at(sec int) time.Time {
	return time.Date(2026, 11, 15, 10, 0, sec, 0, time.UTC)
}

func live(id string, down int64, start int) Entry {
	return Entry{ID: id, Host: id + ".com", Port: 443, Download: down, Start: at(start)}
}

func byID(entries []Entry) map[string]Entry {
	out := make(map[string]Entry, len(entries))
	for _, e := range entries {
		out[e.ID] = e
	}

	return out
}

func TestHistoryRecordsNewConnectionsAsActive(t *testing.T) {
	h := NewHistory(10)

	h.Observe([]Entry{live("a", 1, 1), live("b", 2, 2)}, at(5))

	got := byID(h.Entries())
	if len(got) != 2 || !got["a"].Active || !got["b"].Active {
		t.Fatalf("entries = %+v", got)
	}

	if !got["a"].End.IsZero() {
		t.Fatalf("active entry has an end time: %v", got["a"].End)
	}
}

func TestHistoryKeepsClosedConnections(t *testing.T) {
	h := NewHistory(10)

	h.Observe([]Entry{live("a", 1, 1), live("b", 2, 2)}, at(5))
	h.Observe([]Entry{live("b", 9, 2)}, at(7))

	got := byID(h.Entries())
	if len(got) != 2 {
		t.Fatalf("closed connection dropped: %+v", got)
	}

	if got["a"].Active || !got["a"].End.Equal(at(7)) {
		t.Fatalf("a should be closed at 7s: %+v", got["a"])
	}

	if !got["b"].Active || got["b"].Download != 9 {
		t.Fatalf("b should be active with updated traffic: %+v", got["b"])
	}
}

func TestHistoryClosedEntryKeepsLastSeenTraffic(t *testing.T) {
	h := NewHistory(10)

	h.Observe([]Entry{live("a", 100, 1)}, at(2))
	h.Observe([]Entry{live("a", 250, 1)}, at(3))
	h.Observe(nil, at(4))

	got := byID(h.Entries())["a"]
	if got.Active || got.Download != 250 {
		t.Fatalf("a = %+v, want closed with last seen 250", got)
	}
}

func TestHistoryDoesNotReopenClosedConnection(t *testing.T) {
	h := NewHistory(10)

	h.Observe([]Entry{live("a", 1, 1)}, at(2))
	h.Observe(nil, at(3))
	h.Observe(nil, at(4))

	if got := byID(h.Entries())["a"]; !got.End.Equal(at(3)) {
		t.Fatalf("end time moved on later observations: %v", got.End)
	}
}

func TestHistoryEvictsOldestClosedFirst(t *testing.T) {
	h := NewHistory(3)

	h.Observe([]Entry{live("a", 0, 1), live("b", 0, 2), live("c", 0, 3)}, at(4))
	h.Observe([]Entry{live("c", 0, 3)}, at(5))
	h.Observe([]Entry{live("c", 0, 3), live("d", 0, 6)}, at(7))

	got := byID(h.Entries())
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}

	if _, ok := got["a"]; ok {
		t.Fatalf("oldest closed connection a should be evicted: %+v", got)
	}

	if !got["c"].Active || !got["d"].Active {
		t.Fatalf("active connections must stay: %+v", got)
	}
}

func TestHistoryNeverEvictsActiveConnections(t *testing.T) {
	h := NewHistory(2)

	h.Observe([]Entry{live("a", 0, 1), live("b", 0, 2), live("c", 0, 3)}, at(4))

	got := byID(h.Entries())
	if len(got) != 3 {
		t.Fatalf("active connections were evicted: %+v", got)
	}
}

func TestHistoryRemoveAndClear(t *testing.T) {
	h := NewHistory(10)

	h.Observe([]Entry{live("a", 0, 1), live("b", 0, 2)}, at(3))
	h.Observe([]Entry{live("b", 0, 2)}, at(4))

	h.ClearClosed()

	got := byID(h.Entries())
	if len(got) != 1 || !got["b"].Active {
		t.Fatalf("ClearClosed should keep only active entries: %+v", got)
	}

	h.Reset()

	if len(h.Entries()) != 0 {
		t.Fatal("Reset should drop everything")
	}
}

func TestHistoryEntriesAreACopy(t *testing.T) {
	h := NewHistory(10)

	h.Observe([]Entry{live("a", 1, 1)}, at(2))

	out := h.Entries()
	out[0].Download = 999

	if h.Entries()[0].Download == 999 {
		t.Fatal("Entries exposed internal storage")
	}
}

func TestHistoryZeroCapacityUsesDefault(t *testing.T) {
	h := NewHistory(0)

	h.Observe([]Entry{live("a", 0, 1)}, at(2))

	if len(h.Entries()) != 1 {
		t.Fatal("zero capacity should fall back to a usable default")
	}
}

func TestQueryHonoursActiveOnly(t *testing.T) {
	entries := []Entry{
		{ID: "a", Active: true, Start: at(1)},
		{ID: "b", Active: false, Start: at(2)},
	}

	got := ids(Query(entries, Options{ActiveOnly: true}))
	if want := []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveOnly = %v, want %v", got, want)
	}

	if got := len(Query(entries, Options{})); got != 2 {
		t.Fatalf("without ActiveOnly got %d, want 2", got)
	}
}
