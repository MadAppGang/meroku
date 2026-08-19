package recommend

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The behaviour matrix
// ---------------------------------------------------------------------------
//
// WHAT THIS FILE IS FOR, AND WHAT IT IS NOT FOR.
//
// It is NOT a claim that the instance types in wantTable are the correct
// answers. Nobody has calibrated this recommender against a real fleet, and a
// test cannot make an uncalibrated weight table right.
//
// It is a claim about VISIBILITY. Every number in Params() -- four weights and
// a utilisation target per posture, fifteen constants in total -- silently
// changes every recommendation this tool will ever make. Before this file
// there was no way to see what moving one of them did: the unit tests pinned
// individual sub-score formulas, and the black-box tests pinned invariants
// (weights sum to 1, sub-scores stay in [0,1], output is byte-identical) that
// hold for ANY weight table, correct or not. A weight could be changed from
// 0.20 to 0.45 and the entire suite would stay green.
//
// So this is a table of realistic workloads x the three postures -> the chosen
// instance type, checked in with the reasoning that made each cell acceptable
// at the time it was written. Changing a weight now produces an explicit diff
// here, in a form a reviewer can argue with:
//
//	behaviour_matrix_test.go:NNN: memory-heavy cache / cost-first
//	  chose r6i.large, table says r7i.large
//
// The correct response to that failure is NEVER to update the table quietly.
// It is to decide whether the new cell is better than the old one, and if it
// is, to update the table AND the reasoning in the same commit -- so the next
// reader sees why the answer changed and not merely that it did.
//
// Run with -v to print the full matrix including sub-scores, which is the
// fastest way to understand what a weight change actually did:
//
//	go test ./recommend/ -run TestBehaviourMatrix -v
//
// CON-5: this repository is public. Every price below is an invented round
// number. Instance-type names and their vCPU/memory shapes are public catalog
// facts; nothing here came from a live account.

// ---------------------------------------------------------------------------
// The catalog fixture
// ---------------------------------------------------------------------------

// matrixCatalog is shaped like a real region's recommendable set: the three
// general families across two generations at four sizes each, their Graviton
// siblings, two burstable types, and -- because the matrix has to prove they
// never win -- one accelerator and one storage-optimised type.
//
// PRICING IS LINEAR IN vCPU WITHIN A FAMILY, at these invented per-vCPU rates:
//
//	c7i 0.0425   m7i 0.0500   r7i 0.0660   (current generation, x86)
//	c6i 0.0400   m6i 0.0480   r6i 0.0630   (one generation back, ~6 % cheaper)
//	c7g 0.0340   m7g 0.0400   r7g 0.0530   (Graviton, ~20 % cheaper)
//	inf1 0.0492  i4i 0.0858
//
// The linearity is not a simplification, it is the thing being modelled: EC2
// really does price m7i.24xlarge at 48x m7i.large, so within a family the only
// way an instance can be cheaper per task is to STRAND LESS. A fixture with
// invented non-linear prices would exercise arithmetic that does not exist and
// would make the cost sub-score look far more decisive than it is.
//
// Spot medians are a flat 0.40 of on-demand, which keeps spot from reordering
// anything by itself -- the postures that prefer spot then differ from the
// ones that do not by their weights, which is what the matrix is measuring.
func matrixCatalog() []InstanceType {
	x86 := []string{ArchX8664}
	arm := []string{ArchARM64}

	out := make([]InstanceType, 0, 32)
	add := func(name string, vcpu int, memGiB float64, archs []string, gen int, perVCPU float64, enis int) {
		od := perVCPU * float64(vcpu)
		spot := od * 0.40
		out = append(out, InstanceType{
			Name: name, VCPU: vcpu, MemoryMiB: int64(memGiB * 1024),
			Architectures: archs, CurrentGeneration: true, Generation: gen,
			MaxNetworkInterfaces: enis, SupportsSpot: true,
			OnDemandHourly: fp(od), SpotMedianHourly: fp(spot),
		})
	}

	// family, per-vCPU rate, GiB per vCPU, architecture
	families := []struct {
		prefix  string
		gen     int
		perVCPU float64
		gibPer  float64
		archs   []string
	}{
		{"c7i", 7, 0.0425, 2, x86},
		{"c6i", 6, 0.0400, 2, x86},
		{"c7g", 7, 0.0340, 2, arm},
		{"m7i", 7, 0.0500, 4, x86},
		{"m6i", 6, 0.0480, 4, x86},
		{"m7g", 7, 0.0400, 4, arm},
		{"r7i", 7, 0.0660, 8, x86},
		{"r6i", 6, 0.0630, 8, x86},
		{"r7g", 7, 0.0530, 8, arm},
	}
	sizes := []struct {
		suffix string
		vcpu   int
		enis   int
	}{
		{"large", 2, 3},
		{"xlarge", 4, 4},
		{"2xlarge", 8, 4},
		{"4xlarge", 16, 8},
	}
	for _, f := range families {
		for _, s := range sizes {
			add(f.prefix+"."+s.suffix, s.vcpu, f.gibPer*float64(s.vcpu), f.archs, f.gen, f.perVCPU, s.enis)
		}
	}

	// Burstable. Priced below the general-purpose families for the same shape,
	// which is what makes them attractive to cost-first and is why FR-19 has to
	// keep performance-first away from them.
	out = append(out,
		InstanceType{Name: "t3.small", VCPU: 2, MemoryMiB: 2048, Architectures: x86,
			CurrentGeneration: true, Burstable: true, MaxNetworkInterfaces: 3,
			SupportsSpot: true, OnDemandHourly: fp(0.0208), SpotMedianHourly: fp(0.0083)},
		InstanceType{Name: "t3.medium", VCPU: 2, MemoryMiB: 4096, Architectures: x86,
			CurrentGeneration: true, Burstable: true, MaxNetworkInterfaces: 3,
			SupportsSpot: true, OnDemandHourly: fp(0.0416), SpotMedianHourly: fp(0.0166)},
		InstanceType{Name: "t3.large", VCPU: 2, MemoryMiB: 8192, Architectures: x86,
			CurrentGeneration: true, Burstable: true, MaxNetworkInterfaces: 3,
			SupportsSpot: true, OnDemandHourly: fp(0.0832), SpotMedianHourly: fp(0.0333)},
		InstanceType{Name: "t4g.medium", VCPU: 2, MemoryMiB: 4096, Architectures: arm,
			CurrentGeneration: true, Burstable: true, MaxNetworkInterfaces: 3,
			SupportsSpot: true, OnDemandHourly: fp(0.0336), SpotMedianHourly: fp(0.0134)},

		// The two R10 casualties. Both are current generation, x86, priced,
		// and denser per dollar than anything above -- which is precisely how
		// inf1.24xlarge came to be recommended for a web fleet.
		InstanceType{Name: "inf1.24xlarge", VCPU: 96, MemoryMiB: 196608, Architectures: x86,
			CurrentGeneration: true, MaxNetworkInterfaces: 15,
			SupportsSpot: true, OnDemandHourly: fp(4.7200), SpotMedianHourly: fp(1.8880)},
		InstanceType{Name: "i4i.2xlarge", VCPU: 8, MemoryMiB: 65536, Architectures: x86,
			CurrentGeneration: true, MaxNetworkInterfaces: 4,
			SupportsSpot: true, OnDemandHourly: fp(0.6860), SpotMedianHourly: fp(0.2744)},
	)
	return out
}

// ---------------------------------------------------------------------------
// The workloads
// ---------------------------------------------------------------------------

type matrixWorkload struct {
	name string
	// why records what this row is here to exercise. It is printed on failure
	// so a diff arrives with its own context.
	why      string
	services []ServiceDemand
}

func matrixWorkloads() []matrixWorkload {
	return []matrixWorkload{
		{
			name: "small web service",
			why: "3 tasks of 0.25 vCPU / 0.5 GiB. R_cfg = 2.0, so the c family is the " +
				"shape match. The whole fleet fits on one .large several times over, " +
				"which is what makes achieved occupancy -- not packing capacity -- the " +
				"quantity that decides the size.",
			services: []ServiceDemand{{Name: "web", VCPU: 0.25, MemGiB: 0.5, Count: 3}},
		},
		{
			name: "memory-heavy cache",
			why: "2 tasks of 1 vCPU / 8 GiB. R_cfg = 8.0, the top of the eligible " +
				"catalog's range, so an r-family answer is the only defensible one and " +
				"any m-family answer is a regression.",
			services: []ServiceDemand{{Name: "cache", VCPU: 1, MemGiB: 8, Count: 2}},
		},
		{
			name: "cpu-heavy worker",
			why: "4 tasks of 2 vCPU / 2 GiB, R_cfg = 1.0. Each task needs a whole " +
				"c*.large to itself, so the postures differ on whether to buy four " +
				"small instances or one big one -- a choice nothing else in the matrix " +
				"forces. Note R_eff is NOT clamped: t3.small is 1.0 GiB/vCPU and sets " +
				"the catalog floor there, even though rule R8 then removes every " +
				"burstable type from a cpu_heavy answer. That is C-10 behaving as " +
				"architecture.md 4.4 specifies -- the range is taken before the class " +
				"is known, because the classification needs the clamp -- and the visible " +
				"consequence is that the best available fit on this row is 0.50.",
			services: []ServiceDemand{{Name: "worker", VCPU: 2, MemGiB: 2, Count: 4}},
		},
		{
			name: "mixed fleet",
			why: "Three services of different shapes: R_cfg = 3.75, mid-catalog, so no " +
				"family is an obvious match and the postures have to differ on " +
				"something other than shape. This is the row most sensitive to the " +
				"cost and utilisation weights.",
			services: []ServiceDemand{
				{Name: "api", VCPU: 0.5, MemGiB: 1, Count: 3},
				{Name: "worker", VCPU: 1, MemGiB: 4, Count: 2},
				{Name: "cache", VCPU: 0.5, MemGiB: 4, Count: 1},
			},
		},
		{
			name: "idle fleet",
			why: "The mixed fleet with a full 14-day window showing almost no CPU and " +
				"moderate memory. Two things fire at once. FR-17 blends R_act upward " +
				"and C-10 clamps R_eff to the catalog ceiling of 8.0, so the answer " +
				"moves from the m family to the r family on measured data alone -- the " +
				"one channel measurement is supposed to act through. And FR-18 " +
				"classifies the fleet BURSTABLE (max task 1 vCPU, CPU average under " +
				"20 %, coverage 1.0), which FR-19 suppresses for performance-first " +
				"only. So this row is also the matrix's proof that the posture changes " +
				"the classification and not merely the ranking.",
			services: []ServiceDemand{
				{Name: "api", VCPU: 0.5, MemGiB: 1, Count: 3,
					CPUAvg: 2, CPUPeak: 4, MemAvg: 30, MemPeak: 38, Datapoints: 336},
				{Name: "worker", VCPU: 1, MemGiB: 4, Count: 2,
					CPUAvg: 3, CPUPeak: 6, MemAvg: 41, MemPeak: 52, Datapoints: 336},
				{Name: "cache", VCPU: 0.5, MemGiB: 4, Count: 1,
					CPUAvg: 1, CPUPeak: 3, MemAvg: 60, MemPeak: 71, Datapoints: 336},
			},
		},
		{
			name: "tiny single-task fleet",
			why: "One task of 0.25 vCPU / 0.5 GiB. Every candidate holds it, so " +
				"achieved occupancy is 1 everywhere and the SMALLEST instance is the " +
				"only sane answer. A scorer reading packing CAPACITY instead of " +
				"achieved occupancy rates a 16 vCPU box highly here, which is the " +
				"defect in miniature.",
			services: []ServiceDemand{{Name: "job", VCPU: 0.25, MemGiB: 0.5, Count: 1}},
		},
		{
			name: "large fleet",
			why: "200 tasks of 0.5 vCPU / 2 GiB, R_cfg = 4.0. Big enough that several " +
				"sizes are all genuinely full, so the postures are free to differ on " +
				"instance GRANULARITY at equal provisioned capacity -- and they do: all " +
				"three provision 120 vCPU, performance-first as 30 xlarges and the other " +
				"two as 15 2xlarges. That is the posture direction the UI advertises " +
				"(slack over price against price over slack) showing up in the pool " +
				"shape rather than only in the price.",
			services: []ServiceDemand{{Name: "fleet", VCPU: 0.5, MemGiB: 2, Count: 200}},
		},
	}
}

// ---------------------------------------------------------------------------
// The expected table
// ---------------------------------------------------------------------------

// matrixKey is "workload/posture".
func matrixKey(workload string, p Posture) string { return workload + "/" + string(p) }

// wantTable is the checked-in matrix. See the header: these are RECORDED
// answers with recorded reasoning, not proven-optimal ones.
var wantTable = map[string]string{
	// -- small web service ------------------------------------------------
	// R_eff 2.0 -> the c family on fit (1.00 against m's 0.50). Three tiny
	// tasks fit on one .large six times over, so every larger size is mostly
	// empty and both achieved utilisation and fleet cost per task say so:
	// c7i.xlarge's util score is half c7i.large's and its cost score is half
	// again. All three postures agree on c*.large.
	//
	// KNIFE EDGE, recorded deliberately: balanced picks the previous
	// generation, 0.8290 against 0.8234. Trading modernity 1.00 -> 0.70 at
	// weight 0.05 (-0.015) for cost 0.941 -> 1.000 at weight 0.35 (+0.021) is
	// a net +0.006. If anyone decides balanced should prefer current-generation
	// silicon, this is the cell that will move first, and moving it means
	// raising modernity above 0.07 or narrowing the generation price gap.
	matrixKey("small web service", PosturePerformance): "c7i.large",
	matrixKey("small web service", PostureBalanced):    "c6i.large",
	matrixKey("small web service", PostureCost):        "c6i.large",

	// -- memory-heavy cache -----------------------------------------------
	// R_eff 8.0, so only the r family scores fit 1.00. Two 8 GiB tasks need
	// 16 GiB of USABLE memory, which after FR-21.2's 0.85 reserve puts both on
	// one r*.xlarge (27.2 usable) or one each on two r*.large (13.6 usable).
	// Both provision 4 vCPU / 32 GiB and cost the same per task, so the split
	// is pure posture:
	//   performance-first takes the single xlarge  (util 0.777 at target 0.70)
	//   balanced and cost-first take two larges    (LeanCheapest, and a lower
	//                                               effectiveHourly per unit)
	// Cost-first drops a generation for ~5 %, which is what it is for.
	matrixKey("memory-heavy cache", PosturePerformance): "r7i.xlarge",
	matrixKey("memory-heavy cache", PostureBalanced):    "r7i.large",
	matrixKey("memory-heavy cache", PostureCost):        "r6i.large",

	// -- cpu-heavy worker --------------------------------------------------
	// Each task wants a whole c*.large to itself (2 vCPU), so every answer
	// provisions exactly 8 vCPU and the only question is granularity. Every
	// c-family candidate ties at total 0.7314 under performance-first, because
	// with linear pricing one 2xlarge and four larges are identical on fit,
	// utilisation and cost alike.
	//
	// The postures then split on their FR-24 family lean, which is the point:
	//   performance-first LeanCompute does not discriminate inside the c
	//     family, so it falls through to maxENI and then name -- c7i.2xlarge.
	//   cost-first LeanCheapest breaks the same tie on effectiveHourly and
	//     takes the cheapest single instance -- c6i.large, four of them.
	//   balanced has no lean and lands on c6i.large through cost.
	//
	// FOLLOW-UP, recorded rather than fixed: nothing in the model prefers four
	// small instances to one large one. Blast radius, spot-interruption
	// granularity and scaling step size all favour the four, and all three are
	// unmodelled. performance-first choosing the single 8 vCPU box on a NAME
	// tie-break is the visible symptom.
	matrixKey("cpu-heavy worker", PosturePerformance): "c7i.2xlarge",
	matrixKey("cpu-heavy worker", PostureBalanced):    "c6i.large",
	matrixKey("cpu-heavy worker", PostureCost):        "c6i.large",

	// -- mixed fleet -------------------------------------------------------
	// R_eff 3.75 sits just under the m family's 4.0, so fit is 0.953 for m and
	// 0.453 for r -- the clearest shape signal in the matrix. Six tasks with a
	// 0.667 vCPU / 2.5 GiB mean pack two to an m*.large, so all three postures
	// run three of them and differ only on generation.
	//
	// This row is where achieved utilisation is most visible: m7i.large scores
	// util 0.997 under performance-first, i.e. its real occupancy lands almost
	// exactly on the 0.70 target, while m7i.2xlarge -- which the old capacity-
	// based waste score rated identically -- scores 0.751.
	matrixKey("mixed fleet", PosturePerformance): "m7i.large",
	matrixKey("mixed fleet", PostureBalanced):    "m7i.large",
	matrixKey("mixed fleet", PostureCost):        "m6i.large",

	// -- idle fleet --------------------------------------------------------
	// Same services, now measured. Two independent things fire:
	//
	//   FR-17 blends R_act upward and C-10 clamps R_eff from 3.75 to the
	//   catalog ceiling 8.0, so performance-first's answer moves from the m
	//   family to the r family on measured data alone.
	//
	//   FR-18 classifies the fleet burstable -- largest task 1 vCPU, CPU
	//   averaging 2-3 %, coverage 1.0 -- and FR-19 suppresses that for
	//   performance-first ONLY. So balanced and cost-first are answered from a
	//   different candidate set, not merely a different ranking, and t3.large
	//   is the only burstable type in the catalog whose 6.8 GiB usable can hold
	//   the 4 GiB task.
	//
	// A fleet running at 3 % CPU genuinely is what burstable is for, so this is
	// the classification working rather than misfiring.
	matrixKey("idle fleet", PosturePerformance): "r7i.xlarge",
	matrixKey("idle fleet", PostureBalanced):    "t3.large",
	matrixKey("idle fleet", PostureCost):        "t3.large",

	// -- tiny single-task fleet --------------------------------------------
	// One task. Every candidate carries exactly it, so achieved occupancy is 1
	// everywhere and the smallest instance is the only sane answer. It is the
	// direct regression for the defect the matrix caught on its first run:
	// with cost measured per PACKING SLOT rather than per task, c6i.2xlarge
	// offered 27 slots at $0.0119 against c6i.large's 6 at $0.0133 and won the
	// balanced posture -- an 8 vCPU instance for one 0.25 vCPU task. See
	// CostPerTask in types.go.
	matrixKey("tiny single-task fleet", PosturePerformance): "c7i.large",
	matrixKey("tiny single-task fleet", PostureBalanced):    "c7i.large",
	matrixKey("tiny single-task fleet", PostureCost):        "c6i.large",

	// -- large fleet -------------------------------------------------------
	// 200 tasks, 100 vCPU / 400 GiB of demand. All three postures provision
	// exactly 120 vCPU and differ on GRANULARITY, which is the posture
	// difference the UI advertises showing up in the pool shape:
	//
	//   performance-first  30 x m7i.xlarge   util 0.666, ~six tasks each
	//   balanced           15 x m7i.2xlarge  util 0.999, ~thirteen each
	//   cost-first         15 x m6i.2xlarge  util 0.850, a generation back
	//
	// Note performance-first buying the SMALLER instance. That is the 0.70
	// utilisation target doing exactly what it says: a nearly-full 2xlarge is
	// penalised for having no room to absorb a spike, and the answer is more,
	// emptier instances. It also means instance size is NOT monotone in fleet
	// size under performance-first, which is intended -- see
	// TestBehaviourMatrix_PoolGrowsWithTheFleet for the quantity that IS.
	matrixKey("large fleet", PosturePerformance): "m7i.xlarge",
	matrixKey("large fleet", PostureBalanced):    "m7i.2xlarge",
	matrixKey("large fleet", PostureCost):        "m6i.2xlarge",
}

// ---------------------------------------------------------------------------
// The test
// ---------------------------------------------------------------------------

func matrixInput(services []ServiceDemand, p Posture) Input {
	return Input{
		Region:   "ap-southeast-2",
		Catalog:  matrixCatalog(),
		Services: services,
		Pool:     PoolConstraints{Name: "general"},
		Posture:  p,
		Limit:    50,
	}
}

func TestBehaviourMatrix(t *testing.T) {
	postures := []Posture{PosturePerformance, PostureBalanced, PostureCost}

	for _, w := range matrixWorkloads() {
		for _, p := range postures {
			key := matrixKey(w.name, p)
			t.Run(key, func(t *testing.T) {
				res := Recommend(matrixInput(w.services, p))
				if res.Primary == nil {
					t.Fatalf("no recommendation at all: unsatisfiable=%v constraint=%q",
						res.Unsatisfiable, res.Constraint)
				}
				if testing.Verbose() {
					t.Log("\n" + matrixReport(w, p, res))
				}

				want, ok := wantTable[key]
				if !ok {
					t.Fatalf("no expected value recorded for %q; every workload x posture "+
						"cell has to be in wantTable with its reasoning", key)
				}
				if res.Primary.InstanceType != want {
					t.Errorf(""+
						"chose %s, table says %s\n"+
						"  workload: %s\n"+
						"  posture weights: %+v target utilisation %.2f\n"+
						"  top of ranking:\n%s\n"+
						"THIS IS NOT AUTOMATICALLY A BUG. If a weight moved on purpose, "+
						"decide whether the new answer is better, then update wantTable AND "+
						"its reasoning in the same commit. Do not update the table alone.",
						res.Primary.InstanceType, want, w.why,
						Params(p).Weights, Params(p).TargetUtilisation,
						matrixRanking(res, 5))
				}
			})
		}
	}
}

// TestBehaviourMatrix_NoIneligibleFamilyEverWins is R10 asserted across the
// whole matrix rather than on one fixture. inf1.24xlarge is the densest and
// (per vCPU) among the cheapest records in matrixCatalog, so if the allowlist
// ever regressed this is where it would show first.
func TestBehaviourMatrix_NoIneligibleFamilyEverWins(t *testing.T) {
	for _, w := range matrixWorkloads() {
		for _, p := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
			res := Recommend(matrixInput(w.services, p))
			for _, c := range res.Ranked {
				if fam := familyOf(c.InstanceType); !eligibleFamilies[fam] {
					t.Errorf("%s/%s ranked %s (family %q) at total %.4f",
						w.name, p, c.InstanceType, fam, c.Total)
				}
			}
		}
	}
}

// TestBehaviourMatrix_PostureLeansAreVisible checks the DIRECTIONS the three
// postures promise, which is the part of the weight table a user is actually
// shown. A table that produced identical answers everywhere would pass the
// matrix above (it asserts equality, not difference) while making the posture
// selector a lie.
func TestBehaviourMatrix_PostureLeansAreVisible(t *testing.T) {
	for _, w := range matrixWorkloads() {
		t.Run(w.name, func(t *testing.T) {
			perf := Recommend(matrixInput(w.services, PosturePerformance))
			cost := Recommend(matrixInput(w.services, PostureCost))
			if perf.Primary == nil || cost.Primary == nil {
				t.Fatal("a posture produced no recommendation")
			}

			// Cost-first must never propose a pool that costs MORE per hour at
			// its own floor than performance-first's. This is the one
			// comparison that holds for every workload regardless of shape,
			// and it is the promise the posture's name makes.
			perfHourly := perf.Primary.EffectiveHourly * float64(perf.SuggestedPool.MinSize)
			costHourly := cost.Primary.EffectiveHourly * float64(cost.SuggestedPool.MinSize)
			if costHourly > perfHourly {
				t.Errorf("cost-first floor costs %.4f/hr (%s x%d) against performance-first's "+
					"%.4f/hr (%s x%d); the posture names are inverted",
					costHourly, cost.Primary.InstanceType, cost.SuggestedPool.MinSize,
					perfHourly, perf.Primary.InstanceType, perf.SuggestedPool.MinSize)
			}

			// Performance-first never accepts a burstable type (FR-19): CPU
			// credits make steady-state throughput unpredictable.
			if familyOf(perf.Primary.InstanceType) == "t" {
				t.Errorf("performance-first chose the burstable %s", perf.Primary.InstanceType)
			}
			if perf.SuggestedPool.CapacityType != CapacityOnDemand {
				t.Errorf("performance-first capacity_type = %q, want on_demand",
					perf.SuggestedPool.CapacityType)
			}
		})
	}
}

// TestBehaviourMatrix_PoolGrowsWithTheFleet is the property the achieved
// utilisation term replaced waste and headroom to obtain, asserted on the
// quantity that actually carries it.
//
// The obvious assertion -- "a bigger fleet gets a bigger INSTANCE" -- is false
// by design and was written that way first. Under performance-first, target
// utilisation 0.70 penalises a nearly-full instance, so a 64-task fleet takes
// m7i.4xlarge (occupancy lands near the target) while a 256-task fleet takes
// m7i.xlarge (the 4xlarge would be 87 % subscribed, which is over target).
// Instance size is not monotone in fleet size and should not be.
//
// PROVISIONED CAPACITY is monotone, and that is the real claim: the pool the
// recommender proposes grows with the demand it is proposing for. Under the
// old capacity-based waste score this was much weaker, because an instance was
// rated on how full it COULD be -- a 16 vCPU type scored identically for one
// task and for two hundred, and only cost pushed back.
func TestBehaviourMatrix_PoolGrowsWithTheFleet(t *testing.T) {
	for _, p := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		t.Run(string(p), func(t *testing.T) {
			var prevVCPU int
			var prevDesc string
			for _, count := range []int{1, 4, 16, 64, 256} {
				res := Recommend(matrixInput(
					[]ServiceDemand{{Name: "fleet", VCPU: 0.5, MemGiB: 2, Count: count}}, p))
				if res.Primary == nil {
					t.Fatalf("%d tasks: no recommendation (%s)", count, res.Constraint)
				}
				provisioned := res.Primary.VCPU * res.SuggestedPool.MinSize
				desc := fmt.Sprintf("%d x %s = %d vCPU",
					res.SuggestedPool.MinSize, res.Primary.InstanceType, provisioned)

				if provisioned < prevVCPU {
					t.Errorf("%d tasks provisions %s, LESS than the %s provisioned for the "+
						"previous, smaller fleet", count, desc, prevDesc)
				}
				// And the floor really does hold the fleet, which is the other
				// half of "the pool grew with the demand".
				if held := res.SuggestedPool.MinSize * res.Primary.TasksPerInstance; held < count {
					t.Errorf("%d tasks: the pool floor %s holds only %d of them", count, desc, held)
				}
				prevVCPU, prevDesc = provisioned, desc
			}
		})
	}
}

// TestBehaviourMatrix_NeverGrosslyOverProvisions is the general form of the
// one-task defect: no cell of the matrix may buy wildly more than the workload
// reserves.
//
// The bound is deliberately loose -- three times configured demand, or one
// smallest-purchasable instance, whichever is larger -- because tight bounds
// on an uncalibrated scorer just encode today's answers a second time. It is
// set to catch the failure MODE (an order-of-magnitude wrong instance), not to
// second-guess a size.
func TestBehaviourMatrix_NeverGrosslyOverProvisions(t *testing.T) {
	// The smallest purchasable instance in the fixture, so a one-task fleet is
	// not judged against a bound below anything it could buy.
	smallest := 0
	for _, it := range matrixCatalog() {
		if smallest == 0 || it.VCPU < smallest {
			smallest = it.VCPU
		}
	}

	for _, w := range matrixWorkloads() {
		for _, p := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
			res := Recommend(matrixInput(w.services, p))
			if res.Primary == nil {
				t.Fatalf("%s/%s: no recommendation", w.name, p)
			}
			demand := res.Signals.Configured.VCPU
			provisioned := float64(res.Primary.VCPU * res.SuggestedPool.MinSize)
			bound := maxf(float64(smallest), 3*demand)
			if provisioned > bound {
				t.Errorf("%s/%s provisions %d x %s = %.0f vCPU for %.2f vCPU of demand "+
					"(bound %.0f). An instance an order of magnitude too large is the "+
					"failure this recommender keeps producing; check whether a sub-score "+
					"is measuring hypothetical capacity instead of achieved occupancy.",
					w.name, p, res.SuggestedPool.MinSize, res.Primary.InstanceType,
					provisioned, demand, bound)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

func matrixRanking(res Result, n int) string {
	var b strings.Builder
	for i, c := range res.Ranked {
		if i >= n {
			break
		}
		fmt.Fprintf(&b, "    %-16s total %.4f  fit %.3f util %.3f cost %.3f mod %.3f  k=%-4d n=%d\n",
			c.InstanceType, c.Total, c.Scores.Fit, c.Scores.Utilisation,
			c.Scores.Cost, c.Scores.Modernity, c.TasksPerInstance, c.InstancesAtFloor)
	}
	return b.String()
}

func matrixReport(w matrixWorkload, p Posture, res Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s / %s\n", w.name, p)
	fmt.Fprintf(&b, "  classification %s, basis %s, R_cfg %.3f -> R_eff %.3f (clamped %s)\n",
		res.Classification, res.Basis, res.Signals.Configured.Ratio,
		res.Signals.Ratio.Effective, res.Signals.Ratio.ClampedTo)
	fmt.Fprintf(&b, "  pool: %v min %d max %d %s\n",
		res.SuggestedPool.InstanceTypes, res.SuggestedPool.MinSize,
		res.SuggestedPool.MaxSize, res.SuggestedPool.CapacityType)
	b.WriteString(matrixRanking(res, 8))
	return b.String()
}

// TestBehaviourMatrix_TableIsComplete guards the table itself: a workload or a
// posture added without a matching cell would otherwise be silently untested.
func TestBehaviourMatrix_TableIsComplete(t *testing.T) {
	want := map[string]bool{}
	for _, w := range matrixWorkloads() {
		for _, p := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
			want[matrixKey(w.name, p)] = true
		}
	}
	var missing, extra []string
	for k := range want {
		if _, ok := wantTable[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range wantTable {
		if !want[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("wantTable has no cell for %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("wantTable has stale cells %v", extra)
	}
}
