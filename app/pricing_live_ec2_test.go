package main

import (
	"testing"

	pricingpkg "madappgang.com/meroku/pricing"
)

// No test may reach the AWS Price List API through a pricing call.
func init() { warmLiveEC2Prices = func(string) {} }

func tableRates(t *testing.T) *pricingpkg.PriceRates {
	t.Helper()
	old := warmLiveEC2Prices
	t.Cleanup(func() { warmLiveEC2Prices = old })
	warmLiveEC2Prices = func(string) {}
	rates, err := pricingpkg.NewAWSPricingClient(nil).FetchRates("us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	return rates
}

func TestLiveEC2PricesFillTheTypesTheTableLacks(t *testing.T) {
	region := "ap-test-live-1"
	key := pricingpkg.CacheKey(selectedAWSProfile, region)
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
	var warmed []string
	warmLiveEC2Prices = func(region string) { warmed = append(warmed, region) }
	rates := withLiveEC2Prices(base, "ap-test-cold-1")

	if len(rates.EC2.OnDemandHourly) != len(base.EC2.OnDemandHourly) {
		t.Error("with nothing loaded the table must be returned as it is")
	}
	if len(warmed) != 1 || warmed[0] != "ap-test-cold-1" {
		t.Fatalf("warm calls = %q, want one for the region", warmed)
	}
}
