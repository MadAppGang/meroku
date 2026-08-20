package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// closeTo keeps float comparisons off the == operator.
func closeTo(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}

func TestEgressMonthlyCost_PublicIPScalesWithTasksNotTraffic(t *testing.T) {
	// The load-bearing property of public IPv4: no per-GB component at all.
	low := EgressFootprint{Services: 3, Tasks: 6, AZs: 2, TrafficGB: 50}
	high := EgressFootprint{Services: 3, Tasks: 6, AZs: 2, TrafficGB: 50000}

	lowCost := egressMonthlyCost(EgressPublicIP, low)
	highCost := egressMonthlyCost(EgressPublicIP, high)

	closeTo(t, lowCost, 21.90, "public IPv4 at 50 GB")
	closeTo(t, highCost, 21.90, "public IPv4 at 50 TB")
	if lowCost != highCost {
		t.Errorf("public IPv4 cost must not depend on traffic: %.2f vs %.2f", lowCost, highCost)
	}
}

func TestEgressMonthlyCost_PublicIPIsAZInvariant(t *testing.T) {
	// A task has one address wherever it lands, so zone count is irrelevant.
	base := EgressFootprint{Services: 5, Tasks: 10, TrafficGB: 100}
	var prev float64
	for az := 1; az <= 4; az++ {
		f := base
		f.AZs = az
		got := egressMonthlyCost(EgressPublicIP, f)
		if az > 1 && got != prev {
			t.Errorf("public IPv4 changed with AZ count: %d AZ = %.2f, previous = %.2f", az, got, prev)
		}
		prev = got
	}
	closeTo(t, prev, 36.50, "public IPv4 at 10 tasks")
}

func TestEgressMonthlyCost_NATGatewayScalesWithTraffic(t *testing.T) {
	// Mirror of the above: NAT has no per-task component but does bill per GB.
	f := EgressFootprint{Services: 3, Tasks: 6, AZs: 2, TrafficGB: 50}
	// 32.85 hourly + 50*0.045 processing + cross-AZ 2*0.01*50*0.5
	closeTo(t, egressMonthlyCost(EgressNATGateway, f), 35.60, "single NAT at 50 GB")

	manyTasks := f
	manyTasks.Tasks = 600
	if egressMonthlyCost(EgressNATGateway, manyTasks) != egressMonthlyCost(EgressNATGateway, f) {
		t.Error("NAT Gateway cost must not depend on task count")
	}

	heavy := f
	heavy.TrafficGB = 5000
	// 32.85 + 225 + cross-AZ 50
	closeTo(t, egressMonthlyCost(EgressNATGateway, heavy), 307.85, "single NAT at 5 TB")
}

func TestEgressMonthlyCost_HAAvoidsCrossAZ(t *testing.T) {
	// One NAT per AZ costs double the hourly rate but no cross-AZ transfer, so
	// past a traffic level HA is actually the cheaper NAT topology.
	f := EgressFootprint{Services: 3, Tasks: 6, AZs: 2, TrafficGB: 50000}

	single := egressMonthlyCost(EgressNATGateway, f)
	ha := egressMonthlyCost(EgressNATGatewayHA, f)

	if ha >= single {
		t.Errorf("at 50 TB, HA NAT (%.2f) should undercut single NAT (%.2f) by avoiding cross-AZ", ha, single)
	}
	closeTo(t, ha, 2315.70, "HA NAT at 50 TB")
	closeTo(t, single, 2782.85, "single NAT at 50 TB")
}

func TestCountEgressFootprint_SkipsDisabledServices(t *testing.T) {
	env := &Env{
		Services: []Service{
			{Name: "api", DesiredCount: 2},
			{Name: "worker", DesiredCount: 3, Enabled: boolPtr(false)}, // not deployed
			{Name: "cron", Enabled: boolPtr(true)},                     // no count -> 1
		},
	}
	env.Workload.BackendDesiredCount = 2

	f := countEgressFootprint(env)

	// backend + api + cron = 3 services; 2 + 2 + 1 = 5 tasks
	if f.Services != 3 {
		t.Errorf("Services = %d, want 3 (disabled service must not count)", f.Services)
	}
	if f.Tasks != 5 {
		t.Errorf("Tasks = %d, want 5", f.Tasks)
	}
}

func TestCountEgressFootprint_BackendDefaultsToOneTask(t *testing.T) {
	f := countEgressFootprint(&Env{}) // no desired count set
	if f.Services != 1 {
		t.Errorf("Services = %d, want 1", f.Services)
	}
	if f.Tasks != 1 {
		t.Errorf("Tasks = %d, want 1 when backend_desired_count is unset", f.Tasks)
	}
}

func TestCountEgressFootprint_CountsPgAdmin(t *testing.T) {
	env := &Env{}
	env.Workload.InstallPgAdmin = true

	f := countEgressFootprint(env)
	if f.Services != 2 || f.Tasks != 2 {
		t.Errorf("pgAdmin should add a service and a task: got %d services, %d tasks", f.Services, f.Tasks)
	}
}

func TestCountEgressFootprint_NilEnv(t *testing.T) {
	f := countEgressFootprint(nil)
	if f.Services != 0 || f.Tasks != 0 {
		t.Errorf("nil env should report an empty footprint, got %+v", f)
	}
	if f.AZs != 2 {
		t.Errorf("AZs = %d, want 2", f.AZs)
	}
}

func TestAdviseEgress_BelowThresholdKeepsPublicIP(t *testing.T) {
	env := &Env{Services: []Service{{Name: "api"}, {Name: "worker"}}}
	env.Workload.BackendDesiredCount = 1

	advice := AdviseEgress(env) // 3 services, non-prod

	if advice.ShouldSwitch {
		t.Error("3 services is below the non-prod threshold; should not recommend switching")
	}
	if advice.Recommended != EgressPublicIP {
		t.Errorf("Recommended = %q, want %q", advice.Recommended, EgressPublicIP)
	}
	if advice.ServicesUntilSwitch != 2 {
		t.Errorf("ServicesUntilSwitch = %d, want 2", advice.ServicesUntilSwitch)
	}
	if advice.MonthlySaving != 0 {
		t.Errorf("MonthlySaving = %.2f, want 0 below the threshold", advice.MonthlySaving)
	}
}

func TestAdviseEgress_NonProdSwitchesAtFive(t *testing.T) {
	env := &Env{Services: []Service{
		{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"},
	}}
	env.Workload.BackendDesiredCount = 1

	advice := AdviseEgress(env) // backend + 4 = 5 services

	if !advice.ShouldSwitch {
		t.Fatal("5 services should trip the non-prod threshold")
	}
	if advice.Recommended != EgressNATGateway {
		t.Errorf("Recommended = %q, want %q for non-prod", advice.Recommended, EgressNATGateway)
	}
	if advice.Threshold != switchThresholdNonProd {
		t.Errorf("Threshold = %d, want %d", advice.Threshold, switchThresholdNonProd)
	}
}

func TestAdviseEgress_ProdHoldsUntilTenAndWantsHA(t *testing.T) {
	var services []Service
	for i := 0; i < 4; i++ {
		services = append(services, Service{Name: string(rune('a' + i))})
	}
	env := &Env{IsProd: true, Services: services}
	env.Workload.BackendDesiredCount = 1

	// 5 services in prod: still below the prod threshold of 10.
	advice := AdviseEgress(env)
	if advice.ShouldSwitch {
		t.Error("5 services in prod is below the prod threshold; should not recommend switching")
	}
	if advice.Threshold != switchThresholdProd {
		t.Errorf("Threshold = %d, want %d for prod", advice.Threshold, switchThresholdProd)
	}

	// Grow to 10 services.
	for i := 4; i < 9; i++ {
		env.Services = append(env.Services, Service{Name: string(rune('a' + i))})
	}
	advice = AdviseEgress(env)
	if !advice.ShouldSwitch {
		t.Fatal("10 services in prod should trip the prod threshold")
	}
	if advice.Recommended != EgressNATGatewayHA {
		t.Errorf("Recommended = %q, want %q for prod", advice.Recommended, EgressNATGatewayHA)
	}
}

func TestAdviseEgress_SavingIsPositiveWhenSwitching(t *testing.T) {
	// Twenty services, two tasks each: the case the model was built for.
	var services []Service
	for i := 0; i < 19; i++ {
		services = append(services, Service{Name: string(rune('a' + i%26)), DesiredCount: 2})
	}
	env := &Env{Services: services}
	env.Workload.BackendDesiredCount = 2

	advice := AdviseEgress(env)

	if !advice.ShouldSwitch {
		t.Fatal("20 services should recommend switching")
	}
	if advice.Footprint.Tasks != 40 {
		t.Errorf("Tasks = %d, want 40", advice.Footprint.Tasks)
	}
	closeTo(t, advice.CurrentCost, 146.00, "public IPv4 at 40 tasks")
	if advice.MonthlySaving <= 0 {
		t.Errorf("MonthlySaving = %.2f, want positive", advice.MonthlySaving)
	}
	// Assert on the figure the operator acts on, not on a particular adjective.
	wantSaving := fmt.Sprintf("$%.2f/mo", advice.MonthlySaving)
	if !strings.Contains(advice.Summary, wantSaving) {
		t.Errorf("Summary should quote the saving %s, got %q", wantSaving, advice.Summary)
	}
}

func TestAdviseEgress_NeverErrorsOnNil(t *testing.T) {
	// The advisory must be safe to call anywhere, including before a config
	// is loaded. It is advice, never a failure path.
	advice := AdviseEgress(nil)
	if advice.ShouldSwitch {
		t.Error("nil env should not recommend a switch")
	}
	if advice.Summary == "" {
		t.Error("advice should always carry a summary")
	}
}
