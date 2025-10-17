package pricing

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// PriceCache provides thread-safe caching of pricing data with TTL
// Implements automatic background refresh to keep prices current
type PriceCache struct {
	mu      sync.RWMutex
	data    map[string]*PriceRates // region -> rates
	ttl     time.Duration          // Time-to-live for cached data
	client  *AWSPricingClient
	metrics CacheMetrics
}

// NewPriceCache creates a new price cache with the specified TTL
// ttl: Duration before cached data is considered stale (recommended: 24h)
func NewPriceCache(client *AWSPricingClient, ttl time.Duration) *PriceCache {
	return &PriceCache{
		data:   make(map[string]*PriceRates),
		ttl:    ttl,
		client: client,
		metrics: CacheMetrics{
			Hits:   0,
			Misses: 0,
		},
	}
}

// Get returns cached pricing rates for the specified region
// If cache is stale or missing, fetches fresh data from AWS
// Thread-safe for concurrent access
func (c *PriceCache) Get(region string) (*PriceRates, error) {
	// Try to get from cache first
	c.mu.RLock()
	rates, exists := c.data[region]
	c.mu.RUnlock()

	// Check if cached data is still valid
	if exists && time.Since(rates.LastUpdate) < c.ttl {
		atomic.AddInt64(&c.metrics.Hits, 1)
		return rates, nil
	}

	// Cache miss or stale data, refresh
	atomic.AddInt64(&c.metrics.Misses, 1)
	return c.refresh(region)
}

// refresh fetches fresh pricing data and updates the cache
// Always returns data (uses fallback if AWS API fails)
func (c *PriceCache) refresh(region string) (*PriceRates, error) {
	// Fetch fresh rates from AWS (with fallback)
	rates, err := c.client.FetchRates(region)
	if err != nil {
		atomic.AddInt64(&c.metrics.Errors, 1)
		// Even with error, FetchRates returns fallback data
		// So we can still cache and use it
	}

	// Update cache
	c.mu.Lock()
	c.data[region] = rates
	c.metrics.LastRefresh = time.Now()
	c.mu.Unlock()

	return rates, nil
}

// StartBackgroundRefresh starts a background goroutine that refreshes
// pricing data periodically to keep the cache warm
// Recommended: refresh every 12 hours for 24h TTL
func (c *PriceCache) StartBackgroundRefresh(ctx context.Context, regions []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Silently refresh all regions
				for _, region := range regions {
					c.refresh(region) // Ignore errors - fallback always works
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// GetMetrics returns current cache performance metrics
// Useful for monitoring and debugging
func (c *PriceCache) GetMetrics() CacheMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheMetrics{
		Hits:        atomic.LoadInt64(&c.metrics.Hits),
		Misses:      atomic.LoadInt64(&c.metrics.Misses),
		LastRefresh: c.metrics.LastRefresh,
		Errors:      atomic.LoadInt64(&c.metrics.Errors),
	}
}

// Clear removes all cached data
// Useful for testing or forcing a full refresh
func (c *PriceCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*PriceRates)
	log.Printf("[Pricing] Cache cleared")
}

// Invalidate removes cached data for a specific region
// Useful when you know pricing has changed
func (c *PriceCache) Invalidate(region string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, region)
	log.Printf("[Pricing] Invalidated cache for region: %s", region)
}
