// Package recommend is the pure scoring core of the EC2 compute
// recommendation feature (architecture.md section 4).
//
// Recommend(Input) Result takes no context.Context, returns no error, reads no
// clock, starts no goroutine, logs nothing and touches no network. Everything
// time-dependent -- the CloudWatch window, asOf, cachedAt -- is resolved by the
// caller and never reaches here, because none of it affects the answer.
//
// The package imports fmt, math, sort, strconv and strings, and nothing else.
// A sixth import is a review gate.
//
// Two further properties are load-bearing for NFR-9 (byte-identical output for
// identical input, on any architecture) and are not implied by the signature:
// every float->int conversion goes through num.go's saturating helpers, and
// Rank's comparator is a strict weak ordering over a quantized key.
package recommend

// Posture selects the weight table (FR-23) and the hard parameters (FR-24).
type Posture string

const (
	PosturePerformance Posture = "performance-first"
	PostureBalanced    Posture = "balanced"
	PostureCost        Posture = "cost-first"
)

// Network modes. bridge is the default for EC2 pools (D-6): tasks egress
// through the container instance's own public interface, and there is no ENI
// density cap at all.
const (
	NetworkModeBridge = "bridge"
	NetworkModeAWSVPC = "awsvpc"
)

// AMI families. The default is al2023 and it is x86-only -- defaulting to ""
// let the architecture filter accept everything while the template rendered an
// x86 AMI, which is C-11.
const (
	AMIFamilyAL2023      = "al2023"
	AMIFamilyAL2023ARM64 = "al2023_arm64"
	AMIFamilyAL2023GPU   = "al2023_gpu"
)

// Capacity types, as rendered into instances_distribution.
const (
	CapacityOnDemand     = "on_demand"
	CapacitySpot         = "spot"
	CapacitySpotWithBase = "spot_with_base"
)

// Architectures, as DescribeInstanceTypes reports them.
const (
	ArchX8664 = "x86_64"
	ArchARM64 = "arm64"
)

// The five classifications (FR-18).
const (
	ClassGPU         = "gpu"
	ClassBurstable   = "burstable"
	ClassMemoryHeavy = "memory_heavy"
	ClassBalanced    = "balanced"
	ClassCPUHeavy    = "cpu_heavy"
)

// Density bases -- which branch TasksPerInstance actually took.
const (
	DensityCPUMemoryOnly = "cpu_memory_only"
	DensityMaxENIsMinus1 = "max_enis_minus_one"
	DensityTrunkedTable  = "trunked_table"
	DensityInvalidDemand = "invalid_demand"
)

// Basis values for Result.Basis.
const (
	BasisMeasured   = "measured"
	BasisConfigured = "configured"
	BasisDefault    = "default"
)

// Failed-rule identifiers for Miss.FailedRule. There is deliberately no
// "cannot_hold_peak": D-8 removed R9b as vacuous, and no rule reads MinSize.
const (
	RuleTaskFitMemory     = "task_fit_memory"
	RuleTaskFitVCPU       = "task_fit_vcpu"
	RuleENIDensity        = "eni_density"
	RuleArchitecture      = "architecture"
	RuleGPU               = "gpu"
	RuleUnpriced          = "unpriced"
	RuleGeneration        = "generation"
	RuleBurstableClass    = "burstable_class"
	RuleBareMetal         = "bare_metal"
	RuleFamilyNotEligible = "family_not_eligible"
	RuleMalformedRecord   = "malformed_record"
	RuleZeroDensity       = "zero_density"
	RuleMaxSizeTooSmall   = "max_size_too_small"
	RuleNonFiniteScore    = "non_finite_score"
)

// Units for Miss.Available / Miss.Needed. Empty means the rule has no numeric
// dimension (architecture, generation, bare metal, burstable class).
const (
	UnitGiB       = "GiB"
	UnitVCPU      = "vCPU"
	UnitENIs      = "ENIs"
	UnitTasks     = "tasks"
	UnitInstances = "instances"
	UnitUSDPerHr  = "USD/hr"
)

// Per-service signal statuses.
const (
	StatusOK      = "ok"
	StatusPartial = "partial"
	StatusNoData  = "no_data"
)

// Clamp outcomes for RatioSignal.ClampedTo.
const (
	ClampNone = "none"
	ClampMin  = "min"
	ClampMax  = "max"
)

// TrunkingNotApplicable is what Signals.Trunking reads under bridge: there are
// no task ENIs to trunk, so the account setting is irrelevant.
const TrunkingNotApplicable = "not_applicable"

// coveredDatapoints is FR-17's threshold for a service counting toward
// coverage: 24 hourly datapoints, i.e. one full day.
const coveredDatapoints = 24

// usableMemoryFactor is FR-21.2's ECS agent + OS reserve. A task requesting
// the instance's full memory never places.
const usableMemoryFactor = 0.85

// actualWeightCap is FR-17's cap on the CloudWatch weight. It is a
// requirement, not a tuning constant: ECS places tasks on *requested*
// cpu/memory, so the request shape must dominate what the instance holds.
const actualWeightCap = 0.60

// tieWidth is FR-25's tie width, and the quantum Rank sorts on (note 8).
const tieWidth = 0.01

// InstanceType is the catalog record the core scores. Prices are pointers so
// "unpriced" (EC-2) is distinguishable from "free", which is never true.
type InstanceType struct {
	Name                 string
	Family               string // "m", "r", "c", "t", "g", "inf", "trn" -- prefix before the generation digit
	Generation           int    // 7 from "m7i.large"; 0 when unparseable
	VCPU                 int
	MemoryMiB            int64
	Architectures        []string
	CurrentGeneration    bool
	BareMetal            bool
	Burstable            bool
	GPUCount             int
	MaxNetworkInterfaces int
	SupportsSpot         bool
	OnDemandHourly       *float64
	SpotMedianHourly     *float64
}

// ServiceDemand is one in-scope service, already normalised: vCPU not CPU
// units, GiB not MiB, and utilisation as percentages of *reserved* -- which is
// what AWS/ECS reports.
//
// compute_signals.go has already dropped any service with VCPU <= 0 or
// MemGiB <= 0 (C-9). The core still guards, but the boundary is where the user
// gets told which field was wrong.
type ServiceDemand struct {
	Name       string
	VCPU       float64
	MemGiB     float64
	Count      int
	CPUAvg     float64
	CPUPeak    float64
	MemAvg     float64
	MemPeak    float64
	Datapoints int // 0 means no data, never "zero utilisation"
}

// PoolConstraints are the target pool's fixed facts. NormalizePool fills the
// defaults; nothing here is inferred from the catalog.
type PoolConstraints struct {
	Name string
	// NetworkMode is "bridge" (default) or "awsvpc" (D-6). It decides whether
	// the ENI cap enters TasksPerInstance at all (C-13).
	NetworkMode string
	// AMIFamily is "al2023" (DEFAULT, not ""), "al2023_arm64" or
	// "al2023_gpu". The default matches variables.tf's
	// optional(string, "al2023") exactly (C-11).
	AMIFamily                 string
	CapacityType              string // "on_demand" | "spot" | "spot_with_base"
	OnDemandBase              int    // INSTANCES, not tasks -- see score.go
	MinSize                   *int   // the pool's configured floor when editing an existing
	MaxSize                   *int   // pool; nil for a new one. MaxSize is a hard constraint (R9c).
	IncludePreviousGeneration bool
	ForceGPU                  bool // the gpu=true query param (FR-13, D-1)
	// InstanceTypes is the pool's own instance_types list, and it is read for
	// exactly ONE purpose: it is the operator's explicit opt-in past rule
	// R10's family allowlist (family.go). Naming inf1.24xlarge here makes that
	// one type eligible; it does not widen the allowlist, does not change any
	// other rule, and does not pin the answer -- the named type still has to
	// win on score like everything else.
	//
	// Empty is the normal case and means "allowlist only".
	InstanceTypes []string
}

// Input is everything the core is allowed to see.
type Input struct {
	Region   string
	Catalog  []InstanceType
	Services []ServiceDemand
	Pool     PoolConstraints
	Posture  Posture
	Limit    int

	// TrunkingEnabled is the answer to FR-59's awsvpcTrunking account-setting
	// probe. It is READ ONLY when Pool.NetworkMode == "awsvpc"; under bridge
	// there are no task ENIs to trunk and the field is ignored. false is the
	// safe default: it is what an unanswerable probe means, and it
	// under-promises density rather than over-promising it.
	TrunkingEnabled bool
}

// SubScores are FR-22's scoring dimensions, each in [0,1].
//
// There are FOUR, not five. FR-22's `waste` and `headroom` were near-inverses
// of one another -- waste rose as the instance filled, headroom rose as it
// emptied -- so every posture expressed its real preference as the DIFFERENCE
// between two partially-redundant terms, which is why the individual weights
// looked precise and behaved badly. They are replaced by a single Utilisation
// term. score.go carries the analysis.
type SubScores struct{ Fit, Utilisation, Cost, Modernity float64 }

// Candidate is one scored, ranked instance type.
type Candidate struct {
	InstanceType    string
	VCPU            int
	MemoryMiB       int64
	Architecture    string
	Scores          SubScores
	Total           float64
	EffectiveHourly float64
	// CostPerTask is what this candidate's pool costs per hour, per task it
	// runs: effectiveHourly * N / T, where N = ceil(T/k) is the fleet size.
	//
	// It REPLACES CostPerTaskSlot, which was effectiveHourly / k -- the price
	// of a slot the instance could offer, whether or not any task occupies it.
	// That denominator prices a counterfactual, and on a small fleet it prices
	// a large counterfactual: for ONE task of 0.25 vCPU / 0.5 GiB, c6i.2xlarge
	// offered 27 slots at $0.0119 each against c6i.large's 6 at $0.0133, so
	// the cost sub-score rated an 8 vCPU instance the better buy for a fleet
	// that will occupy 4 % of it. The pool really costs $0.32/hr against
	// $0.08/hr. Under the balanced posture that was enough to make
	// c6i.2xlarge the primary recommendation, and it is the same failure shape
	// as the removed waste/headroom pair: a cost term measured on hypothetical
	// capacity pulling against a utilisation term measured on achieved
	// occupancy.
	//
	// The two terms now agree on direction and still measure different things.
	// With EC2's within-family linear pricing, fleet cost per task factors as
	// (per-vCPU rate) * (task vCPU) / (achieved CPU utilisation): Utilisation
	// carries the second factor, CostPerTask carries both, and what CostPerTask
	// adds on top is the per-vCPU RATE -- m6i against m7i against Graviton --
	// which Utilisation is blind to. Reinforcing is fine; cancelling was not.
	CostPerTask      float64
	TasksPerInstance int
	InstancesAtFloor int // n_floor -- what SuggestedPool.MinSize will be for this candidate
	SpotMedianHourly *float64
	Reason           string
}

// Miss records one exclusion, for EC-11's nearest-miss list.
type Miss struct {
	InstanceType string
	FailedRule   string
	Needed       float64
	Available    float64
	Unit         string
}

// ServiceSignal is the per-service half of FR-29, so the UI can name what is
// missing rather than showing an empty chart.
type ServiceSignal struct {
	Name                             string
	Datapoints                       int
	CPUAvg, CPUPeak, MemAvg, MemPeak float64
	Status                           string
}

// Shape is a demand vector: total vCPU, total memory, and their ratio.
type Shape struct{ VCPU, MemGiB, Ratio float64 }

// RatioSignal reports the C-10 clamp. It is never silent.
type RatioSignal struct {
	Raw        float64
	Effective  float64
	CatalogMin float64
	CatalogMax float64
	ClampedTo  string // "none" | "min" | "max"
}

// Signals is FR-29, machine-readable.
type Signals struct {
	Configured          Shape
	ConfiguredTaskCount int
	Actual              *Shape
	Coverage            float64
	WeightConfigured    float64
	WeightActual        float64
	Ratio               RatioSignal
	NetworkMode         string
	// Trunking is set by the caller for awsvpc pools. The core sets it to
	// "not_applicable" under bridge, where the account setting is irrelevant.
	Trunking string
	// DensityBasis is set by the core: it is the branch TasksPerInstance
	// actually took for the primary candidate.
	DensityBasis string
	// CloudWatch is set by the caller: ok|partial|no_data|unavailable|timeout.
	// The distinction between the last three is an infrastructure fact.
	CloudWatch string
	// Dropped holds candidates removed for a non-finite score (C-4). A
	// non-empty slice here is a bug, and tests assert it empty.
	Dropped  []Miss
	Services []ServiceSignal
}

// SuggestedPool is a complete pool object, ready to write into YAML (FR-26).
type SuggestedPool struct {
	Name           string
	InstanceTypes  []string
	CapacityType   string
	OnDemandBase   int
	MinSize        int
	MaxSize        int
	TargetCapacity int
	NetworkMode    string
	// AMIFamily is DERIVED from the chosen types, never inherited blindly
	// (C-11): FR-26 lets ami_family and instance_types disagree, and after
	// this they cannot.
	AMIFamily    string
	RootVolumeGB int
	// Downgraded records EC-6: spot was asked for, spot was unavailable for
	// the primary type, and the pool fell back to on_demand.
	Downgraded bool
}

// Result is the whole answer.
type Result struct {
	Primary *Candidate
	Ranked  []Candidate
	// Classification is the class the answer was actually built under -- the
	// one every ranked candidate satisfies (FR-21.8). When the class FR-18
	// inferred could not be served it is the fallback, not the inference, so
	// that "classification" and "ranked" can never describe different runs.
	Classification string
	// ClassificationSuppressed is non-empty ONLY when a fallback happened, and
	// carries the class FR-18 inferred but the region could not serve. It
	// exists so the fallback is never silent: the inference is not something
	// the user chose, and no YAML field changes it, so a response that simply
	// showed a different class than the utilisation implies would be
	// unexplainable. Explain names it in the reason too.
	ClassificationSuppressed string
	Basis                    string
	Unsatisfiable            bool
	Constraint               string
	NearestMisses            []Miss
	Signals                  Signals
	SuggestedPool            SuggestedPool
}
