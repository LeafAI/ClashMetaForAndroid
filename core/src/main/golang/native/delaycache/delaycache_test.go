package delaycache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// newTestCache returns a cache with a profile already chosen, which is the state
// the app is in once a config has been loaded.
func newTestCache(t *testing.T) (*Cache, string) {
	t.Helper()

	file := filepath.Join(t.TempDir(), "delays.json")
	c := New(func() string { return file }, 10*time.Millisecond)

	if err := c.SwitchProfile("test"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	return c, file
}

func rec(delay int, at time.Time) Record {
	return Record{Delay: delay, At: at}
}

func TestStoreKeepsNewestRecord(t *testing.T) {
	c, _ := newTestCache(t)
	now := time.Now()

	c.Store("a", rec(100, now))
	c.Store("a", rec(999, now.Add(-time.Minute)))

	got, ok := c.Load("a")
	if !ok || got.Delay != 100 {
		t.Fatalf("Load = %+v, %v; want delay 100", got, ok)
	}

	c.Store("a", rec(50, now.Add(time.Minute)))

	if got, _ := c.Load("a"); got.Delay != 50 {
		t.Fatalf("newer record lost: %+v", got)
	}
}

func TestLoadMissing(t *testing.T) {
	c, _ := newTestCache(t)

	if _, ok := c.Load("nope"); ok {
		t.Fatal("Load of missing name reported ok")
	}
}

func TestResolvePrefersNewerLiveRecord(t *testing.T) {
	c, _ := newTestCache(t)
	now := time.Now()

	c.Store("a", rec(100, now))

	delay, pending := c.Resolve("a", rec(30, now.Add(time.Second)), true)
	if delay != 30 || pending {
		t.Fatalf("Resolve = %d, %v; want 30, false", delay, pending)
	}

	if got, _ := c.Load("a"); got.Delay != 30 {
		t.Fatalf("live record not cached: %+v", got)
	}
}

func TestResolveFallsBackToCachedWhenLiveIsOlderOrMissing(t *testing.T) {
	c, _ := newTestCache(t)
	now := time.Now()

	c.Store("a", rec(100, now))

	if delay, _ := c.Resolve("a", rec(5, now.Add(-time.Hour)), true); delay != 100 {
		t.Fatalf("older live record won: %d", delay)
	}

	if delay, _ := c.Resolve("a", Record{}, false); delay != 100 {
		t.Fatalf("missing live record should use cache: %d", delay)
	}
}

func TestResolveNeverTestedIsInvalid(t *testing.T) {
	c, _ := newTestCache(t)

	delay, pending := c.Resolve("a", Record{}, false)
	if delay != Invalid || pending {
		t.Fatalf("Resolve = %d, %v; want %d, false", delay, pending, Invalid)
	}
}

func TestPendingUntilFreshResultArrives(t *testing.T) {
	c, _ := newTestCache(t)
	before := time.Now().Add(-time.Minute)

	c.Store("a", rec(100, before))
	since := c.BeginTest([]string{"a", "b"})

	if _, pending := c.Resolve("a", Record{}, false); !pending {
		t.Fatal("a should be pending while its result predates the test")
	}

	if _, pending := c.Resolve("b", Record{}, false); !pending {
		t.Fatal("untested b should be pending")
	}

	delay, pending := c.Resolve("a", rec(42, since.Add(time.Millisecond)), true)
	if pending || delay != 42 {
		t.Fatalf("a after result = %d, %v; want 42, false", delay, pending)
	}

	if _, pending := c.Resolve("b", Record{}, false); !pending {
		t.Fatal("b should still be pending")
	}

	c.EndTest([]string{"a", "b"})

	if _, pending := c.Resolve("b", Record{}, false); pending {
		t.Fatal("nothing should be pending after the test ends")
	}
}

func TestPendingIgnoresProxiesOutsideTheTest(t *testing.T) {
	c, _ := newTestCache(t)

	c.BeginTest([]string{"a"})

	if _, pending := c.Resolve("other", Record{}, false); pending {
		t.Fatal("other was never part of the test")
	}
}

func TestOverlappingTestsKeepPendingUntilAllEnd(t *testing.T) {
	c, _ := newTestCache(t)

	c.BeginTest([]string{"a"})
	c.BeginTest([]string{"a"})
	c.EndTest([]string{"a"})

	if _, pending := c.Resolve("a", Record{}, false); !pending {
		t.Fatal("a should stay pending while a second test is running")
	}

	c.EndTest([]string{"a"})

	if _, pending := c.Resolve("a", Record{}, false); pending {
		t.Fatal("a should be settled")
	}
}

func TestEndTestWithoutBeginIsHarmless(t *testing.T) {
	c, _ := newTestCache(t)

	c.EndTest([]string{"ghost"})

	if _, pending := c.Resolve("ghost", Record{}, false); pending {
		t.Fatal("ghost pending")
	}
}

func TestSwitchProfileShowsOnlyTheActiveProfilesEntries(t *testing.T) {
	c, _ := newTestCache(t)

	if err := c.SwitchProfile("p1"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	c.Store("a", rec(100, time.Now()))
	c.BeginTest([]string{"a"})

	if err := c.SwitchProfile("p2"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	if _, ok := c.Load("a"); ok {
		t.Fatal("p1's entries are visible while p2 is active")
	}

	if _, pending := c.Resolve("a", Record{}, false); pending {
		t.Fatal("pending state survived a profile switch")
	}
}

func TestSwitchToSameProfileKeepsEntries(t *testing.T) {
	c, _ := newTestCache(t)

	_ = c.SwitchProfile("p1")
	c.Store("a", rec(100, time.Now()))
	_ = c.SwitchProfile("p1")

	if _, ok := c.Load("a"); !ok {
		t.Fatal("entries lost on reload of the same profile")
	}
}

func TestPersistedAcrossProcessRestart(t *testing.T) {
	c, file := newTestCache(t)

	_ = c.SwitchProfile("p1")
	at := time.Now().Truncate(time.Millisecond)
	c.Store("a", rec(123, at))
	c.Store("🚀 节点", rec(45, at))

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	restarted := New(func() string { return file }, time.Hour)

	if err := restarted.SwitchProfile("p1"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	for name, want := range map[string]int{"a": 123, "🚀 节点": 45} {
		got, ok := restarted.Load(name)
		if !ok || got.Delay != want || !got.At.Equal(at) {
			t.Fatalf("restarted Load(%q) = %+v, %v; want delay %d at %v", name, got, ok, want, at)
		}
	}
}

func TestPersistedDataOfAnotherProfileIsIgnored(t *testing.T) {
	c, file := newTestCache(t)

	_ = c.SwitchProfile("p1")
	c.Store("a", rec(123, time.Now()))
	_ = c.Flush()

	restarted := New(func() string { return file }, time.Hour)
	_ = restarted.SwitchProfile("p2")

	if _, ok := restarted.Load("a"); ok {
		t.Fatal("loaded another profile's delays")
	}
}

func TestCorruptOrMissingFileStartsEmpty(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")

	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := New(func() string { return file }, time.Hour)

	if err := c.SwitchProfile("p1"); err == nil {
		t.Fatal("corrupt file should be reported")
	}

	if _, ok := c.Load("a"); ok {
		t.Fatal("corrupt file produced entries")
	}

	missing := New(func() string { return filepath.Join(t.TempDir(), "none.json") }, time.Hour)

	if err := missing.SwitchProfile("p1"); err != nil {
		t.Fatalf("a missing file is normal, got %v", err)
	}
}

func TestStoreSchedulesDebouncedSave(t *testing.T) {
	c, file := newTestCache(t)

	_ = c.SwitchProfile("p1")
	c.Store("a", rec(7, time.Now()))

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(file); err == nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("delays were not saved in the background")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestIdenticalStoreDoesNotRewrite(t *testing.T) {
	c, file := newTestCache(t)

	_ = c.SwitchProfile("p1")
	at := time.Now()
	c.Store("a", rec(7, at))

	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	// Let any save the first Store scheduled run out, otherwise it could
	// recreate the file after it is removed below.
	time.Sleep(60 * time.Millisecond)

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	c.Store("a", rec(7, at))
	time.Sleep(60 * time.Millisecond)

	if _, err := os.Stat(file); err == nil {
		t.Fatal("an unchanged record triggered a save")
	}
}

func TestFlushWithoutPathIsNoop(t *testing.T) {
	c := New(func() string { return "" }, time.Hour)

	c.Store("a", rec(1, time.Now()))

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestConcurrentUse(t *testing.T) {
	c, _ := newTestCache(t)
	_ = c.SwitchProfile("p")

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			name := string(rune('a' + i))

			for j := 0; j < 200; j++ {
				c.BeginTest([]string{name})
				c.Store(name, rec(j, time.Now()))
				c.Resolve(name, Record{}, false)
				c.EndTest([]string{name})
			}
		}(i)
	}

	wg.Wait()
}

func TestFailedTestDoesNotReplaceSuccessfulResult(t *testing.T) {
	c, _ := newTestCache(t)
	now := time.Now()

	c.Store("a", rec(120, now))
	c.Store("a", rec(Invalid, now.Add(time.Minute)))

	got, ok := c.Load("a")
	if !ok || got.Delay != 120 {
		t.Fatalf("a failed test replaced the last good delay: %+v", got)
	}
}

func TestFailedTestIsStoredWhenNothingBetterIsKnown(t *testing.T) {
	c, _ := newTestCache(t)

	c.Store("a", rec(Invalid, time.Now()))

	got, ok := c.Load("a")
	if !ok || got.Delay != Invalid {
		t.Fatalf("failure not recorded: %+v, %v", got, ok)
	}
}

func TestSuccessReplacesEarlierFailure(t *testing.T) {
	c, _ := newTestCache(t)
	now := time.Now()

	c.Store("a", rec(Invalid, now))
	c.Store("a", rec(80, now.Add(time.Second)))

	if got, _ := c.Load("a"); got.Delay != 80 {
		t.Fatalf("success lost to older failure: %+v", got)
	}
}

func TestResolveKeepsLastGoodDelayWhenLiveTestFailed(t *testing.T) {
	c, _ := newTestCache(t)
	now := time.Now()

	c.Store("a", rec(120, now))

	delay, pending := c.Resolve("a", rec(Invalid, now.Add(time.Minute)), true)
	if delay != 120 || pending {
		t.Fatalf("Resolve = %d, %v; want the last good 120", delay, pending)
	}
}

func TestPendingEndsWhenFailedResultArrives(t *testing.T) {
	c, _ := newTestCache(t)
	before := time.Now().Add(-time.Minute)

	c.Store("a", rec(120, before))
	since := c.BeginTest([]string{"a"})

	delay, pending := c.Resolve("a", rec(Invalid, since.Add(time.Millisecond)), true)
	if pending || delay != 120 {
		t.Fatalf("Resolve = %d, %v; want 120, not pending", delay, pending)
	}
}

func TestSwitchingBackRestoresTheEarlierProfilesDelays(t *testing.T) {
	c, _ := newTestCache(t)
	at := time.Now()

	_ = c.SwitchProfile("A")
	c.Store("a", rec(100, at))

	_ = c.SwitchProfile("B")
	c.Store("b", rec(200, at))

	_ = c.SwitchProfile("A")

	got, ok := c.Load("a")
	if !ok || got.Delay != 100 {
		t.Fatalf("A's delay lost after A -> B -> A: %+v, %v", got, ok)
	}

	if _, ok := c.Load("b"); ok {
		t.Fatal("B's entries leaked into A")
	}

	_ = c.SwitchProfile("B")

	if got, ok := c.Load("b"); !ok || got.Delay != 200 {
		t.Fatalf("B's delay lost: %+v, %v", got, ok)
	}
}

func TestEveryProfileSurvivesAnAppRestart(t *testing.T) {
	c, file := newTestCache(t)
	at := time.Now().Truncate(time.Millisecond)

	_ = c.SwitchProfile("A")
	c.Store("a", rec(100, at))
	_ = c.SwitchProfile("B")
	c.Store("b", rec(200, at))

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	restarted := New(func() string { return file }, time.Hour)

	_ = restarted.SwitchProfile("A")

	if got, ok := restarted.Load("a"); !ok || got.Delay != 100 {
		t.Fatalf("A after restart = %+v, %v", got, ok)
	}

	_ = restarted.SwitchProfile("B")

	if got, ok := restarted.Load("b"); !ok || got.Delay != 200 {
		t.Fatalf("B after restart = %+v, %v", got, ok)
	}
}

func TestSwitchingProfileSavesTheOneLeft(t *testing.T) {
	c, file := newTestCache(t)

	_ = c.SwitchProfile("A")
	c.Store("a", rec(100, time.Now()))
	_ = c.SwitchProfile("B")

	restarted := New(func() string { return file }, time.Hour)
	_ = restarted.SwitchProfile("A")

	if got, ok := restarted.Load("a"); !ok || got.Delay != 100 {
		t.Fatalf("A was not saved when switching away: %+v, %v", got, ok)
	}
}

func TestFirstVersionFileIsStillReadable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	old := `{"version":1,"profile":"A","entries":{"a":{"delay":321,"at":"2026-11-16T09:35:03Z"}}}`

	if err := os.WriteFile(file, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	c := New(func() string { return file }, time.Hour)

	if err := c.SwitchProfile("A"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	if got, ok := c.Load("a"); !ok || got.Delay != 321 {
		t.Fatalf("version 1 data not migrated: %+v, %v", got, ok)
	}

	_ = c.SwitchProfile("B")
	c.Store("b", rec(5, time.Now()))

	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	again := New(func() string { return file }, time.Hour)
	_ = again.SwitchProfile("A")

	if got, ok := again.Load("a"); !ok || got.Delay != 321 {
		t.Fatalf("migrated data lost after rewrite: %+v, %v", got, ok)
	}
}

func TestPruneDropsProfilesThatNoLongerExist(t *testing.T) {
	c, file := newTestCache(t)

	for _, name := range []string{"A", "B", "C"} {
		_ = c.SwitchProfile(name)
		c.Store("n", rec(1, time.Now()))
	}

	_ = c.SwitchProfile("A")

	c.Prune(func(profile string) bool { return profile != "B" })

	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	restarted := New(func() string { return file }, time.Hour)

	_ = restarted.SwitchProfile("B")

	if _, ok := restarted.Load("n"); ok {
		t.Fatal("deleted profile B was kept")
	}

	for _, name := range []string{"A", "C"} {
		_ = restarted.SwitchProfile(name)

		if _, ok := restarted.Load("n"); !ok {
			t.Fatalf("profile %s should have been kept", name)
		}
	}
}

func TestPruneNeverDropsTheActiveProfile(t *testing.T) {
	c, _ := newTestCache(t)

	_ = c.SwitchProfile("A")
	c.Store("a", rec(1, time.Now()))

	c.Prune(func(string) bool { return false })

	if _, ok := c.Load("a"); !ok {
		t.Fatal("Prune dropped the active profile")
	}
}

func TestTooManyProfilesEvictsTheLeastRecentlyUsed(t *testing.T) {
	c, _ := newTestCache(t)

	for i := 0; i < MaxProfiles+3; i++ {
		_ = c.SwitchProfile(string(rune('a' + i)))
		c.Store("n", rec(i+1, time.Now()))
	}

	if got := c.ProfileCount(); got != MaxProfiles {
		t.Fatalf("ProfileCount = %d, want capped at %d", got, MaxProfiles)
	}

	_ = c.SwitchProfile("a")

	if _, ok := c.Load("n"); ok {
		t.Fatal("the oldest profile should have been evicted")
	}

	last := string(rune('a' + MaxProfiles + 2))

	_ = c.SwitchProfile(last)

	if _, ok := c.Load("n"); !ok {
		t.Fatal("the newest profile should be kept")
	}
}

func TestProfileCorruptEntryDoesNotHideOthers(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	content := `{"version":2,"profiles":{"A":{"entries":{"a":{"delay":7,"at":"2026-11-16T09:35:03Z"}},"used":"2026-11-16T09:35:03Z"}}}`

	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	c := New(func() string { return file }, time.Hour)

	if err := c.SwitchProfile("A"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	if got, ok := c.Load("a"); !ok || got.Delay != 7 {
		t.Fatalf("version 2 file not read: %+v, %v", got, ok)
	}
}

func savedBy(t *testing.T, file string, profile string, delay int) {
	t.Helper()

	first := New(func() string { return file }, time.Hour)

	_ = first.SwitchProfile(profile)
	first.Store("n", rec(delay, time.Now()))

	if err := first.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestQueriesBeforeAnyProfileDoNotBlockLoadingTheSavedFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	savedBy(t, file, "A", 321)

	// A restarted process: the UI may ask for delays before the config is loaded.
	c := New(func() string { return file }, time.Hour)

	c.Load("n")
	c.Resolve("n", Record{}, false)

	if err := c.SwitchProfile("A"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	if got, ok := c.Load("n"); !ok || got.Delay != 321 {
		t.Fatalf("saved delay not loaded after an early query: %+v, %v", got, ok)
	}
}

func TestResultsBeforeAnyProfileAreNotKeptUnderABlankProfile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	savedBy(t, file, "A", 321)

	c := New(func() string { return file }, time.Hour)

	c.Store("n", rec(5, time.Now().Add(time.Hour)))

	if got := c.ProfileCount(); got != 0 {
		t.Fatalf("a blank profile was created: ProfileCount = %d", got)
	}

	if err := c.SwitchProfile("A"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	if got, _ := c.Load("n"); got.Delay != 321 {
		t.Fatalf("a result from before the profile was known replaced the saved one: %+v", got)
	}
}

func TestEarlyQueriesDoNotWipeTheSavedFileOnFlush(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	savedBy(t, file, "A", 321)

	c := New(func() string { return file }, time.Hour)

	c.Resolve("n", Record{}, false)
	c.BeginTest([]string{"n"})
	c.EndTest([]string{"n"})

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	again := New(func() string { return file }, time.Hour)
	_ = again.SwitchProfile("A")

	if got, ok := again.Load("n"); !ok || got.Delay != 321 {
		t.Fatalf("saved file was damaged by early use: %+v, %v", got, ok)
	}
}

func TestServiceRestartInTheSameProcessKeepsDelays(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	c := New(func() string { return file }, time.Hour)

	_ = c.SwitchProfile("A")
	c.Store("n", rec(321, time.Now()))

	// Stopping the service flushes, then the next start loads the config again.
	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	_ = c.SwitchProfile("A")

	if got, ok := c.Load("n"); !ok || got.Delay != 321 {
		t.Fatalf("delay lost across a service restart: %+v, %v", got, ok)
	}
}

func TestFlushBeforeAnythingWasLoadedKeepsTheSavedFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	savedBy(t, file, "A", 321)

	// What a freshly started process does: reset() flushes before any config is
	// loaded, then the profile is chosen.
	c := New(func() string { return file }, time.Hour)

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if err := c.SwitchProfile("A"); err != nil {
		t.Fatalf("SwitchProfile: %v", err)
	}

	if got, ok := c.Load("n"); !ok || got.Delay != 321 {
		t.Fatalf("the saved delay did not survive an early flush: %+v, %v", got, ok)
	}
}

func TestFlushWithoutChangesNeverTouchesTheFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	savedBy(t, file, "A", 321)

	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	c := New(func() string { return file }, time.Hour)

	_ = c.Flush()
	_ = c.SwitchProfile("A")
	_ = c.Flush()

	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	if string(before) != string(after) {
		t.Fatalf("an unchanged cache rewrote the file:\n%s\n%s", before, after)
	}
}

func TestSwitchingProfilesNeverDropsOtherProfilesOnDisk(t *testing.T) {
	file := filepath.Join(t.TempDir(), "delays.json")
	savedBy(t, file, "A", 321)
	savedBy(t, file, "B", 654)

	c := New(func() string { return file }, time.Hour)

	_ = c.SwitchProfile("B")
	c.Store("m", rec(1, time.Now()))

	if err := c.Flush(); err != nil {
		t.Fatal(err)
	}

	again := New(func() string { return file }, time.Hour)
	_ = again.SwitchProfile("A")

	if got, ok := again.Load("n"); !ok || got.Delay != 321 {
		t.Fatalf("profile A was dropped while working on B: %+v, %v", got, ok)
	}
}
