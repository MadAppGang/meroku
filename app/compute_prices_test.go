package main

// C-16 and aws-verified-contracts.md section 4: one region-wide GetProducts,
// and the Reserved matrix never survives the parse.
//
// The SKU fixture below is synthetic but structurally faithful: opaque term and
// rate codes, a single OnDemand term, and twelve Reserved offerings, because
// that ratio is the whole reason projection has to happen at decode time.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/smithy-go"
)

// syntheticSKU builds one price-list entry with the shape AWS actually returns:
// an opaque OnDemand term code, an opaque rate code, and a full Reserved matrix
// alongside. reservedMarker is planted in every Reserved rate code so a test can
// prove none of it survived.
func syntheticSKU(t *testing.T, instanceType, usd, reservedMarker string) string {
	t.Helper()

	reserved := map[string]any{}
	for i := 0; i < 12; i++ {
		termCode := fmt.Sprintf("%s.RESERVED%02d", reservedMarker, i)
		reserved[termCode] = map[string]any{
			"priceDimensions": map[string]any{
				fmt.Sprintf("%s.RATE%02d", reservedMarker, i): map[string]any{
					"unit":         "Hrs",
					"description":  "Linux/UNIX (Amazon VPC), reserved",
					"pricePerUnit": map[string]any{"USD": "9.9900000000"},
				},
			},
			"termAttributes": map[string]any{
				"LeaseContractLength": "3yr",
				"OfferingClass":       "convertible",
				"PurchaseOption":      "All Upfront",
			},
		}
	}

	sku := map[string]any{
		"product": map[string]any{
			"productFamily": "Compute Instance",
			"sku":           "FAKESKU00000000",
			"attributes": map[string]any{
				"instanceType":   instanceType,
				"regionCode":     "ap-southeast-2",
				"tenancy":        "Shared",
				"capacitystatus": "Used",
			},
		},
		"terms": map[string]any{
			"OnDemand": map[string]any{
				"OPAQUEONDEMANDTERM.JRTCKXETXF": map[string]any{
					"priceDimensions": map[string]any{
						"OPAQUEONDEMANDTERM.JRTCKXETXF.6YS6EN2CT7": map[string]any{
							"unit":         "Hrs",
							"description":  "$" + usd + " per On Demand Linux " + instanceType + " Instance Hour",
							"pricePerUnit": map[string]any{"USD": usd},
						},
					},
				},
			},
			"Reserved": reserved,
		},
	}

	data, err := json.Marshal(sku)
	if err != nil {
		t.Fatalf("marshal SKU fixture: %v", err)
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// C-16 -- one call for the region, not one per type
// ---------------------------------------------------------------------------

func TestComputePrices_SingleRegionQuery(t *testing.T) {
	// Two pages of a hundred SKUs each: a catalog this size fetched per type
	// would be two hundred requests, which is the failure this test pins.
	page := func(names []string, next *string) *awspricing.GetProductsOutput {
		out := &awspricing.GetProductsOutput{NextToken: next}
		for i, n := range names {
			out.PriceList = append(out.PriceList, syntheticSKU(t, n, fmt.Sprintf("0.%04d000000", i+100), "RSV"))
		}
		return out
	}

	var first, second []string
	for i := 0; i < 100; i++ {
		first = append(first, fmt.Sprintf("m7i.size%03d", i))
		second = append(second, fmt.Sprintf("r7i.size%03d", i))
	}

	api := &fakePricing{pages: []*awspricing.GetProductsOutput{
		page(first, aws.String("page2")),
		page(second, nil),
	}}

	got, err := fetchRegionOnDemandPrices(context.Background(), api, "ap-southeast-2")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if n := atomic.LoadInt32(&api.calls); n != 2 {
		t.Errorf("GetProducts called %d times for a 200-type region, want 2 (one paginated "+
			"region-wide query). One call per type is ~903 requests against a 4s budget and "+
			"throttles on the first user request", n)
	}
	if len(got) != 200 {
		t.Errorf("priced %d types, want 200", len(got))
	}

	t.Run("the filter set is the verified one", func(t *testing.T) {
		if len(api.allFilt) == 0 {
			t.Fatal("no filters recorded")
		}
		filters := strings.Join(api.allFilt[0], " ")
		for _, want := range []string{
			"productFamily=Compute Instance",
			"regionCode=ap-southeast-2",
			"tenancy=Shared",
			"operatingSystem=Linux",
			"preInstalledSw=NA",
			"capacitystatus=Used",
		} {
			if !strings.Contains(filters, want) {
				t.Errorf("filter %q is missing from %q", want, filters)
			}
		}
	})

	t.Run("regionCode is a filter, never the client's region", func(t *testing.T) {
		// The client this ran against is the us-east-1-pinned one; the only
		// place ap-southeast-2 may appear is in the filter value.
		if !strings.Contains(strings.Join(api.allFilt[0], " "), "regionCode=ap-southeast-2") {
			t.Error("the environment's region did not travel as the regionCode filter value (C-2)")
		}
	})

	t.Run("the Reserved matrix is never retained", func(t *testing.T) {
		// The fixture plants "RSV" in every reserved term and rate code. If any
		// of it survived into the cache entry, it would show up here.
		blob, merr := json.Marshal(got)
		if merr != nil {
			t.Fatalf("marshal result: %v", merr)
		}
		if strings.Contains(string(blob), "RSV") {
			t.Error("a Reserved rate code survived into the parsed result; " +
				"terms.Reserved must be discarded as the page is decoded, not after")
		}
		if strings.Contains(string(blob), "9.99") {
			t.Error("a Reserved price survived into the parsed result")
		}
		// And the value is the OnDemand figure, not a reserved one.
		if v := got["m7i.size000"]; v != 0.0100 {
			t.Errorf("m7i.size000 = %v, want 0.0100 (the OnDemand rate)", v)
		}
	})
}

func TestComputePrices_UnparseablePriceIsNotZero(t *testing.T) {
	api := &fakePricing{pages: []*awspricing.GetProductsOutput{{
		PriceList: []string{
			syntheticSKU(t, "m7i.large", "0.1197000000", "RSV"),
			syntheticSKU(t, "broken.large", "not-a-number", "RSV"),
		},
	}}}

	got, err := fetchRegionOnDemandPrices(context.Background(), api, "ap-southeast-2")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, ok := got["broken.large"]; ok {
		t.Error("an unparseable price became a map entry; a 0 here would make the type free " +
			"and collapse every cost sub-score that divides by the cheapest candidate")
	}
	if got["m7i.large"] != 0.1197 {
		t.Errorf("m7i.large = %v, want 0.1197", got["m7i.large"])
	}
}

func TestResolveComputePrices_FallsBackWithTheRegionLabelled(t *testing.T) {
	ce := testComputeEnv(t)
	ce.pricingAPI = &fakePricing{err: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "synthetic"}}

	got := ce.resolveComputePrices(context.Background(), false)

	if got.source != sourceFallback {
		t.Errorf("source = %q, want %q", got.source, sourceFallback)
	}
	if got.credState != credDenied {
		t.Errorf("credentialsState = %q, want %q — a restricted role must not look healthy", got.credState, credDenied)
	}
	if got.pricingRegion != "us-east-1" {
		t.Errorf("pricingRegion = %q, want us-east-1: the fallback table is us-east-1 list "+
			"pricing and ap-southeast-2 runs 15-25%% above it", got.pricingRegion)
	}
	if !strings.Contains(got.notice, actionGetProducts) {
		t.Errorf("notice %q does not name the action the profile cannot call", got.notice)
	}
	if len(got.prices) == 0 {
		t.Error("fallback price map is empty")
	}
}

// ---------------------------------------------------------------------------
// NFR-1 -- the cold price fetch is waited for, not waited on forever
// ---------------------------------------------------------------------------

// Measured live: DescribeInstanceTypes is 2.2s for 903 types and GetProducts is
// 6.6s for 832 SKUs / 6.5 MB, so even run concurrently a cold catalog request
// would take 7s against a 4s budget. The endpoint therefore stops WAITING at
// the budget without stopping the FETCH.
func TestResolveCatalogAndPrices_StopsWaitingWithoutStoppingFetching(t *testing.T) {
	ce := testComputeEnv(t)
	ce.ec2 = &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{{
		InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("m7i.large", 2, 8192, "x86_64")},
	}}}
	ce.pricingAPI = &fakePricing{
		delay: 600 * time.Millisecond,
		pages: []*awspricing.GetProductsOutput{{
			PriceList: []string{syntheticSKU(t, "m7i.large", "0.1197000000", "RSV")},
		}},
	}

	// A request whose ctx is cancelled the instant it returns -- exactly what an
	// http.Request's context does.
	ctx, cancel := context.WithCancel(context.Background())

	start := time.Now()
	cat, prices := ce.resolveCatalogAndPrices(ctx, false, 100*time.Millisecond)
	elapsed := time.Since(start)
	cancel()

	if elapsed > 400*time.Millisecond {
		t.Errorf("waited %v for a 100ms budget", elapsed)
	}
	if cat.source != sourceAWSAPI {
		t.Errorf("catalog source = %q; the catalog read is fast and must not be held up by prices", cat.source)
	}
	if prices.source != sourceFallback {
		t.Errorf("prices source = %q, want %q while the live fetch is still in flight", prices.source, sourceFallback)
	}
	if prices.pricingRegion != "us-east-1" {
		t.Errorf("pricingRegion = %q; an indicative price must never be attributed to the "+
			"selected region", prices.pricingRegion)
	}
	if !strings.Contains(prices.notice, "still loading") {
		t.Errorf("notice = %q; a user shown indicative prices must be told they will be replaced", prices.notice)
	}

	// The fetch was detached from the request context, so cancelling the request
	// must not have thrown the result away. Without that, the NEXT request pays
	// the same 6.6s again -- and again.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, ok := computeOnDemandCache.Peek(ce.key); ok {
			later := ce.resolveComputePrices(context.Background(), false)
			if later.source != sourceAWSAPI {
				t.Errorf("after the fetch completed, source = %q, want %q", later.source, sourceAWSAPI)
			}
			if later.prices["m7i.large"] != 0.1197 {
				t.Errorf("cached price = %v, want 0.1197", later.prices["m7i.large"])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the price fetch never populated the cache; cancelling the request killed it, so " +
		"every subsequent request would pay the full cold cost again")
}

func TestResolveCatalogAndPrices_ConcurrentRequestsShareOneWaitingWindow(t *testing.T) {
	ce := testComputeEnv(t)
	ce.ec2 = &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{{
		InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("m7i.large", 2, 8192, "x86_64")},
	}}}
	ce.pricingAPI = &fakePricing{
		delay: 2 * time.Second,
		pages: []*awspricing.GetProductsOutput{{
			PriceList: []string{syntheticSKU(t, "m7i.large", "0.1197000000", "RSV")},
		}},
	}

	ctx := context.Background()
	first := time.Now()
	ce.resolveCatalogAndPrices(ctx, false, 150*time.Millisecond)
	firstWait := time.Since(first)

	second := time.Now()
	ce.resolveCatalogAndPrices(ctx, false, 150*time.Millisecond)
	secondWait := time.Since(second)

	if firstWait < 100*time.Millisecond {
		t.Errorf("the first request waited %v; it should spend its budget waiting for live prices", firstWait)
	}
	if secondWait > 60*time.Millisecond {
		t.Errorf("the second request waited %v; the window is measured from when the FETCH "+
			"started, so a polling UI must not pay the budget once per poll", secondWait)
	}
}

// ---------------------------------------------------------------------------
// applyPrices
// ---------------------------------------------------------------------------

func TestApplyPrices_UnpricedIsNullNotZero(t *testing.T) {
	types := []ComputeInstanceType{
		{InstanceType: "m7i.large"},
		{InstanceType: "exotic.42xlarge"},
	}
	usedFallback := applyPrices(types, map[string]float64{"m7i.large": 0.10}, priceSourceAWS)

	if usedFallback {
		t.Error("applyPrices reported a fallback price for a live map")
	}
	if types[0].OnDemandHourly == nil || *types[0].OnDemandHourly != 0.10 {
		t.Errorf("m7i.large price = %v, want 0.10", types[0].OnDemandHourly)
	}
	if types[0].PriceSource != priceSourceAWS {
		t.Errorf("m7i.large priceSource = %q, want %q", types[0].PriceSource, priceSourceAWS)
	}
	if types[1].OnDemandHourly != nil {
		t.Error("an unpriced type got a number; FR-5 requires null, and a 0 would mean free")
	}
	if types[1].PriceSource != priceSourceUnavailable {
		t.Errorf("unpriced priceSource = %q, want %q", types[1].PriceSource, priceSourceUnavailable)
	}

	t.Run("a fallback hit is reported so the envelope can label the region", func(t *testing.T) {
		fb := []ComputeInstanceType{{InstanceType: "m7i.large"}}
		if !applyPrices(fb, map[string]float64{"m7i.large": 0.10}, priceSourceFallback) {
			t.Error("a fallback price was applied without reporting it; the envelope would " +
				"attribute a us-east-1 figure to the selected region")
		}
	})
}

// ---------------------------------------------------------------------------
// Spot
// ---------------------------------------------------------------------------

func spotRecord(instanceType, az, price string, ts time.Time) ec2types.SpotPrice {
	return ec2types.SpotPrice{
		InstanceType:     ec2types.InstanceType(instanceType),
		AvailabilityZone: aws.String(az),
		SpotPrice:        aws.String(price),
		Timestamp:        aws.Time(ts),
	}
}

func TestFetchSpotPriceHistory_OneCallForEveryType(t *testing.T) {
	now := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	api := &fakeEC2{spotPages: []*ec2.DescribeSpotPriceHistoryOutput{{
		SpotPriceHistory: []ec2types.SpotPrice{
			spotRecord("m7i.large", "ap-southeast-2a", "0.0412", now),
			spotRecord("m7i.large", "ap-southeast-2b", "0.0431", now),
			spotRecord("m7i.large", "ap-southeast-2c", "0.0450", now),
			spotRecord("r7i.large", "ap-southeast-2a", "0.0600", now),
		},
	}}}

	got, err := fetchSpotPriceHistory(context.Background(), api, []string{"m7i.large", "r7i.large", "exotic.42xlarge"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if n := atomic.LoadInt32(&api.spotCalls); n != 1 {
		t.Errorf("DescribeSpotPriceHistory called %d times for 3 types, want 1 (NFR-8)", n)
	}
	if len(got) != 3 {
		t.Fatalf("got %d quotes, want one per requested type", len(got))
	}

	byName := map[string]SpotQuote{}
	for _, q := range got {
		byName[q.InstanceType] = q
	}

	m := byName["m7i.large"]
	if !m.SpotAvailable {
		t.Error("m7i.large reported no spot market")
	}
	if len(m.ByAZ) != 3 {
		t.Errorf("m7i.large has %d AZs, want 3", len(m.ByAZ))
	}
	if m.Min == nil || *m.Min != 0.0412 {
		t.Errorf("min = %v, want 0.0412", m.Min)
	}
	if m.Max == nil || *m.Max != 0.0450 {
		t.Errorf("max = %v, want 0.0450", m.Max)
	}
	if m.Median == nil || *m.Median != 0.0431 {
		t.Errorf("median = %v, want 0.0431", m.Median)
	}

	missing := byName["exotic.42xlarge"]
	if missing.SpotAvailable {
		t.Error("a type with no history reported spotAvailable true")
	}
	if missing.Median != nil {
		t.Error("a type with no history got a median; EC-5 requires null so the UI cannot " +
			"print a bid nobody quoted")
	}
}

func TestFetchSpotPriceHistory_RetriesTheVPCProductDescription(t *testing.T) {
	now := time.Now()
	api := &fakeEC2{spotPages: []*ec2.DescribeSpotPriceHistoryOutput{
		{}, // Linux/UNIX: nothing, as a VPC-only account answers
		{SpotPriceHistory: []ec2types.SpotPrice{spotRecord("m7i.large", "ap-southeast-2a", "0.0412", now)}},
	}}

	got, err := fetchSpotPriceHistory(context.Background(), api, []string{"m7i.large"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if n := atomic.LoadInt32(&api.spotCalls); n != 2 {
		t.Errorf("called %d times, want 2 — an empty Linux/UNIX result is not proof that spot "+
			"is unavailable", n)
	}
	if !got[0].SpotAvailable {
		t.Error("the retry's data was discarded")
	}
	if api.lastSpot == nil || api.lastSpot.ProductDescriptions[0] != spotProductLinuxVPC {
		t.Errorf("retry used %v, want %q", api.lastSpot.ProductDescriptions, spotProductLinuxVPC)
	}
}

func TestSpotMedians(t *testing.T) {
	median := 0.05
	quotes := []SpotQuote{
		{InstanceType: "m7i.large", Median: &median, SpotAvailable: true},
		{InstanceType: "r7i.large"},
	}
	got := spotMedians(quotes)
	if got["m7i.large"] != 0.05 {
		t.Errorf("m7i.large = %v, want 0.05", got["m7i.large"])
	}
	if _, ok := got["r7i.large"]; ok {
		t.Error("a type with no median produced a map entry; the recommender would price it at 0")
	}
}
