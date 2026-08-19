package main

// Handler tests: the status codes the caller is allowed to see, and the
// envelope shapes P11's UI is built against (architecture.md section 3).
//
// The one rule these pin hardest: an AWS failure never produces a non-200. A
// test that finds a 500 on a credentials failure has found a blank screen.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/smithy-go"

	"madappgang.com/meroku/recommend"
)

// stubResolver installs a computeEnv built from fakes for the duration of a
// test, so a handler can be driven end to end without an AWS account.
func stubResolver(t *testing.T, build func(t *testing.T) *computeEnv) {
	t.Helper()
	prev := resolveComputeEnvFn
	resolveComputeEnvFn = func(_ context.Context, envName string) (*computeEnv, error) {
		if envName != "dev" {
			return nil, fmt.Errorf("environment %q not found", envName)
		}
		return build(t), nil
	}
	t.Cleanup(func() { resolveComputeEnvFn = prev })
}

func doGet(t *testing.T, h http.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// ---------------------------------------------------------------------------
// Status codes reserved for the caller's own mistakes
// ---------------------------------------------------------------------------

func TestComputeHandlers_MethodGuard(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"instance-types": getComputeInstanceTypes,
		"spot-prices":    getComputeSpotPrices,
		"recommendation": getComputeRecommendation,
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(http.MethodPost, "/api/compute/"+name, nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST returned %d, want 405", rec.Code)
			}
		})
	}
}

func TestComputeHandlers_MissingEnv(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"instance-types": getComputeInstanceTypes,
		"spot-prices":    getComputeSpotPrices,
		"recommendation": getComputeRecommendation,
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			rec := doGet(t, h, "/api/compute/"+name)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
			var body ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error != "env parameter is required" {
				t.Errorf("error = %q, want %q", body.Error, "env parameter is required")
			}
		})
	}
}

func TestComputeSpotPrices_TypeLimits(t *testing.T) {
	t.Run("types is required", func(t *testing.T) {
		rec := doGet(t, getComputeSpotPrices, "/api/compute/spot-prices?env=dev")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "types parameter is required") {
			t.Errorf("body = %s", rec.Body.String())
		}
	})

	t.Run("more than twenty names the limit and the count", func(t *testing.T) {
		names := make([]string, 27)
		for i := range names {
			names[i] = fmt.Sprintf("m7i.size%02d", i)
		}
		rec := doGet(t, getComputeSpotPrices, "/api/compute/spot-prices?env=dev&types="+strings.Join(names, ","))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
		want := "at most 20 instance types per request, got 27"
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body = %s, want it to contain %q", rec.Body.String(), want)
		}
	})
}

func TestComputeRecommendation_RejectsUnknownPosture(t *testing.T) {
	rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev&posture=cheapest")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"performance-first", "balanced", "cost-first"} {
		if !strings.Contains(body, want) {
			t.Errorf("the 400 does not name %q: %s", want, body)
		}
	}
}

// ---------------------------------------------------------------------------
// Envelopes -- the shapes P11 is built against
// ---------------------------------------------------------------------------

func liveComputeEnv(t *testing.T) *computeEnv {
	t.Helper()
	ce := testComputeEnv(t)
	ce.env = Env{
		Project: "fixture",
		Env:     "dev",
		Region:  "ap-southeast-2",
		Workload: Workload{
			BackendCPU:          "512",
			BackendMemory:       "1024",
			BackendDesiredCount: 2,
		},
		Services: []Service{{Name: "worker", CPU: 256, Memory: 512, DesiredCount: 3}},
	}
	ce.ec2 = &fakeEC2{
		typePages: []*ec2.DescribeInstanceTypesOutput{{InstanceTypes: []ec2types.InstanceTypeInfo{
			typeRecord("m7i.large", 2, 8192, "x86_64"),
			typeRecord("r7i.large", 2, 16384, "x86_64"),
			typeRecord("c7i.large", 2, 4096, "x86_64"),
			typeRecord("m7g.large", 2, 8192, "arm64"),
		}}},
		spotPages: []*ec2.DescribeSpotPriceHistoryOutput{{SpotPriceHistory: []ec2types.SpotPrice{
			spotRecord("m7i.large", "ap-southeast-2a", "0.0400", time.Now()),
			spotRecord("m7i.large", "ap-southeast-2b", "0.0420", time.Now()),
			spotRecord("r7i.large", "ap-southeast-2a", "0.0500", time.Now()),
		}}},
	}
	ce.pricingAPI = &fakePricing{pages: []*awspricing.GetProductsOutput{{PriceList: []string{
		syntheticSKU(t, "m7i.large", "0.1000000000", "RSV"),
		syntheticSKU(t, "r7i.large", "0.1400000000", "RSV"),
		syntheticSKU(t, "c7i.large", "0.0900000000", "RSV"),
		syntheticSKU(t, "m7g.large", "0.0800000000", "RSV"),
	}}}}
	// CloudWatch answers with nothing: the fan-out itself is exercised in
	// compute_signals_test.go, and what matters here is that a configured-only
	// recommendation still produces a complete envelope.
	ce.cloudwatch = &fakeCloudWatch{}
	return ce
}

func TestComputeInstanceTypes_Envelope(t *testing.T) {
	stubResolver(t, liveComputeEnv)

	rec := doGet(t, getComputeInstanceTypes, "/api/compute/instance-types?env=dev")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	var body computeInstanceTypesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Region != "ap-southeast-2" {
		t.Errorf("region = %q", body.Region)
	}
	if body.Source != sourceAWSAPI {
		t.Errorf("source = %q, want %q", body.Source, sourceAWSAPI)
	}
	if body.CredentialsState != credOK {
		t.Errorf("credentialsState = %q, want ok", body.CredentialsState)
	}
	if !body.Filtered {
		t.Error("filtered = false without all=true")
	}
	if body.TotalAvailable != 4 {
		t.Errorf("totalAvailable = %d, want 4 (the UNFILTERED count)", body.TotalAvailable)
	}
	if !body.AvailabilityVerified {
		t.Error("availabilityVerified = false on live data")
	}
	if body.PricingRegion != nil {
		t.Errorf("pricingRegion = %v, want null when every price came from the Pricing API", *body.PricingRegion)
	}
	if body.CachedAt == nil {
		t.Error("cachedAt is null on a live read")
	}
	if len(body.InstanceTypes) != 4 {
		t.Fatalf("got %d types, want 4", len(body.InstanceTypes))
	}

	byName := map[string]ComputeInstanceType{}
	for _, it := range body.InstanceTypes {
		byName[it.InstanceType] = it
	}
	m := byName["m7i.large"]
	if m.OnDemandHourly == nil || *m.OnDemandHourly != 0.10 {
		t.Errorf("m7i.large onDemandHourly = %v, want 0.10", m.OnDemandHourly)
	}
	if m.PriceSource != priceSourceAWS {
		t.Errorf("m7i.large priceSource = %q", m.PriceSource)
	}
	if m.GPUCount != 0 || m.GPUMemoryMiB != nil || m.GPUName != nil {
		t.Errorf("a nil GpuInfo did not project to 0/null/null: %+v", m)
	}

	t.Run("the payload is JSON-stable across two identical requests", func(t *testing.T) {
		again := doGet(t, getComputeInstanceTypes, "/api/compute/instance-types?env=dev")
		if again.Body.String() != rec.Body.String() {
			t.Error("two identical requests produced different bodies; the UI list would reshuffle")
		}
	})
}

func TestComputeInstanceTypes_DegradesTo200(t *testing.T) {
	stubResolver(t, func(t *testing.T) *computeEnv {
		ce := testComputeEnv(t)
		ce.creds = &countingCreds{err: fmt.Errorf("no credentials configured")}
		ce.ec2 = &fakeEC2{}
		ce.pricingAPI = &fakePricing{}
		return ce
	})

	rec := doGet(t, getComputeInstanceTypes, "/api/compute/instance-types?env=dev")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 — no credentials must never mean a blank screen", rec.Code)
	}

	var body computeInstanceTypesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.InstanceTypes) == 0 {
		t.Fatal("degraded payload is empty")
	}
	if body.Source != sourceFallback {
		t.Errorf("source = %q, want %q", body.Source, sourceFallback)
	}
	if body.CredentialsState != credMissing {
		t.Errorf("credentialsState = %q, want %q", body.CredentialsState, credMissing)
	}
	if body.AvailabilityVerified {
		t.Error("availabilityVerified = true under fallback")
	}
	if body.InstanceDataDate == nil {
		t.Error("instanceDataDate is null under fallback; the records are undated")
	}
	if body.PricingRegion == nil || *body.PricingRegion != "us-east-1" {
		t.Errorf("pricingRegion = %v, want us-east-1 — the fallback prices are not the "+
			"selected region's", body.PricingRegion)
	}
	if body.Notice == nil || *body.Notice == "" {
		t.Error("no notice on a degraded payload")
	}
}

func TestComputeInstanceTypes_AllBypassesTheFilter(t *testing.T) {
	stubResolver(t, func(t *testing.T) *computeEnv {
		ce := testComputeEnv(t)
		old := typeRecord("m4.large", 2, 8192, "x86_64")
		old.CurrentGeneration = aws.Bool(false)
		ce.ec2 = &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{{
			InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("m7i.large", 2, 8192, "x86_64"), old},
		}}}
		ce.pricingAPI = &fakePricing{pages: []*awspricing.GetProductsOutput{{
			PriceList: []string{syntheticSKU(t, "m7i.large", "0.1000000000", "RSV")},
		}}}
		return ce
	})

	filtered := doGet(t, getComputeInstanceTypes, "/api/compute/instance-types?env=dev")
	all := doGet(t, getComputeInstanceTypes, "/api/compute/instance-types?env=dev&all=true")

	var f, a computeInstanceTypesResponse
	if err := json.Unmarshal(filtered.Body.Bytes(), &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := json.Unmarshal(all.Body.Bytes(), &a); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(f.InstanceTypes) != 1 || !f.Filtered {
		t.Errorf("default response listed %d types (filtered=%v), want 1 filtered", len(f.InstanceTypes), f.Filtered)
	}
	if len(a.InstanceTypes) != 2 || a.Filtered {
		t.Errorf("all=true listed %d types (filtered=%v), want 2 unfiltered", len(a.InstanceTypes), a.Filtered)
	}
	if f.TotalAvailable != a.TotalAvailable {
		t.Errorf("totalAvailable differs between the two views: %d vs %d", f.TotalAvailable, a.TotalAvailable)
	}
}

func TestComputeSpotPrices_Envelope(t *testing.T) {
	stubResolver(t, liveComputeEnv)

	rec := doGet(t, getComputeSpotPrices, "/api/compute/spot-prices?env=dev&types=m7i.large,exotic.42xlarge")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body computeSpotPricesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Prices) != 2 {
		t.Fatalf("got %d quotes, want one per requested type", len(body.Prices))
	}
	// The order is the sorted type list, not the request's: the cache key is the
	// sorted list, so two requests naming the same set in different orders share
	// one entry and must therefore share one canonical order.
	byName := map[string]SpotQuote{}
	for _, q := range body.Prices {
		byName[q.InstanceType] = q
	}
	if m := byName["m7i.large"]; !m.SpotAvailable || m.Median == nil {
		t.Errorf("m7i.large quote = %+v", m)
	}
	if x := byName["exotic.42xlarge"]; x.SpotAvailable || x.Median != nil {
		t.Errorf("a type with no market reported %+v, want spotAvailable false and a null median", x)
	}
}

func TestComputeSpotPrices_DegradesTo200(t *testing.T) {
	stubResolver(t, func(t *testing.T) *computeEnv {
		ce := testComputeEnv(t)
		ce.ec2 = &fakeEC2{spotErr: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "synthetic"}}
		return ce
	})

	rec := doGet(t, getComputeSpotPrices, "/api/compute/spot-prices?env=dev&types=m7i.large")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	var body computeSpotPricesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.CredentialsState != credDenied {
		t.Errorf("credentialsState = %q, want %q", body.CredentialsState, credDenied)
	}
	if len(body.Prices) != 1 || body.Prices[0].SpotAvailable {
		t.Errorf("prices = %+v, want one unavailable row", body.Prices)
	}
	if body.Notice == nil || !strings.Contains(*body.Notice, actionDescribeSpotPriceHistory) {
		t.Errorf("notice = %v, want it to name the action", body.Notice)
	}
}

func TestComputeRecommendation_Envelope(t *testing.T) {
	stubResolver(t, liveComputeEnv)

	rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev&posture=balanced&limit=3")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body computeRecommendationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Posture != "balanced" {
		t.Errorf("posture = %q", body.Posture)
	}
	if body.Primary == nil {
		t.Fatalf("no primary candidate: unsatisfiable=%v constraint=%v", body.Unsatisfiable, body.Constraint)
	}
	if body.Primary.Reason == "" {
		t.Error("the primary candidate carries no reason sentence")
	}
	if len(body.Ranked) == 0 || len(body.Ranked) > 3 {
		t.Errorf("ranked has %d entries, want 1..3 (limit=3)", len(body.Ranked))
	}
	if body.Ranked[0].InstanceType != body.Primary.InstanceType {
		t.Error("primary is not the head of ranked")
	}
	if body.Signals.NetworkMode != recommend.NetworkModeBridge {
		t.Errorf("networkMode = %q, want bridge (the default, D-6)", body.Signals.NetworkMode)
	}
	if body.Signals.Trunking != recommend.TrunkingNotApplicable {
		t.Errorf("trunking = %q, want not_applicable under bridge", body.Signals.Trunking)
	}
	if body.Signals.DensityBasis != recommend.DensityCPUMemoryOnly {
		t.Errorf("densityBasis = %q, want cpu_memory_only under bridge", body.Signals.DensityBasis)
	}
	if len(body.Signals.Dropped) != 0 {
		t.Errorf("signals.dropped = %+v, want empty: a non-finite score is a defect", body.Signals.Dropped)
	}
	if body.Signals.Configured.TaskCount != 5 {
		t.Errorf("configured taskCount = %d, want 5 (2 backend + 3 worker)", body.Signals.Configured.TaskCount)
	}
	if body.SuggestedPool.NetworkMode != recommend.NetworkModeBridge {
		t.Errorf("suggestedPool.network_mode = %q, want bridge", body.SuggestedPool.NetworkMode)
	}
	if len(body.SuggestedPool.InstanceTypes) == 0 {
		t.Error("suggestedPool has no instance types")
	}

	t.Run("suggestedPool keys are snake_case, ready for YAML", func(t *testing.T) {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode: %v", err)
		}
		var pool map[string]json.RawMessage
		if err := json.Unmarshal(raw["suggestedPool"], &pool); err != nil {
			t.Fatalf("decode pool: %v", err)
		}
		for _, key := range []string{"instance_types", "capacity_type", "min_size", "max_size",
			"target_capacity", "network_mode", "ami_family", "root_volume_gb", "on_demand_base"} {
			if _, ok := pool[key]; !ok {
				t.Errorf("suggestedPool is missing %q", key)
			}
		}
	})

	t.Run("the architecture filter follows ami_family both ways", func(t *testing.T) {
		armRec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev&ami_family=al2023_arm64")
		var arm computeRecommendationResponse
		if err := json.Unmarshal(armRec.Body.Bytes(), &arm); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, c := range arm.Ranked {
			if c.Architecture != recommend.ArchARM64 {
				t.Errorf("ami_family=al2023_arm64 returned %s (%s); the rendered AMI would "+
					"refuse to boot it", c.InstanceType, c.Architecture)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Envelope helpers
// ---------------------------------------------------------------------------

func TestMergeSourcesAndCredState(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{sourceAWSAPI, sourceAWSAPI, sourceAWSAPI},
		{sourceFallback, sourceFallback, sourceFallback},
		{sourceAWSAPI, sourceFallback, sourcePartial},
		{sourcePartial, sourceAWSAPI, sourcePartial},
	}
	for _, tc := range cases {
		if got := mergeSources(tc.a, tc.b); got != tc.want {
			t.Errorf("mergeSources(%q,%q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}

	credCases := []struct{ a, b, want credState }{
		{credOK, credOK, credOK},
		{credOK, credDenied, credDenied},
		{credDenied, credOK, credDenied},
		{credDenied, credMissing, credMissing},
		{credExpired, credDenied, credExpired},
	}
	for _, tc := range credCases {
		if got := worstCredState(tc.a, tc.b); got != tc.want {
			t.Errorf("worstCredState(%q,%q) = %q, want %q — a denial discovered by one call "+
				"must not be masked by another that succeeded", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSplitTypesParam(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   ", 0},
		{"m7i.large", 1},
		{"m7i.large,r7i.large", 2},
		{"m7i.large,,r7i.large,", 2},
		{" m7i.large , r7i.large ", 2},
	}
	for _, tc := range cases {
		if got := splitTypesParam(tc.in); len(got) != tc.want {
			t.Errorf("splitTypesParam(%q) = %v, want %d entries", tc.in, got, tc.want)
		}
	}
}

func TestPoolConstraintsFor(t *testing.T) {
	spot := "spot"
	base := 2
	minSize := 3
	env := Env{Compute: Compute{Pools: []ComputePool{{
		Name:         "batch",
		NetworkMode:  recommend.NetworkModeAWSVPC,
		AMIFamily:    recommend.AMIFamilyAL2023ARM64,
		CapacityType: spot,
		OnDemandBase: &base,
		MinSize:      &minSize,
	}}}}

	t.Run("an existing pool supplies its own constraints", func(t *testing.T) {
		got := poolConstraintsFor(env, "batch", map[string][]string{})
		if got.NetworkMode != recommend.NetworkModeAWSVPC {
			t.Errorf("networkMode = %q", got.NetworkMode)
		}
		if got.AMIFamily != recommend.AMIFamilyAL2023ARM64 {
			t.Errorf("amiFamily = %q", got.AMIFamily)
		}
		if got.CapacityType != spot || got.OnDemandBase != 2 {
			t.Errorf("capacity = %q base %d", got.CapacityType, got.OnDemandBase)
		}
		if got.MinSize == nil || *got.MinSize != 3 {
			t.Errorf("minSize = %v", got.MinSize)
		}
	})

	t.Run("a pool that does not exist yet defaults to bridge and al2023", func(t *testing.T) {
		got := poolConstraintsFor(Env{}, "", map[string][]string{})
		if got.NetworkMode != recommend.NetworkModeBridge {
			t.Errorf("networkMode = %q, want bridge (D-6)", got.NetworkMode)
		}
		if got.AMIFamily != recommend.AMIFamilyAL2023 {
			t.Errorf("amiFamily = %q, want al2023 explicitly — \"\" let the architecture filter "+
				"accept arm64 while the template rendered an x86 AMI (C-11)", got.AMIFamily)
		}
	})

	t.Run("the query overrides the pool", func(t *testing.T) {
		got := poolConstraintsFor(env, "batch", map[string][]string{
			"ami_family":   {recommend.AMIFamilyAL2023},
			"network_mode": {recommend.NetworkModeBridge},
			"gpu":          {"true"},
		})
		if got.AMIFamily != recommend.AMIFamilyAL2023 {
			t.Errorf("amiFamily = %q, want the query's value", got.AMIFamily)
		}
		if got.NetworkMode != recommend.NetworkModeBridge {
			t.Errorf("networkMode = %q, want the query's value", got.NetworkMode)
		}
		if !got.ForceGPU {
			t.Error("gpu=true did not force the GPU classification")
		}
	})

	t.Run("an unrecognised value normalises rather than propagating", func(t *testing.T) {
		got := poolConstraintsFor(Env{}, "", map[string][]string{
			"network_mode": {"host"},
			"ami_family":   {"ubuntu"},
		})
		if got.NetworkMode != recommend.NetworkModeBridge || got.AMIFamily != recommend.AMIFamilyAL2023 {
			t.Errorf("got %q/%q, want the normalised defaults", got.NetworkMode, got.AMIFamily)
		}
	})
}

func TestOptionalString(t *testing.T) {
	if optionalString("") != nil {
		t.Error("an empty string must render as JSON null, not as \"\"")
	}
	if got := optionalString("x"); got == nil || *got != "x" {
		t.Errorf("optionalString(\"x\") = %v", got)
	}
}
