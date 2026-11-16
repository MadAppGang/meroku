package main

import (
	"context"
	"log"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingpkg "madappgang.com/meroku/pricing"
)

// liveEC2Snapshot is the cached live on-demand map liveEC2PricesFor read for a
// region, and its fetch time. prices is nil when nothing was cached.
//
// refreshing is true when the snapshot was not fresh and a background load was
// in flight, or was started, when it was taken. Prices built from such a
// snapshot may already be superseded even if the load has ended by the time
// the caller checks liveEC2PricesLoading.
type liveEC2Snapshot struct {
	prices     map[string]float64
	fetchedAt  time.Time
	refreshing bool
}

// fresh reports whether the snapshot was read from AWS within the TTL.
func (s liveEC2Snapshot) fresh() bool {
	return s.prices != nil && time.Since(s.fetchedAt) < computeOnDemandTTL
}

// withLiveEC2Prices returns a copy of rates whose EC2 on-demand prices are
// the live ones for region over the dated table. See liveEC2PricesFor.
func withLiveEC2Prices(rates *pricingpkg.PriceRates, region string) *pricingpkg.PriceRates {
	out, _ := liveEC2PricesFor(rates, region)
	return out
}

// liveEC2PricesFor lays the cached live on-demand map of region (memory, else
// the disk file) over a copy of rates and returns it at once; it never waits
// for AWS. The table prices a dozen types at us-east-1 rates; the live map
// prices every type the region sells.
//
// With nothing cached, or an entry older than computeOnDemandTTL, it starts
// one background load and answers with what it has: the stale map, or the
// table alone, where a type the table lacks stays unknown. The next request
// after the load has the region's prices. No load starts within
// liveEC2RetryAfterFailure of a failed one.
func liveEC2PricesFor(rates *pricingpkg.PriceRates, region string) (*pricingpkg.PriceRates, liveEC2Snapshot) {
	if rates == nil || region == "" {
		return rates, liveEC2Snapshot{}
	}
	key := pricingpkg.CacheKey(selectedAWSProfile, region)
	var snap liveEC2Snapshot
	if live, at, ok := peekOnDemandPrices(key); ok && len(live) > 0 {
		snap = liveEC2Snapshot{prices: live, fetchedAt: at}
	}
	if !snap.fresh() {
		snap.refreshing = warmLiveEC2Prices(region)
	}
	if snap.prices == nil {
		return rates, snap
	}
	out := *rates
	merged := make(map[string]float64, len(rates.EC2.OnDemandHourly)+len(snap.prices))
	for k, v := range rates.EC2.OnDemandHourly {
		merged[k] = v
	}
	for k, v := range snap.prices {
		if usablePriceUSD(v) {
			merged[k] = v
		}
	}
	out.EC2.OnDemandHourly = merged
	return &out, snap
}

// warmLiveEC2Prices loads region's live EC2 prices into the compute cache in
// the background, once at a time per region, and reports whether a load is in
// flight on return: one it started or one already running. Tests replace it.
var warmLiveEC2Prices = startLiveEC2PriceLoad

// liveEC2LoadTimeout bounds one background load. The web UI keeps asking
// while a load runs for at least this long (REFRESH_LOAD_TIMEOUT_MS in
// web/src/hooks/use-pricing.ts); change the two together.
const liveEC2LoadTimeout = 2 * time.Minute

// liveEC2Loading holds the cache keys with a background load in flight.
var liveEC2Loading sync.Map

// liveEC2LoadFailed holds, per cache key, the time the last background load
// failed. An expired SSO session or a profile without pricing:GetProducts
// fails every time; without this, every request and every UI poll would try
// again.
var liveEC2LoadFailed sync.Map

// liveEC2RetryAfterFailure is how long after a failed load no new one starts.
// Requests in that time get the stale map or the table and are not marked
// refreshing, because nothing is loading.
var liveEC2RetryAfterFailure = 5 * time.Minute

// liveEC2Now is the clock the failure cooldown reads. Tests replace it.
var liveEC2Now = time.Now

// liveEC2CoolingDown reports whether key's last load failed within
// liveEC2RetryAfterFailure.
func liveEC2CoolingDown(key string) bool {
	failedAt, ok := liveEC2LoadFailed.Load(key)
	return ok && liveEC2Now().Sub(failedAt.(time.Time)) < liveEC2RetryAfterFailure
}

// liveEC2PricesLoading reports whether a background load for region, under
// the selected profile, is in flight.
func liveEC2PricesLoading(region string) bool {
	_, loading := liveEC2Loading.Load(pricingpkg.CacheKey(selectedAWSProfile, region))
	return loading
}

// fetchLiveEC2Prices reads region's on-demand prices from the Price List API.
// Tests replace it.
var fetchLiveEC2Prices = func(ctx context.Context, profile, region string) (map[string]float64, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(profile),
		awsconfig.WithRegion(computePricingAPIRegion),
	)
	if err != nil {
		return nil, err
	}
	return fetchRegionOnDemandPrices(ctx, awspricing.NewFromConfig(cfg), region)
}

// startLiveEC2PriceLoad starts one background load for region unless one is
// already running or the last one failed within liveEC2RetryAfterFailure, and
// reports whether a load is in flight. The load is an unforced GetOrFetch: an
// entry past computeOnDemandTTL is refetched by it, and the stale entry stays
// readable until the new one replaces it. A load the instance picker already
// has in flight for the key is joined rather than repeated.
func startLiveEC2PriceLoad(region string) bool {
	profile := selectedAWSProfile
	key := pricingpkg.CacheKey(profile, region)
	if liveEC2CoolingDown(key) {
		return false
	}
	if _, busy := liveEC2Loading.LoadOrStore(key, true); busy {
		return true
	}
	go func() {
		defer liveEC2Loading.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), liveEC2LoadTimeout)
		defer cancel()
		_, _, err := computeOnDemandCache.GetOrFetch(key, computeOnDemandTTL, false, func() (map[string]float64, error) {
			prices, err := fetchLiveEC2Prices(ctx, profile, region)
			if err == nil {
				saveLiveEC2Prices(key, region, prices, time.Now())
			}
			return prices, err
		})
		// Recorded before the deferred Delete runs, so no request in between
		// can start another load.
		if err != nil {
			liveEC2LoadFailed.Store(key, liveEC2Now())
			log.Printf("[Pricing] live EC2 prices for %s: %v (next try in %v)", region, err, liveEC2RetryAfterFailure)
			return
		}
		liveEC2LoadFailed.Delete(key)
	}()
	return true
}
