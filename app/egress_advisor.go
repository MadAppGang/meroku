package main

import "fmt"

// Egress cost advisory.
//
// Every ECS task in awsvpc mode needs outbound access to pull its image, ship
// logs and call third-party APIs. meroku gives each task a public IPv4 address,
// which is the cheapest option for a small project and the most expensive one
// for a large project: a public IP costs per task and nothing per GB, while a
// NAT Gateway costs a flat hourly rate and nothing per task. The two cross.
//
// This file works out where a given environment sits relative to that crossing
// and returns a recommendation. It is advisory only — nothing here changes
// generated Terraform, and no caller should treat its output as an error.
//
// The reasoning, the growth curves and the source pricing are in
// ai_docs/EGRESS_COST_MODEL.md. Keep the thresholds below in step with it.

// EgressStrategy names a way of giving tasks outbound network access.
type EgressStrategy string

const (
	// EgressPublicIP puts a public IPv4 address on every task. What meroku
	// generates today, and the cheapest option below the switch threshold.
	EgressPublicIP EgressStrategy = "public_ip"
	// EgressNATGateway puts tasks in private subnets behind one NAT Gateway.
	// Cheapest past the threshold for non-production, where a single zonal
	// NAT is an acceptable failure domain.
	EgressNATGateway EgressStrategy = "nat_gateway"
	// EgressNATGatewayHA runs one NAT Gateway per AZ. The production form:
	// twice the hourly rate, no single-AZ egress outage, and no cross-AZ
	// transfer charge for tasks outside the NAT's zone.
	EgressNATGatewayHA EgressStrategy = "nat_gateway_ha"
)

// us-east-1 list prices, August 2026. A month is 730 hours.
//
// These are deliberately the cheapest common region: a recommendation that
// holds here holds everywhere, since every other region prices NAT at or above
// us-east-1 (ap-southeast-2 charges $0.059/hr, roughly 30% more).
const (
	hoursPerMonth = 730.0

	publicIPv4Hourly   = 0.005
	natGatewayHourly   = 0.045
	natGatewayPerGB    = 0.045
	crossAZPerGBPerLeg = 0.01

	publicIPv4Monthly = publicIPv4Hourly * hoursPerMonth // $3.65
	natGatewayMonthly = natGatewayHourly * hoursPerMonth // $32.85
)

// Switch thresholds, in services. Production carries the cost of a NAT per AZ,
// which roughly doubles the fixed side and so pushes its break-even out to
// about ten services. See the crossing table in ai_docs/EGRESS_COST_MODEL.md.
const (
	switchThresholdNonProd = 5
	switchThresholdProd    = 10
)

// defaultEgressGB is the monthly per-environment egress assumed when a caller
// has no measured figure. Tasks behind API Gateway send user-facing responses
// back through the gateway rather than their own ENI, so their own egress is
// image pulls, log shipping and outbound API calls — small. The estimate only
// moves the NAT side of the comparison, and moving it up makes NAT look worse,
// so a low default is the conservative choice for a recommendation to switch.
const defaultEgressGB = 100.0

// EgressFootprint is the part of an environment that drives egress cost.
type EgressFootprint struct {
	// Services counts long-running ECS services, including the backend.
	// This is what the switch threshold is expressed in.
	Services int
	// Tasks counts steady-state task instances across those services. This
	// is what public IPv4 actually bills for.
	Tasks int
	// AZs is the zone count the VPC spans. modules/vpc hardcodes 2.
	AZs int
	// TrafficGB is monthly egress. Only affects the NAT side.
	TrafficGB float64
}

// EgressAdvice is a non-blocking recommendation about egress strategy.
type EgressAdvice struct {
	Footprint     EgressFootprint `json:"footprint"`
	Current       EgressStrategy  `json:"current"`
	Recommended   EgressStrategy  `json:"recommended"`
	ShouldSwitch  bool            `json:"shouldSwitch"`
	Threshold     int             `json:"threshold"`
	CurrentCost   float64         `json:"currentMonthlyCost"`
	SwitchedCost  float64         `json:"switchedMonthlyCost"`
	MonthlySaving float64         `json:"monthlySaving"`
	// ServicesUntilSwitch is how many more services would reach the
	// threshold. Zero once the threshold is met.
	ServicesUntilSwitch int    `json:"servicesUntilSwitch"`
	Summary             string `json:"summary"`
}

// countEgressFootprint works out how many services and steady-state tasks an
// environment runs.
//
// Scheduled and event-processor tasks are deliberately excluded. They are
// short-lived, and a public IPv4 address is billed only while it is attached,
// so a task that runs for five minutes a day contributes a fraction of a cent.
// Counting them would inflate the case for switching.
func countEgressFootprint(env *Env) EgressFootprint {
	if env == nil {
		return EgressFootprint{AZs: 2, TrafficGB: defaultEgressGB}
	}

	// The backend is always one service. Its desired count defaults to 1 when
	// unset rather than 0 — an environment with no explicit count still runs
	// a task.
	services := 1
	tasks := int(env.Workload.BackendDesiredCount)
	if tasks < 1 {
		tasks = 1
	}

	for i := range env.Services {
		svc := &env.Services[i]
		if svc.Enabled != nil && !*svc.Enabled {
			continue // config retained, nothing deployed, nothing billed
		}
		services++
		if svc.DesiredCount > 0 {
			tasks += svc.DesiredCount
		} else {
			tasks++
		}
	}

	if env.Workload.InstallPgAdmin {
		services++
		tasks++
	}

	return EgressFootprint{
		Services:  services,
		Tasks:     tasks,
		AZs:       2, // modules/vpc hardcodes 2 AZs
		TrafficGB: defaultEgressGB,
	}
}

// egressMonthlyCost prices one strategy for a footprint, in USD per month.
//
// Internet data-transfer-out is excluded throughout: every strategy pays
// $0.09/GB on it identically, so including it would add the same constant to
// each and change no comparison.
func egressMonthlyCost(s EgressStrategy, f EgressFootprint) float64 {
	switch s {
	case EgressPublicIP:
		// No per-GB component at all. Identical at 50 GB and 50 TB.
		return publicIPv4Monthly * float64(f.Tasks)

	case EgressNATGateway:
		// One NAT lives in one AZ. Tasks in the other zones reach it across
		// an AZ boundary, which bills in both directions.
		crossAZShare := 0.0
		if f.AZs > 1 {
			crossAZShare = float64(f.AZs-1) / float64(f.AZs)
		}
		crossAZ := 2 * crossAZPerGBPerLeg * f.TrafficGB * crossAZShare
		return natGatewayMonthly + natGatewayPerGB*f.TrafficGB + crossAZ

	case EgressNATGatewayHA:
		// One NAT per AZ: every task egresses in its own zone, so there is
		// no cross-AZ leg to pay for.
		azs := f.AZs
		if azs < 1 {
			azs = 1
		}
		return natGatewayMonthly*float64(azs) + natGatewayPerGB*f.TrafficGB

	default:
		return 0
	}
}

// switchThreshold is the service count at which a NAT Gateway becomes the
// cheaper strategy for this environment.
func switchThreshold(isProd bool) int {
	if isProd {
		return switchThresholdProd
	}
	return switchThresholdNonProd
}

// recommendedStrategy is the strategy an environment of this kind should use
// once it has crossed the threshold.
func recommendedStrategy(isProd bool) EgressStrategy {
	if isProd {
		return EgressNATGatewayHA
	}
	return EgressNATGateway
}

// AdviseEgress returns a non-blocking recommendation about how an environment
// should give its tasks outbound access.
//
// Below the threshold it recommends staying on public IPs and reports how much
// headroom is left. At or above it, it recommends the NAT form appropriate to
// the environment and reports the saving. Callers should present this as a
// suggestion; it is never an error, and acting on it is optional.
func AdviseEgress(env *Env) EgressAdvice {
	isProd := env != nil && env.IsProd

	footprint := countEgressFootprint(env)
	threshold := switchThreshold(isProd)
	current := EffectiveEgressStrategy(env)

	// Below the threshold public IPs are cheapest; at or above it, the NAT form
	// appropriate to this kind of environment is.
	recommended := EgressPublicIP
	if footprint.Services >= threshold {
		recommended = recommendedStrategy(isProd)
	}

	// The default VPC has no private subnets, so a NAT strategy is not
	// reachable there however many services are running. Recommending one would
	// be advice the operator cannot take.
	pinnedToPublic := env != nil && env.UseDefaultVPC
	if pinnedToPublic {
		recommended = EgressPublicIP
	}

	currentCost := egressMonthlyCost(current, footprint)
	recommendedCost := egressMonthlyCost(recommended, footprint)

	advice := EgressAdvice{
		Footprint:    footprint,
		Current:      current,
		Recommended:  recommended,
		Threshold:    threshold,
		CurrentCost:  currentCost,
		SwitchedCost: recommendedCost,
		ShouldSwitch: current != recommended,
	}
	if advice.ShouldSwitch {
		advice.MonthlySaving = currentCost - recommendedCost
	}
	if footprint.Services < threshold {
		advice.ServicesUntilSwitch = threshold - footprint.Services
	}

	switch {
	case pinnedToPublic && footprint.Services >= threshold:
		advice.Summary = fmt.Sprintf(
			"%d service(s), %d task(s): public IPv4 costs $%.2f/mo and a NAT Gateway would be "+
				"cheaper, but this environment uses the default VPC, which has no private "+
				"subnets. Set use_default_vpc: false to make the switch available.",
			footprint.Services, footprint.Tasks, currentCost)

	case !advice.ShouldSwitch && recommended == EgressPublicIP:
		advice.Summary = fmt.Sprintf(
			"%d service(s), %d task(s): %s is the cheapest option at $%.2f/mo. Revisit at "+
				"%d services (%d more), where a %s becomes cheaper.",
			footprint.Services, footprint.Tasks, natLabel(current), currentCost,
			threshold, advice.ServicesUntilSwitch, natLabel(recommendedStrategy(isProd)))

	case !advice.ShouldSwitch:
		advice.Summary = fmt.Sprintf(
			"%d service(s), %d task(s): %s at $%.2f/mo is the right choice at this size, and "+
				"its cost stays flat as you add more services.",
			footprint.Services, footprint.Tasks, natLabel(current), currentCost)

	case advice.MonthlySaving > 0:
		advice.Summary = fmt.Sprintf(
			"%d service(s), %d task(s): %s costs $%.2f/mo and grows with every task. A %s "+
				"costs $%.2f/mo flat, saving about $%.2f/mo ($%.0f/yr), and the tasks no "+
				"longer need public addresses at all.",
			footprint.Services, footprint.Tasks, natLabel(current), currentCost,
			natLabel(recommended), recommendedCost, advice.MonthlySaving, advice.MonthlySaving*12)

	default:
		// Already on a NAT while still small: it works, it is just not the
		// cheapest shape yet. Worth saying, never worth alarming about.
		advice.Summary = fmt.Sprintf(
			"%d service(s), %d task(s): %s costs $%.2f/mo, about $%.2f/mo more than %s at this "+
				"size. It pays for itself from %d services, so this is only worth changing if "+
				"you are not about to grow into it.",
			footprint.Services, footprint.Tasks, natLabel(current), currentCost,
			-advice.MonthlySaving, natLabel(recommended), threshold)
	}

	return advice
}

// ValidEgressStrategies lists the accepted values, in the order they should be
// offered: cheapest-and-simplest first. Kept in step with the validation block
// on modules/vpc's egress_strategy variable.
var ValidEgressStrategies = []EgressStrategy{
	EgressPublicIP,
	EgressNATGateway,
	EgressNATGatewayHA,
}

// ValidateEgressStrategy checks an environment's configured strategy.
//
// Two things can be wrong: an unrecognised value, or a NAT strategy on the
// default VPC. The second is worth its own message because it is not obvious —
// the default VPC has only public subnets, so there is nowhere for a NAT
// strategy to put the tasks.
//
// An empty value is valid and means public_ip, so environments written before
// schema v27 validate without being migrated first.
func ValidateEgressStrategy(env *Env) error {
	if env == nil || env.EgressStrategy == "" {
		return nil
	}

	strategy := EgressStrategy(env.EgressStrategy)

	valid := false
	for _, candidate := range ValidEgressStrategies {
		if strategy == candidate {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf(
			"egress_strategy %q is not recognised: expected one of public_ip, nat_gateway, nat_gateway_ha",
			env.EgressStrategy)
	}

	if env.UseDefaultVPC && strategy != EgressPublicIP {
		return fmt.Errorf(
			"egress_strategy %q needs private subnets, which the default VPC does not have: "+
				"set use_default_vpc: false to let meroku create a VPC, or use public_ip",
			env.EgressStrategy)
	}

	return nil
}

// EffectiveEgressStrategy is the strategy that will actually be applied, after
// the empty-means-public_ip default. Callers that need to render or price the
// current setting should use this rather than reading the field directly.
func EffectiveEgressStrategy(env *Env) EgressStrategy {
	if env == nil || env.EgressStrategy == "" {
		return EgressPublicIP
	}
	return EgressStrategy(env.EgressStrategy)
}

// natLabel renders a strategy for a sentence.
func natLabel(s EgressStrategy) string {
	switch s {
	case EgressNATGatewayHA:
		return "NAT Gateway per AZ"
	case EgressNATGateway:
		return "single NAT Gateway"
	case EgressPublicIP:
		return "public IPv4 address per task"
	default:
		return string(s)
	}
}
