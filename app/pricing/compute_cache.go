package pricing

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TTLCache is a generic, TTL-bounded, single-flight cache.
//
// It exists beside PriceCache (cache.go) rather than inside it: PriceCache's
// entry type is fixed to *PriceRates, its TTL is fixed at construction, and its
// refresh has no single-flight, so hanging a several-hundred-record instance
// catalogue and a 15-minute spot table off it would make every environment-cost
// request in the app pay for an EC2 describe. What is reused is its shape --
// same package, same RWMutex idiom, same CacheMetrics type (types.go), same
// Invalidate(key) semantics.
//
// # Key scope: what is per-region and what is global
//
// Serving one region's prices for another is silent and expensive, so the rule
// is written down here rather than left to each call site:
//
//   - PER PROFILE AND PER REGION (never global): the instance-type catalogue,
//     the on-demand hourly map, and the spot quotes. Their keys are
//     CacheKey(profile, region) and, for spot, CacheKey(profile, region,
//     sortedTypes). The profile is part of the key because switching AWS
//     profile must not serve another account's view of a region; the region is
//     part of the key because prices, availability and spot markets all differ
//     by region. The empty profile is a legitimate key, not an error.
//   - PER PROFILE AND PER REGION: the credentials probe result (60s TTL), for
//     the same reason.
//   - GLOBAL: nothing in this cache. The fallback tables in
//     compute_fallback.go are compile-time constants, not cache entries, and
//     are labelled FALLBACK_PRICING_REGION (us-east-1) precisely because they
//     are not the selected region's data.
//
// # Concurrency
//
// Reads take RLock. A miss registers an in-flight call under the write lock;
// the winner runs fetch OUTSIDE the lock and every loser waits on the call's
// done channel, so N concurrent cold callers produce exactly ONE upstream
// request. Stored values are treated as immutable: a fetch builds a fresh value
// and the cache swaps the pointer to it, so no reader can observe a torn
// update. Callers must not mutate a value they received -- copy it first (the
// catalogue is sorted and filtered into a per-request copy for this reason).
type TTLCache[T any] struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry[T]
	calls   map[string]*cacheCall[T] // in-flight fetches, keyed by flight key

	// Counters are atomics rather than fields of an embedded CacheMetrics so
	// that Metrics() never has to take a lock and so that -race stays quiet
	// when a fetch updates them outside the mutex.
	hits        atomic.Int64
	misses      atomic.Int64
	errors      atomic.Int64
	lastRefresh atomic.Int64 // UnixNano of the last successful fetch; 0 = never
}

// cacheEntry is one stored value and the instant it was fetched.
type cacheEntry[T any] struct {
	val      T
	cachedAt time.Time
}

// cacheCall is one in-flight fetch. Its fields are written by the goroutine
// that runs fetch and read by waiters only after done is closed, so the channel
// close provides the happens-before edge.
type cacheCall[T any] struct {
	done     chan struct{}
	val      T
	cachedAt time.Time
	err      error
}

// forceFlightSuffix namespaces the in-flight entry of a forced fetch so that a
// forced caller can never join an unforced one and receive pre-refresh data.
//
// It is a NUL byte, not "|force": cache keys are "|"-joined from profile,
// region and instance-type names, so a real key can contain "|" but can never
// contain a NUL. Any printable suffix would be forgeable by a key whose last
// part happened to be "force".
const forceFlightSuffix = "\x00force"

// NewTTLCache creates an empty cache. The TTL is supplied per call rather than
// per cache, because the catalogue (24h) and the spot table (15m) share this
// type.
func NewTTLCache[T any]() *TTLCache[T] {
	return &TTLCache[T]{
		entries: make(map[string]*cacheEntry[T]),
		calls:   make(map[string]*cacheCall[T]),
	}
}

// CacheKey joins key parts with "|". It is the canonical way to build a key for
// this cache: CacheKey(profile, region) for the catalogue and the on-demand
// map, CacheKey(profile, region, strings.Join(sortedTypes, ",")) for spot. The
// parts are joined rather than concatenated so that ("a", "bc") and ("ab", "c")
// cannot collide.
func CacheKey(parts ...string) string {
	return strings.Join(parts, "|")
}

// GetOrFetch returns the cached value when it is younger than ttl. Otherwise it
// runs fetch exactly once per key, even under concurrent callers, and every
// caller of that one fetch receives its result.
//
// force=true bypasses the age check and, critically, refuses to join an
// unforced fetch already in flight -- a refresh that returned pre-refresh data
// would not be a refresh.
//
// On a fetch error the returned value is the ZERO value of T and err is
// non-nil; the previous entry is deliberately left in place, and the degraded
// path is to call Peek for it (live -> Peek at any age -> the fallback table).
// Returning stale data through the success path would leave a caller unable to
// tell fresh from stale.
//
// The returned time is the instant the value was fetched, for the envelope's
// cachedAt field. It is the zero time when err is non-nil.
func (c *TTLCache[T]) GetOrFetch(key string, ttl time.Duration, force bool, fetch func() (T, error)) (T, time.Time, error) {
	if !force {
		if val, cachedAt, ok := c.get(key, ttl); ok {
			c.hits.Add(1)
			return val, cachedAt, nil
		}
	}

	flightKey := key
	if force {
		flightKey = key + forceFlightSuffix
	}

	c.mu.Lock()
	// Re-check under the write lock: another goroutine may have stored a fresh
	// entry between the RUnlock above and this Lock.
	if !force {
		if e, ok := c.entries[key]; ok && fresh(e.cachedAt, ttl) {
			c.mu.Unlock()
			c.hits.Add(1)
			return e.val, e.cachedAt, nil
		}
	}
	if existing, ok := c.calls[flightKey]; ok {
		// Join the in-flight fetch. This counts as a miss: nothing was served
		// from a fresh entry, even though no upstream call is made.
		c.mu.Unlock()
		c.misses.Add(1)
		<-existing.done
		return existing.val, existing.cachedAt, existing.err
	}
	call := &cacheCall[T]{done: make(chan struct{})}
	c.calls[flightKey] = call
	c.mu.Unlock()
	c.misses.Add(1)

	// fetch runs outside the lock so that readers of other keys -- and Peek on
	// this one -- never block behind a network call.
	val, err := fetch()
	now := time.Now()

	c.mu.Lock()
	delete(c.calls, flightKey)
	if err == nil {
		c.entries[key] = &cacheEntry[T]{val: val, cachedAt: now}
	}
	c.mu.Unlock()

	if err != nil {
		c.errors.Add(1)
		var zero T
		call.val, call.cachedAt, call.err = zero, time.Time{}, err
		close(call.done)
		return zero, time.Time{}, err
	}

	c.lastRefresh.Store(now.UnixNano())
	call.val, call.cachedAt, call.err = val, now, nil
	close(call.done)
	return val, now, nil
}

// Peek returns the last stored value for key regardless of its age, together
// with the instant it was fetched. It never blocks on a fetch and never starts
// one: it is the throttled / expired-credentials path, where serving a stale
// answer with its date beats serving nothing.
func (c *TTLCache[T]) Peek(key string) (T, time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, time.Time{}, false
	}
	return e.val, e.cachedAt, true
}

// Invalidate drops the stored entry for key. It does not cancel or detach an
// in-flight fetch: a fetch already running will still store its result, which
// is what the refresh=true path wants (invalidate, then refetch synchronously).
func (c *TTLCache[T]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// Metrics returns a snapshot of the counters.
//
// Semantics, because "hit rate" is otherwise ambiguous here:
//   - Hits -- requests served from an entry younger than the requested TTL.
//   - Misses -- every other request, INCLUDING one that joined an in-flight
//     fetch rather than issuing its own. Twelve concurrent cold callers
//     therefore record twelve misses and one upstream call.
//   - Errors -- one per failed fetch, not one per caller waiting on it.
//   - LastRefresh -- the last SUCCESSFUL fetch; a failed fetch does not move it.
func (c *TTLCache[T]) Metrics() CacheMetrics {
	m := CacheMetrics{
		Hits:   c.hits.Load(),
		Misses: c.misses.Load(),
		Errors: c.errors.Load(),
	}
	if ns := c.lastRefresh.Load(); ns != 0 {
		m.LastRefresh = time.Unix(0, ns)
	}
	return m
}

// Len returns the number of stored entries. Diagnostics and test helper.
func (c *TTLCache[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// get is the fast read path: RLock, and a fresh entry or nothing.
func (c *TTLCache[T]) get(key string, ttl time.Duration) (T, time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if e, ok := c.entries[key]; ok && fresh(e.cachedAt, ttl) {
		return e.val, e.cachedAt, true
	}
	var zero T
	return zero, time.Time{}, false
}

// fresh reports whether an entry fetched at cachedAt is still within ttl. A
// non-positive ttl means "always stale", which is how a caller asks for an
// unconditional refetch without the force semantics.
func fresh(cachedAt time.Time, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	return time.Since(cachedAt) < ttl
}
