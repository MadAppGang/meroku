package main

// Black-box tests for the compute HTTP surface and for generate-time
// validation.
//
// The only inputs these tests control are the ones the product does not own:
// what AWS answers, and what the user's YAML says. The only outputs they assert
// are the ones a client can see: the status code, the response body, and the
// text of a refusal. No handler internal is touched.
//
// Every expectation is derived from requirements.md (FR-5, FR-12, FR-52, EC-2,
// EC-5, EC-11, NFR-6), from decisions.md, and from architecture.md section 3's
// response contract. Where a document contradicts another the comment names
// both.
//
// CON-5: every fixture value is synthetic -- invented prices, invented
// instance-type records, no account identifiers.

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/smithy-go"
)

// bbEnv is a small, ordinary environment: a backend and one worker, both
// Fargate-shaped, both well formed.
func bbEnv() Env {
	return Env{
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
}

// bbTypes is the catalog the fake EC2 returns: four small current-generation
// types, nothing exotic.
func bbTypes() []ec2types.InstanceTypeInfo {
	return []ec2types.InstanceTypeInfo{
		typeRecord("m7i.large", 2, 8192, "x86_64"),
		typeRecord("r7i.large", 2, 16384, "x86_64"),
		typeRecord("c7i.large", 2, 4096, "x86_64"),
		typeRecord("m7g.large", 2, 8192, "arm64"),
	}
}

func bbPricePages(t *testing.T, usd string) []*awspricing.GetProductsOutput {
	t.Helper()
	return []*awspricing.GetProductsOutput{{PriceList: []string{
		syntheticSKU(t, "m7i.large", usd, "RSV"),
		syntheticSKU(t, "r7i.large", "0.1400000000", "RSV"),
		syntheticSKU(t, "c7i.large", "0.0900000000", "RSV"),
		syntheticSKU(t, "m7g.large", "0.0800000000", "RSV"),
	}}}
}

func bbSpotPages(spotPrice string) []*ec2.DescribeSpotPriceHistoryOutput {
	now := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	return []*ec2.DescribeSpotPriceHistoryOutput{{SpotPriceHistory: []ec2types.SpotPrice{
		spotRecord("m7i.large", "ap-southeast-2a", spotPrice, now),
		spotRecord("m7i.large", "ap-southeast-2b", spotPrice, now),
		spotRecord("r7i.large", "ap-southeast-2a", "0.0500", now),
	}}}
}

// bbComputeEnv assembles a computeEnv over the fakes, with the AWS answers a
// test wants to control passed in.
func bbComputeEnv(t *testing.T, env Env, onDemandUSD, spotUSD string) *computeEnv {
	t.Helper()
	ce := testComputeEnv(t)
	ce.env = env
	ce.ec2 = &fakeEC2{typePages: []*ec2.DescribeInstanceTypesOutput{{InstanceTypes: bbTypes()}},
		spotPages: bbSpotPages(spotUSD)}
	ce.pricingAPI = &fakePricing{pages: bbPricePages(t, onDemandUSD)}
	ce.cloudwatch = &fakeCloudWatch{}
	return ce
}

// bbNoCredentialsEnv is a machine with no AWS credentials at all.
//
// Both halves matter. The SDK resolves credentials as part of signing, so on
// such a machine the API CALL is what fails, not some separate probe -- a fake
// that answers happily while the credential provider is broken is a machine
// that cannot exist, and a test built on one would assert nothing.
func bbNoCredentialsEnv(t *testing.T) *computeEnv {
	t.Helper()
	ce := bbComputeEnv(t, bbEnv(), "0.1000000000", "0.0400")
	unresolvable := fmt.Errorf("failed to refresh cached credentials, no EC2 IMDS role found")
	ce.creds = &countingCreds{err: unresolvable}
	ce.ec2 = &fakeEC2{typesErr: unresolvable, spotErr: unresolvable}
	ce.pricingAPI = &fakePricing{err: unresolvable}
	return ce
}

// bbDecode is the assertion every compute response has to survive before any
// other assertion is meaningful: the body parses.
//
// NFR-6 is explicit -- "a blank screen, a spinner that never resolves, or a 500
// is a defect". A 200 whose body stops halfway through is all three at once:
// the browser's json() rejects, the tab renders nothing, and the server logged
// success.
func bbDecode(t *testing.T, body []byte, into any, what string) {
	t.Helper()
	if err := json.Unmarshal(body, into); err != nil {
		t.Fatalf("%s: the 200 response body is not valid JSON: %v\nbody (%d bytes): %s",
			what, err, len(body), body)
	}
}

// ---------------------------------------------------------------------------
// The headline scenario: a non-finite number reaching encoding/json
// ---------------------------------------------------------------------------

// TestBlackBox_APrivatePriceStringNeverTruncatesTheBody drives the two priced
// endpoints with an AWS price the code's own guards were written to reject and
// do not: strconv.ParseFloat accepts "Infinity" and "NaN" without error, and
// both survive a `<= 0` test because every comparison against NaN is false and
// +Inf is genuinely greater than zero.
//
// What must happen, per EC-2 and EC-5, is that an unusable price is reported as
// null -- "price unknown", "spot unavailable" -- and never as a value. What must
// not happen, per NFR-6, is a 200 with a body the client cannot parse:
// encoding/json aborts the whole object on the first non-finite float, so the
// user sees an empty Compute tab and the server sees a successful request.
func TestBlackBox_APriceStringNeverTruncatesTheBody(t *testing.T) {
	poison := []string{"Infinity", "NaN", "-Infinity"}

	for _, bad := range poison {
		t.Run("on-demand "+bad, func(t *testing.T) {
			stubResolver(t, func(t *testing.T) *computeEnv {
				return bbComputeEnv(t, bbEnv(), bad, "0.0400")
			})

			rec := doGet(t, getComputeInstanceTypes, "/api/compute/instance-types?env=dev")
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}

			var body computeInstanceTypesResponse
			bbDecode(t, rec.Body.Bytes(), &body, "instance-types with an on-demand price of "+bad)

			for _, it := range body.InstanceTypes {
				if it.OnDemandHourly == nil {
					continue
				}
				if v := *it.OnDemandHourly; math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("%s publishes onDemandHourly = %v; EC-2 requires a type with no "+
						"usable price to carry null and priceSource \"unavailable\"",
						it.InstanceType, v)
				}
			}
		})

		t.Run("spot "+bad, func(t *testing.T) {
			stubResolver(t, func(t *testing.T) *computeEnv {
				return bbComputeEnv(t, bbEnv(), "0.1000000000", bad)
			})

			rec := doGet(t, getComputeRecommendation,
				"/api/compute/recommendation?env=dev&posture=balanced")
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}

			var body computeRecommendationResponse
			bbDecode(t, rec.Body.Bytes(), &body, "recommendation with a spot price of "+bad)

			for _, c := range body.Ranked {
				if c.SpotMedianHourly == nil {
					continue
				}
				if v := *c.SpotMedianHourly; math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("%s publishes spotMedianHourly = %v; EC-5 requires an unavailable "+
						"spot price to be null, and \"never a 0 %% or a nominal saving\"",
						c.InstanceType, v)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// X-7 -- the source vocabulary
// ---------------------------------------------------------------------------

// TestBlackBox_X7_SourceVocabulary.
//
// Contradiction X-7: the feature brief describes `source` as
// live | partial | fallback; AC-1 and architecture 3.1 both say
// aws_api | partial | fallback. The two documents that define the contract
// agree, so "live" must never appear -- a UI switching on it would fall through
// to its default branch on every live response.
func TestBlackBox_X7_SourceVocabulary(t *testing.T) {
	legal := map[string]bool{"aws_api": true, "partial": true, "fallback": true}

	cases := []struct {
		name  string
		build func(t *testing.T) *computeEnv
		want  string
	}{
		{
			name:  "live AWS answers",
			build: func(t *testing.T) *computeEnv { return bbComputeEnv(t, bbEnv(), "0.1000000000", "0.0400") },
			want:  sourceAWSAPI,
		},
		{
			name:  "no credentials at all",
			build: bbNoCredentialsEnv,
			want:  sourceFallback,
		},
		{
			name: "catalog answers, pricing does not",
			build: func(t *testing.T) *computeEnv {
				ce := bbComputeEnv(t, bbEnv(), "0.1000000000", "0.0400")
				ce.pricingAPI = &fakePricing{err: &smithy.GenericAPIError{
					Code: "InternalFailure", Message: "synthetic"}}
				return ce
			},
			want: sourcePartial,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubResolver(t, tc.build)

			rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev")
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200: an AWS failure never produces a non-200 (NFR-6)", rec.Code)
			}
			var body computeRecommendationResponse
			bbDecode(t, rec.Body.Bytes(), &body, tc.name)

			if !legal[body.Source] {
				t.Errorf("source = %q, which is not one of aws_api | partial | fallback (AC-1)",
					body.Source)
			}
			if body.Source == "live" {
				t.Error(`source = "live"; the brief's vocabulary, not the contract's (X-7)`)
			}
			if body.Source != tc.want {
				t.Errorf("source = %q, want %q", body.Source, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// X-10 -- credentialsState has four values, and denied is one of them
// ---------------------------------------------------------------------------

// TestBlackBox_X10_CredentialsStateHasFourDistinctValues.
//
// Contradiction X-10: FR-12 lists three credentialsState values; architecture
// 3.1 lists four, adding "denied", and D-10 records the fourth occurring in
// practice. Four is asserted.
//
// The distinction is not cosmetic. "missing" tells a user to log in, and
// logging in will not fix an SCP or a missing IAM permission; collapsing denied
// into missing sends them round a loop that cannot terminate.
func TestBlackBox_X10_CredentialsStateHasFourDistinctValues(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) *computeEnv
		want  credState
	}{
		{
			name:  "healthy",
			build: func(t *testing.T) *computeEnv { return bbComputeEnv(t, bbEnv(), "0.1000000000", "0.0400") },
			want:  credOK,
		},
		{
			name:  "no credentials configured",
			build: bbNoCredentialsEnv,
			want:  credMissing,
		},
		{
			name: "SSO session expired",
			build: func(t *testing.T) *computeEnv {
				ce := bbComputeEnv(t, bbEnv(), "0.1000000000", "0.0400")
				ce.ec2 = &fakeEC2{typesErr: &smithy.GenericAPIError{
					Code: "ExpiredTokenException", Message: "synthetic"}}
				return ce
			},
			want: credExpired,
		},
		{
			name: "permission denied by policy",
			build: func(t *testing.T) *computeEnv {
				ce := bbComputeEnv(t, bbEnv(), "0.1000000000", "0.0400")
				ce.ec2 = &fakeEC2{typesErr: &smithy.GenericAPIError{
					Code: "UnauthorizedOperation", Message: "synthetic"}}
				return ce
			},
			want: credDenied,
		},
	}

	seen := map[credState]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubResolver(t, tc.build)

			rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev")
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200 (EC-1, NFR-6): %s", rec.Code, rec.Body.String())
			}
			var body computeRecommendationResponse
			bbDecode(t, rec.Body.Bytes(), &body, tc.name)

			if body.CredentialsState != tc.want {
				t.Errorf("credentialsState = %q, want %q", body.CredentialsState, tc.want)
			}
			if prev, dup := seen[body.CredentialsState]; dup {
				t.Errorf("%q reports the same credentialsState as %q; the two are different "+
					"problems with different fixes (X-10)", tc.name, prev)
			}
			seen[body.CredentialsState] = tc.name

			// EC-1: whatever the credential state, the recommendation still
			// answers from the configured shape.
			if body.Primary == nil && !body.Unsatisfiable && body.Basis != "default" {
				t.Errorf("no primary, not unsatisfiable, basis %q: the endpoint answered nothing "+
					"at all", body.Basis)
			}
		})
	}

	if len(seen) != 4 {
		t.Errorf("only %d distinct credentialsState values were reachable: %v. Architecture 3.1 "+
			"defines four, and D-10 records the fourth occurring in practice.", len(seen), seen)
	}
}

// ---------------------------------------------------------------------------
// D-24 -- the two primary: null cases stay distinguishable
// ---------------------------------------------------------------------------

// TestBlackBox_D24_TheTwoPrimaryNullCasesAreDistinguishable.
//
// Architecture 3.3 promises a client can tell "you have not configured anything
// yet" from "nothing in this region can hold what you configured". Conflated,
// an empty new project is told its workload cannot be satisfied -- the worst
// possible first impression, on the most common state there is.
func TestBlackBox_D24_TheTwoPrimaryNullCasesAreDistinguishable(t *testing.T) {
	t.Run("nothing configured is the FR-28 default", func(t *testing.T) {
		empty := bbEnv()
		empty.Workload = Workload{}
		empty.Services = nil
		stubResolver(t, func(t *testing.T) *computeEnv {
			return bbComputeEnv(t, empty, "0.1000000000", "0.0400")
		})

		rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev")
		var body computeRecommendationResponse
		bbDecode(t, rec.Body.Bytes(), &body, "empty environment")

		if body.Primary != nil {
			t.Errorf("primary = %q, want null", body.Primary.InstanceType)
		}
		if body.Basis != "default" {
			t.Errorf("basis = %q, want \"default\" (FR-28)", body.Basis)
		}
		if body.Unsatisfiable {
			t.Error("unsatisfiable = true for an environment with nothing configured; an empty " +
				"project must never be told its workload cannot be satisfied")
		}
		if body.Constraint != nil {
			t.Errorf("constraint = %q, want null: nothing failed", *body.Constraint)
		}
		if len(body.NearestMisses) != 0 {
			t.Errorf("nearestMisses = %+v, want none", body.NearestMisses)
		}
		if len(body.SuggestedPool.InstanceTypes) == 0 {
			t.Error("suggestedPool.instance_types is empty; FR-28 requires a usable default pool")
		}
	})

	t.Run("demand nothing can hold is an explained refusal", func(t *testing.T) {
		huge := bbEnv()
		huge.Services = []Service{{Name: "warehouse", CPU: 4096, Memory: 204800, DesiredCount: 1}}
		stubResolver(t, func(t *testing.T) *computeEnv {
			return bbComputeEnv(t, huge, "0.1000000000", "0.0400")
		})

		rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev")
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200: EC-11 degrades inside a 200", rec.Code)
		}
		var body computeRecommendationResponse
		bbDecode(t, rec.Body.Bytes(), &body, "200 GiB task")

		if body.Primary != nil {
			t.Errorf("primary = %q for a 200 GiB task against a 16 GiB catalog",
				body.Primary.InstanceType)
		}
		if !body.Unsatisfiable {
			t.Error("unsatisfiable = false; EC-11 requires the refusal to be explicit")
		}
		if body.Basis == "default" {
			t.Error("basis = \"default\" together with unsatisfiable: the two primary-null cases " +
				"are no longer distinguishable (architecture 3.3)")
		}
		if body.Constraint == nil || *body.Constraint == "" {
			t.Fatal("constraint is null; EC-11 requires the rule and the value it needed")
		}
		if !strings.Contains(*body.Constraint, "200") {
			t.Errorf("constraint = %q.\n"+
				"EC-11 spells out what this sentence has to say: \"no instance type in {region} "+
				"can hold {service} ({cpu}/{mem})\". The offending reservation is the warehouse "+
				"service's 200 GiB -- a number the user can find in their YAML and change. A "+
				"demand-weighted mean across every in-scope task is a figure that appears in no "+
				"config file and in no signals field, so the user searches for it and finds "+
				"nothing.", *body.Constraint)
		}
		if !strings.Contains(*body.Constraint, "warehouse") {
			t.Logf("constraint does not name the service either: %q (EC-11's UI text names it)",
				*body.Constraint)
		}
		if len(body.NearestMisses) == 0 {
			t.Error("nearestMisses is empty; EC-11 forbids an empty list with no explanation")
		}
		for _, m := range body.NearestMisses {
			if m.InstanceType == "" || m.FailedRule == "" || m.Unit == "" {
				t.Errorf("nearest miss %+v does not quantify the margin (AC-20)", m)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// C-9 -- a service the boundary refused is reported, not silently dropped
// ---------------------------------------------------------------------------

// TestBlackBox_AServiceWithNoCPUReservationIsNamedNotSilentlyDropped.
//
// `cpu: 0` is legal YAML on EC2, where task-level CPU is optional, and it is
// the shortest path to a division by zero in the demand vector. Whatever the
// sizing answer, two things are required: the response still parses (NFR-6),
// and the user is told which service was ignored and why (FR-29) rather than
// being left to notice a missing row.
//
// requirements.md EC-12 asks for memory-only sizing here; architecture.md C-14
// replaces that with "the service is excluded and reported". C-14 is the later
// correction and is what is asserted. Both agree on everything below.
func TestBlackBox_AServiceWithNoCPUReservationIsNamedNotSilentlyDropped(t *testing.T) {
	env := bbEnv()
	env.Services = []Service{
		{Name: "worker", CPU: 256, Memory: 512, DesiredCount: 3},
		{Name: "sidecar", CPU: 0, Memory: 512, DesiredCount: 2},
	}
	stubResolver(t, func(t *testing.T) *computeEnv {
		return bbComputeEnv(t, env, "0.1000000000", "0.0400")
	})

	rec := doGet(t, getComputeRecommendation, "/api/compute/recommendation?env=dev")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	var body computeRecommendationResponse
	bbDecode(t, rec.Body.Bytes(), &body, "a service with cpu: 0")

	if len(body.Signals.Dropped) != 0 {
		t.Errorf("signals.dropped = %+v, want empty: architecture 3.3 records a non-empty "+
			"dropped as a defect, not as a way of reporting bad input", body.Signals.Dropped)
	}

	var sidecar *computeServiceSignal
	for i := range body.Signals.Services {
		if body.Signals.Services[i].Name == "sidecar" {
			sidecar = &body.Signals.Services[i]
		}
	}
	if sidecar == nil {
		t.Fatalf("the sidecar is absent from signals.services entirely: %+v\n"+
			"\"we ignored your worker\" is not something a UI should have to infer from a "+
			"missing row (FR-29)", body.Signals.Services)
	}
	if sidecar.Status != "no_data" {
		t.Errorf("sidecar status = %q, want no_data", sidecar.Status)
	}
	if sidecar.Reason == "" {
		t.Error("the sidecar carries no reason; the user cannot tell which field was wrong")
	}
	if !strings.Contains(sidecar.Reason, "cpu") {
		t.Errorf("reason = %q, which does not name the offending key", sidecar.Reason)
	}
	if !strings.Contains(sidecar.Reason, "sidecar") {
		t.Errorf("reason = %q, which does not name the service", sidecar.Reason)
	}

	// The well-formed services still drove a recommendation.
	if body.Primary == nil {
		t.Errorf("no primary, though two well-formed services remain: unsatisfiable=%v",
			body.Unsatisfiable)
	}
}

// ---------------------------------------------------------------------------
// Feature area C -- every refusal names the key AND the fix
// ---------------------------------------------------------------------------

// TestBlackBox_EveryComputeRefusalNamesTheKeyAndTheFix is FR-52's message
// quality requirement applied as a property over the whole rule set rather than
// rule by rule.
//
// The stated model is validateALBConfigMap, whose message names both keys and
// both ways out because the AWS error it replaces names neither and arrives too
// late to act on. A refusal that says only "invalid configuration" has moved
// the failure earlier without making it any more answerable.
//
// Three things are required of every message:
//  1. the YAML key path the user has to edit,
//  2. the pool or service the rule fired on,
//  3. an imperative fix -- something to do, not just something that is wrong.
func TestBlackBox_EveryComputeRefusalNamesTheKeyAndTheFix(t *testing.T) {
	tests := []struct {
		rule    string
		config  map[string]interface{}
		wantKey string // the YAML key path the user must edit
		wantWho string // the pool or service the rule fired on
	}{
		{
			rule: "1 — compute_pool names a pool that does not exist",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "gpu-pool", nil)},
			),
			wantKey: "compute_pool", wantWho: "api",
		},
		{
			rule: "2 — min_size above max_size",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"min_size": 3, "max_size": 2})},
				nil, nil),
			wantKey: "min_size", wantWho: "general",
		},
		{
			rule: "3 — on_demand capacity with a base",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"capacity_type": "on_demand", "on_demand_base": 1})},
				nil, nil),
			wantKey: "on_demand_base", wantWho: "general",
		},
		{
			rule: "4 — GPU AMI with no GPU instance type",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("gpu", map[string]interface{}{
					"ami_family":     "al2023_gpu",
					"instance_types": []interface{}{"m7i.large"}})},
				nil, nil),
			wantKey: "ami_family", wantWho: "gpu",
		},
		{
			rule: "5 — arm64 AMI with an x86 instance type",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("arm", map[string]interface{}{
					"ami_family":     "al2023_arm64",
					"instance_types": []interface{}{"m7i.large"}})},
				nil, nil),
			wantKey: "instance_types", wantWho: "arm",
		},
		{
			rule: "10 — x86 AMI with an arm64 instance type (the mirror of rule 5)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"ami_family":     "al2023",
					"instance_types": []interface{}{"m7g.large"}})},
				nil, nil),
			wantKey: "instance_types", wantWho: "general",
		},
		{
			rule: "6 — a task larger than the largest instance in the pool",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"instance_types": []interface{}{"m7i.large"}})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"memory": 16384})},
			),
			wantKey: "memory", wantWho: "api",
		},
		{
			rule: "7 — two pools with the same name",
			config: computeConfigFixture(
				[]map[string]interface{}{
					computePoolFixture("general", nil),
					computePoolFixture("general", nil),
				}, nil, nil),
			wantKey: "compute.pools", wantWho: "general",
		},
		{
			rule: "8 — a reference to a disabled pool",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"enabled": false})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", nil)},
			),
			wantKey: "enabled", wantWho: "api",
		},
		{
			rule: "9 — runtime ec2 with no pool named",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", nil)},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "", nil)},
			),
			wantKey: "compute_pool", wantWho: "api",
		},
		{
			rule: "11 — awsvpc with no egress path (D-6 / D-7)",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"network_mode": "awsvpc"})},
				nil, nil),
			wantKey: "network_mode", wantWho: "general",
		},
		{
			rule: "12 — X-Ray under bridge",
			config: computeConfigFixture(
				[]map[string]interface{}{computePoolFixture("general", map[string]interface{}{
					"network_mode": "bridge"})},
				nil,
				[]map[string]interface{}{computeEC2Service("api", "general", map[string]interface{}{
					"xray_enabled": true})},
			),
			wantKey: "xray_enabled", wantWho: "api",
		},
	}

	// An imperative: something the user can do. "Either ... or ..." is the
	// house style (validateALBConfigMap), but any of these is a fix.
	fixVerbs := []string{"Either", "Remove", "Rename", "Set ", "set ", "lower ", "add ", "drop "}

	for _, tc := range tests {
		t.Run(tc.rule, func(t *testing.T) {
			err := validateComputeConfigMap(tc.config)
			if err == nil {
				t.Fatal("generation was allowed to proceed; FR-52 requires a refusal here, and " +
					"the AWS-side failure this replaces arrives after ~100 resources exist")
			}
			msg := err.Error()

			if !strings.Contains(msg, tc.wantKey) {
				t.Errorf("the message does not name the key %q the user has to edit:\n%s",
					tc.wantKey, msg)
			}
			if !strings.Contains(msg, tc.wantWho) {
				t.Errorf("the message does not name the pool or service %q it fired on:\n%s",
					tc.wantWho, msg)
			}
			found := false
			for _, verb := range fixVerbs {
				if strings.Contains(msg, verb) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("the message states a problem but no action to take. FR-52's model names "+
					"both keys and both ways out, because the AWS error it replaces names "+
					"neither:\n%s", msg)
			}
		})
	}
}
