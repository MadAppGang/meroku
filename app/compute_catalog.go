package main

// EC2 instance-type catalog: the live DescribeInstanceTypes read, the FR-3
// projection, the FR-4 filter, and the degradation ladder that keeps every
// compute endpoint on HTTP 200.
//
// Layering (architecture.md section 1): this is the APPLICATION layer. It may
// call AWS and the cache; it must never write to an http.ResponseWriter, and it
// must never score anything -- scoring lives in the pure app/recommend core.
//
// The one field that panics if you trust the SDK's shape is GpuInfo. AWS
// returns it as **null**, not as an empty object, for roughly 870 of a region's
// 903 instance types (aws-verified-contracts.md section 1), so nil safety here
// is the common path and not an edge case: *it.GpuInfo.TotalGpuMemoryInMiB
// panics on m7i.large.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/smithy-go"

	pricingpkg "madappgang.com/meroku/pricing"
	"madappgang.com/meroku/recommend"
)

// ---------------------------------------------------------------------------
// TTLs and budgets
// ---------------------------------------------------------------------------

const (
	// computeCatalogTTL and computeOnDemandTTL are FR-9's 24h. Hardware records
	// and list prices both change on the order of weeks.
	computeCatalogTTL  = 24 * time.Hour
	computeOnDemandTTL = 24 * time.Hour
	// computeSpotTTL is FR-9's 15 minutes. Spot moves.
	computeSpotTTL = 15 * time.Minute
	// computeCredProbeTTL keeps EC-1 (an expired SSO session) detectable within
	// a minute at a cost of at most one Retrieve per minute per profile+region.
	computeCredProbeTTL = 60 * time.Second
	// computeCredProbeTimeout bounds the probe.
	//
	// It is 5s and not the 1.5s architecture.md section 2.4 proposed, because
	// 1.5s is measurably too tight: a COLD SSO Retrieve against this
	// repository's own profile takes ~1.45s (it reads the cached token from
	// disk and then calls sso:GetRoleCredentials over the network), and a probe
	// that times out at 1.5s classifies a perfectly valid session as "missing"
	// and serves the fallback table to a user whose credentials work. Warm
	// retrievals are ~0ms.
	//
	// A generous timeout is affordable here only because of the second half of
	// the fix: the probe no longer gates the fetch (see explainFailure). It runs
	// AFTER a failure, to label it, so its cost is paid on the degraded path
	// rather than in front of NFR-1's 4s budget.
	computeCredProbeTimeout = 5 * time.Second
)

// ---------------------------------------------------------------------------
// Caches -- three data caches plus the probe cache, all per profile AND region
// ---------------------------------------------------------------------------
//
// Per profile because switching AWS profile must not serve another account's
// view of a region (NFR-4); per region because availability, list price and the
// spot market all differ by region. The empty profile is a legitimate key.
//
// These are package vars rather than a sync.Once-guarded lazy init because
// constructing a TTLCache touches nothing: it is a map and a mutex. What cannot
// be built at startup is the SDK config, and that is built per request in
// resolveComputeEnv, where the currently selected profile is known.
var (
	computeCatalogCache  = pricingpkg.NewTTLCache[[]ComputeInstanceType]()
	computeOnDemandCache = pricingpkg.NewTTLCache[map[string]float64]()
	computeSpotCache     = pricingpkg.NewTTLCache[[]SpotQuote]()
	computeCredCache     = pricingpkg.NewTTLCache[credState]()
)

// ---------------------------------------------------------------------------
// Credential state (C-20)
// ---------------------------------------------------------------------------

// credState is the four-value enum the envelopes report. The fourth value,
// "denied", is the one that stops a restricted role from looking healthy:
// without it a profile that cannot call pricing:GetProducts classifies as "ok"
// with source "fallback", and the user sees stale prices with no explanation --
// the exact silent degradation this envelope exists to prevent.
type credState string

const (
	credOK      credState = "ok"
	credMissing credState = "missing"
	credExpired credState = "expired"
	credDenied  credState = "denied"
)

// The four read-only actions this feature needs. They belong to the OPERATOR's
// profile, not to any task role, and none of them is granted by this product's
// Terraform.
const (
	actionDescribeInstanceTypes    = "ec2:DescribeInstanceTypes"
	actionDescribeSpotPriceHistory = "ec2:DescribeSpotPriceHistory"
	actionGetProducts              = "pricing:GetProducts"
	actionGetMetricStatistics      = "cloudwatch:GetMetricStatistics"
)

// computeAuthError is an AWS failure classified as a credentials problem. It
// carries the action so the notice can name it: "this profile cannot call
// pricing:GetProducts" can be acted on; "AWS error" cannot.
type computeAuthError struct {
	state  credState
	action string
	err    error
}

func (e *computeAuthError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.action, e.state, e.err)
}

func (e *computeAuthError) Unwrap() error { return e.err }

// classifyComputeError maps an AWS error to a credState, or leaves it alone
// when the failure has nothing to do with credentials (a timeout, a throttle, a
// network partition). Those still degrade the response, but they must not tell
// the user their credentials are wrong.
//
// Matching is on the smithy error CODE rather than on a concrete per-service
// exception type: EC2, Pricing and CloudWatch each model these differently, and
// three type switches would be three chances to miss one.
func classifyComputeError(err error, action string) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ExpiredToken", "ExpiredTokenException", "RequestExpired", "InvalidClientTokenId":
			return &computeAuthError{state: credExpired, action: action, err: err}
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation",
			"AuthorizationError", "AuthFailure", "UnrecognizedClientException":
			return &computeAuthError{state: credDenied, action: action, err: err}
		}
		return err
	}
	if looksLikeExpiredSession(err) {
		return &computeAuthError{state: credExpired, action: action, err: err}
	}
	return err
}

// looksLikeExpiredSession catches the SSO-shaped retrieval failures that never
// reach a service API at all, so they carry no smithy code.
func looksLikeExpiredSession(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "expired"):
		return true
	case strings.Contains(msg, "sso session"):
		return true
	case strings.Contains(msg, "refresh cached sso token"):
		return true
	default:
		return false
	}
}

// authStateOf extracts the credState from an error produced by
// classifyComputeError, and reports whether it was a credentials problem at all.
func authStateOf(err error) (credState, string, bool) {
	var authErr *computeAuthError
	if errors.As(err, &authErr) {
		return authErr.state, authErr.action, true
	}
	return credOK, "", false
}

// ---------------------------------------------------------------------------
// The AWS seam
// ---------------------------------------------------------------------------
//
// Narrow interfaces, one method group each, so a test can supply a counting
// fake without an SDK or a network. They are deliberately not one "AWS"
// interface: the spot endpoint has no business being able to call CloudWatch.

type ec2ComputeAPI interface {
	DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput, ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

type pricingProductsAPI interface {
	GetProducts(context.Context, *awspricing.GetProductsInput, ...func(*awspricing.Options)) (*awspricing.GetProductsOutput, error)
}

type cloudwatchComputeAPI interface {
	GetMetricStatistics(context.Context, *cloudwatch.GetMetricStatisticsInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
}

type ecsSettingsAPI interface {
	ListAccountSettings(context.Context, *ecs.ListAccountSettingsInput, ...func(*ecs.Options)) (*ecs.ListAccountSettingsOutput, error)
}

// computeEnv is one resolved request context: the environment's config, the two
// AWS configs, the cache key, and the clients.
//
// It is the concrete form of architecture.md section 2.4's resolveComputeEnv.
// That section sketches a five-value return; a struct carries the same
// information with the pairing enforced -- awsCfg and pricingCfg differ ONLY in
// region and swapping them at a call site is the C-2 failure, so they travel
// together and are never re-derived. credState is NOT a field: producing it
// costs a probe that must not run in front of a warm cache hit (section 2.4
// rule 1), so it is a method instead.
type computeEnv struct {
	env        Env
	region     string
	profile    string
	key        string // pricingpkg.CacheKey(profile, region)
	awsCfg     aws.Config
	pricingCfg aws.Config

	creds      aws.CredentialsProvider
	ec2        ec2ComputeAPI
	pricingAPI pricingProductsAPI
	cloudwatch cloudwatchComputeAPI
	ecsAPI     ecsSettingsAPI
}

// resolveComputeEnv loads {env}.yaml and builds TWO AWS configs, deliberately.
//
//	awsCfg     -- the environment's region, from {env}.yaml. EC2
//	              (DescribeInstanceTypes, DescribeSpotPriceHistory), CloudWatch
//	              and ECS all use it.
//	pricingCfg -- PINNED to us-east-1, ALWAYS, regardless of the environment's
//	              region. The AWS Price List API is served from us-east-1 and
//	              ap-south-1 only; app/api_pricing.go:95 already does exactly
//	              this. The environment's region travels as the `regionCode`
//	              FILTER VALUE inside GetProducts, never as the client's region.
//	              Every environment in this repository is ap-southeast-2, so
//	              getting this backwards fails 100% of them (C-2).
//
// No existing call site does both profile and region: api_rds.go:41-43 passes
// the profile and no region option, api_autoscaling.go:113 is a bare
// LoadDefaultConfig and takes both from the ambient shell. Every compute
// endpoint reports `region` in its envelope and caches per region, so
// inheriting AWS_REGION would let a response contradict its own region field.
func resolveComputeEnv(ctx context.Context, envName string) (*computeEnv, error) {
	envConfig, err := loadEnv(envName)
	if err != nil {
		return nil, err
	}

	region := envConfig.Region
	if region == "" {
		region = computePricingAPIRegion
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(selectedAWSProfile),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	pricingCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(selectedAWSProfile),
		awsconfig.WithRegion(computePricingAPIRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	ce := &computeEnv{
		env:        envConfig,
		region:     region,
		profile:    selectedAWSProfile,
		key:        pricingpkg.CacheKey(selectedAWSProfile, region),
		awsCfg:     awsCfg,
		pricingCfg: pricingCfg,
	}
	ce.creds = awsCfg.Credentials
	ce.ec2 = ec2.NewFromConfig(awsCfg)
	ce.pricingAPI = awspricing.NewFromConfig(pricingCfg)
	ce.cloudwatch = cloudwatch.NewFromConfig(awsCfg)
	ce.ecsAPI = ecs.NewFromConfig(awsCfg)
	return ce, nil
}

// explainFailure turns a failed AWS call into an error the envelope can label.
//
// The ordering here is the important part, and it is the opposite of the
// obvious one: the CALL runs first and the probe only runs to explain a
// failure.
//
//   - Probing first and refusing the fetch on a failed probe makes a slow
//     credential resolution indistinguishable from an absent one. A cold SSO
//     Retrieve on this repository's own profile takes ~1.45s, so any timeout
//     tight enough to protect NFR-1 also mislabels a working session as
//     "missing" and serves the fallback table to a user who has credentials.
//   - Calling first has no such failure mode: the SDK resolves credentials as
//     part of the request anyway, so a machine with none fails fast with a
//     classifiable error, and a machine with slow-but-valid ones simply
//     succeeds. A successful call is also the strongest possible evidence that
//     the credentials are fine, so the happy path costs no probe at all.
//
// The probe therefore runs only when the error is NOT already auth-shaped --
// to separate "no credentials" from "AWS unreachable", which the caller must
// word differently.
func (ce *computeEnv) explainFailure(ctx context.Context, err error, action string) error {
	if err == nil {
		return nil
	}
	if state, _, ok := authStateOf(err); ok {
		ce.noteCredState(state)
		return err
	}
	if state := ce.credentials(ctx); state != credOK {
		return &computeAuthError{state: state, action: action, err: err}
	}
	return err
}

// credentials probes the profile, cached for computeCredProbeTTL.
//
// It is called from INSIDE a cache-miss fetch and only after a failure, never
// before a cache lookup: probing first would put its whole timeout in front of
// every warm request too, making NFR-2 (warm <= 50ms) unreachable on the one
// path testable without AWS.
func (ce *computeEnv) credentials(ctx context.Context) credState {
	state, _, err := computeCredCache.GetOrFetch(ce.key, computeCredProbeTTL, false, func() (credState, error) {
		if ce.creds == nil {
			return credMissing, nil
		}
		probeCtx, cancel := context.WithTimeout(ctx, computeCredProbeTimeout)
		defer cancel()
		if _, rerr := ce.creds.Retrieve(probeCtx); rerr != nil {
			if looksLikeExpiredSession(rerr) {
				return credExpired, nil
			}
			return credMissing, nil
		}
		return credOK, nil
	})
	if err != nil {
		return credMissing
	}
	return state
}

// lastKnownCredState reports the cached probe result regardless of age, without
// probing. It is what a warm request reports: the data being served was fetched
// with working credentials, so claiming otherwise would be a lie in the other
// direction.
func (ce *computeEnv) lastKnownCredState() credState {
	if state, _, ok := computeCredCache.Peek(ce.key); ok {
		return state
	}
	return credOK
}

// noteCredState records a state discovered by an actual API call rather than by
// the probe.
//
// Both directions matter. "denied" is a state no Retrieve can detect -- holding
// a valid credential says nothing about what it is allowed to do -- so it can
// only ever arrive this way. And "ok" has to be written back too, or a denial
// recorded by one call would outlive the permission being granted: the next
// successful call would still report "denied" until the 60s TTL expired.
func (ce *computeEnv) noteCredState(state credState) {
	//nolint:errcheck // the fetch closure cannot fail; this is a cache write
	computeCredCache.GetOrFetch(ce.key, 0, true, func() (credState, error) { return state, nil })
}

// ---------------------------------------------------------------------------
// FR-3 projection
// ---------------------------------------------------------------------------

// ComputeInstanceType is one catalog record, projected to exactly the fields
// architecture.md section 3.1 lists and no raw AWS envelope.
//
// Nullable fields are pointers because the contract distinguishes zero from
// unknown: a gpuCount of 0 is a fact, a gpuMemoryMiB of null is an absence, and
// an onDemandHourly of 0 would mean "free", which is never true.
type ComputeInstanceType struct {
	InstanceType             string   `json:"instanceType"`
	VCPU                     int      `json:"vcpu"`
	MemoryMiB                int64    `json:"memoryMiB"`
	Architectures            []string `json:"architectures"`
	CurrentGeneration        bool     `json:"currentGeneration"`
	NetworkPerformance       string   `json:"networkPerformance"`
	BaselineBandwidthMbps    *float64 `json:"baselineBandwidthMbps"`
	MaximumNetworkInterfaces int      `json:"maximumNetworkInterfaces"`
	GPUCount                 int      `json:"gpuCount"`
	GPUMemoryMiB             *int     `json:"gpuMemoryMiB"`
	GPUName                  *string  `json:"gpuName"`
	Burstable                bool     `json:"burstable"`
	BareMetal                bool     `json:"bareMetal"`
	SupportedUsageClasses    []string `json:"supportedUsageClasses"`
	OnDemandHourly           *float64 `json:"onDemandHourly"`
	PriceSource              string   `json:"priceSource"`
}

// Per-type price provenance (FR-5).
const (
	priceSourceAWS         = "aws_api"
	priceSourceFallback    = "fallback"
	priceSourceUnavailable = "unavailable"
)

// Envelope-level provenance (FR-12).
const (
	sourceAWSAPI   = "aws_api"
	sourceFallback = "fallback"
	sourcePartial  = "partial"
)

// projectInstanceType is the FR-3 projection. Every dereference is guarded:
// DescribeInstanceTypes models nearly every field as a pointer, and GpuInfo in
// particular is null for the overwhelming majority of types.
func projectInstanceType(rec ec2types.InstanceTypeInfo) ComputeInstanceType {
	out := ComputeInstanceType{
		InstanceType: string(rec.InstanceType),
		PriceSource:  priceSourceUnavailable,
	}

	if rec.VCpuInfo != nil && rec.VCpuInfo.DefaultVCpus != nil {
		out.VCPU = int(*rec.VCpuInfo.DefaultVCpus)
	}
	if rec.MemoryInfo != nil && rec.MemoryInfo.SizeInMiB != nil {
		out.MemoryMiB = *rec.MemoryInfo.SizeInMiB
	}
	if rec.ProcessorInfo != nil {
		for _, a := range rec.ProcessorInfo.SupportedArchitectures {
			out.Architectures = append(out.Architectures, string(a))
		}
	}
	out.CurrentGeneration = aws.ToBool(rec.CurrentGeneration)
	out.BareMetal = aws.ToBool(rec.BareMetal)
	out.Burstable = aws.ToBool(rec.BurstablePerformanceSupported)

	if rec.NetworkInfo != nil {
		out.NetworkPerformance = aws.ToString(rec.NetworkInfo.NetworkPerformance)
		out.MaximumNetworkInterfaces = int(aws.ToInt32(rec.NetworkInfo.MaximumNetworkInterfaces))
		// FR-3: baselineBandwidthMbps is NetworkCards[0].BaselineBandwidthInGbps
		// x 1000, null when absent. Smaller and older types report no baseline
		// at all, which is an absence and not a zero.
		if len(rec.NetworkInfo.NetworkCards) > 0 && rec.NetworkInfo.NetworkCards[0].BaselineBandwidthInGbps != nil {
			mbps := *rec.NetworkInfo.NetworkCards[0].BaselineBandwidthInGbps * 1000
			out.BaselineBandwidthMbps = &mbps
		}
	}

	for _, u := range rec.SupportedUsageClasses {
		out.SupportedUsageClasses = append(out.SupportedUsageClasses, string(u))
	}

	// GpuInfo is null for ~870 of 903 types. This is the common path, and the
	// reason this block exists rather than a chain of dereferences.
	if rec.GpuInfo != nil {
		if rec.GpuInfo.TotalGpuMemoryInMiB != nil {
			mem := int(*rec.GpuInfo.TotalGpuMemoryInMiB)
			out.GPUMemoryMiB = &mem
		}
		for _, g := range rec.GpuInfo.Gpus {
			if g.Count != nil {
				out.GPUCount += int(*g.Count)
			}
			if out.GPUName == nil && g.Name != nil {
				name := strings.TrimSpace(aws.ToString(g.Manufacturer) + " " + *g.Name)
				out.GPUName = &name
			}
		}
	}

	return out
}

// ---------------------------------------------------------------------------
// FR-4 filter
// ---------------------------------------------------------------------------

// computeFamilyAllowlist is FR-4's documented recommendable set. The "-flex"
// variants parse to the same family prefix ("m7i-flex.large" -> "m"), so they
// are covered without a separate entry.
var computeFamilyAllowlist = map[string]bool{
	"t": true, "m": true, "c": true, "r": true, "x": true,
	"i": true, "g": true, "p": true, "inf": true, "trn": true,
}

// filterRecommendableTypes applies FR-4: current generation, not bare metal,
// family in the allowlist. It returns a COPY, because cache entries are
// immutable and a handler that filtered the cached slice in place would corrupt
// it for every other request.
func filterRecommendableTypes(all []ComputeInstanceType) []ComputeInstanceType {
	out := make([]ComputeInstanceType, 0, len(all))
	for _, it := range all {
		if !it.CurrentGeneration || it.BareMetal {
			continue
		}
		family, _ := recommend.ParseFamilyGeneration(it.InstanceType)
		if !computeFamilyAllowlist[family] {
			continue
		}
		out = append(out, it)
	}
	return out
}

// copyComputeTypes returns a per-request copy of a cached catalog.
func copyComputeTypes(all []ComputeInstanceType) []ComputeInstanceType {
	out := make([]ComputeInstanceType, len(all))
	copy(out, all)
	return out
}

// sortComputeTypes gives the payload a stable order -- vCPU, then memory, then
// name -- so two identical requests produce identical bodies and the UI's list
// does not reshuffle between refreshes.
func sortComputeTypes(types []ComputeInstanceType) {
	sort.SliceStable(types, func(i, j int) bool {
		a, b := types[i], types[j]
		if a.VCPU != b.VCPU {
			return a.VCPU < b.VCPU
		}
		if a.MemoryMiB != b.MemoryMiB {
			return a.MemoryMiB < b.MemoryMiB
		}
		return a.InstanceType < b.InstanceType
	})
}

// ---------------------------------------------------------------------------
// The live read
// ---------------------------------------------------------------------------

// describeInstanceTypesPageSize is the API maximum. A region carries ~903
// types, so a full read is ~10 sequential pages.
const describeInstanceTypesPageSize = 100

// fetchInstanceTypeCatalog pages DescribeInstanceTypes for the whole region and
// projects each record. The price fields are left empty: prices are a separate
// cache with a separate failure mode, and merging them here would let one
// throttled GetProducts invalidate a perfectly good hardware catalog.
func fetchInstanceTypeCatalog(ctx context.Context, api ec2ComputeAPI) ([]ComputeInstanceType, error) {
	var (
		out   []ComputeInstanceType
		token *string
	)
	for {
		page, err := api.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
			MaxResults: aws.Int32(describeInstanceTypesPageSize),
			NextToken:  token,
		})
		if err != nil {
			return nil, classifyComputeError(err, actionDescribeInstanceTypes)
		}
		for _, rec := range page.InstanceTypes {
			out = append(out, projectInstanceType(rec))
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	if len(out) == 0 {
		return nil, errors.New("DescribeInstanceTypes returned no instance types")
	}
	return out, nil
}

// loadComputeCatalog is the cached read. force bypasses the age check and
// refuses to join an unforced fetch already in flight (FR-10).
//
// It deliberately does NOT call Invalidate first, although FR-10 phrases the
// refresh as "invalidate, then refetch". force=true already bypasses the stored
// value and refetches synchronously, and GetOrFetch leaves the previous entry in
// place when a fetch fails -- so the stale rung of the degradation ladder
// survives a failed refresh. Dropping the entry up front would mean that
// pressing Refresh with an expired token replaced a slightly stale live catalog
// with the region-agnostic fallback table, which is a downgrade the user did not
// ask for.
func (ce *computeEnv) loadComputeCatalog(ctx context.Context, force bool) ([]ComputeInstanceType, time.Time, error) {
	return computeCatalogCache.GetOrFetch(ce.key, computeCatalogTTL, force, func() ([]ComputeInstanceType, error) {
		types, err := fetchInstanceTypeCatalog(ctx, ce.ec2)
		if err != nil {
			return nil, ce.explainFailure(ctx, err, actionDescribeInstanceTypes)
		}
		// A successful call is proof the credentials work, and cheaper proof
		// than a probe.
		ce.noteCredState(credOK)
		return types, nil
	})
}

// fallbackComputeCatalog converts the dated fallback table into the response
// shape. The markers travel back with it, so the caller cannot serve the
// records and forget the caveats.
func fallbackComputeCatalog() ([]ComputeInstanceType, pricingpkg.FallbackCatalog) {
	fb := pricingpkg.GetFallbackCatalog()
	out := make([]ComputeInstanceType, 0, len(fb.InstanceTypes))
	for _, r := range fb.InstanceTypes {
		out = append(out, ComputeInstanceType{
			InstanceType:             r.InstanceType,
			VCPU:                     r.VCPU,
			MemoryMiB:                int64(r.MemoryMiB),
			Architectures:            r.Architectures,
			CurrentGeneration:        r.CurrentGeneration,
			NetworkPerformance:       r.NetworkPerformance,
			BaselineBandwidthMbps:    r.BaselineBandwidthMbps,
			MaximumNetworkInterfaces: r.MaximumNetworkInterfaces,
			GPUCount:                 r.GPUCount,
			GPUMemoryMiB:             r.GPUMemoryMiB,
			GPUName:                  r.GPUName,
			Burstable:                r.Burstable,
			BareMetal:                r.BareMetal,
			SupportedUsageClasses:    r.SupportedUsageClasses,
			PriceSource:              priceSourceUnavailable,
		})
	}
	return out, fb
}

// ---------------------------------------------------------------------------
// The degradation ladder
// ---------------------------------------------------------------------------

// catalogResolution is one resolved catalog plus everything the envelope has to
// say about how it was obtained. Data and caveats are returned in one value so
// a caller cannot pick up the records and drop the markers.
type catalogResolution struct {
	types                []ComputeInstanceType
	source               string
	credState            credState
	cachedAt             time.Time
	instanceDataDate     string
	availabilityVerified bool
	notice               string
}

// resolveComputeCatalog walks the ladder: live -> Peek at any age -> the dated
// fallback table. It returns no error, because FR-12 admits no failure that
// justifies a blank screen.
func (ce *computeEnv) resolveComputeCatalog(ctx context.Context, force bool) catalogResolution {
	types, cachedAt, err := ce.loadComputeCatalog(ctx, force)
	if err == nil {
		return catalogResolution{
			types:                types,
			source:               sourceAWSAPI,
			credState:            ce.lastKnownCredState(),
			cachedAt:             cachedAt,
			availabilityVerified: true,
		}
	}

	state, action, isAuth := authStateOf(err)
	if isAuth {
		ce.noteCredState(state)
	} else {
		state = ce.lastKnownCredState()
	}

	// Rung two: the last value we had, at any age. Stale-and-dated beats
	// region-agnostic-and-undated (EC-1, NFR-8).
	if stale, staleAt, ok := computeCatalogCache.Peek(ce.key); ok {
		return catalogResolution{
			types:                stale,
			source:               sourcePartial,
			credState:            state,
			cachedAt:             staleAt,
			availabilityVerified: true,
			notice: computeDegradedNotice(state, action, ce.profile,
				"showing the last catalog read from AWS, from "+staleAt.UTC().Format(time.RFC3339)),
		}
	}

	// Rung three: the compiled-in table. It is region-agnostic, so it makes no
	// availability claim about the region the user selected.
	fbTypes, fb := fallbackComputeCatalog()
	return catalogResolution{
		types:                fbTypes,
		source:               sourceFallback,
		credState:            state,
		instanceDataDate:     fb.InstanceDataDate,
		availabilityVerified: fb.AvailabilityVerified,
		notice: computeDegradedNotice(state, action, ce.profile,
			"showing a built-in instance list dated "+fb.InstanceDataDate+", which has not been checked against "+ce.region),
	}
}

// computeDegradedNotice is the one human sentence a degraded envelope carries.
// It names the profile and, for a denial, the action -- because "this profile
// cannot call pricing:GetProducts" can be fixed and "AWS error" cannot.
func computeDegradedNotice(state credState, action, profile, consequence string) string {
	who := profile
	if who == "" {
		who = "the default profile"
	}
	switch state {
	case credExpired:
		return fmt.Sprintf("your AWS session for %s has expired; run `aws sso login` — %s", who, consequence)
	case credMissing:
		return fmt.Sprintf("no AWS credentials for %s — %s", who, consequence)
	case credDenied:
		if action != "" {
			return fmt.Sprintf("%s cannot call %s — %s", who, action, consequence)
		}
		return fmt.Sprintf("%s is not permitted to read this data — %s", who, consequence)
	default:
		return fmt.Sprintf("AWS could not be reached — %s", consequence)
	}
}

// ---------------------------------------------------------------------------
// Catalog -> recommender input
// ---------------------------------------------------------------------------

// toRecommendCatalog converts the projected catalog into the pure core's input
// records, attaching spot medians where they are known. A type with no
// on-demand price keeps a nil OnDemandHourly: rule R6 excludes it from ranking,
// and a zero would make it free.
func toRecommendCatalog(types []ComputeInstanceType, spot map[string]float64) []recommend.InstanceType {
	out := make([]recommend.InstanceType, 0, len(types))
	for _, it := range types {
		family, generation := recommend.ParseFamilyGeneration(it.InstanceType)
		rec := recommend.InstanceType{
			Name:                 it.InstanceType,
			Family:               family,
			Generation:           generation,
			VCPU:                 it.VCPU,
			MemoryMiB:            it.MemoryMiB,
			Architectures:        it.Architectures,
			CurrentGeneration:    it.CurrentGeneration,
			BareMetal:            it.BareMetal,
			Burstable:            it.Burstable,
			GPUCount:             it.GPUCount,
			MaxNetworkInterfaces: it.MaximumNetworkInterfaces,
			SupportsSpot:         supportsSpot(it.SupportedUsageClasses),
			OnDemandHourly:       it.OnDemandHourly,
		}
		if v, ok := spot[it.InstanceType]; ok && v > 0 {
			median := v
			rec.SpotMedianHourly = &median
		}
		out = append(out, rec)
	}
	return out
}

func supportsSpot(classes []string) bool {
	for _, c := range classes {
		if c == "spot" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// awsvpc trunking -- read only when a pool actually asks for awsvpc
// ---------------------------------------------------------------------------

// Trunking states for the recommendation envelope's signals.trunking.
const (
	trunkingEnabled  = "enabled"
	trunkingDisabled = "disabled"
	trunkingUnknown  = "unknown"
)

// readAWSVPCTrunking reports the account's awsvpcTrunking opt-in setting.
//
// It is called ONLY for a pool whose network_mode is awsvpc. Under bridge --
// the default (D-6) -- there are no task ENIs to trunk, the setting is
// irrelevant, and asking would spend an AWS call on an answer nobody reads.
//
// An unanswerable probe returns (false, "unknown"), which under-promises
// density rather than over-promising it.
func readAWSVPCTrunking(ctx context.Context, api ecsSettingsAPI) (bool, string) {
	if api == nil {
		return false, trunkingUnknown
	}
	out, err := api.ListAccountSettings(ctx, &ecs.ListAccountSettingsInput{
		Name:              ecstypes.SettingNameAwsvpcTrunking,
		EffectiveSettings: true,
	})
	if err != nil || out == nil || len(out.Settings) == 0 {
		return false, trunkingUnknown
	}
	switch strings.ToLower(aws.ToString(out.Settings[0].Value)) {
	case "enabled":
		return true, trunkingEnabled
	case "disabled":
		return false, trunkingDisabled
	default:
		return false, trunkingUnknown
	}
}
