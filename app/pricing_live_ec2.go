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

// withLiveEC2Prices returns a copy of rates whose EC2 on-demand prices are
// the live ones for region, read from AWS and cached by the compute catalog,
// over the dated table. The table prices a dozen types at us-east-1 rates;
// the live map prices every type the region sells. With nothing cached yet
// it starts one background load and returns rates as they are, so the next
// request has the region's prices.
func withLiveEC2Prices(rates *pricingpkg.PriceRates, region string) *pricingpkg.PriceRates {
	if rates == nil || region == "" {
		return rates
	}
	live, _, ok := computeOnDemandCache.Peek(pricingpkg.CacheKey(selectedAWSProfile, region))
	if !ok || len(live) == 0 {
		warmLiveEC2Prices(region)
		return rates
	}
	out := *rates
	merged := make(map[string]float64, len(rates.EC2.OnDemandHourly)+len(live))
	for k, v := range rates.EC2.OnDemandHourly {
		merged[k] = v
	}
	for k, v := range live {
		if usablePriceUSD(v) {
			merged[k] = v
		}
	}
	out.EC2.OnDemandHourly = merged
	return &out
}

// warmLiveEC2Prices loads region's live EC2 prices into the compute cache in
// the background, once at a time per region. Tests replace it.
var warmLiveEC2Prices = startLiveEC2PriceLoad

var liveEC2Loading sync.Map

func startLiveEC2PriceLoad(region string) {
	key := pricingpkg.CacheKey(selectedAWSProfile, region)
	if _, busy := liveEC2Loading.LoadOrStore(key, true); busy {
		return
	}
	go func() {
		defer liveEC2Loading.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithSharedConfigProfile(selectedAWSProfile),
			awsconfig.WithRegion(computePricingAPIRegion),
		)
		if err != nil {
			log.Printf("[Pricing] live EC2 prices for %s: %v", region, err)
			return
		}
		api := awspricing.NewFromConfig(cfg)
		if _, _, err := computeOnDemandCache.GetOrFetch(key, computeOnDemandTTL, false, func() (map[string]float64, error) {
			return fetchRegionOnDemandPrices(ctx, api, region)
		}); err != nil {
			log.Printf("[Pricing] live EC2 prices for %s: %v", region, err)
		}
	}()
}
