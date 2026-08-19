package main

// EC2 prices: on-demand from the Price List API, spot from
// DescribeSpotPriceHistory.
//
// Two rules govern this file, and both are the answer to a measured failure
// rather than a preference (architecture.md section 2.5, aws-verified-contracts.md
// section 4):
//
//  1. ONE paginated GetProducts for the whole region, never one per instance
//     type. A region has ~903 types; per-type fetching is ~903 requests against
//     NFR-1's 4s budget, and a wide fan-out turns the Pricing API's throttle
//     into a ThrottlingException on the first request a user makes.
//
//  2. Projection happens at DECODE time and the raw response is never cached. A
//     single-type GetProducts response is ~8 KB because `terms` carries the full
//     Reserved matrix -- twelve reserved offerings alongside the one OnDemand
//     term this feature wants. That is ~7 MB per region per full refresh, and it
//     is the same ~7 MB whether it arrives in 903 responses or in 10. Batching
//     fixes the call count, not the payload; decoding into a struct that has no
//     field for Reserved is what fixes the payload, because encoding/json
//     discards what it cannot store.

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"

	pricingpkg "madappgang.com/meroku/pricing"
)

// computePricingAPIRegion is where the AWS Price List API lives. It is a
// property of the API, not a choice: the service is served from us-east-1 and
// ap-south-1 only. The environment's region is expressed as the `regionCode`
// FILTER VALUE, never as the client's region (C-2).
const computePricingAPIRegion = "us-east-1"

// getProductsPageSize is the API cap. ~903 SKUs per region is ~10 pages.
const getProductsPageSize = 100

// maxSpotHistoryPages bounds the region-wide spot read. ~903 types x ~3 AZs is
// ~2,700 records at 1,000 per page, so five pages is headroom rather than a
// truncation risk -- but an unbounded loop against a paginated API is a hang
// waiting for a bad day.
const (
	maxSpotHistoryPages = 5
	spotHistoryPageSize = 1000
)

// spotProductLinux and spotProductLinuxVPC are the two product descriptions a
// modern account may report. Classic-EC2-era accounts answer to the first; the
// second is what a VPC-only account returns, and an empty result on the first
// is not proof that spot is unavailable (research.md:2045-2048).
const (
	spotProductLinux    = "Linux/UNIX"
	spotProductLinuxVPC = "Linux/UNIX (Amazon VPC)"
)

// ---------------------------------------------------------------------------
// On-demand -- one region-wide GetProducts
// ---------------------------------------------------------------------------

// pricingSKU is the decode-time projection, and the whole point of this file.
//
// It has NO field for terms.Reserved, so the twelve reserved offerings in every
// SKU are parsed and dropped rather than retained. Decoding a page into
// map[string]interface{} and extracting afterwards would hold ~800 KB of live
// heap per page for ~1 KB of useful data.
//
// Both keys under terms.OnDemand and under priceDimensions are opaque generated
// codes (aws-verified-contracts.md section 3), so the parser takes the single
// child rather than looking up a known key.
type pricingSKU struct {
	Product struct {
		Attributes struct {
			InstanceType string `json:"instanceType"`
		} `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				PricePerUnit struct {
					USD string `json:"USD"`
				} `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// usablePriceUSD is the single test every price string in this file has to
// pass before it becomes a number the product will publish or divide by.
//
// The err/<=0 pair it replaces looked total and is not. strconv.ParseFloat
// accepts the literals "Infinity", "+Inf" and "NaN" with a nil error -- they
// are legal float64 syntax -- and neither survives the `<= 0` gate: +Inf is
// genuinely greater than zero, and EVERY comparison against NaN is false, so
// `NaN <= 0` is false too. Both then travel the whole pipeline and reach
// encoding/json, which refuses to encode either and aborts the response
// mid-object: the client gets HTTP 200 with a truncated body, and the server
// logs a success (NFR-6).
//
// A non-finite or non-positive price is treated exactly as an ABSENT price,
// which the design already answers: the type keeps a nil OnDemandHourly, is
// listed with priceSource "unavailable" and stays manually selectable (FR-5,
// EC-2), and rule R7 excludes it from ranking. It is never substituted with 0.
// Zero is the cheapest price there is; it would win the cost dimension outright
// and turn a bad AWS record into a confidently wrong recommendation.
func usablePriceUSD(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}

// onDemandUSD extracts terms.OnDemand.<opaque>.priceDimensions.<opaque>
// .pricePerUnit.USD. It returns ok=false rather than 0 when the figure is
// missing, unparseable or not a real number, because a 0 here would make the
// type free and collapse every cost sub-score that divides by the cheapest
// candidate.
func (s pricingSKU) onDemandUSD() (float64, bool) {
	for _, term := range s.Terms.OnDemand {
		for _, dim := range term.PriceDimensions {
			v, err := strconv.ParseFloat(dim.PricePerUnit.USD, 64)
			if err != nil || !usablePriceUSD(v) {
				continue
			}
			return v, true
		}
	}
	return 0, false
}

// regionOnDemandFilters is the filter set verified against a live account
// (aws-verified-contracts.md section 3). It returns exactly one SKU per
// instance type.
//
// capacitystatus=Used is the one that is easy to omit and expensive to omit:
// without it the query also returns reserved-capacity SKUs, and the resulting
// map silently carries the wrong price.
func regionOnDemandFilters(regionCode string) []pricingtypes.Filter {
	term := pricingtypes.FilterTypeTermMatch
	f := func(field, value string) pricingtypes.Filter {
		return pricingtypes.Filter{Type: term, Field: aws.String(field), Value: aws.String(value)}
	}
	return []pricingtypes.Filter{
		f("productFamily", "Compute Instance"),
		f("regionCode", regionCode), // NOT the client's region -- the filter value
		f("tenancy", "Shared"),
		f("operatingSystem", "Linux"),
		f("preInstalledSw", "NA"),
		f("capacitystatus", "Used"),
		f("licenseModel", "No License required"),
		f("marketoption", "OnDemand"),
	}
}

// fetchRegionOnDemandPrices runs ONE paginated GetProducts for the whole region
// and returns instanceType -> USD/hour.
//
// The client must be the us-east-1-pinned one; regionCode must be the
// environment's region. Getting those two backwards is the C-2 failure and is
// the first thing to check when every price in ap-southeast-2 looks like a
// us-east-1 price.
func fetchRegionOnDemandPrices(ctx context.Context, api pricingProductsAPI, regionCode string) (map[string]float64, error) {
	out := make(map[string]float64, 1024)
	var token *string
	for {
		page, err := api.GetProducts(ctx, &awspricing.GetProductsInput{
			ServiceCode: aws.String("AmazonEC2"),
			Filters:     regionOnDemandFilters(regionCode),
			MaxResults:  aws.Int32(getProductsPageSize),
			NextToken:   token,
		})
		if err != nil {
			return nil, classifyComputeError(err, actionGetProducts)
		}
		for _, raw := range page.PriceList {
			var sku pricingSKU
			// Unknown fields -- terms.Reserved above all -- are discarded here,
			// as the page is parsed. Nothing upstream of this loop survives it.
			if uerr := json.Unmarshal([]byte(raw), &sku); uerr != nil {
				continue
			}
			name := sku.Product.Attributes.InstanceType
			if name == "" {
				continue
			}
			if usd, ok := sku.onDemandUSD(); ok {
				out[name] = usd
			}
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	if len(out) == 0 {
		return nil, errors.New("GetProducts returned no priced instance types")
	}
	return out, nil
}

// loadOnDemandPrices is the cached read (FR-9: 24h, per profile and region).
// Like the catalog, it does not Invalidate before a forced fetch -- see
// loadComputeCatalog for why the stale rung has to survive a failed refresh.
func (ce *computeEnv) loadOnDemandPrices(ctx context.Context, force bool) (map[string]float64, time.Time, error) {
	return computeOnDemandCache.GetOrFetch(ce.key, computeOnDemandTTL, force, func() (map[string]float64, error) {
		prices, err := fetchRegionOnDemandPrices(ctx, ce.pricingAPI, ce.region)
		if err != nil {
			return nil, ce.explainFailure(ctx, err, actionGetProducts)
		}
		return prices, nil
	})
}

// priceResolution is a resolved price map plus what the envelope must say about
// it. pricingRegion is non-empty exactly when at least one price came from the
// fallback table -- those are us-east-1 list prices, and ap-southeast-2 runs
// 15-25% above them by a premium that is not uniform across families, so
// attributing them to the selected region would be wrong in a way
// availabilityVerified does not admit (C-18).
type priceResolution struct {
	prices        map[string]float64
	source        string
	pricingRegion string
	pricingDate   string
	credState     credState
	notice        string
}

// resolveComputePrices walks the same ladder as the catalog: live -> Peek at
// any age -> the dated fallback table.
func (ce *computeEnv) resolveComputePrices(ctx context.Context, force bool) priceResolution {
	prices, _, err := ce.loadOnDemandPrices(ctx, force)
	if err == nil {
		return priceResolution{prices: prices, source: sourceAWSAPI, credState: ce.lastKnownCredState()}
	}

	state, action, isAuth := authStateOf(err)
	if isAuth {
		ce.noteCredState(state)
	} else {
		state = ce.lastKnownCredState()
	}

	if stale, staleAt, ok := computeOnDemandCache.Peek(ce.key); ok {
		return priceResolution{
			prices:    stale,
			source:    sourcePartial,
			credState: state,
			notice: computeDegradedNotice(state, action, ce.profile,
				"prices are the last read from AWS, from "+staleAt.UTC().Format(time.RFC3339)),
		}
	}

	return priceResolution{
		prices:        pricingpkg.GetFallbackEC2Hourly(),
		source:        sourceFallback,
		pricingRegion: pricingpkg.FALLBACK_PRICING_REGION,
		pricingDate:   pricingpkg.FALLBACK_PRICING_DATE,
		credState:     state,
		notice: computeDegradedNotice(state, action, ce.profile,
			"prices are indicative "+pricingpkg.FALLBACK_PRICING_REGION+" list prices dated "+pricingpkg.FALLBACK_PRICING_DATE+", not "+ce.region+" prices"),
	}
}

// Cold-start waiting budgets for the price fetch, per endpoint (NFR-1).
//
// The catalog endpoint's budget is 4s and the recommendation's is 6s; the
// catalog read costs ~2.2s of that, so these are what is left for prices before
// the endpoint has to answer with what it has.
const (
	catalogPriceWaitBudget        = 3500 * time.Millisecond
	recommendationPriceWaitBudget = 5500 * time.Millisecond
)

// resolveCatalogAndPrices runs the two region reads CONCURRENTLY, and stops
// WAITING for prices at waitBudget without stopping the fetch.
//
// Measured against a live account in ap-southeast-2, cold:
//
//	ec2:DescribeInstanceTypes   2.2s   903 types,  10 pages
//	pricing:GetProducts         6.6s   832 SKUs,    9 pages, 6.5 MB of JSON
//
// The two are independent, so running them in sequence costs the sum (9.4s
// measured) and running them together costs the larger (7.1s measured). That is
// still over NFR-1's 4s, and no amount of batching fixes it: MaxResults on
// GetProducts is hard-capped at 100 by the API (verified -- 500 returns
// InvalidParameterException), the bytes are ~6.5 MB because every SKU carries
// the full Reserved matrix, and NextToken paging is inherently sequential.
//
// So the endpoint stops waiting rather than stops fetching. Past the budget it
// answers from the price ladder's lower rungs -- the last live map at any age,
// else the dated fallback table -- and labels it exactly as it labels any other
// degraded price: source "partial" or "fallback", a non-null pricingRegion, and
// a notice. The fetch continues on a context DETACHED from the request, so it
// survives the response, populates the 24h cache, and the next poll a few
// seconds later is live and warm (~3ms measured). One cold render of clearly
// labelled indicative pricing beats seven seconds of spinner.
func (ce *computeEnv) resolveCatalogAndPrices(ctx context.Context, force bool, waitBudget time.Duration) (catalogResolution, priceResolution) {
	// The deadline is measured from when the FETCH started, not from when this
	// request did. Otherwise every request arriving during the ~7s cold window
	// restarts its own budget and pays the full wait again -- a polling UI would
	// see three consecutive 3.5s responses instead of one.
	deadline := priceFetchDeadline(ce.key, waitBudget)

	// context.WithoutCancel, not ctx: when the budget expires the handler
	// returns and ctx is cancelled, and a fetch cancelled at that moment would
	// leave the cache empty, so the NEXT request would pay the same 6.6s again.
	fetchCtx := context.WithoutCancel(ctx)
	priceCh := make(chan priceResolution, 1)
	go func() {
		defer clearPriceFetchStart(ce.key)
		priceCh <- ce.resolveComputePrices(fetchCtx, force)
	}()

	cat := ce.resolveComputeCatalog(ctx, force)

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case prices := <-priceCh:
		return cat, prices
	case <-timer.C:
		return cat, ce.pricesWithoutWaiting()
	}
}

// priceFetchStarts remembers when the in-flight price fetch for a key began, so
// that concurrent and closely-following requests share ONE waiting window
// rather than each opening their own.
//
// It is not a second cache: it holds a timestamp, never a value, and the single
// -flight that actually collapses the upstream calls is TTLCache's.
var priceFetchStarts = struct {
	mu     sync.Mutex
	starts map[string]time.Time
}{starts: make(map[string]time.Time)}

func priceFetchDeadline(key string, budget time.Duration) time.Time {
	priceFetchStarts.mu.Lock()
	defer priceFetchStarts.mu.Unlock()

	start, ok := priceFetchStarts.starts[key]
	if !ok {
		start = time.Now()
		priceFetchStarts.starts[key] = start
	}
	return start.Add(budget)
}

func clearPriceFetchStart(key string) {
	priceFetchStarts.mu.Lock()
	defer priceFetchStarts.mu.Unlock()

	delete(priceFetchStarts.starts, key)
}

// pricesWithoutWaiting is the price ladder minus its top rung: it never starts
// or joins a fetch, so it cannot block. It is what the budget expiry falls back
// to while the real fetch is still in flight.
func (ce *computeEnv) pricesWithoutWaiting() priceResolution {
	if stale, staleAt, ok := computeOnDemandCache.Peek(ce.key); ok {
		return priceResolution{
			prices:    stale,
			source:    sourcePartial,
			credState: ce.lastKnownCredState(),
			notice: "live " + ce.region + " prices are still loading; the figures below are the last read from AWS, from " +
				staleAt.UTC().Format(time.RFC3339),
		}
	}
	return priceResolution{
		prices:        pricingpkg.GetFallbackEC2Hourly(),
		source:        sourceFallback,
		pricingRegion: pricingpkg.FALLBACK_PRICING_REGION,
		pricingDate:   pricingpkg.FALLBACK_PRICING_DATE,
		credState:     ce.lastKnownCredState(),
		notice: "live " + ce.region + " prices are still loading (the AWS Price List API returns ~6.5 MB for a region and takes several seconds); " +
			"the figures below are indicative " + pricingpkg.FALLBACK_PRICING_REGION + " list prices dated " +
			pricingpkg.FALLBACK_PRICING_DATE + ", and a refresh in a few seconds will show " + ce.region + " prices",
	}
}

// applyPrices attaches prices to a catalog copy in place and reports whether any
// price came from the fallback table.
//
// liveSource says which provenance a hit in `prices` carries: a stale-but-live
// map is still aws_api data, a fallback map is not.
func applyPrices(types []ComputeInstanceType, prices map[string]float64, liveSource string) (usedFallback bool) {
	for i := range types {
		// The last gate before publication, and it covers the fallback table
		// as well as the live map -- neither is trusted to be finite here
		// rather than at whichever boundary produced it.
		if v, ok := prices[types[i].InstanceType]; ok && usablePriceUSD(v) {
			price := v
			types[i].OnDemandHourly = &price
			types[i].PriceSource = liveSource
			if liveSource == priceSourceFallback {
				usedFallback = true
			}
			continue
		}
		// FR-5: a type with no Pricing record is still listed and still
		// manually selectable; it is simply excluded from ranking.
		types[i].OnDemandHourly = nil
		types[i].PriceSource = priceSourceUnavailable
	}
	return usedFallback
}

// ---------------------------------------------------------------------------
// Spot
// ---------------------------------------------------------------------------

// SpotAZPrice is one availability zone's current spot price for one type.
type SpotAZPrice struct {
	AZ     string  `json:"az"`
	Hourly float64 `json:"hourly"`
	AsOf   string  `json:"asOf"`
}

// SpotQuote is the per-type spot answer of architecture.md section 3.2.
//
// Min/Max/Median are pointers because "no spot market for this type in this
// region" is an absence, not a zero, and EC-5 requires the two to stay
// distinguishable.
type SpotQuote struct {
	InstanceType  string        `json:"instanceType"`
	ByAZ          []SpotAZPrice `json:"byAz"`
	Min           *float64      `json:"min"`
	Max           *float64      `json:"max"`
	Median        *float64      `json:"median"`
	SpotAvailable bool          `json:"spotAvailable"`
}

// maxSpotTypesPerRequest is FR-6's hard limit on the spot endpoint.
const maxSpotTypesPerRequest = 20

// fetchSpotPriceHistory issues ONE DescribeSpotPriceHistory for every requested
// type (never one call per type, NFR-8) and reduces it to one quote per type.
//
// StartTime == EndTime == now returns the last price change before that instant,
// i.e. the price currently in effect, per AZ (research.md:2034-2040).
//
// types may be empty, which asks for the whole region -- that is the
// recommendation path, where every candidate needs a spot median and a
// per-candidate call would be ~900 requests.
func fetchSpotPriceHistory(ctx context.Context, api ec2ComputeAPI, types []string) ([]SpotQuote, error) {
	quotes, err := describeSpot(ctx, api, types, spotProductLinux)
	if err != nil {
		return nil, err
	}
	if len(quotes) == 0 {
		// A VPC-only account answers to the other product description. An
		// empty first result is not proof that spot is unavailable.
		quotes, err = describeSpot(ctx, api, types, spotProductLinuxVPC)
		if err != nil {
			return nil, err
		}
	}

	out := make([]SpotQuote, 0, len(types))
	if len(types) > 0 {
		// Preserve the caller's set exactly: a type with no market still gets a
		// row, with spotAvailable false (EC-5).
		for _, t := range types {
			if q, ok := quotes[t]; ok {
				out = append(out, q)
				continue
			}
			out = append(out, SpotQuote{InstanceType: t, ByAZ: []SpotAZPrice{}})
		}
		return out, nil
	}

	for _, q := range quotes {
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceType < out[j].InstanceType })
	return out, nil
}

// describeSpot runs the paged call for one product description and reduces the
// raw history into per-type quotes.
func describeSpot(ctx context.Context, api ec2ComputeAPI, types []string, product string) (map[string]SpotQuote, error) {
	instanceTypes := make([]ec2types.InstanceType, 0, len(types))
	for _, t := range types {
		instanceTypes = append(instanceTypes, ec2types.InstanceType(t))
	}

	now := time.Now()
	// latest[type][az] keeps the most recent record, because a history page can
	// carry several changes for the same pair and only the newest is "current".
	latest := make(map[string]map[string]SpotAZPrice)

	var token *string
	for page := 0; page < maxSpotHistoryPages; page++ {
		in := &ec2.DescribeSpotPriceHistoryInput{
			ProductDescriptions: []string{product},
			StartTime:           aws.Time(now),
			EndTime:             aws.Time(now),
			MaxResults:          aws.Int32(spotHistoryPageSize),
			NextToken:           token,
		}
		if len(instanceTypes) > 0 {
			in.InstanceTypes = instanceTypes
		}
		resp, err := api.DescribeSpotPriceHistory(ctx, in)
		if err != nil {
			return nil, classifyComputeError(err, actionDescribeSpotPriceHistory)
		}
		for _, rec := range resp.SpotPriceHistory {
			name := string(rec.InstanceType)
			az := aws.ToString(rec.AvailabilityZone)
			if name == "" || az == "" || rec.SpotPrice == nil {
				continue
			}
			// Same gate as the on-demand side, for the same reason, and it
			// matters more here: the spot median is published verbatim as
			// spotMedianHourly, so a "NaN" in one AZ's record would poison the
			// median for the type and abort the encode of the whole response.
			// A dropped record leaves the type with no spot market, which EC-5
			// already answers with median: null.
			price, perr := strconv.ParseFloat(*rec.SpotPrice, 64)
			if perr != nil || !usablePriceUSD(price) {
				continue
			}
			ts := time.Time{}
			if rec.Timestamp != nil {
				ts = *rec.Timestamp
			}
			byAZ, ok := latest[name]
			if !ok {
				byAZ = make(map[string]SpotAZPrice, 3)
				latest[name] = byAZ
			}
			if prev, seen := byAZ[az]; seen && prev.AsOf >= ts.UTC().Format(time.RFC3339) {
				continue
			}
			byAZ[az] = SpotAZPrice{AZ: az, Hourly: price, AsOf: ts.UTC().Format(time.RFC3339)}
		}
		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		token = resp.NextToken
	}

	out := make(map[string]SpotQuote, len(latest))
	for name, byAZ := range latest {
		out[name] = reduceSpotQuote(name, byAZ)
	}
	return out, nil
}

// reduceSpotQuote turns one type's per-AZ prices into the section 3.2 shape.
// AZs are sorted by name so the payload is stable across requests.
func reduceSpotQuote(name string, byAZ map[string]SpotAZPrice) SpotQuote {
	q := SpotQuote{InstanceType: name, ByAZ: make([]SpotAZPrice, 0, len(byAZ))}
	for _, p := range byAZ {
		q.ByAZ = append(q.ByAZ, p)
	}
	sort.Slice(q.ByAZ, func(i, j int) bool { return q.ByAZ[i].AZ < q.ByAZ[j].AZ })
	if len(q.ByAZ) == 0 {
		return q
	}

	prices := make([]float64, 0, len(q.ByAZ))
	for _, p := range q.ByAZ {
		prices = append(prices, p.Hourly)
	}
	sort.Float64s(prices)

	minV, maxV := prices[0], prices[len(prices)-1]
	med := medianOf(prices)
	q.Min, q.Max, q.Median = &minV, &maxV, &med
	q.SpotAvailable = true
	return q
}

// medianOf assumes a sorted, non-empty slice.
func medianOf(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// loadSpotQuotes is the cached read (FR-9: 15 minutes). The key carries the
// sorted type list, because a quote set for {a,b} is not a quote set for {a,c}.
func (ce *computeEnv) loadSpotQuotes(ctx context.Context, types []string) ([]SpotQuote, time.Time, error) {
	sorted := append([]string(nil), types...)
	sort.Strings(sorted)
	keyPart := "__region__"
	if len(sorted) > 0 {
		keyPart = joinTypes(sorted)
	}
	key := pricingpkg.CacheKey(ce.profile, ce.region, keyPart)

	return computeSpotCache.GetOrFetch(key, computeSpotTTL, false, func() ([]SpotQuote, error) {
		quotes, err := fetchSpotPriceHistory(ctx, ce.ec2, sorted)
		if err != nil {
			return nil, ce.explainFailure(ctx, err, actionDescribeSpotPriceHistory)
		}
		return quotes, nil
	})
}

func joinTypes(sorted []string) string {
	out := ""
	for i, t := range sorted {
		if i > 0 {
			out += ","
		}
		out += t
	}
	return out
}

// spotMedians reduces quotes to the map the recommender's catalog wants.
func spotMedians(quotes []SpotQuote) map[string]float64 {
	out := make(map[string]float64, len(quotes))
	for _, q := range quotes {
		if q.Median != nil && *q.Median > 0 {
			out[q.InstanceType] = *q.Median
		}
	}
	return out
}
