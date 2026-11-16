// Package delaycache remembers the latest delay of every proxy.
//
// It serves three purposes: results survive a config reload (switching mode
// recreates every proxy and drops its live history), results are shared by all
// groups regardless of which test URL measured them, and results survive an app
// restart because they are saved to disk. Delays are kept per profile, so
// switching to another profile and back does not lose them. It also tracks
// which proxies are currently being tested so the UI can show progress instead
// of a stale value.
//
// The package has no mihomo dependency so it can be unit tested on the host.
package delaycache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Invalid is the delay reported for a proxy that has no usable result.
const Invalid = 0xffff

// MaxProfiles bounds how many profiles' delays are kept; beyond it the least
// recently used profile is dropped.
const MaxProfiles = 32

const (
	fileVersion       = 2
	legacyFileVersion = 1
)

// Record is one delay test result. Delay is Invalid when the test failed.
type Record struct {
	Delay int       `json:"delay"`
	At    time.Time `json:"at"`
}

type profileData struct {
	entries map[string]Record
	used    time.Time
}

// Cache is safe for concurrent use.
type Cache struct {
	pathFn   func() string
	debounce time.Duration

	mu       sync.Mutex
	loaded   bool
	profile  string
	profiles map[string]*profileData
	testing  map[string]int
	started  map[string]time.Time
	dirty    bool
	saveTask *time.Timer
}

// New returns an empty Cache. pathFn locates the file the cache is saved to; it
// is called lazily so the home directory may be configured after construction.
// A blank path disables saving. Writes are batched by debounce.
func New(pathFn func() string, debounce time.Duration) *Cache {
	return &Cache{
		pathFn:   pathFn,
		debounce: debounce,
		profiles: map[string]*profileData{},
		testing:  map[string]int{},
		started:  map[string]time.Time{},
	}
}

type fileProfile struct {
	Entries map[string]Record `json:"entries"`
	Used    time.Time         `json:"used"`
}

type fileContent struct {
	Version  int                    `json:"version"`
	Profiles map[string]fileProfile `json:"profiles,omitempty"`

	// Version 1 held a single profile.
	Profile string            `json:"profile,omitempty"`
	Entries map[string]Record `json:"entries,omitempty"`
}

// SwitchProfile makes profile the active one. The delays of the profile that was
// active are kept and come back when it is switched to again, and every profile
// is saved, so they survive an app restart too. A corrupt save file is
// reported, but the cache stays usable.
func (c *Cache) SwitchProfile(profile string) error {
	c.mu.Lock()

	if c.profile == profile && profile != "" && c.profiles[profile] != nil {
		c.mu.Unlock()

		return nil
	}

	loadErr := c.ensureLoadedLocked()

	leaving := c.profile != ""

	c.profile = profile
	c.testing = map[string]int{}
	c.started = map[string]time.Time{}

	c.activeLocked().used = time.Now()
	c.evictLocked()

	// Leaving a profile is the moment its delays must not be lost, so write them
	// now instead of waiting for the debounce.
	c.dirty = c.dirty || leaving
	snapshot, path := c.snapshotLocked()
	c.mu.Unlock()

	saveErr := save(path, snapshot)

	if loadErr != nil {
		return loadErr
	}

	return saveErr
}

// activeLocked returns the active profile's data, creating it when missing. It
// returns nil until a profile has been chosen with SwitchProfile: results that
// arrive earlier cannot be attributed to any profile, so they are not kept.
func (c *Cache) activeLocked() *profileData {
	if c.profile == "" {
		return nil
	}

	data := c.profiles[c.profile]
	if data == nil {
		data = &profileData{entries: map[string]Record{}}
		c.profiles[c.profile] = data
	}

	return data
}

// entryLocked returns the cached result of a proxy in the active profile.
func (c *Cache) entryLocked(name string) (Record, bool) {
	data := c.activeLocked()
	if data == nil {
		return Record{}, false
	}

	record, ok := data.entries[name]

	return record, ok
}

func (c *Cache) evictLocked() {
	for len(c.profiles) > MaxProfiles {
		oldest, oldestUsed := "", time.Time{}

		for name, data := range c.profiles {
			if name == c.profile {
				continue
			}

			if oldest == "" || data.used.Before(oldestUsed) {
				oldest, oldestUsed = name, data.used
			}
		}

		if oldest == "" {
			return
		}

		delete(c.profiles, oldest)
	}
}

// ensureLoadedLocked reads the save file once. Whatever happens it is not tried
// again, so a corrupt file is reported once and then replaced by the next save.
func (c *Cache) ensureLoadedLocked() error {
	if c.loaded {
		return nil
	}

	c.loaded = true

	return c.loadLocked()
}

func (c *Cache) loadLocked() error {
	path := c.path()
	if path == "" {
		return nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read delay cache: %w", err)
	}

	var content fileContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return fmt.Errorf("parse delay cache: %w", err)
	}

	switch content.Version {
	case fileVersion:
		for name, profile := range content.Profiles {
			entries := profile.Entries
			if entries == nil {
				entries = map[string]Record{}
			}

			c.profiles[name] = &profileData{entries: entries, used: profile.Used}
		}
	case legacyFileVersion:
		if content.Profile != "" {
			c.profiles[content.Profile] = &profileData{entries: nonNil(content.Entries)}
		}
	}

	return nil
}

func nonNil(entries map[string]Record) map[string]Record {
	if entries == nil {
		return map[string]Record{}
	}

	return entries
}

func (c *Cache) path() string {
	if c.pathFn == nil {
		return ""
	}

	return c.pathFn()
}

// Store records a result unless a newer one is already known.
func (c *Cache) Store(name string, record Record) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.storeLocked(name, record)
}

func (c *Cache) storeLocked(name string, record Record) {
	data := c.activeLocked()
	if data == nil {
		return
	}

	if old, ok := data.entries[name]; ok {
		if old.At.After(record.At) || old == record {
			return
		}

		// A failed test says nothing about how fast the proxy is, so keep the
		// last delay that was actually measured.
		if record.Delay == Invalid && old.Delay != Invalid {
			return
		}
	}

	data.entries[name] = record
	data.used = record.At
	c.scheduleSaveLocked()
}

// Load returns the cached result of a proxy.
func (c *Cache) Load(name string) (Record, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.entryLocked(name)
}

// BeginTest marks the proxies as being tested and returns the time the test
// started. A proxy stays pending until a result newer than that time arrives or
// EndTest is called for it. Calls may overlap; each BeginTest needs its EndTest.
func (c *Cache) BeginTest(names []string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	for _, name := range names {
		if c.testing[name] == 0 {
			c.started[name] = now
		}

		c.testing[name]++
	}

	return now
}

// EndTest releases one BeginTest for each proxy.
func (c *Cache) EndTest(names []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, name := range names {
		switch count := c.testing[name]; {
		case count > 1:
			c.testing[name] = count - 1
		case count == 1:
			delete(c.testing, name)
			delete(c.started, name)
		}
	}
}

// Resolve combines a live result (hasLive) with the cached one and reports the
// delay to show and whether the proxy is still waiting for a fresh result.
// The newest of the two wins, and the winning live result is cached. A proxy
// that was never tested reports Invalid.
func (c *Cache) Resolve(name string, live Record, hasLive bool) (delay int, pending bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if hasLive {
		c.storeLocked(name, live)
	}

	best, found := c.entryLocked(name)

	if start, testing := c.started[name]; testing {
		answered := hasLive && live.At.After(start)

		pending = !answered && (!found || !best.At.After(start))
	}

	if !found {
		return Invalid, pending
	}

	return best.Delay, pending
}

func (c *Cache) scheduleSaveLocked() {
	c.dirty = true

	if c.path() == "" || c.saveTask != nil {
		return
	}

	c.saveTask = time.AfterFunc(c.debounce, func() {
		_ = c.Flush()
	})
}

// Flush writes the cache to disk now if anything changed since the last save.
func (c *Cache) Flush() error {
	c.mu.Lock()
	snapshot, path := c.snapshotLocked()
	c.mu.Unlock()

	return save(path, snapshot)
}

// snapshotLocked takes what has to be written and clears the dirty flag. It
// returns a nil snapshot when there is nothing to save.
func (c *Cache) snapshotLocked() (*fileContent, string) {
	if c.saveTask != nil {
		c.saveTask.Stop()
		c.saveTask = nil
	}

	path := c.path()
	if path == "" || !c.dirty {
		return nil, ""
	}

	content := &fileContent{Version: fileVersion, Profiles: make(map[string]fileProfile, len(c.profiles))}

	for name, data := range c.profiles {
		entries := make(map[string]Record, len(data.entries))
		for key, record := range data.entries {
			entries[key] = record
		}

		content.Profiles[name] = fileProfile{Entries: entries, Used: data.used}
	}

	c.dirty = false

	return content, path
}

func save(path string, content *fileContent) error {
	if content == nil {
		return nil
	}

	return writeFile(path, *content)
}

func writeFile(path string, content fileContent) error {
	raw, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("encode delay cache: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create delay cache dir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write delay cache: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace delay cache: %w", err)
	}

	return nil
}

// Prune forgets the delays of every profile for which keep returns false. The
// active profile is never dropped. Use it to clean up after profiles that were
// deleted.
func (c *Cache) Prune(keep func(profile string) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	changed := false

	for name := range c.profiles {
		if name == c.profile || keep(name) {
			continue
		}

		delete(c.profiles, name)

		changed = true
	}

	if changed {
		c.scheduleSaveLocked()
	}
}

// ProfileCount reports how many profiles currently have delays remembered.
func (c *Cache) ProfileCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.profiles)
}
