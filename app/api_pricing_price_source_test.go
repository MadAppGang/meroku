package main

import (
	"testing"
	"time"
)

func poolEnv(region string, types ...string) Env {
	minSize, maxSize := 1, 2
	return Env{
		Region: region,
		Compute: Compute{Pools: []ComputePool{{
			Name:          "workers",
			InstanceTypes: types,
			CapacityType:  "on_demand",
			MinSize:       &minSize,
			MaxSize:       &maxSize,
		}}},
	}
}

func poolLevelsPriceSource(t *testing.T, resp PricingResponse) string {
	t.Helper()
	node, ok := resp.Nodes[computePoolNodeKey("workers")]
	if !ok {
		t.Fatal("no pool node in the response")
	}
	source := node.Levels["startup"].Details["price_source"]
	for level, price := range node.Levels {
		if price.Details["price_source"] != source {
			t.Errorf("level %s price_source = %q, startup has %q", level, price.Details["price_source"], source)
		}
	}
	return source
}

func TestPricingResponsePoolPriceSource(t *testing.T) {
	useEC2PriceDiskCache(t)
	tableRates(t)

	t.Run("live within TTL is aws_api", func(t *testing.T) {
		region := "ap-test-src-live"
		key := forgetRegion(t, region)
		computeOnDemandCache.SeedIfAbsent(key, map[string]float64{"t3.small": 0.0264}, time.Now())
		resp := PricingResponse{Nodes: map[string]NodePricing{}}

		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)

		if got := poolLevelsPriceSource(t, resp); got != "aws_api" {
			t.Errorf("price_source = %q, want aws_api", got)
		}
		if resp.PricesRefreshing {
			t.Error("pricesRefreshing is true with fresh live prices")
		}
	})

	t.Run("live past TTL is cached and refreshing", func(t *testing.T) {
		region := "ap-test-src-stale"
		key := forgetRegion(t, region)
		computeOnDemandCache.SeedIfAbsent(key, map[string]float64{"t3.small": 0.0200}, time.Now().Add(-computeOnDemandTTL-time.Hour))
		loader := &stubLoader{release: make(chan struct{}), prices: map[string]float64{"t3.small": 0.0264}}
		useStubLoader(t, loader)
		resp := PricingResponse{Nodes: map[string]NodePricing{}}

		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)

		if got := poolLevelsPriceSource(t, resp); got != "cached" {
			t.Errorf("price_source = %q, want cached", got)
		}
		if !resp.PricesRefreshing {
			t.Error("pricesRefreshing is false while the refresh this request started is in flight")
		}

		close(loader.release)
		waitLoadDone(t, region)
		resp = PricingResponse{Nodes: map[string]NodePricing{}}
		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)
		if resp.PricesRefreshing {
			t.Error("pricesRefreshing is true after the refresh finished")
		}
		if got := poolLevelsPriceSource(t, resp); got != "aws_api" {
			t.Errorf("price_source after the refresh = %q, want aws_api", got)
		}
	})

	t.Run("table price is fallback", func(t *testing.T) {
		region := "ap-test-src-table"
		forgetRegion(t, region)
		loader := &stubLoader{release: make(chan struct{}), prices: map[string]float64{"t3.small": 0.0264}}
		useStubLoader(t, loader)
		resp := PricingResponse{Nodes: map[string]NodePricing{}}

		addComputePoolNodes(&resp, poolEnv(region, "t3.medium"), region)

		if got := poolLevelsPriceSource(t, resp); got != "fallback" {
			t.Errorf("price_source = %q, want fallback", got)
		}
		if !resp.PricesRefreshing {
			t.Error("pricesRefreshing is false while the first load is in flight")
		}
		close(loader.release)
		waitLoadDone(t, region)
	})
}

// A response is marked refreshing whenever the prices it carries may already
// be superseded: the snapshot was not fresh and a load was running or started
// when it was taken. That holds even when the load ends before the response
// is finished, the case where reading the loading flag alone says "done" for
// table prices and the UI stops asking.
func TestPricingResponseRefreshingWhenLoadEndsBeforeFlagRead(t *testing.T) {
	tableRates(t)

	t.Run("load started by the request", func(t *testing.T) {
		region := "ap-test-race-start"
		forgetRegion(t, region)
		useStubLoader(t, &stubLoader{prices: map[string]float64{"t3.small": 0.0264}})
		stubWarm(t, func(r string) bool {
			loading := startLiveEC2PriceLoad(r)
			waitLoadDone(t, r) // the load ends before the response is finished
			return loading
		})
		resp := PricingResponse{Nodes: map[string]NodePricing{}}

		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)

		if got := poolLevelsPriceSource(t, resp); got != "" {
			t.Fatalf("price_source = %q, want none: t3.small has no table price", got)
		}
		if !resp.PricesRefreshing {
			t.Error("pricesRefreshing is false for table prices whose load ended mid-request")
		}

		resp = PricingResponse{Nodes: map[string]NodePricing{}}
		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)
		if got := poolLevelsPriceSource(t, resp); got != "aws_api" {
			t.Errorf("next response price_source = %q, want aws_api", got)
		}
		if resp.PricesRefreshing {
			t.Error("next response: pricesRefreshing is true with fresh live prices")
		}
	})

	t.Run("load already in flight", func(t *testing.T) {
		region := "ap-test-race-inflight"
		key := forgetRegion(t, region)
		computeOnDemandCache.SeedIfAbsent(key, map[string]float64{"t3.small": 0.0200}, time.Now().Add(-computeOnDemandTTL-time.Hour))
		loader := &stubLoader{release: make(chan struct{}), prices: map[string]float64{"t3.small": 0.0264}}
		useStubLoader(t, loader)
		startLiveEC2PriceLoad(region)
		stubWarm(t, func(r string) bool {
			loading := startLiveEC2PriceLoad(r)
			close(loader.release)
			waitLoadDone(t, r)
			return loading
		})
		resp := PricingResponse{Nodes: map[string]NodePricing{}}

		addComputePoolNodes(&resp, poolEnv(region, "t3.small"), region)

		if got := poolLevelsPriceSource(t, resp); got != "cached" {
			t.Fatalf("price_source = %q, want cached", got)
		}
		if !resp.PricesRefreshing {
			t.Error("pricesRefreshing is false for stale prices whose refresh ended mid-request")
		}
		if got := loader.calls.Load(); got != 1 {
			t.Errorf("Price List calls = %d, want 1", got)
		}
	})
}
