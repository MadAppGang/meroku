package pricing

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// All fixtures here are synthetic: invented keys, invented values, no account
// identifiers, no real AWS response bodies.

const testKey = "profile-a|region-x"

// TestTTLCache_SingleFlight is AC-7: twelve concurrent callers on a cold key
// produce exactly ONE upstream fetch, and all twelve receive that one value.
// Run under -race.
func TestTTLCache_SingleFlight(t *testing.T) {
	c := NewTTLCache[string]()

	const n = 12

	// The fetch does not return until all twelve callers are accounted for in
	// the slow path. Misses is the observable for that: the winner increments
	// it after registering the in-flight call, and each joiner increments it at
	// the moment it commits to waiting. If single-flight were broken the extra
	// callers would run their own fetch instead of joining, this wait would
	// never be satisfied by twelve joins, and the test fails on the deadline
	// rather than passing by luck.
	var calls atomic.Int64
	fetch := func() (string, error) {
		calls.Add(1)
		if !waitUntil(func() bool { return c.Metrics().Misses == n }) {
			return "", errors.New("callers did not all reach the in-flight call")
		}
		return "value-1", nil
	}

	var wg sync.WaitGroup
	got := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			v, _, err := c.GetOrFetch(testKey, time.Hour, false, fetch)
			got[i], errs[i] = v, err
		}(i)
	}

	close(start)
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("fetch called %d times, want exactly 1", calls.Load())
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: unexpected error %v", i, errs[i])
		}
		if got[i] != "value-1" {
			t.Fatalf("caller %d: got %q, want %q", i, got[i], "value-1")
		}
	}

	m := c.Metrics()
	if m.Misses != n {
		t.Errorf("Misses = %d, want %d (a joined flight is still a miss)", m.Misses, n)
	}
	if m.Hits != 0 {
		t.Errorf("Hits = %d, want 0", m.Hits)
	}
	if m.LastRefresh.IsZero() {
		t.Error("LastRefresh not set after a successful fetch")
	}
}

// TestTTLCache_ForceDoesNotJoinStale: with a slow unforced fetch in flight, a
// force=true caller must get the value from a SECOND fetch, never the one the
// refresh was meant to replace.
func TestTTLCache_ForceDoesNotJoinStale(t *testing.T) {
	c := NewTTLCache[string]()

	var calls atomic.Int64
	firstInFlight := make(chan struct{})
	releaseFirst := make(chan struct{})

	slow := func() (string, error) {
		calls.Add(1)
		close(firstInFlight)
		<-releaseFirst
		return "stale", nil
	}
	forced := func() (string, error) {
		calls.Add(1)
		return "fresh", nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if v, _, err := c.GetOrFetch(testKey, time.Hour, false, slow); err != nil || v != "stale" {
			t.Errorf("unforced caller got (%q, %v), want (\"stale\", nil)", v, err)
		}
	}()

	<-firstInFlight

	forcedDone := make(chan string, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, _, err := c.GetOrFetch(testKey, time.Hour, true, forced)
		if err != nil {
			t.Errorf("forced caller: unexpected error %v", err)
		}
		forcedDone <- v
	}()

	select {
	case v := <-forcedDone:
		if v != "fresh" {
			t.Fatalf("forced caller got %q, want %q — it joined the in-flight unforced fetch", v, "fresh")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forced caller blocked on the unforced fetch")
	}

	close(releaseFirst)
	wg.Wait()

	if calls.Load() != 2 {
		t.Fatalf("fetch called %d times, want 2 (one unforced, one forced)", calls.Load())
	}
}

// TestTTLCache_ForceSingleFlightAmongForced: forced callers still collapse
// among themselves, so a burst of refresh=true requests is one AWS call.
func TestTTLCache_ForceSingleFlightAmongForced(t *testing.T) {
	c := NewTTLCache[string]()

	const n = 6

	var calls atomic.Int64
	fetch := func() (string, error) {
		calls.Add(1)
		if !waitUntil(func() bool { return c.Metrics().Misses == n }) {
			return "", errors.New("forced callers did not all join one flight")
		}
		return "forced-value", nil
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _, err := c.GetOrFetch(testKey, time.Hour, true, fetch)
			if err != nil || v != "forced-value" {
				t.Errorf("got (%q, %v), want (\"forced-value\", nil)", v, err)
			}
		}()
	}

	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("fetch called %d times, want 1", calls.Load())
	}
}

// TestTTLCache_PeekReturnsStale is the EC-1 / NFR-8 path: past its TTL the
// entry is still readable, with its date, without blocking on a fetch.
func TestTTLCache_PeekReturnsStale(t *testing.T) {
	c := NewTTLCache[string]()

	if _, _, ok := c.Peek(testKey); ok {
		t.Fatal("Peek on an empty cache reported a value")
	}

	stored, at, err := c.GetOrFetch(testKey, time.Hour, false, func() (string, error) {
		return "v1", nil
	})
	if err != nil || stored != "v1" {
		t.Fatalf("seed fetch got (%q, %v)", stored, err)
	}
	if at.IsZero() {
		t.Fatal("cachedAt is the zero time after a successful fetch")
	}

	// A nanosecond TTL makes the entry stale immediately, without a sleep.
	time.Sleep(2 * time.Millisecond)
	if _, _, ok := c.get(testKey, time.Nanosecond); ok {
		t.Fatal("entry still considered fresh under a 1ns TTL")
	}

	v, peekedAt, ok := c.Peek(testKey)
	if !ok {
		t.Fatal("Peek did not return the stale entry")
	}
	if v != "v1" {
		t.Fatalf("Peek returned %q, want %q", v, "v1")
	}
	if !peekedAt.Equal(at) {
		t.Fatalf("Peek cachedAt = %v, want %v", peekedAt, at)
	}

	// And the stale entry does not stop a refetch from replacing it.
	v2, _, err := c.GetOrFetch(testKey, time.Nanosecond, false, func() (string, error) {
		return "v2", nil
	})
	if err != nil || v2 != "v2" {
		t.Fatalf("stale-TTL fetch got (%q, %v), want (\"v2\", nil)", v2, err)
	}
}

// TestTTLCache_ErrorKeepsStaleEntry: a failed fetch returns the zero value and
// the error, never a silent stale success — but it leaves the previous entry in
// place so the caller can fall back through Peek.
func TestTTLCache_ErrorKeepsStaleEntry(t *testing.T) {
	c := NewTTLCache[string]()

	if _, _, err := c.GetOrFetch(testKey, time.Hour, false, func() (string, error) {
		return "good", nil
	}); err != nil {
		t.Fatalf("seed fetch failed: %v", err)
	}
	before := c.Metrics()

	wantErr := errors.New("throttled")
	v, at, err := c.GetOrFetch(testKey, time.Nanosecond, false, func() (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if v != "" {
		t.Fatalf("value = %q on error, want the zero value", v)
	}
	if !at.IsZero() {
		t.Fatalf("cachedAt = %v on error, want the zero time", at)
	}

	stale, _, ok := c.Peek(testKey)
	if !ok || stale != "good" {
		t.Fatalf("Peek after a failed fetch got (%q, %v), want (\"good\", true)", stale, ok)
	}

	m := c.Metrics()
	if m.Errors != 1 {
		t.Errorf("Errors = %d, want 1", m.Errors)
	}
	if !m.LastRefresh.Equal(before.LastRefresh) {
		t.Error("a failed fetch moved LastRefresh")
	}
}

// TestTTLCache_ErrorIsSharedByJoiners: one failed fetch, N callers, one Errors
// increment.
func TestTTLCache_ErrorIsSharedByJoiners(t *testing.T) {
	c := NewTTLCache[string]()

	const n = 5

	var calls atomic.Int64
	wantErr := errors.New("access denied")
	fetch := func() (string, error) {
		calls.Add(1)
		if !waitUntil(func() bool { return c.Metrics().Misses == n }) {
			return "", errors.New("callers did not all reach the in-flight call")
		}
		return "", wantErr
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := c.GetOrFetch(testKey, time.Hour, false, fetch); !errors.Is(err, wantErr) {
				t.Errorf("err = %v, want %v", err, wantErr)
			}
		}()
	}
	wg.Wait()

	if got := c.Metrics().Errors; got != 1 {
		t.Fatalf("Errors = %d, want 1 (one failed fetch, not one per caller)", got)
	}
	if c.Len() != 0 {
		t.Fatalf("Len = %d, want 0 (a failed fetch must not store an entry)", c.Len())
	}
}

// TestTTLCache_WarmHitDoesNotFetch is the NFR-2 shape: a warm read never calls
// the fetcher, so it never pays for a credentials probe or a network round trip.
func TestTTLCache_WarmHitDoesNotFetch(t *testing.T) {
	c := NewTTLCache[int]()

	var calls atomic.Int64
	fetch := func() (int, error) {
		calls.Add(1)
		return 42, nil
	}

	for i := 0; i < 10; i++ {
		v, _, err := c.GetOrFetch(testKey, time.Hour, false, fetch)
		if err != nil || v != 42 {
			t.Fatalf("iteration %d got (%d, %v)", i, v, err)
		}
	}

	if calls.Load() != 1 {
		t.Fatalf("fetch called %d times over 10 warm reads, want 1", calls.Load())
	}
	m := c.Metrics()
	if m.Hits != 9 || m.Misses != 1 {
		t.Fatalf("Hits/Misses = %d/%d, want 9/1", m.Hits, m.Misses)
	}
}

// TestTTLCache_KeysAreIndependent: two regions under one profile are two
// entries. Serving one region's payload for another is the failure this key
// scheme exists to prevent.
func TestTTLCache_KeysAreIndependent(t *testing.T) {
	c := NewTTLCache[string]()

	keySydney := CacheKey("profile-a", "region-x")
	keyOregon := CacheKey("profile-a", "region-y")
	otherProfile := CacheKey("profile-b", "region-x")

	for _, tc := range []struct{ key, val string }{
		{keySydney, "catalog-x"},
		{keyOregon, "catalog-y"},
		{otherProfile, "catalog-x-other-profile"},
	} {
		v, _, err := c.GetOrFetch(tc.key, time.Hour, false, func() (string, error) {
			return tc.val, nil
		})
		if err != nil || v != tc.val {
			t.Fatalf("%s: got (%q, %v)", tc.key, v, err)
		}
	}

	if v, _, _ := c.Peek(keySydney); v != "catalog-x" {
		t.Errorf("region-x entry = %q, want catalog-x", v)
	}
	if v, _, _ := c.Peek(keyOregon); v != "catalog-y" {
		t.Errorf("region-y entry = %q, want catalog-y", v)
	}
	if v, _, _ := c.Peek(otherProfile); v != "catalog-x-other-profile" {
		t.Errorf("other-profile entry = %q, want catalog-x-other-profile", v)
	}

	// Invalidating one region leaves the others alone (the refresh=true rule).
	c.Invalidate(keySydney)
	if _, _, ok := c.Peek(keySydney); ok {
		t.Error("Invalidate did not drop the entry")
	}
	if _, _, ok := c.Peek(keyOregon); !ok {
		t.Error("Invalidate dropped another region's entry")
	}
	if _, _, ok := c.Peek(otherProfile); !ok {
		t.Error("Invalidate dropped another profile's entry")
	}
}

// TestCacheKey covers the two properties the key scheme relies on: parts cannot
// run together, and no real key can contain the NUL that namespaces forced
// flights.
func TestCacheKey(t *testing.T) {
	if got := CacheKey("profile", "ap-southeast-2"); got != "profile|ap-southeast-2" {
		t.Fatalf("CacheKey = %q", got)
	}
	if CacheKey("a", "bc") == CacheKey("ab", "c") {
		t.Fatal("CacheKey collided on differently split parts")
	}
	if CacheKey("") != "" {
		t.Fatal("the empty profile must be a legitimate key, not an error")
	}
	for _, k := range []string{
		CacheKey("", "ap-southeast-2"),
		CacheKey("profile", "us-east-1", "m6i.large,m7g.large"),
		CacheKey("profile", "force"),
	} {
		if strings.Contains(k, "\x00") {
			t.Fatalf("CacheKey produced a NUL byte in %q — it would collide with the force flight namespace", k)
		}
	}
}

// TestTTLCache_FlightKeyCannotCollide: a key that merely looks like a forced
// flight key must not receive another key's forced result.
func TestTTLCache_FlightKeyCannotCollide(t *testing.T) {
	c := NewTTLCache[string]()

	inFlight := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, _, err := c.GetOrFetch(testKey, time.Hour, true, func() (string, error) {
			close(inFlight)
			<-release
			return "forced-payload", nil
		})
		if err != nil || v != "forced-payload" {
			t.Errorf("forced caller got (%q, %v)", v, err)
		}
	}()

	<-inFlight

	// A hostile-but-legal key that would equal the forced flight key if the
	// suffix were printable.
	lookalike := testKey + "force"
	done := make(chan string, 1)
	go func() {
		v, _, err := c.GetOrFetch(lookalike, time.Hour, false, func() (string, error) {
			return "own-payload", nil
		})
		if err != nil {
			t.Errorf("lookalike key: unexpected error %v", err)
		}
		done <- v
	}()

	select {
	case v := <-done:
		if v != "own-payload" {
			t.Errorf("lookalike key got %q, want %q — flight keys collided", v, "own-payload")
		}
	case <-time.After(5 * time.Second):
		t.Error("lookalike key blocked on another key's forced fetch — flight keys collided")
	}

	close(release)
	wg.Wait()
}

// TestTTLCache_ConcurrentMixedAccess is the race detector's target: readers,
// writers, forced refreshes, invalidations and metric reads all at once across
// several keys. It asserts no outcome beyond "nothing tore".
func TestTTLCache_ConcurrentMixedAccess(t *testing.T) {
	c := NewTTLCache[[]string]()

	keys := []string{
		CacheKey("p1", "region-x"),
		CacheKey("p1", "region-y"),
		CacheKey("p2", "region-x"),
		CacheKey("", "region-x"),
	}

	var wg sync.WaitGroup
	for g := 0; g < 24; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			key := keys[g%len(keys)]
			for i := 0; i < 40; i++ {
				switch (g + i) % 5 {
				case 0:
					c.GetOrFetch(key, time.Hour, true, func() ([]string, error) {
						return []string{fmt.Sprintf("forced-%d", g)}, nil
					})
				case 1:
					c.Invalidate(key)
				case 2:
					c.Peek(key)
				case 3:
					c.Metrics()
				default:
					v, _, err := c.GetOrFetch(key, 5*time.Millisecond, false, func() ([]string, error) {
						return []string{"a", "b", "c"}, nil
					})
					if err == nil && len(v) == 0 {
						t.Errorf("got an empty slice from a successful fetch")
					}
				}
			}
		}(g)
	}
	wg.Wait()

	m := c.Metrics()
	if m.Hits+m.Misses == 0 {
		t.Fatal("no cache traffic recorded")
	}
}

// waitUntil spins until cond holds, and reports whether it did before the
// deadline. It is used instead of a fixed sleep so that the flight-window
// assertions are deterministic rather than timing-dependent, and it returns a
// bool rather than calling t.Fatal because it runs inside fetch, on a goroutine
// that is not the test's.
func waitUntil(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Microsecond)
	}
	return true
}
