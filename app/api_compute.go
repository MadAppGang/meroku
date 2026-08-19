package main

// The three compute endpoints: instance catalog, spot prices, recommendation.
//
// This is the PRESENTATION layer (architecture.md section 1). It parses query
// parameters, builds envelopes and sets headers. It calls no AWS SDK directly
// and it scores nothing -- the first belongs to compute_catalog.go /
// compute_prices.go / compute_signals.go, the second to the pure app/recommend
// core.
//
// Conventions taken from this repository rather than invented: the single-method
// guard returning http.Error(..., StatusMethodNotAllowed) (api_fargate.go:35-38),
// errors as ErrorResponse{Error string} (api.go:38-40) encoded with
// json.NewEncoder, Content-Type set before the body, and every variable in a
// QUERY parameter rather than a path segment -- mainRouter is a plain
// http.ServeMux with a catch-all at spa_server.go:178, so /api/compute/pools/{name}
// would return the SPA's HTML with a 200 (CON-3).
//
// One rule governs the status codes: an AWS failure NEVER produces a non-200.
// 4xx and 5xx are reserved for the caller's own mistakes (a missing env, an
// unreadable {env}.yaml, an unrecognised posture, more than twenty spot types).
// Everything else degrades inside a 200 body and says so in `source`,
// `credentialsState` and `notice` (FR-12, NFR-6, AC-2).

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"madappgang.com/meroku/recommend"
)

// ---------------------------------------------------------------------------
// Envelopes -- architecture.md section 3, field for field
// ---------------------------------------------------------------------------

type computeInstanceTypesResponse struct {
	Region               string                `json:"region"`
	Source               string                `json:"source"`
	CredentialsState     credState             `json:"credentialsState"`
	Filtered             bool                  `json:"filtered"`
	TotalAvailable       int                   `json:"totalAvailable"`
	CachedAt             *string               `json:"cachedAt"`
	PricingDate          *string               `json:"pricingDate"`
	PricingRegion        *string               `json:"pricingRegion"`
	InstanceDataDate     *string               `json:"instanceDataDate"`
	AvailabilityVerified bool                  `json:"availabilityVerified"`
	Notice               *string               `json:"notice"`
	InstanceTypes        []ComputeInstanceType `json:"instanceTypes"`
}

type computeSpotPricesResponse struct {
	Region           string      `json:"region"`
	Source           string      `json:"source"`
	CredentialsState credState   `json:"credentialsState"`
	AsOf             string      `json:"asOf"`
	Notice           *string     `json:"notice"`
	Prices           []SpotQuote `json:"prices"`
}

// computeScores mirrors recommend.SubScores. There are FOUR dimensions, not
// five: `waste` and `headroom` were near-inverses of one axis and are replaced
// by a single `utilisation` term -- how close the instance's ACHIEVED
// occupancy lands to the target the posture sets. app/recommend/score.go
// carries the analysis. This is a breaking change to the recommendation
// envelope and the web client's ComputeCandidateScores has to follow it.
type computeScores struct {
	Fit         float64 `json:"fit"`
	Utilisation float64 `json:"utilisation"`
	Cost        float64 `json:"cost"`
	Modernity   float64 `json:"modernity"`
}

type computeCandidate struct {
	InstanceType     string        `json:"instanceType"`
	VCPU             int           `json:"vcpu"`
	MemoryMiB        int64         `json:"memoryMiB"`
	Architecture     string        `json:"architecture"`
	Scores           computeScores `json:"scores"`
	Total            float64       `json:"total"`
	EffectiveHourly  float64       `json:"effectiveHourly"`
	TasksPerInstance int           `json:"tasksPerInstance"`
	InstancesAtFloor int           `json:"instancesAtFloor"`
	CostPerTask      float64       `json:"costPerTask"`
	SpotMedianHourly *float64      `json:"spotMedianHourly"`
	Reason           string        `json:"reason"`
}

type computeMiss struct {
	InstanceType string  `json:"instanceType"`
	FailedRule   string  `json:"failedRule"`
	Needed       float64 `json:"needed"`
	Available    float64 `json:"available"`
	Unit         string  `json:"unit"`
}

type computeShape struct {
	VCPU      float64 `json:"vcpu"`
	MemGiB    float64 `json:"memGiB"`
	Ratio     float64 `json:"ratio"`
	TaskCount int     `json:"taskCount,omitempty"`
}

type computeWeights struct {
	Configured float64 `json:"configured"`
	Actual     float64 `json:"actual"`
}

type computeRatioSignal struct {
	Raw        float64 `json:"raw"`
	Effective  float64 `json:"effective"`
	CatalogMin float64 `json:"catalogMin"`
	CatalogMax float64 `json:"catalogMax"`
	ClampedTo  string  `json:"clampedTo"`
}

type computeServiceSignal struct {
	Name       string  `json:"name"`
	Datapoints int     `json:"datapoints"`
	CPUAvg     float64 `json:"cpuAvg"`
	CPUPeak    float64 `json:"cpuPeak"`
	MemAvg     float64 `json:"memAvg"`
	MemPeak    float64 `json:"memPeak"`
	Status     string  `json:"status"`
	// Reason names the field that disqualified a dropped service (C-9). It is
	// omitted for a healthy one, so a UI can render its presence as the
	// explanation without checking status first.
	Reason string `json:"reason,omitempty"`
}

type computeSignalsEnvelope struct {
	Configured   computeShape           `json:"configured"`
	Actual       *computeShape          `json:"actual"`
	Coverage     float64                `json:"coverage"`
	Weights      computeWeights         `json:"weights"`
	Ratio        computeRatioSignal     `json:"ratio"`
	CloudWatch   string                 `json:"cloudwatch"`
	NetworkMode  string                 `json:"networkMode"`
	Trunking     string                 `json:"trunking"`
	DensityBasis string                 `json:"densityBasis"`
	Dropped      []computeMiss          `json:"dropped"`
	Services     []computeServiceSignal `json:"services"`
}

// computeSuggestedPool is snake_case on purpose: it is a YAML-ready pool object
// the UI writes straight into compute.pools, and camelCase here would mean the
// UI had to translate every key before saving.
type computeSuggestedPool struct {
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	InstanceTypes  []string `json:"instance_types"`
	CapacityType   string   `json:"capacity_type"`
	OnDemandBase   int      `json:"on_demand_base"`
	MinSize        int      `json:"min_size"`
	MaxSize        int      `json:"max_size"`
	TargetCapacity int      `json:"target_capacity"`
	NetworkMode    string   `json:"network_mode"`
	AMIFamily      string   `json:"ami_family"`
	RootVolumeGB   int      `json:"root_volume_gb"`
	// Downgraded records EC-6: spot was asked for, spot was unavailable for the
	// primary type, and the pool fell back to on_demand.
	Downgraded bool `json:"downgraded"`
}

type computeRecommendationResponse struct {
	Region           string    `json:"region"`
	Source           string    `json:"source"`
	CredentialsState credState `json:"credentialsState"`
	Posture          string    `json:"posture"`
	Classification   string    `json:"classification"`
	// ClassificationSuppressed is additive to section 3.3 and is present only
	// when the two differ: the class FR-18 read off the utilisation, which the
	// region carries no type able to serve, so sizing fell back to the ratio
	// class in `classification`. Without it the fallback is silent, and a user
	// looking at a fleet that idles at 9 % CPU being sized memory-heavy has no
	// way to tell whether the tool read the utilisation at all.
	ClassificationSuppressed string                 `json:"classificationSuppressed,omitempty"`
	Basis                    string                 `json:"basis"`
	Unsatisfiable            bool                   `json:"unsatisfiable"`
	Constraint               *string                `json:"constraint"`
	Primary                  *computeCandidate      `json:"primary"`
	Ranked                   []computeCandidate     `json:"ranked"`
	NearestMisses            []computeMiss          `json:"nearestMisses,omitempty"`
	Signals                  computeSignalsEnvelope `json:"signals"`
	SuggestedPool            computeSuggestedPool   `json:"suggestedPool"`
	// PricingRegion and Notice are additive to section 3.3 and carry the same
	// meaning they carry on the catalog envelope: a non-null pricingRegion says
	// the prices that drove this ranking are indicative us-east-1 list prices,
	// which is a caveat a ranking needs at least as much as a price list does.
	PricingRegion *string `json:"pricingRegion"`
	Notice        *string `json:"notice"`
}

// resolveComputeEnvFn is the one seam in this file. It exists so a test can
// exercise a whole envelope -- the shape P11's UI is built against -- against
// counting fakes rather than an AWS account, without any handler having to
// accept a client parameter it would otherwise never need.
var resolveComputeEnvFn = resolveComputeEnv

// ---------------------------------------------------------------------------
// GET /api/compute/instance-types
// ---------------------------------------------------------------------------

// getComputeInstanceTypes serves the region's instance catalog with prices.
//
// Query: env (required), refresh=true (FR-10), all=true (FR-4).
func getComputeInstanceTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	if envName == "" {
		writeComputeError(w, http.StatusBadRequest, "env parameter is required")
		return
	}

	ctx := r.Context()
	ce, err := resolveComputeEnvFn(ctx, envName)
	if err != nil {
		writeComputeError(w, http.StatusInternalServerError, "failed to parse environment config")
		return
	}

	force := r.URL.Query().Get("refresh") == "true"
	showAll := r.URL.Query().Get("all") == "true"

	cat, prices := ce.resolveCatalogAndPrices(ctx, force, catalogPriceWaitBudget)

	// The catalog is cached unfiltered; filtering happens per request into a
	// copy, so `all=true` and the default share one cache entry and one fetch.
	types := copyComputeTypes(cat.types)
	total := len(types)
	if !showAll {
		types = filterRecommendableTypes(types)
	}
	sortComputeTypes(types)

	priceProvenance := priceSourceAWS
	if prices.source == sourceFallback {
		priceProvenance = priceSourceFallback
	}
	usedFallbackPrices := applyPrices(types, prices.prices, priceProvenance)

	resp := computeInstanceTypesResponse{
		Region:               ce.region,
		Source:               mergeSources(cat.source, prices.source),
		CredentialsState:     worstCredState(cat.credState, prices.credState),
		Filtered:             !showAll,
		TotalAvailable:       total,
		AvailabilityVerified: cat.availabilityVerified,
		InstanceTypes:        types,
		Notice:               optionalString(firstNonEmpty(cat.notice, prices.notice)),
		InstanceDataDate:     optionalString(cat.instanceDataDate),
	}
	if !cat.cachedAt.IsZero() {
		resp.CachedAt = optionalString(cat.cachedAt.UTC().Format(time.RFC3339))
	}
	if usedFallbackPrices {
		// C-18: label the money with the region it actually describes. The UI
		// renders "indicative us-east-1 pricing" rather than a figure attributed
		// to ap-southeast-2, which runs 15-25% higher by a non-uniform premium.
		resp.PricingRegion = optionalString(prices.pricingRegion)
		resp.PricingDate = optionalString(prices.pricingDate)
	}

	writeComputeJSON(w, resp)
}

// ---------------------------------------------------------------------------
// GET /api/compute/spot-prices
// ---------------------------------------------------------------------------

// getComputeSpotPrices serves current spot prices per AZ for up to twenty
// instance types, in ONE DescribeSpotPriceHistory call (NFR-8).
func getComputeSpotPrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	envName := q.Get("env")
	if envName == "" {
		writeComputeError(w, http.StatusBadRequest, "env parameter is required")
		return
	}
	rawTypes := q.Get("types")
	if strings.TrimSpace(rawTypes) == "" {
		writeComputeError(w, http.StatusBadRequest, "types parameter is required")
		return
	}
	types := splitTypesParam(rawTypes)
	if len(types) == 0 {
		writeComputeError(w, http.StatusBadRequest, "types parameter is required")
		return
	}
	if len(types) > maxSpotTypesPerRequest {
		writeComputeError(w, http.StatusBadRequest,
			"at most "+strconv.Itoa(maxSpotTypesPerRequest)+" instance types per request, got "+strconv.Itoa(len(types)))
		return
	}

	ctx := r.Context()
	ce, err := resolveComputeEnvFn(ctx, envName)
	if err != nil {
		writeComputeError(w, http.StatusInternalServerError, "failed to parse environment config")
		return
	}

	resp := computeSpotPricesResponse{
		Region:           ce.region,
		Source:           sourceAWSAPI,
		CredentialsState: credOK,
		AsOf:             time.Now().UTC().Format(time.RFC3339),
		Prices:           []SpotQuote{},
	}

	quotes, asOf, err := ce.loadSpotQuotes(ctx, types)
	if err != nil {
		// There is no fallback spot table and there must not be one: a stale
		// spot price is a wrong bid, and inventing one would be worse than
		// admitting the market is unreadable. Every type comes back with
		// spotAvailable false (EC-5) and the notice says why.
		state, action, isAuth := authStateOf(err)
		if isAuth {
			ce.noteCredState(state)
		} else {
			state = ce.lastKnownCredState()
		}
		resp.Source = sourceFallback
		resp.CredentialsState = state
		resp.Notice = optionalString(computeDegradedNotice(state, action, ce.profile,
			"spot prices are unavailable; the types below are shown without a spot market"))
		for _, t := range types {
			resp.Prices = append(resp.Prices, SpotQuote{InstanceType: t, ByAZ: []SpotAZPrice{}})
		}
		writeComputeJSON(w, resp)
		return
	}

	resp.CredentialsState = ce.lastKnownCredState()
	resp.Prices = quotes
	if !asOf.IsZero() {
		resp.AsOf = asOf.UTC().Format(time.RFC3339)
	}
	writeComputeJSON(w, resp)
}

// ---------------------------------------------------------------------------
// GET /api/compute/recommendation
// ---------------------------------------------------------------------------

// legalPostures is the 400's message and the validation set in one place.
var legalPostures = []string{
	string(recommend.PosturePerformance),
	string(recommend.PostureBalanced),
	string(recommend.PostureCost),
}

// getComputeRecommendation sizes a pool for the workload the environment
// actually declares, blended with what CloudWatch says it consumes.
//
// Query: env (required), posture, pool, services, window, limit, gpu (FR-13),
// plus ami_family and network_mode (section 3.3), which decide the architecture
// filter and whether the ENI density cap applies at all.
func getComputeRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	envName := q.Get("env")
	if envName == "" {
		writeComputeError(w, http.StatusBadRequest, "env parameter is required")
		return
	}

	posture := recommend.PosturePerformance
	if raw := q.Get("posture"); raw != "" {
		normalized := recommend.NormalizePosture(recommend.Posture(raw))
		if string(normalized) != raw {
			writeComputeError(w, http.StatusBadRequest,
				"unrecognised posture "+strconv.Quote(raw)+"; must be one of "+strings.Join(legalPostures, ", "))
			return
		}
		posture = normalized
	}

	ctx := r.Context()
	ce, err := resolveComputeEnvFn(ctx, envName)
	if err != nil {
		writeComputeError(w, http.StatusInternalServerError, "failed to parse environment config")
		return
	}

	poolName := q.Get("pool")
	pool := poolConstraintsFor(ce.env, poolName, q)

	// ---- catalog and prices -------------------------------------------
	cat, prices := ce.resolveCatalogAndPrices(ctx, false, recommendationPriceWaitBudget)

	types := filterRecommendableTypes(copyComputeTypes(cat.types))
	priceProvenance := priceSourceAWS
	if prices.source == sourceFallback {
		priceProvenance = priceSourceFallback
	}
	usedFallbackPrices := applyPrices(types, prices.prices, priceProvenance)

	// Spot medians are needed by two of the three postures and displayed by all
	// three. One region-wide call, cached for 15 minutes, under its own budget
	// so a slow spot read can never spend NFR-1's whole allowance.
	spotCtx, cancelSpot := context.WithTimeout(ctx, 3*time.Second)
	spot := map[string]float64{}
	if quotes, _, serr := ce.loadSpotQuotes(spotCtx, nil); serr == nil {
		spot = spotMedians(quotes)
	}
	cancelSpot()

	// ---- trunking, read only when it can matter -----------------------
	trunkingOn, trunkingState := false, recommend.TrunkingNotApplicable
	if pool.NetworkMode == recommend.NetworkModeAWSVPC {
		trunkingOn, trunkingState = readAWSVPCTrunking(ctx, ce.ecsAPI)
	}

	// ---- demand -------------------------------------------------------
	scoped := scopeServices(ce.env, scopeRequest{pool: poolName, services: splitTypesParam(q.Get("services"))})
	var (
		metrics []serviceMetrics
		cwState = cwUnavailable
	)
	if cat.credState == credOK && cat.source == sourceAWSAPI {
		metrics, cwState = fetchServiceActuals(ctx, ce.cloudwatch,
			ce.env.Project, ce.env.Env, scoped, normalizeWindowHours(q.Get("window")))
	}
	signals := buildDemandSignals(scoped, metrics, cwState)

	// ---- the pure core ------------------------------------------------
	result := recommend.Recommend(recommend.Input{
		Region:          ce.region,
		Catalog:         toRecommendCatalog(types, spot),
		Services:        signals.services,
		Pool:            pool,
		Posture:         posture,
		Limit:           parseLimit(q.Get("limit")),
		TrunkingEnabled: trunkingOn,
	})

	// ---- envelope ------------------------------------------------------
	resp := computeRecommendationResponse{
		Region:           ce.region,
		Source:           mergeSources(cat.source, prices.source),
		CredentialsState: worstCredState(cat.credState, prices.credState),
		Posture:          string(posture),
		Classification:   result.Classification,

		ClassificationSuppressed: result.ClassificationSuppressed,
		Basis:                    result.Basis,
		Unsatisfiable:            result.Unsatisfiable,
		Constraint:               optionalString(result.Constraint),
		Ranked:                   toComputeCandidates(result.Ranked),
		NearestMisses:            toComputeMisses(result.NearestMisses),
		Signals:                  toComputeSignals(result.Signals, signals, dropReasons(scoped), trunkingState),
		SuggestedPool:            toComputeSuggestedPool(result.SuggestedPool),
		Notice:                   optionalString(firstNonEmpty(cat.notice, prices.notice)),
	}
	if result.Primary != nil {
		primary := toComputeCandidate(*result.Primary)
		resp.Primary = &primary
	}
	if usedFallbackPrices {
		resp.PricingRegion = optionalString(prices.pricingRegion)
	}

	writeComputeJSON(w, resp)
}

// poolConstraintsFor builds the pure core's PoolConstraints from the named pool
// when one exists, then lets the query override the two fields a
// not-yet-created pool has to be able to explore: ami_family and network_mode.
//
// AMIFamily is always set explicitly rather than left to NormalizePool, because
// "" and "al2023" mean the same thing to the template but not to a reader:
// section 3.3 documents the default as al2023 and C-11 is the bug where an
// unset family let the architecture filter accept arm64 types that the rendered
// x86 AMI would refuse to boot.
func poolConstraintsFor(env Env, poolName string, q map[string][]string) recommend.PoolConstraints {
	pc := recommend.PoolConstraints{
		Name:        poolName,
		NetworkMode: recommend.NetworkModeBridge,
		AMIFamily:   recommend.AMIFamilyAL2023,
	}
	if pc.Name == "" {
		pc.Name = "default"
	}

	for _, p := range env.Compute.Pools {
		if poolName == "" || p.Name != poolName {
			continue
		}
		if p.NetworkMode != "" {
			pc.NetworkMode = p.NetworkMode
		}
		if p.AMIFamily != "" {
			pc.AMIFamily = p.AMIFamily
		}
		pc.CapacityType = p.CapacityType
		if p.OnDemandBase != nil {
			pc.OnDemandBase = *p.OnDemandBase
		}
		pc.MinSize = p.MinSize
		pc.MaxSize = p.MaxSize
		// The pool's own instance_types is the operator's explicit opt-in
		// past rule R10's family allowlist: naming inf1.24xlarge here makes
		// that one type eligible for scoring. It does not pin the answer --
		// the named type still has to win on score.
		pc.InstanceTypes = p.InstanceTypes
		break
	}

	if v := firstQueryValue(q, "ami_family"); v != "" {
		pc.AMIFamily = v
	}
	if v := firstQueryValue(q, "network_mode"); v != "" {
		pc.NetworkMode = v
	}
	if firstQueryValue(q, "gpu") == "true" {
		pc.ForceGPU = true
	}

	return recommend.NormalizePool(pc)
}

func firstQueryValue(q map[string][]string, key string) string {
	if vs, ok := q[key]; ok && len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func parseLimit(raw string) int {
	if raw == "" {
		return 0 // the core's default
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ---------------------------------------------------------------------------
// Result -> envelope
// ---------------------------------------------------------------------------

func toComputeCandidate(c recommend.Candidate) computeCandidate {
	return computeCandidate{
		InstanceType: c.InstanceType,
		VCPU:         c.VCPU,
		MemoryMiB:    c.MemoryMiB,
		Architecture: c.Architecture,
		Scores: computeScores{
			Fit:         c.Scores.Fit,
			Utilisation: c.Scores.Utilisation,
			Cost:        c.Scores.Cost,
			Modernity:   c.Scores.Modernity,
		},
		Total:            c.Total,
		EffectiveHourly:  c.EffectiveHourly,
		TasksPerInstance: c.TasksPerInstance,
		InstancesAtFloor: c.InstancesAtFloor,
		CostPerTask:      c.CostPerTask,
		SpotMedianHourly: c.SpotMedianHourly,
		Reason:           c.Reason,
	}
}

func toComputeCandidates(in []recommend.Candidate) []computeCandidate {
	out := make([]computeCandidate, 0, len(in))
	for _, c := range in {
		out = append(out, toComputeCandidate(c))
	}
	return out
}

func toComputeMisses(in []recommend.Miss) []computeMiss {
	out := make([]computeMiss, 0, len(in))
	for _, m := range in {
		out = append(out, computeMiss{
			InstanceType: m.InstanceType,
			FailedRule:   m.FailedRule,
			Needed:       m.Needed,
			Available:    m.Available,
			Unit:         m.Unit,
		})
	}
	return out
}

// toComputeSignals merges what the core computed with what only the boundary
// knows: the CloudWatch status word, the trunking state, and the per-service
// drop reasons.
func toComputeSignals(s recommend.Signals, demand demandSignals, reasons map[string]string, trunkingState string) computeSignalsEnvelope {
	trunking := s.Trunking
	if trunking == "" {
		trunking = trunkingState
	}

	out := computeSignalsEnvelope{
		Configured: computeShape{
			VCPU:      s.Configured.VCPU,
			MemGiB:    s.Configured.MemGiB,
			Ratio:     s.Configured.Ratio,
			TaskCount: s.ConfiguredTaskCount,
		},
		Coverage: s.Coverage,
		Weights:  computeWeights{Configured: s.WeightConfigured, Actual: s.WeightActual},
		Ratio: computeRatioSignal{
			Raw:        s.Ratio.Raw,
			Effective:  s.Ratio.Effective,
			CatalogMin: s.Ratio.CatalogMin,
			CatalogMax: s.Ratio.CatalogMax,
			ClampedTo:  s.Ratio.ClampedTo,
		},
		CloudWatch:   demand.cloudwatchState,
		NetworkMode:  s.NetworkMode,
		Trunking:     trunking,
		DensityBasis: s.DensityBasis,
		Dropped:      toComputeMisses(s.Dropped),
		Services:     make([]computeServiceSignal, 0, len(s.Services)+len(demand.dropped)),
	}
	if s.Actual != nil {
		out.Actual = &computeShape{VCPU: s.Actual.VCPU, MemGiB: s.Actual.MemGiB, Ratio: s.Actual.Ratio}
	}

	for _, sig := range s.Services {
		out.Services = append(out.Services, computeServiceSignal{
			Name:       sig.Name,
			Datapoints: sig.Datapoints,
			CPUAvg:     sig.CPUAvg,
			CPUPeak:    sig.CPUPeak,
			MemAvg:     sig.MemAvg,
			MemPeak:    sig.MemPeak,
			Status:     sig.Status,
			Reason:     reasons[sig.Name],
		})
	}
	// The services the boundary dropped never reached the core, so the core
	// cannot report them. They are reported here, with the field that
	// disqualified them, because "we ignored your worker" is not something a UI
	// should have to infer from a missing row (C-9).
	for _, sig := range demand.dropped {
		out.Services = append(out.Services, computeServiceSignal{
			Name:   sig.Name,
			Status: sig.Status,
			Reason: reasons[sig.Name],
		})
	}

	return out
}

func toComputeSuggestedPool(p recommend.SuggestedPool) computeSuggestedPool {
	types := p.InstanceTypes
	if types == nil {
		types = []string{}
	}
	return computeSuggestedPool{
		Name:           p.Name,
		Enabled:        true,
		InstanceTypes:  types,
		CapacityType:   p.CapacityType,
		OnDemandBase:   p.OnDemandBase,
		MinSize:        p.MinSize,
		MaxSize:        p.MaxSize,
		TargetCapacity: p.TargetCapacity,
		NetworkMode:    p.NetworkMode,
		AMIFamily:      p.AMIFamily,
		RootVolumeGB:   p.RootVolumeGB,
		Downgraded:     p.Downgraded,
	}
}

// ---------------------------------------------------------------------------
// Small shared helpers
// ---------------------------------------------------------------------------

// writeComputeJSON encodes a compute envelope, and REPORTS a failure to encode
// it.
//
// The error used to be discarded. Two failures hide behind it and they are not
// the same: a client that hung up mid-write is not actionable, but
// encoding/json refusing a non-finite float is a defect in this program, and it
// is the one shape of bug that is otherwise invisible from both ends. The
// status line and the headers are already on the wire when Encode gives up, so
// the client sees HTTP 200 with a body that stops in the middle of an object --
// its json() rejects, the Compute tab renders nothing (NFR-6) -- while the
// server, having discarded the only evidence, records a successful request. A
// blank screen with a clean log is the hardest possible thing to diagnose.
func writeComputeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("[Compute] response encode failed after the header was sent, "+
			"so the client received a truncated 200: %v", err)
	}
}

func writeComputeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: msg}); err != nil {
		log.Printf("[Compute] failed to write the %d error body %q: %v", status, msg, err)
	}
}

// splitTypesParam parses a comma-separated list, dropping empties so that
// "a,,b" and a trailing comma do not produce a phantom instance type.
func splitTypesParam(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// mergeSources reports the weaker of two provenances: a payload whose catalog is
// live and whose prices are the fallback table is "partial", not "aws_api".
func mergeSources(a, b string) string {
	if a == b {
		return a
	}
	if a == sourceFallback && b == sourceFallback {
		return sourceFallback
	}
	return sourcePartial
}

// worstCredState reports the more informative of two states. "ok" is the only
// value that loses to everything: a denial discovered by the pricing call must
// not be masked by a catalog call that succeeded.
func worstCredState(a, b credState) credState {
	rank := map[credState]int{credOK: 0, credDenied: 1, credExpired: 2, credMissing: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// optionalString renders "" as JSON null. Every nullable string in section 3 is
// a pointer for this reason: an empty notice and an absent notice are the same
// fact, and the contract spells it null.
func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
