package main

// The boundary tests: FR-14 scoping, the C-9 guard that stops malformed demand
// before the pure core, and DEV-12's rule that one failing CloudWatch read must
// not discard the readings that succeeded.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"madappgang.com/meroku/recommend"
)

func boolPtr(v bool) *bool { return &v }

// utilizationOutput builds n hourly datapoints at a fixed average and maximum.
func utilizationOutput(n int, avg, max float64) *cloudwatch.GetMetricStatisticsOutput {
	out := &cloudwatch.GetMetricStatisticsOutput{}
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out.Datapoints = append(out.Datapoints, cloudwatchtypes.Datapoint{
			Timestamp: aws.Time(base.Add(time.Duration(i) * time.Hour)),
			Average:   aws.Float64(avg),
			Maximum:   aws.Float64(max),
		})
	}
	return out
}

func signalEnv() Env {
	return Env{
		Project: "fixture",
		Env:     "dev",
		Region:  "ap-southeast-2",
		Workload: Workload{
			BackendCPU:          "512",
			BackendMemory:       "1024",
			BackendDesiredCount: 2,
			BackendComputePool:  "general",
		},
		Services: []Service{
			{Name: "worker", CPU: 256, Memory: 512, DesiredCount: 3, Runtime: "ec2", ComputePool: "general"},
			{Name: "reports", CPU: 1024, Memory: 2048, DesiredCount: 1},
			{Name: "retired", CPU: 256, Memory: 512, DesiredCount: 1, Enabled: boolPtr(false)},
		},
	}
}

// ---------------------------------------------------------------------------
// FR-14 -- scope
// ---------------------------------------------------------------------------

func TestScopeServices(t *testing.T) {
	env := signalEnv()

	cases := []struct {
		name string
		req  scopeRequest
		want []string
	}{
		{
			name: "no selection takes the backend and every enabled service",
			req:  scopeRequest{},
			want: []string{"backend", "worker", "reports"},
		},
		{
			name: "a pool takes only its ec2 members, backend included by backend_compute_pool",
			req:  scopeRequest{pool: "general"},
			want: []string{"backend", "worker"},
		},
		{
			name: "an explicit subset takes exactly that subset",
			req:  scopeRequest{services: []string{"reports"}},
			want: []string{"reports"},
		},
		{
			name: "an explicit subset may name the backend",
			req:  scopeRequest{services: []string{"backend", "worker"}},
			want: []string{"backend", "worker"},
		},
		{
			name: "a pool nothing references scopes to nothing",
			req:  scopeRequest{pool: "batch"},
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scopeServices(env, tc.req)
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.name)
			}
			if strings.Join(names, ",") != strings.Join(tc.want, ",") {
				t.Errorf("scope = %v, want %v", names, tc.want)
			}
		})
	}

	t.Run("a disabled service is never in the default scope", func(t *testing.T) {
		for _, s := range scopeServices(env, scopeRequest{}) {
			if s.name == "retired" {
				t.Error("a service with enabled: false was sized for")
			}
		}
	})
}

func TestScopeServices_Normalisation(t *testing.T) {
	env := signalEnv()
	got := scopeServices(env, scopeRequest{})

	byName := map[string]scopedService{}
	for _, s := range got {
		byName[s.name] = s
	}

	backend := byName["backend"]
	if backend.vcpu != 0.5 || backend.memGiB != 1.0 {
		t.Errorf("backend = %.3f vCPU / %.3f GiB, want 0.5 / 1.0 (cpu and memory are STRINGS in "+
			"workload, divided by 1024)", backend.vcpu, backend.memGiB)
	}
	if backend.count != 2 {
		t.Errorf("backend count = %d, want the desired count 2", backend.count)
	}

	worker := byName["worker"]
	if worker.vcpu != 0.25 || worker.memGiB != 0.5 {
		t.Errorf("worker = %.3f vCPU / %.3f GiB, want 0.25 / 0.5", worker.vcpu, worker.memGiB)
	}

	t.Run("autoscaling uses the median of the range", func(t *testing.T) {
		e := signalEnv()
		e.Workload.BackendAutoscalingEnabled = true
		e.Workload.BackendAutoscalingMinCapacity = 2
		e.Workload.BackendAutoscalingMaxCapacity = 8
		s := scopeServices(e, scopeRequest{services: []string{"backend"}})
		if len(s) != 1 || s[0].count != 5 {
			t.Errorf("autoscaled backend count = %v, want 5 (the median of 2..8, matching "+
				"api_pricing.go's instanceCount convention)", s)
		}
	})
}

// ---------------------------------------------------------------------------
// C-9 -- malformed demand stops at the boundary
// ---------------------------------------------------------------------------

func TestSignals_DropsZeroCPUService(t *testing.T) {
	env := signalEnv()
	env.Services = append(env.Services, Service{Name: "ghost", CPU: 0, Memory: 512, DesiredCount: 1})
	env.Services = append(env.Services, Service{Name: "phantom", CPU: 256, Memory: 0, DesiredCount: 1})

	scoped := scopeServices(env, scopeRequest{})
	signals := buildDemandSignals(scoped, nil, cwNoData)

	for _, s := range signals.services {
		if s.Name == "ghost" || s.Name == "phantom" {
			t.Errorf("%s reached recommend.Input; a zero divisor inside the maths becomes an "+
				"int(+/-Inf) whose value differs between amd64 and arm64", s.Name)
		}
	}

	droppedNames := map[string]string{}
	for _, d := range signals.dropped {
		droppedNames[d.Name] = d.Status
	}
	for _, want := range []string{"ghost", "phantom"} {
		status, ok := droppedNames[want]
		if !ok {
			t.Errorf("%s was dropped silently; the user cannot tell it was ignored", want)
			continue
		}
		if status != recommend.StatusNoData {
			t.Errorf("%s status = %q, want %q", want, status, recommend.StatusNoData)
		}
	}

	reasons := dropReasons(scoped)
	if !strings.Contains(reasons["ghost"], "cpu") {
		t.Errorf("ghost's reason %q does not name the field that was wrong", reasons["ghost"])
	}
	if !strings.Contains(reasons["phantom"], "memory") {
		t.Errorf("phantom's reason %q does not name the field that was wrong", reasons["phantom"])
	}
}

func TestSignals_DropsUnparseableBackendResources(t *testing.T) {
	env := signalEnv()
	env.Workload.BackendCPU = ""
	scoped := scopeServices(env, scopeRequest{services: []string{"backend"}})
	if len(scoped) != 1 {
		t.Fatalf("scoped %d services, want 1", len(scoped))
	}
	if scoped[0].dropReason == "" {
		t.Fatal("a backend with no CPU string passed the guard")
	}
	if !strings.Contains(scoped[0].dropReason, "backend_cpu") {
		t.Errorf("reason %q does not name workload.backend_cpu", scoped[0].dropReason)
	}
}

// ---------------------------------------------------------------------------
// DEV-12 -- one failure never discards the rest
// ---------------------------------------------------------------------------

func TestSignals_OneServiceErrorKeepsTheRest(t *testing.T) {
	env := signalEnv()
	scoped := scopeServices(env, scopeRequest{})

	// getFullServiceName maps "backend" to project_service_env and everything
	// else to project_service_name_env; the fake keys on the full names for
	// exactly that reason.
	api := &fakeCloudWatch{
		byService: map[string]*cloudwatch.GetMetricStatisticsOutput{
			"fixture_service_dev":         utilizationOutput(336, 12.4, 31.0),
			"fixture_service_reports_dev": utilizationOutput(48, 61.2, 78.0),
		},
		errFor: map[string]error{
			"fixture_service_worker_dev": errors.New("synthetic CloudWatch failure"),
		},
	}

	metrics, state := fetchServiceActuals(context.Background(), api, "fixture", "dev", scoped, 336)

	if state != cwPartial {
		t.Errorf("cloudwatch = %q, want %q", state, cwPartial)
	}

	byName := map[string]serviceMetrics{}
	for _, m := range metrics {
		byName[m.name] = m
	}

	if byName["backend"].datapoints != 336 {
		t.Errorf("backend datapoints = %d, want 336 — a sibling's failure must not cancel it",
			byName["backend"].datapoints)
	}
	if byName["reports"].datapoints != 48 {
		t.Errorf("reports datapoints = %d, want 48", byName["reports"].datapoints)
	}
	if byName["worker"].status != metricStatusError {
		t.Errorf("worker status = %q, want %q", byName["worker"].status, metricStatusError)
	}
	if byName["worker"].datapoints != 0 {
		t.Errorf("a failed read reported %d datapoints, want 0", byName["worker"].datapoints)
	}

	t.Run("a failed read is no data, never zero utilisation", func(t *testing.T) {
		signals := buildDemandSignals(scoped, metrics, state)
		for _, s := range signals.services {
			if s.Name != "worker" {
				continue
			}
			if s.Datapoints != 0 {
				t.Errorf("worker carried %d datapoints into the core", s.Datapoints)
			}
			if s.CPUPeak != 0 || s.MemPeak != 0 {
				t.Error("a failed read produced utilisation figures")
			}
		}
		// And the core must read that as uncovered rather than as idle.
		if cov := recommend.Coverage(signals.services); cov >= 1.0 {
			t.Errorf("coverage = %v with one service unmeasured, want < 1", cov)
		}
	})
}

func TestSignals_AbsentMetricsAreNoDataNotZero(t *testing.T) {
	env := signalEnv()
	scoped := scopeServices(env, scopeRequest{services: []string{"worker"}})

	// CloudWatch answers, with nothing in it: a real, dormant-but-deployed
	// service. Reading this as "0% utilisation" would recommend the smallest
	// instance in the catalog.
	api := &fakeCloudWatch{byService: map[string]*cloudwatch.GetMetricStatisticsOutput{}}
	metrics, state := fetchServiceActuals(context.Background(), api, "fixture", "dev", scoped, 336)

	if state != cwNoData {
		t.Errorf("cloudwatch = %q, want %q", state, cwNoData)
	}
	signals := buildDemandSignals(scoped, metrics, state)
	if len(signals.services) != 1 {
		t.Fatalf("built %d demands, want 1", len(signals.services))
	}
	s := signals.services[0]
	if s.Datapoints != 0 {
		t.Errorf("datapoints = %d, want 0", s.Datapoints)
	}
	if s.VCPU != 0.25 || s.MemGiB != 0.5 {
		t.Errorf("the configured shape was lost: %.3f vCPU / %.3f GiB", s.VCPU, s.MemGiB)
	}
	if recommend.Coverage(signals.services) != 0 {
		t.Error("an unmeasured service counted toward coverage")
	}
}

// ---------------------------------------------------------------------------
// FR-16 -- the reduction
// ---------------------------------------------------------------------------

func TestReduceUtilization_PeakIsTheTopDecile(t *testing.T) {
	// Twenty points: nineteen quiet, one spike. The top decile is two points,
	// so the spike is averaged with the highest quiet point rather than
	// defining the fleet's shape on its own.
	out := &cloudwatch.GetMetricStatisticsOutput{}
	for i := 0; i < 19; i++ {
		out.Datapoints = append(out.Datapoints, cloudwatchtypes.Datapoint{
			Average: aws.Float64(10), Maximum: aws.Float64(20),
		})
	}
	out.Datapoints = append(out.Datapoints, cloudwatchtypes.Datapoint{
		Average: aws.Float64(10), Maximum: aws.Float64(100),
	})

	got := reduceUtilization(out.Datapoints)
	if got.count != 20 {
		t.Errorf("count = %d, want 20", got.count)
	}
	if got.avg != 10 {
		t.Errorf("avg = %v, want 10", got.avg)
	}
	if got.peak != 60 {
		t.Errorf("peak = %v, want 60 (the mean of the top decile: (100+20)/2). A single "+
			"deploy spike must not define a fortnight's shape", got.peak)
	}

	t.Run("no datapoints is no data", func(t *testing.T) {
		empty := reduceUtilization(nil)
		if empty.count != 0 || empty.avg != 0 || empty.peak != 0 {
			t.Errorf("empty series reduced to %+v", empty)
		}
	})
}

func TestNormalizeWindowHours(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", defaultMetricWindowHours},
		{"168", 168},
		{"0", defaultMetricWindowHours},
		{"-5", defaultMetricWindowHours},
		{"nonsense", defaultMetricWindowHours},
		{"99999", maxMetricWindowHours},
	}
	for _, tc := range cases {
		if got := normalizeWindowHours(tc.in); got != tc.want {
			t.Errorf("normalizeWindowHours(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSummariseCloudWatchStatus(t *testing.T) {
	cases := []struct {
		name    string
		in      []serviceMetrics
		expired bool
		want    string
	}{
		{name: "all read", in: []serviceMetrics{{status: metricStatusOK}, {status: metricStatusOK}}, want: cwOK},
		{name: "one failed", in: []serviceMetrics{{status: metricStatusOK}, {status: metricStatusError}}, want: cwPartial},
		{name: "all failed", in: []serviceMetrics{{status: metricStatusError}}, want: cwUnavailable},
		{name: "nothing recorded", in: []serviceMetrics{{status: metricStatusNoData}}, want: cwNoData},
		{name: "budget expiry outranks everything", in: []serviceMetrics{{status: metricStatusOK}}, expired: true, want: cwTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summariseCloudWatchStatus(tc.in, tc.expired); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
