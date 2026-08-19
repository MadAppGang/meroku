package main

// P8's boundary tests: the two AWS configs, the probe that must not run in
// front of a warm cache, the FR-3 projection, the FR-4 filter, and the four-value
// credential enum.
//
// Fixtures are synthetic (CON-5): invented instance-type records and obviously
// round prices. No value is copied from a live account.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/smithy-go"
	"gopkg.in/yaml.v2"

	pricingpkg "madappgang.com/meroku/pricing"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeEC2 counts calls and replays canned pages, so a test can assert both the
// answer and the number of requests it took.
type fakeEC2 struct {
	typePages  []*ec2.DescribeInstanceTypesOutput
	typesErr   error
	typesCalls int32

	spotPages []*ec2.DescribeSpotPriceHistoryOutput
	spotErr   error
	spotCalls int32
	lastSpot  *ec2.DescribeSpotPriceHistoryInput
}

func (f *fakeEC2) DescribeInstanceTypes(_ context.Context, _ *ec2.DescribeInstanceTypesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	n := atomic.AddInt32(&f.typesCalls, 1)
	if f.typesErr != nil {
		return nil, f.typesErr
	}
	idx := int(n) - 1
	if idx >= len(f.typePages) {
		return &ec2.DescribeInstanceTypesOutput{}, nil
	}
	return f.typePages[idx], nil
}

func (f *fakeEC2) DescribeSpotPriceHistory(_ context.Context, in *ec2.DescribeSpotPriceHistoryInput, _ ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	n := atomic.AddInt32(&f.spotCalls, 1)
	f.lastSpot = in
	if f.spotErr != nil {
		return nil, f.spotErr
	}
	idx := int(n) - 1
	if idx >= len(f.spotPages) {
		return &ec2.DescribeSpotPriceHistoryOutput{}, nil
	}
	return f.spotPages[idx], nil
}

// fakePricing counts GetProducts calls and replays canned PriceList pages.
type fakePricing struct {
	pages   []*awspricing.GetProductsOutput
	err     error
	delay   time.Duration // stands in for the ~6.6s a live region read costs
	calls   int32
	lastIn  *awspricing.GetProductsInput
	allFilt [][]string // one [field, value] pair list per call
}

func (f *fakePricing) GetProducts(ctx context.Context, in *awspricing.GetProductsInput, _ ...func(*awspricing.Options)) (*awspricing.GetProductsOutput, error) {
	n := atomic.AddInt32(&f.calls, 1)
	f.lastIn = in
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	pairs := []string{}
	for _, filt := range in.Filters {
		pairs = append(pairs, aws.ToString(filt.Field)+"="+aws.ToString(filt.Value))
	}
	f.allFilt = append(f.allFilt, pairs)
	if f.err != nil {
		return nil, f.err
	}
	idx := int(n) - 1
	if idx >= len(f.pages) {
		return &awspricing.GetProductsOutput{}, nil
	}
	return f.pages[idx], nil
}

// fakeCloudWatch answers per metric+service, and can fail for one service only.
type fakeCloudWatch struct {
	byService map[string]*cloudwatch.GetMetricStatisticsOutput
	errFor    map[string]error
	calls     int32
}

func (f *fakeCloudWatch) GetMetricStatistics(_ context.Context, in *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	atomic.AddInt32(&f.calls, 1)
	service := ""
	for _, d := range in.Dimensions {
		if aws.ToString(d.Name) == "ServiceName" {
			service = aws.ToString(d.Value)
		}
	}
	if err, ok := f.errFor[service]; ok {
		return nil, err
	}
	if out, ok := f.byService[service]; ok {
		return out, nil
	}
	return &cloudwatch.GetMetricStatisticsOutput{}, nil
}

type fakeECSSettings struct {
	value string
	err   error
}

func (f *fakeECSSettings) ListAccountSettings(_ context.Context, _ *ecs.ListAccountSettingsInput, _ ...func(*ecs.Options)) (*ecs.ListAccountSettingsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ecs.ListAccountSettingsOutput{
		Settings: []ecstypes.Setting{{
			Name:  ecstypes.SettingNameAwsvpcTrunking,
			Value: aws.String(f.value),
		}},
	}, nil
}

// countingCreds is the probe fake: it sleeps like a cold SSO retrieval and
// counts how many times it was asked.
type countingCreds struct {
	calls atomic.Int32
	delay time.Duration
	err   error
}

func (c *countingCreds) Retrieve(ctx context.Context) (aws.Credentials, error) {
	c.calls.Add(1)
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return aws.Credentials{}, ctx.Err()
		}
	}
	if c.err != nil {
		return aws.Credentials{}, c.err
	}
	return aws.Credentials{AccessKeyID: "AKIAFAKE", SecretAccessKey: "fake-key-do-not-use", Source: "test"}, nil
}

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

func testComputeEnv(t *testing.T) *computeEnv {
	t.Helper()
	key := pricingpkg.CacheKey("test-"+t.Name(), "ap-southeast-2")
	t.Cleanup(func() {
		computeCatalogCache.Invalidate(key)
		computeOnDemandCache.Invalidate(key)
		computeCredCache.Invalidate(key)
	})
	return &computeEnv{
		env:     Env{Project: "fixture", Env: "dev", Region: "ap-southeast-2"},
		region:  "ap-southeast-2",
		profile: "test-" + t.Name(),
		key:     key,
		creds:   &countingCreds{},
	}
}

func typeRecord(name string, vcpu int32, memMiB int64, arch string) ec2types.InstanceTypeInfo {
	return ec2types.InstanceTypeInfo{
		InstanceType:      ec2types.InstanceType(name),
		VCpuInfo:          &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(vcpu)},
		MemoryInfo:        &ec2types.MemoryInfo{SizeInMiB: aws.Int64(memMiB)},
		ProcessorInfo:     &ec2types.ProcessorInfo{SupportedArchitectures: []ec2types.ArchitectureType{ec2types.ArchitectureType(arch)}},
		CurrentGeneration: aws.Bool(true),
		BareMetal:         aws.Bool(false),
		NetworkInfo: &ec2types.NetworkInfo{
			MaximumNetworkInterfaces: aws.Int32(3),
			NetworkPerformance:       aws.String("Up to 12.5 Gigabit"),
			NetworkCards: []ec2types.NetworkCardInfo{
				{BaselineBandwidthInGbps: aws.Float64(0.781)},
			},
		},
		SupportedUsageClasses: []ec2types.UsageClassType{"on-demand", "spot"},
		// GpuInfo deliberately absent: AWS returns null for ~870 of 903 types.
	}
}

// ---------------------------------------------------------------------------
// C-2 -- the pricing client is pinned, the environment's region is a filter
// ---------------------------------------------------------------------------

func TestResolveComputeEnv_PricingClientIsUSEast1(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"schema_version": 26,
		"env":            "dev",
		"project":        "fixture",
		"region":         "ap-southeast-2",
		"account_id":     "000000000000",
		"aws_profile":    "none",
		"workload":       map[string]interface{}{"backend_image_port": 8080},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if werr := os.WriteFile(filepath.Join(dir, "dev.yaml"), data, 0o644); werr != nil {
		t.Fatalf("write fixture: %v", werr)
	}
	chdir(t, dir)

	ce, err := resolveComputeEnv(context.Background(), "dev")
	if err != nil {
		t.Fatalf("resolveComputeEnv: %v", err)
	}

	if ce.awsCfg.Region != "ap-southeast-2" {
		t.Errorf("awsCfg.Region = %q, want ap-southeast-2 (the environment's region)", ce.awsCfg.Region)
	}
	if ce.pricingCfg.Region != "us-east-1" {
		t.Errorf("pricingCfg.Region = %q, want us-east-1 — the Price List API is served from "+
			"us-east-1 and ap-south-1 only, and the environment's region travels as the regionCode filter", ce.pricingCfg.Region)
	}
	if ce.region != "ap-southeast-2" {
		t.Errorf("ce.region = %q, want the environment's region: it is the regionCode filter value", ce.region)
	}
}

// ---------------------------------------------------------------------------
// NFR-2 -- a warm request never probes
// ---------------------------------------------------------------------------

func TestResolveComputeEnv_CacheBeforeProbe(t *testing.T) {
	ce := testComputeEnv(t)
	creds := &countingCreds{delay: 1500 * time.Millisecond}
	ce.creds = creds
	ce.ec2 = &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{
		{InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("m7i.large", 2, 8192, "x86_64")}},
	}}

	// Warm the cache directly, bypassing the fetch: this is the state a second
	// request finds, and the point of the test is what that second request does.
	if _, _, err := computeCatalogCache.GetOrFetch(ce.key, computeCatalogTTL, false, func() ([]ComputeInstanceType, error) {
		return []ComputeInstanceType{{InstanceType: "m7i.large", VCPU: 2, MemoryMiB: 8192}}, nil
	}); err != nil {
		t.Fatalf("warm: %v", err)
	}

	start := time.Now()
	got, _, err := ce.loadComputeCatalog(context.Background(), false)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("warm read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("warm read returned %d types, want 1", len(got))
	}
	if n := creds.calls.Load(); n != 0 {
		t.Errorf("credentials probed %d times on a warm request, want 0 — probing before the "+
			"cache lookup puts up to 1.5s in front of every request, warm ones included", n)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("warm read took %v, want <= 50ms (NFR-2)", elapsed)
	}
}

// TestResolveComputeCatalog_SlowCredentialsAreNotMissingCredentials pins the
// finding that changed the probe's role.
//
// A cold SSO Retrieve on this repository's own profile measures ~1.45s, so a
// probe that gates the fetch behind a tight timeout labels a working session
// "missing" and serves the built-in table to a user whose credentials are fine.
// The call now runs first and the probe only explains a failure, so a slow
// provider costs nothing at all on the happy path.
func TestResolveComputeCatalog_SlowCredentialsAreNotMissingCredentials(t *testing.T) {
	ce := testComputeEnv(t)
	creds := &countingCreds{delay: 2 * time.Second} // slower than any gate would allow
	ce.creds = creds
	ce.ec2 = &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{{
		InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("m7i.large", 2, 8192, "x86_64")},
	}}}

	start := time.Now()
	got := ce.resolveComputeCatalog(context.Background(), false)
	elapsed := time.Since(start)

	if got.source != sourceAWSAPI {
		t.Errorf("source = %q, want %q — a slow credential provider is not an absent one", got.source, sourceAWSAPI)
	}
	if got.credState != credOK {
		t.Errorf("credentialsState = %q, want ok", got.credState)
	}
	if n := creds.calls.Load(); n != 0 {
		t.Errorf("probed %d times on a successful fetch, want 0 — a call that worked is the "+
			"strongest evidence the credentials work, and cheaper than a Retrieve", n)
	}
	if elapsed > time.Second {
		t.Errorf("cold fetch took %v; the credential provider's latency leaked into it", elapsed)
	}
}

// ---------------------------------------------------------------------------
// C-20 -- denied is not ok
// ---------------------------------------------------------------------------

func TestResolveComputeEnv_ClassifiesAccessDenied(t *testing.T) {
	cases := []struct {
		name  string
		code  string
		want  credState
		state string
	}{
		{name: "access denied", code: "AccessDeniedException", want: credDenied},
		{name: "unauthorized operation", code: "UnauthorizedOperation", want: credDenied},
		{name: "expired token", code: "ExpiredTokenException", want: credExpired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := &smithy.GenericAPIError{Code: tc.code, Message: "synthetic"}
			classified := classifyComputeError(apiErr, actionGetProducts)

			state, action, isAuth := authStateOf(classified)
			if !isAuth {
				t.Fatalf("classifyComputeError(%s) was not recognised as a credentials problem", tc.code)
			}
			if state != tc.want {
				t.Errorf("state = %q, want %q", state, tc.want)
			}
			if action != actionGetProducts {
				t.Errorf("action = %q, want %q — the notice has to name what the profile cannot call", action, actionGetProducts)
			}

			notice := computeDegradedNotice(state, action, "mag", "prices are the built-in table")
			if state == credDenied && !strings.Contains(notice, actionGetProducts) {
				t.Errorf("notice %q does not name the action; a user cannot fix %q", notice, "AWS error")
			}
		})
	}

	t.Run("a throttle is not a credentials problem", func(t *testing.T) {
		apiErr := &smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down"}
		if _, _, isAuth := authStateOf(classifyComputeError(apiErr, actionGetProducts)); isAuth {
			t.Error("a throttle was classified as a credentials failure; the user would be told to re-login for a rate limit")
		}
	})
}

// ---------------------------------------------------------------------------
// FR-3 -- projection, with GpuInfo null as the common path
// ---------------------------------------------------------------------------

func TestProjectInstanceType_NilGpuInfoIsTheCommonPath(t *testing.T) {
	got := projectInstanceType(typeRecord("m7i.large", 2, 8192, "x86_64")) // must not panic

	if got.GPUCount != 0 {
		t.Errorf("gpuCount = %d, want 0", got.GPUCount)
	}
	if got.GPUMemoryMiB != nil {
		t.Errorf("gpuMemoryMiB = %v, want null", *got.GPUMemoryMiB)
	}
	if got.GPUName != nil {
		t.Errorf("gpuName = %v, want null", *got.GPUName)
	}
	if got.VCPU != 2 || got.MemoryMiB != 8192 {
		t.Errorf("vcpu/memory = %d/%d, want 2/8192", got.VCPU, got.MemoryMiB)
	}
	if got.BaselineBandwidthMbps == nil || *got.BaselineBandwidthMbps != 781 {
		t.Errorf("baselineBandwidthMbps = %v, want 781 (Gbps x 1000)", got.BaselineBandwidthMbps)
	}
	if got.PriceSource != priceSourceUnavailable {
		t.Errorf("priceSource = %q, want %q before prices are applied", got.PriceSource, priceSourceUnavailable)
	}

	t.Run("a populated GpuInfo projects all three", func(t *testing.T) {
		rec := typeRecord("g5.xlarge", 4, 16384, "x86_64")
		rec.GpuInfo = &ec2types.GpuInfo{
			TotalGpuMemoryInMiB: aws.Int32(24576),
			Gpus: []ec2types.GpuDeviceInfo{
				{Count: aws.Int32(1), Manufacturer: aws.String("NVIDIA"), Name: aws.String("A10G")},
			},
		}
		g := projectInstanceType(rec)
		if g.GPUCount != 1 {
			t.Errorf("gpuCount = %d, want 1", g.GPUCount)
		}
		if g.GPUMemoryMiB == nil || *g.GPUMemoryMiB != 24576 {
			t.Errorf("gpuMemoryMiB = %v, want 24576", g.GPUMemoryMiB)
		}
		if g.GPUName == nil || *g.GPUName != "NVIDIA A10G" {
			t.Errorf("gpuName = %v, want \"NVIDIA A10G\"", g.GPUName)
		}
	})

	t.Run("an empty GpuInfo is not the same shape and must not panic", func(t *testing.T) {
		rec := typeRecord("m7i-flex.large", 2, 8192, "x86_64")
		rec.GpuInfo = &ec2types.GpuInfo{}
		g := projectInstanceType(rec)
		if g.GPUCount != 0 || g.GPUMemoryMiB != nil || g.GPUName != nil {
			t.Errorf("empty GpuInfo projected to %+v, want zero/null/null", g)
		}
	})
}

// ---------------------------------------------------------------------------
// FR-4 -- the recommendable filter
// ---------------------------------------------------------------------------

func TestFilterRecommendableTypes(t *testing.T) {
	all := []ComputeInstanceType{
		{InstanceType: "m7i.large", CurrentGeneration: true},
		{InstanceType: "m7i-flex.large", CurrentGeneration: true},
		{InstanceType: "m4.large", CurrentGeneration: false}, // previous generation
		{InstanceType: "m7i.metal-24xl", CurrentGeneration: true, BareMetal: true},
		{InstanceType: "mac2.metal", CurrentGeneration: true, BareMetal: true},
		{InstanceType: "u-6tb1.112xlarge", CurrentGeneration: true}, // family not in the allowlist
		{InstanceType: "inf2.xlarge", CurrentGeneration: true},
	}

	got := filterRecommendableTypes(all)
	names := map[string]bool{}
	for _, it := range got {
		names[it.InstanceType] = true
	}

	for _, want := range []string{"m7i.large", "m7i-flex.large", "inf2.xlarge"} {
		if !names[want] {
			t.Errorf("%s was filtered out, want kept", want)
		}
	}
	for _, unwanted := range []string{"m4.large", "m7i.metal-24xl", "mac2.metal", "u-6tb1.112xlarge"} {
		if names[unwanted] {
			t.Errorf("%s survived the FR-4 filter, want removed", unwanted)
		}
	}

	t.Run("the input is not mutated", func(t *testing.T) {
		if len(all) != 7 {
			t.Fatalf("filter mutated its input: %d records left, want 7", len(all))
		}
	})
}

// ---------------------------------------------------------------------------
// FR-12 -- no credentials never means a blank screen
// ---------------------------------------------------------------------------

func TestResolveComputeCatalog_FallsBackWithMarkers(t *testing.T) {
	ce := testComputeEnv(t)
	ce.creds = &countingCreds{err: errors.New("no credentials configured")}
	ce.ec2 = &fakeEC2{}

	got := ce.resolveComputeCatalog(context.Background(), false)

	if len(got.types) == 0 {
		t.Fatal("degraded catalog is empty; no credentials must never produce a blank screen")
	}
	if got.source != sourceFallback {
		t.Errorf("source = %q, want %q", got.source, sourceFallback)
	}
	if got.credState != credMissing {
		t.Errorf("credentialsState = %q, want %q", got.credState, credMissing)
	}
	if got.availabilityVerified {
		t.Error("availabilityVerified = true on the fallback table; nothing there has been checked against the region")
	}
	if got.instanceDataDate != pricingpkg.FALLBACK_INSTANCE_DATA_DATE {
		t.Errorf("instanceDataDate = %q, want %q", got.instanceDataDate, pricingpkg.FALLBACK_INSTANCE_DATA_DATE)
	}
	if got.notice == "" {
		t.Error("degraded response carries no notice; the user cannot tell live data from a built-in table")
	}

	// The default pool's three types must be present, or the zero-config path
	// shows a user a pool it cannot describe.
	names := map[string]bool{}
	for _, it := range got.types {
		names[it.InstanceType] = true
	}
	for _, want := range pricingpkg.FallbackDefaultPoolInstanceTypes() {
		if !names[want] {
			t.Errorf("fallback catalog is missing default-pool type %s", want)
		}
	}
}

func TestResolveComputeCatalog_PrefersStaleOverFallback(t *testing.T) {
	ce := testComputeEnv(t)
	ce.ec2 = &fakeEC2{typesErr: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down"}}

	// A previous successful read, now well past its TTL.
	if _, _, err := computeCatalogCache.GetOrFetch(ce.key, computeCatalogTTL, false, func() ([]ComputeInstanceType, error) {
		return []ComputeInstanceType{{InstanceType: "m7i.large", VCPU: 2, MemoryMiB: 8192, CurrentGeneration: true}}, nil
	}); err != nil {
		t.Fatalf("warm: %v", err)
	}

	got := ce.resolveComputeCatalog(context.Background(), true) // force past the TTL

	if got.source != sourcePartial {
		t.Errorf("source = %q, want %q — a stale live read beats a region-agnostic table", got.source, sourcePartial)
	}
	if len(got.types) != 1 || got.types[0].InstanceType != "m7i.large" {
		t.Errorf("served %v, want the stale live entry", got.types)
	}
	if got.cachedAt.IsZero() {
		t.Error("stale data was served with no cachedAt; the user cannot judge how old it is")
	}
}

// ---------------------------------------------------------------------------
// Live read
// ---------------------------------------------------------------------------

func TestFetchInstanceTypeCatalog_PagesUntilExhausted(t *testing.T) {
	api := &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{
		{InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("m7i.large", 2, 8192, "x86_64")}, NextToken: aws.String("p2")},
		{InstanceTypes: []ec2types.InstanceTypeInfo{typeRecord("r7i.large", 2, 16384, "x86_64")}},
	}}

	got, err := fetchInstanceTypeCatalog(context.Background(), api)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d types across 2 pages, want 2", len(got))
	}
	if n := atomic.LoadInt32(&api.typesCalls); n != 2 {
		t.Errorf("DescribeInstanceTypes called %d times, want 2", n)
	}
}

func TestReadAWSVPCTrunking(t *testing.T) {
	cases := []struct {
		name      string
		api       ecsSettingsAPI
		wantOn    bool
		wantState string
	}{
		{name: "enabled", api: &fakeECSSettings{value: "enabled"}, wantOn: true, wantState: trunkingEnabled},
		{name: "disabled", api: &fakeECSSettings{value: "disabled"}, wantOn: false, wantState: trunkingDisabled},
		{name: "unreadable", api: &fakeECSSettings{err: errors.New("denied")}, wantOn: false, wantState: trunkingUnknown},
		{name: "absent client", api: nil, wantOn: false, wantState: trunkingUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			on, state := readAWSVPCTrunking(context.Background(), tc.api)
			if on != tc.wantOn || state != tc.wantState {
				t.Errorf("got (%v, %q), want (%v, %q)", on, state, tc.wantOn, tc.wantState)
			}
		})
	}
}
