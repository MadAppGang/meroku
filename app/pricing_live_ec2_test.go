package main

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pricingpkg "madappgang.com/meroku/pricing"
)

// No test may reach the AWS Price List API through a pricing call, and no test
// may read or write the user's real price cache. Tests that exercise the disk
// cache point it at t.TempDir() with useEC2PriceDiskCache.
func init() {
	warmLiveEC2Prices = func(string) bool { return false }
	fetchLiveEC2Prices = func(context.Context, string, string) (map[string]float64, error) {
		return nil, errors.New("tests never call the AWS Price List API")
	}
	ec2PriceCacheDir = func() string { return "" }
}

func tableRates(t *testing.T) *pricingpkg.PriceRates {
	t.Helper()
	old := warmLiveEC2Prices
	t.Cleanup(func() { warmLiveEC2Prices = old })
	warmLiveEC2Prices = func(string) bool { return false }
	rates, err := pricingpkg.NewAWSPricingClient(nil).FetchRates("us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	return rates
}

// useEC2PriceDiskCache points the disk cache at a fresh temp dir for one test.
func useEC2PriceDiskCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := ec2PriceCacheDir
	t.Cleanup(func() { ec2PriceCacheDir = old })
	ec2PriceCacheDir = func() string { return dir }
	return dir
}

// stubLoader is the real background loader over a fake Price List call. Every
// fetch waits on release (closed immediately when nil) and is counted.
type stubLoader struct {
	calls   atomic.Int32
	release chan struct{}
	prices  map[string]float64
	err     error
}

func useStubLoader(t *testing.T, l *stubLoader) {
	t.Helper()
	oldWarm, oldFetch := warmLiveEC2Prices, fetchLiveEC2Prices
	t.Cleanup(func() { warmLiveEC2Prices, fetchLiveEC2Prices = oldWarm, oldFetch })
	warmLiveEC2Prices = startLiveEC2PriceLoad
	fetchLiveEC2Prices = func(_ context.Context, _ string, _ string) (map[string]float64, error) {
		l.calls.Add(1)
		if l.release != nil {
			<-l.release
		}
		return l.prices, l.err
	}
}

// stubWarm replaces warmLiveEC2Prices for one test.
func stubWarm(t *testing.T, warm func(string) bool) {
	t.Helper()
	old := warmLiveEC2Prices
	t.Cleanup(func() { warmLiveEC2Prices = old })
	warmLiveEC2Prices = warm
}

// forgetRegion drops region's in-memory entry and any recorded load failure
// before and after the test.
func forgetRegion(t *testing.T, region string) string {
	t.Helper()
	key := pricingpkg.CacheKey(selectedAWSProfile, region)
	forget := func() {
		computeOnDemandCache.Invalidate(key)
		liveEC2LoadFailed.Delete(key)
	}
	forget()
	t.Cleanup(forget)
	return key
}

// useClock sets the clock the background loader reads for one test.
func useClock(t *testing.T, now *time.Time) {
	t.Helper()
	old := liveEC2Now
	t.Cleanup(func() { liveEC2Now = old })
	liveEC2Now = func() time.Time { return *now }
}

// waitLoadDone polls until no background load is in flight for region.
func waitLoadDone(t *testing.T, region string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for liveEC2PricesLoading(region) {
		if time.Now().After(deadline) {
			t.Fatal("the background load never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLiveEC2PricesFillTheTypesTheTableLacks(t *testing.T) {
	region := "ap-test-live-1"
	key := forgetRegion(t, region)
	if _, _, err := computeOnDemandCache.GetOrFetch(key, computeOnDemandTTL, true, func() (map[string]float64, error) {
		return map[string]float64{"t3.small": 0.0264, "t3.medium": 0.0528}, nil
	}); err != nil {
		t.Fatal(err)
	}
	base := tableRates(t)
	if _, ok := base.EC2.OnDemandHourly["t3.small"]; ok {
		t.Fatal("precondition: the fallback table should not price t3.small")
	}
	tableMedium := base.EC2.OnDemandHourly["t3.medium"]

	rates := withLiveEC2Prices(base, region)

	hourly, ok := pricingpkg.EC2PoolHourly(pricingpkg.EC2PoolConfig{InstanceTypes: []string{"t3.small"}, InstanceCount: 1, CapacityType: "on_demand"}, rates)
	if !ok || hourly != 0.0264 {
		t.Fatalf("t3.small = %v, %v; want the live 0.0264", hourly, ok)
	}
	if rates.EC2.OnDemandHourly["t3.medium"] != 0.0528 {
		t.Errorf("t3.medium = %v, want the live regional price over the table's", rates.EC2.OnDemandHourly["t3.medium"])
	}
	if base.EC2.OnDemandHourly["t3.medium"] != tableMedium {
		t.Error("withLiveEC2Prices changed the rates it was given")
	}
}

func TestLiveEC2PricesNotLoadedYetStartsOneLoad(t *testing.T) {
	base := tableRates(t)
	forgetRegion(t, "ap-test-cold-1")
	var warmed []string
	stubWarm(t, func(region string) bool { warmed = append(warmed, region); return false })
	rates := withLiveEC2Prices(base, "ap-test-cold-1")

	if len(rates.EC2.OnDemandHourly) != len(base.EC2.OnDemandHourly) {
		t.Error("with nothing loaded the table must be returned as it is")
	}
	if len(warmed) != 1 || warmed[0] != "ap-test-cold-1" {
		t.Fatalf("warm calls = %q, want one for the region", warmed)
	}
}

// A region with nothing cached anywhere answers from the table at once, while
// its load runs in the background; the request after the load is live, and the
// load is on disk for the next start of meroku.
func TestLiveEC2PricesNeverWaitForTheLoad(t *testing.T) {
	dir := useEC2PriceDiskCache(t)
	base := tableRates(t)
	region := "ap-test-first-1"
	key := forgetRegion(t, region)
	loader := &stubLoader{release: make(chan struct{}), prices: map[string]float64{"t3.small": 0.0264}}
	useStubLoader(t, loader)

	start := time.Now()
	rates := withLiveEC2Prices(base, region)
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Fatalf("withLiveEC2Prices took %v with the load blocked; it must not wait", took)
	}
	if _, ok := rates.EC2.OnDemandHourly["t3.small"]; ok {
		t.Error("t3.small was priced before its load finished; with nothing cached it must stay unknown")
	}
	if !liveEC2PricesLoading(region) {
		t.Error("no background load in flight after a request found nothing cached")
	}

	close(loader.release)
	waitLoadDone(t, region)

	rates = withLiveEC2Prices(base, region)
	if rates.EC2.OnDemandHourly["t3.small"] != 0.0264 {
		t.Fatalf("t3.small = %v after the load, want the live 0.0264", rates.EC2.OnDemandHourly["t3.small"])
	}
	if got := loader.calls.Load(); got != 1 {
		t.Errorf("Price List calls = %d, want 1", got)
	}
	path, ok := ec2PriceCacheFile(key)
	if !ok {
		t.Fatal("no cache file path with a cache dir set")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the loaded prices were not written to disk under %s: %v", dir, err)
	}
}

// After a restart the memory cache is empty; the prices saved by the last
// load answer at once, without a Price List call while they are within TTL.
func TestLiveEC2PricesComeFromDiskAfterRestart(t *testing.T) {
	useEC2PriceDiskCache(t)
	base := tableRates(t)
	region := "ap-test-disk-1"
	key := forgetRegion(t, region)
	loader := &stubLoader{prices: map[string]float64{"t3.small": 0.0264}}
	useStubLoader(t, loader)

	withLiveEC2Prices(base, region)
	waitLoadDone(t, region)
	if loader.calls.Load() != 1 {
		t.Fatalf("Price List calls = %d, want 1 for the first load", loader.calls.Load())
	}

	computeOnDemandCache.Invalidate(key) // what a restart leaves behind

	rates := withLiveEC2Prices(base, region)
	if rates.EC2.OnDemandHourly["t3.small"] != 0.0264 {
		t.Fatalf("t3.small = %v, want 0.0264 from the disk cache", rates.EC2.OnDemandHourly["t3.small"])
	}
	waitLoadDone(t, region)
	if got := loader.calls.Load(); got != 1 {
		t.Errorf("Price List calls = %d, want still 1: fresh prices on disk need no fetch", got)
	}
	if liveEC2PricesLoading(region) {
		t.Error("a load is in flight for fresh prices from disk")
	}
}

// An entry older than the TTL is served as it is, and one refresh starts
// however many requests arrive while it runs.
func TestLiveEC2PricesStaleEntryServedAndRefreshedOnce(t *testing.T) {
	useEC2PriceDiskCache(t)
	base := tableRates(t)
	region := "ap-test-stale-1"
	key := forgetRegion(t, region)
	computeOnDemandCache.SeedIfAbsent(key, map[string]float64{"t3.small": 0.0200}, time.Now().Add(-computeOnDemandTTL-time.Hour))
	loader := &stubLoader{release: make(chan struct{}), prices: map[string]float64{"t3.small": 0.0264}}
	useStubLoader(t, loader)

	var wg sync.WaitGroup
	var slow atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			rates := withLiveEC2Prices(base, region)
			if time.Since(start) > 100*time.Millisecond || rates.EC2.OnDemandHourly["t3.small"] != 0.0200 {
				slow.Add(1)
			}
		}()
	}
	wg.Wait()
	if slow.Load() != 0 {
		t.Errorf("%d of 8 requests did not get the stale price at once", slow.Load())
	}
	if !liveEC2PricesLoading(region) {
		t.Fatal("no refresh started for an entry past its TTL")
	}

	close(loader.release)
	waitLoadDone(t, region)
	if got := loader.calls.Load(); got != 1 {
		t.Errorf("Price List calls = %d, want exactly one refresh", got)
	}
	if rates := withLiveEC2Prices(base, region); rates.EC2.OnDemandHourly["t3.small"] != 0.0264 {
		t.Errorf("t3.small = %v after the refresh, want 0.0264", rates.EC2.OnDemandHourly["t3.small"])
	}
}

func TestLiveEC2PricesCorruptDiskFileIgnored(t *testing.T) {
	useEC2PriceDiskCache(t)
	base := tableRates(t)
	for name, body := range map[string]string{
		"garbage": "{not json",
		"empty":   `{"fetchedAt":"2026-01-01T00:00:00Z","region":"ap-test-corrupt-1","prices":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			region := "ap-test-corrupt-1"
			key := forgetRegion(t, region)
			path, _ := ec2PriceCacheFile(key)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			warmed := 0
			stubWarm(t, func(string) bool { warmed++; return false })

			rates := withLiveEC2Prices(base, region)

			if len(rates.EC2.OnDemandHourly) != len(base.EC2.OnDemandHourly) {
				t.Error("an unusable cache file changed the prices")
			}
			if warmed != 1 {
				t.Errorf("warm calls = %d, want 1: an unusable file is the same as none", warmed)
			}
		})
	}
}

func TestLiveEC2PricesFailedLoadWritesNothing(t *testing.T) {
	useEC2PriceDiskCache(t)
	base := tableRates(t)
	region := "ap-test-failed-1"
	key := forgetRegion(t, region)
	useStubLoader(t, &stubLoader{err: errors.New("throttled")})

	withLiveEC2Prices(base, region)
	waitLoadDone(t, region)

	path, _ := ec2PriceCacheFile(key)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a failed load left a cache file (stat err %v)", err)
	}
}

// The instance picker's read saves what it fetched, and after a restart reads
// it back from disk without calling the Price List API.
func TestInstancePickerPricesUseTheDiskCache(t *testing.T) {
	useEC2PriceDiskCache(t)
	ce := testComputeEnv(t)
	fake := &fakePricing{pages: bbPricePages(t, "0.1197000000")}
	ce.pricingAPI = fake

	if got := ce.resolveComputePrices(context.Background(), false); got.source != sourceAWSAPI {
		t.Fatalf("first read source = %q, want %q", got.source, sourceAWSAPI)
	}
	path, _ := ec2PriceCacheFile(ce.key)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the picker's fetch was not saved: %v", err)
	}

	computeOnDemandCache.Invalidate(ce.key)
	calls := atomic.LoadInt32(&fake.calls)

	got := ce.resolveComputePrices(context.Background(), false)
	if got.source != sourceAWSAPI || got.prices["m7i.large"] != 0.1197 {
		t.Fatalf("after restart: source %q, m7i.large %v; want aws_api 0.1197 from disk", got.source, got.prices["m7i.large"])
	}
	if atomic.LoadInt32(&fake.calls) != calls {
		t.Error("fresh prices on disk were fetched again")
	}
}

// A load that fails is not retried on every request: inside the cooldown no
// load starts and nothing reports a refresh, so the UI stops asking. After the
// cooldown the next request tries again.
func TestLiveEC2PricesFailedLoadCoolsDown(t *testing.T) {
	tableRates(t)
	region := "ap-test-cooldown-1"
	forgetRegion(t, region)
	now := time.Now()
	useClock(t, &now)
	loader := &stubLoader{err: errors.New("the SSO session has expired")}
	useStubLoader(t, loader)

	request := func() PricingResponse {
		resp := PricingResponse{Nodes: map[string]NodePricing{}}
		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)
		waitLoadDone(t, region)
		return resp
	}

	request()
	if got := loader.calls.Load(); got != 1 {
		t.Fatalf("Price List calls = %d after the first request, want 1", got)
	}
	for i := 0; i < 3; i++ {
		now = now.Add(time.Minute)
		if resp := request(); resp.PricesRefreshing {
			t.Errorf("request %d inside the cooldown: pricesRefreshing is true with no load running", i+2)
		}
	}
	if got := loader.calls.Load(); got != 1 {
		t.Errorf("Price List calls = %d inside the cooldown, want still 1", got)
	}

	now = now.Add(liveEC2RetryAfterFailure)
	request()
	if got := loader.calls.Load(); got != 2 {
		t.Errorf("Price List calls = %d after the cooldown, want 2", got)
	}
}

// A saved fetch time in the future (a clock change, a file from another
// machine) would read as fresh until the clock caught up. Beyond a little
// clock skew such a file counts as no file.
func TestLiveEC2PricesFutureFetchTimeIgnored(t *testing.T) {
	useEC2PriceDiskCache(t)
	for name, tc := range map[string]struct {
		ahead  time.Duration
		usable bool
	}{
		"an hour ahead":  {time.Hour, false},
		"a minute ahead": {time.Minute, true},
	} {
		t.Run(name, func(t *testing.T) {
			key := forgetRegion(t, "ap-test-future-1")
			path, _ := ec2PriceCacheFile(key)
			file := ec2PriceFile{FetchedAt: time.Now().Add(tc.ahead).UTC(), Region: "ap-test-future-1", Prices: map[string]float64{"t3.small": 0.0264}}
			if err := writeEC2PriceFile(path, file); err != nil {
				t.Fatal(err)
			}

			_, _, ok := peekOnDemandPrices(key)

			if ok != tc.usable {
				t.Errorf("file with fetchedAt %v ahead: usable = %v, want %v", tc.ahead, ok, tc.usable)
			}
		})
	}
}
