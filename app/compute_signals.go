package main

// Demand signals: YAML -> configured shape (FR-14), CloudWatch -> measured
// shape (FR-15, FR-16), and the assembly of recommend.Input.
//
// This file is the boundary. Everything downstream of it is pure: the scoring
// core takes plain structs, reads no clock and calls no API. Everything the core
// is protected from is stopped here -- most importantly a service whose cpu or
// memory is 0, which would reach a division inside the maths and produce an
// int(+Inf) whose value differs between amd64 and arm64 (C-9).
//
// Absent metrics mean NO DATA, never zero utilisation. Reading absence as zero
// would size a dormant-but-real service down to the smallest instance in the
// catalog, which is the failure this whole signal exists to avoid.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"madappgang.com/meroku/recommend"
)

// ---------------------------------------------------------------------------
// Budgets (NFR-3)
// ---------------------------------------------------------------------------

const (
	// cloudwatchFanOutLimit bounds concurrent GetMetricStatistics calls.
	cloudwatchFanOutLimit = 8
	// cloudwatchBudget bounds the whole fan-out. On expiry the endpoint keeps
	// whatever arrived; configured-only is what happens when NOTHING did.
	cloudwatchBudget = 6 * time.Second
	// cloudwatchPeriod is FR-15's hourly period.
	cloudwatchPeriod = 3600
	// defaultMetricWindowHours is FR-13's default lookback: 14 days.
	defaultMetricWindowHours = 336
	// maxMetricWindowHours keeps a hand-typed window from asking CloudWatch for
	// a year of hourly points (and from blowing the 6s budget on paging).
	maxMetricWindowHours = 1440 // 60 days
)

// CloudWatch statuses for the recommendation envelope's signals.cloudwatch.
const (
	cwOK          = "ok"
	cwPartial     = "partial"
	cwNoData      = "no_data"
	cwUnavailable = "unavailable"
	cwTimeout     = "timeout"
)

// ---------------------------------------------------------------------------
// FR-14 -- scope and configured shape
// ---------------------------------------------------------------------------

// scopedService is one in-scope workload after normalisation: vCPU not CPU
// units, GiB not MiB, and a count.
//
// dropReason is non-empty when the boundary guard rejected it. A dropped
// service still appears in signals.services with status "no_data" and a reason
// naming the field, because "we ignored your worker" is information the user
// needs; it simply never reaches the demand vector.
type scopedService struct {
	name       string // logical name: "backend", or the service's YAML name
	vcpu       float64
	memGiB     float64
	count      int
	dropReason string
}

// scopeRequest is the FR-13 selection: a pool, an explicit subset, or neither.
type scopeRequest struct {
	pool     string
	services []string
}

// scopeServices implements FR-14's scope rules, in FR-13's precedence order:
// `pool` wins, then `services`, then "everything enabled".
func scopeServices(env Env, req scopeRequest) []scopedService {
	var out []scopedService

	backendIncluded := false
	switch {
	case req.pool != "":
		backendIncluded = env.Workload.BackendComputePool == req.pool
	case len(req.services) > 0:
		backendIncluded = containsString(req.services, "backend")
	default:
		backendIncluded = true
	}
	if backendIncluded {
		out = append(out, backendDemand(env.Workload))
	}

	for _, svc := range env.Services {
		include := false
		switch {
		case req.pool != "":
			include = svc.Runtime == "ec2" && svc.ComputePool == req.pool
		case len(req.services) > 0:
			include = containsString(req.services, svc.Name)
		default:
			include = svc.Enabled == nil || *svc.Enabled
		}
		if !include {
			continue
		}
		out = append(out, serviceDemand(svc))
	}

	return out
}

// backendDemand normalises the backend. Its cpu/memory are STRINGS in YAML
// (workload.backend_cpu / backend_memory) and are parsed as api_pricing.go:270-279
// parses them; services carry ints.
//
// Its count follows the instanceCount convention already used at
// api_pricing.go:113-128: the median of the autoscaling range when autoscaling
// is on, the desired count otherwise, and 1 when neither is set.
func backendDemand(w Workload) scopedService {
	s := scopedService{name: "backend", count: 1}

	cpu := parseIntOrZero(w.BackendCPU)
	mem := parseIntOrZero(w.BackendMemory)

	switch {
	case cpu <= 0:
		s.dropReason = "workload.backend_cpu is not a positive number"
	case mem <= 0:
		s.dropReason = "workload.backend_memory is not a positive number"
	default:
		s.vcpu = float64(cpu) / 1024.0
		s.memGiB = float64(mem) / 1024.0
	}

	switch {
	case w.BackendAutoscalingEnabled:
		minCap, maxCap := w.BackendAutoscalingMinCapacity, w.BackendAutoscalingMaxCapacity
		if minCap == 0 {
			minCap = 1
		}
		if maxCap == 0 {
			maxCap = 10
		}
		s.count = int((minCap + maxCap) / 2)
	case w.BackendDesiredCount > 0:
		s.count = int(w.BackendDesiredCount)
	}
	if s.count < 1 {
		s.count = 1
	}
	return s
}

// serviceDemand normalises one named service.
func serviceDemand(svc Service) scopedService {
	s := scopedService{name: svc.Name, count: svc.DesiredCount}
	switch {
	case svc.CPU <= 0:
		s.dropReason = fmt.Sprintf("services[%s].cpu is %d; a task with no CPU reservation cannot be sized", svc.Name, svc.CPU)
	case svc.Memory <= 0:
		s.dropReason = fmt.Sprintf("services[%s].memory is %d; a task with no memory reservation cannot be sized", svc.Name, svc.Memory)
	default:
		s.vcpu = float64(svc.CPU) / 1024.0
		s.memGiB = float64(svc.Memory) / 1024.0
	}
	if s.count < 1 {
		s.count = 1
	}
	return s
}

func parseIntOrZero(v string) int {
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// FR-15 / FR-16 -- CloudWatch actuals
// ---------------------------------------------------------------------------

// serviceMetrics is one service's reduced CloudWatch answer.
//
// status distinguishes three things a single empty chart cannot: the metric was
// read and has history (ok), the metric was read and CloudWatch has no points
// for it (no_data), and the read itself failed (error). Only the first
// contributes to coverage.
type serviceMetrics struct {
	name       string
	cpuAvg     float64
	cpuPeak    float64
	memAvg     float64
	memPeak    float64
	datapoints int
	status     string
	err        error
}

const (
	metricStatusOK     = "ok"
	metricStatusNoData = "no_data"
	metricStatusError  = "error"
)

// fetchServiceActuals reads CPUUtilization and MemoryUtilization for every
// in-scope service, bounded at cloudwatchFanOutLimit in flight and by
// cloudwatchBudget overall.
//
// Two design points, and the first is a correctness point rather than a style
// one:
//
//   - the per-service worker NEVER propagates an error. errgroup.WithContext
//     cancels every sibling on the first non-nil error, which would throw away
//     data that had already arrived -- silently lowering coverage, which lowers
//     w_act, which can change the classification. So the pool is a pool, and a
//     failure is recorded on the failing service's own result.
//   - context cancellation is reserved for the budget and for nothing else.
//
// The namespace is AWS/ECS and the metrics are the free ones. Container Insights
// is NOT required: aws-verified-contracts.md section 5 shows both Average and
// Maximum populating on a cluster this tool generated, whose aws_ecs_cluster has
// no Container Insights setting block.
func fetchServiceActuals(ctx context.Context, api cloudwatchComputeAPI, project, envName string, services []scopedService, windowHours int) ([]serviceMetrics, string) {
	if api == nil {
		return nil, cwUnavailable
	}
	live := make([]scopedService, 0, len(services))
	for _, s := range services {
		if s.dropReason == "" {
			live = append(live, s)
		}
	}
	if len(live) == 0 {
		return nil, cwNoData
	}

	budgetCtx, cancel := context.WithTimeout(ctx, cloudwatchBudget)
	defer cancel()

	clusterName := fmt.Sprintf("%s_cluster_%s", project, envName)
	end := time.Now()
	start := end.Add(-time.Duration(windowHours) * time.Hour)

	results := make([]serviceMetrics, len(live))
	sem := make(chan struct{}, cloudwatchFanOutLimit)
	var wg sync.WaitGroup

	for i, svc := range live {
		wg.Add(1)
		go func(i int, svc scopedService) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-budgetCtx.Done():
				results[i] = serviceMetrics{name: svc.name, status: metricStatusNoData}
				return
			}
			results[i] = readServiceMetrics(budgetCtx, api,
				getFullServiceName(project, svc.name, envName), clusterName, start, end)
			results[i].name = svc.name
		}(i, svc)
	}
	wg.Wait()

	return results, summariseCloudWatchStatus(results, budgetCtx.Err() != nil)
}

// readServiceMetrics issues the two GetMetricStatistics calls for one service.
// It returns a result, never an error: its failures live on the result.
func readServiceMetrics(ctx context.Context, api cloudwatchComputeAPI, fullServiceName, clusterName string, start, end time.Time) serviceMetrics {
	out := serviceMetrics{status: metricStatusNoData}

	cpu, cpuErr := getECSUtilization(ctx, api, "CPUUtilization", fullServiceName, clusterName, start, end)
	mem, memErr := getECSUtilization(ctx, api, "MemoryUtilization", fullServiceName, clusterName, start, end)
	if cpuErr != nil || memErr != nil {
		out.status = metricStatusError
		if cpuErr != nil {
			out.err = cpuErr
		} else {
			out.err = memErr
		}
		return out
	}

	// datapoints is the smaller of the two series: a service with CPU history
	// and no memory history cannot produce a shape, and claiming the larger
	// count would overstate coverage.
	out.datapoints = min(cpu.count, mem.count)
	if out.datapoints == 0 {
		return out
	}
	out.cpuAvg, out.cpuPeak = cpu.avg, cpu.peak
	out.memAvg, out.memPeak = mem.avg, mem.peak
	out.status = metricStatusOK
	return out
}

// utilizationSeries is one reduced metric series.
type utilizationSeries struct {
	avg   float64
	peak  float64
	count int
}

// getECSUtilization is the same call shape as api_autoscaling.go:207-240, with
// FR-15's three required differences: both statistics, an hourly period, and a
// window that reaches back rather than five minutes.
func getECSUtilization(ctx context.Context, api cloudwatchComputeAPI, metric, fullServiceName, clusterName string, start, end time.Time) (utilizationSeries, error) {
	out, err := api.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/ECS"),
		MetricName: aws.String(metric),
		Dimensions: []cloudwatchtypes.Dimension{
			{Name: aws.String("ServiceName"), Value: aws.String(fullServiceName)},
			{Name: aws.String("ClusterName"), Value: aws.String(clusterName)},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(cloudwatchPeriod),
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage, cloudwatchtypes.StatisticMaximum},
	})
	if err != nil {
		return utilizationSeries{}, classifyComputeError(err, actionGetMetricStatistics)
	}
	return reduceUtilization(out.Datapoints), nil
}

// reduceUtilization is FR-16's reduction: avg is the mean of the Average
// series, peak is the mean of the TOP DECILE of the Maximum series.
//
// The top decile rather than the single maximum, because one 100% spike from a
// deploy or a GC pause would otherwise define the fleet's shape for a fortnight.
func reduceUtilization(points []cloudwatchtypes.Datapoint) utilizationSeries {
	if len(points) == 0 {
		return utilizationSeries{}
	}
	avgs := make([]float64, 0, len(points))
	maxes := make([]float64, 0, len(points))
	for _, p := range points {
		if p.Average != nil {
			avgs = append(avgs, *p.Average)
		}
		if p.Maximum != nil {
			maxes = append(maxes, *p.Maximum)
		}
	}
	if len(avgs) == 0 || len(maxes) == 0 {
		return utilizationSeries{}
	}

	var sum float64
	for _, v := range avgs {
		sum += v
	}

	sorted := append([]float64(nil), maxes...)
	sort.Float64s(sorted)
	decile := len(sorted) / 10
	if decile < 1 {
		decile = 1
	}
	var peakSum float64
	for _, v := range sorted[len(sorted)-decile:] {
		peakSum += v
	}

	return utilizationSeries{
		avg:   sum / float64(len(avgs)),
		peak:  peakSum / float64(decile),
		count: min(len(avgs), len(maxes)),
	}
}

// summariseCloudWatchStatus collapses the per-service results into the one
// envelope-level word. A budget expiry outranks everything: "timeout" tells the
// user the answer is thin because we ran out of time, not because their
// services are idle.
func summariseCloudWatchStatus(results []serviceMetrics, budgetExpired bool) string {
	if budgetExpired {
		return cwTimeout
	}
	var ok, failed int
	for _, r := range results {
		switch r.status {
		case metricStatusOK:
			ok++
		case metricStatusError:
			failed++
		}
	}
	switch {
	case ok == len(results) && failed == 0:
		return cwOK
	case ok > 0:
		return cwPartial
	case failed > 0:
		return cwUnavailable
	default:
		return cwNoData
	}
}

// ---------------------------------------------------------------------------
// Assembly
// ---------------------------------------------------------------------------

// demandSignals is everything the handler needs to build both recommend.Input
// and the parts of the envelope the core does not fill.
type demandSignals struct {
	services        []recommend.ServiceDemand
	dropped         []recommend.ServiceSignal
	cloudwatchState string
}

// buildDemandSignals joins the configured shape to the measured one.
//
// The join is by logical service name, and a service the metrics fan-out could
// not answer for keeps datapoints 0 -- which recommend.Coverage reads as "not
// covered" and never as "measured at zero".
func buildDemandSignals(scoped []scopedService, metrics []serviceMetrics, cwState string) demandSignals {
	byName := make(map[string]serviceMetrics, len(metrics))
	for _, m := range metrics {
		byName[m.name] = m
	}

	out := demandSignals{cloudwatchState: cwState}
	for _, s := range scoped {
		if s.dropReason != "" {
			// C-9: malformed demand stops HERE. It never reaches the core, and
			// the user is told which field was wrong.
			out.dropped = append(out.dropped, recommend.ServiceSignal{
				Name:   s.name,
				Status: recommend.StatusNoData,
			})
			continue
		}
		d := recommend.ServiceDemand{
			Name:   s.name,
			VCPU:   s.vcpu,
			MemGiB: s.memGiB,
			Count:  s.count,
		}
		if m, ok := byName[s.name]; ok && m.status == metricStatusOK {
			d.CPUAvg, d.CPUPeak = m.cpuAvg, m.cpuPeak
			d.MemAvg, d.MemPeak = m.memAvg, m.memPeak
			d.Datapoints = m.datapoints
		}
		out.services = append(out.services, d)
	}
	return out
}

// dropReasons maps a service name to the sentence explaining why it was
// dropped, for the envelope's per-service reason field.
func dropReasons(scoped []scopedService) map[string]string {
	out := make(map[string]string)
	for _, s := range scoped {
		if s.dropReason != "" {
			out[s.name] = s.dropReason
		}
	}
	return out
}

// normalizeWindowHours clamps FR-13's `window` into something CloudWatch will
// answer for at an hourly period.
func normalizeWindowHours(raw string) int {
	if raw == "" {
		return defaultMetricWindowHours
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultMetricWindowHours
	}
	if n > maxMetricWindowHours {
		return maxMetricWindowHours
	}
	return n
}
