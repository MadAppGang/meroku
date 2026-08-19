package main

// Generate-time refusals for the EC2 compute pools (schema v26, FR-52/FR-53).
//
// This file is the only thing standing between a hand-edited `compute:` block
// and a `terraform plan` that fails inside a `locals` expression the user has
// never seen. Every message here follows validateALBConfigMap
// (app/validation.go:494-519): name the offending key, name the fix, and say
// what AWS does otherwise — because the errors these replace name none of the
// three and arrive after a hundred resources already exist.
//
// It runs from preprocessEnvMap (app/deploy.go), AFTER filterDisabledItems and
// AFTER synthesizeDefaultComputePool, so it sees exactly the map the template
// will render: no disabled service's dangling pool reference, and never an
// empty pool list in the zero-config case FR-58 exists to make work.
//
// Everything is read through accessors that tolerate BOTH map shapes. The
// generate path hands us map[string]interface{} (loadEnvToMap ->
// convertToJSONCompatible), but yaml.v2 produces map[interface{}]interface{}
// for nested nodes and a caller that skipped the conversion must not be able to
// bypass a check by accident.

import (
	"fmt"
	"strconv"
	"strings"

	pricingpkg "madappgang.com/meroku/pricing"
)

const (
	computeRuntimeEC2 = "ec2"

	// computeNetworkBridge is the default (D-6): tasks egress through the
	// container instance's own primary interface, which carries a public IP.
	computeNetworkBridge = "bridge"
	computeNetworkAWSVPC = "awsvpc"

	computeAMIFamilyAL2023      = "al2023"
	computeAMIFamilyAL2023ARM64 = "al2023_arm64"
	computeAMIFamilyAL2023GPU   = "al2023_gpu"

	computeArchX86   = "x86_64"
	computeArchARM64 = "arm64"
)

// computePoolView is one entry of compute.pools, read once and normalised, so
// that twelve rules do not each re-implement "absent means bridge".
type computePoolView struct {
	index         int // position in compute.pools, 1-based, for the duplicate message
	name          string
	enabled       bool
	instanceTypes []string
	capacityType  string
	onDemandBase  *int
	minSize       *int
	maxSize       *int
	networkMode   string
	assumeEgress  bool
	amiFamily     string
}

// computeUnitView is one thing that can run on a pool: the backend, or a named
// service. The key fields carry the exact YAML path so a message can point at
// the line the user has to edit rather than at a concept.
type computeUnitView struct {
	label      string // "the backend" | `services["api"]`
	runtimeKey string // workload.backend_runtime | services["api"].runtime
	poolKey    string
	xrayKey    string
	memoryKey  string

	runtime   string
	pool      string
	xray      bool
	memoryMiB int
	hasMemory bool
}

func (u computeUnitView) isEC2() bool { return u.runtime == computeRuntimeEC2 }

// validateComputeConfigMap refuses a compute configuration that cannot work.
//
// It returns the first refusal rather than a list: the rules are independent,
// the fix for each is a single edit, and one message a user reads to the end is
// worth more than five they skim. Warnings (FR-53) are printed by
// computeWarnings and never returned.
func validateComputeConfigMap(envMap map[string]interface{}) error {
	pools := computePoolViews(envMap)
	units := computeUnitViews(envMap)

	if err := computeCheckDuplicatePools(pools); err != nil {
		return err
	}
	for _, p := range pools {
		if err := computeCheckPool(p); err != nil {
			return err
		}
	}
	if err := computeCheckUnits(units, pools); err != nil {
		return err
	}

	for _, w := range computeWarnings(envMap) {
		fmt.Printf("⚠️  %s\n", w)
	}
	return nil
}

// computeCheckDuplicatePools is rule 7 (EC-13).
func computeCheckDuplicatePools(pools []computePoolView) error {
	seen := make(map[string]int, len(pools))
	for _, p := range pools {
		if p.name == "" {
			continue
		}
		if first, dup := seen[p.name]; dup {
			return fmt.Errorf(
				"compute.pools defines the name %q twice — entries %d and %d.\n\n"+
					"Terraform keys every pool resource by name with for_each, so the two entries collapse\n"+
					"into one Auto Scaling group and one capacity provider, silently, with whichever set of\n"+
					"settings for_each happened to keep. Nothing in the plan says a pool went missing.\n\n"+
					"Rename one of them, or delete the duplicate.",
				p.name, first, p.index)
		}
		seen[p.name] = p.index
	}
	return nil
}

// computeCheckPool runs every rule that can be decided from one pool alone:
// rules 2, 3, 4, 5, 10 and 11.
func computeCheckPool(p computePoolView) error {
	key := p.key()

	// Rule 2 — an ASG whose floor is above its ceiling.
	if p.minSize != nil && p.maxSize != nil && *p.minSize > *p.maxSize {
		return fmt.Errorf(
			"%s.min_size is %d but %s.max_size is %d.\n\n"+
				"An Auto Scaling group cannot have a minimum above its maximum: AWS rejects\n"+
				"CreateAutoScalingGroup outright, so the pool never exists, and every task placed on it\n"+
				"sits in PROVISIONING with no capacity to run on.\n\n"+
				"Either lower %s.min_size to %d or raise %s.max_size to %d.",
			key, *p.minSize, key, *p.maxSize, key, *p.maxSize, key, *p.minSize)
	}

	// Rule 3 — a base that means nothing where it is written. Both non-base
	// capacity types are refused, because the field is inert under either and
	// under spot it is inert in the dangerous direction: the module renders
	// `capacity_type == "spot_with_base" ? on_demand_base : 0`
	// (modules/workloads/ec2_capacity.tf), so writing it turns the pool 100%
	// spot AND suppresses the pure-spot advisory below, which fires only when
	// on_demand_base is absent. The operator asks for one guaranteed instance
	// and is left with none and no warning.
	if p.capacityType != "spot_with_base" && p.onDemandBase != nil {
		consequence := "Under on_demand every instance is on-demand already, so the value is ignored — and a\n" +
			"config that reads as \"one guaranteed instance, the rest spot\" quietly costs full price\n" +
			"for all of them."
		if p.capacityType == "spot" {
			consequence = "Under spot the module renders the base as 0, so the pool is 100% spot and the instance\n" +
				"you asked to keep on-demand is never launched. A capacity rebalance can then reclaim\n" +
				"every instance at once with nothing left to serve traffic — the exact outcome\n" +
				"on_demand_base was written to prevent."
		}
		return fmt.Errorf(
			"%s.capacity_type is %q but %s.on_demand_base is set to %d.\n\n"+
				"on_demand_base is the number of INSTANCES that must be on-demand before the pool starts\n"+
				"buying spot, and it only means anything under capacity_type: spot_with_base.\n\n"+
				"%s\n\n"+
				"Either drop %s.on_demand_base, or set %s.capacity_type: spot_with_base.",
			key, p.capacityType, key, *p.onDemandBase, consequence, key, key)
	}

	// Rules 4, 5 and 10 — the AMI and the instances have to agree.
	if err := computeCheckAMIFamily(p); err != nil {
		return err
	}

	// Rule 11 — awsvpc with no egress path (C-1 / D-6 / D-7).
	if p.networkMode == computeNetworkAWSVPC && !p.assumeEgress {
		return fmt.Errorf(
			"%s.network_mode is \"awsvpc\" but this environment has no egress path.\n\n"+
				"Under awsvpc each ECS task gets its own network interface, and AWS assigns that interface a\n"+
				"private address only — assign_public_ip is Fargate-only, and map_public_ip_on_launch on the\n"+
				"subnet applies to the instance's primary interface, not to the task's. The task starts, passes\n"+
				"its health check and serves traffic, and every outbound call it makes — S3, SES, SQS, Cognito,\n"+
				"Secrets Manager, ECS Exec, any third-party API — times out silently.\n\n"+
				"Either set %s.network_mode: bridge (the default; tasks egress through the\n"+
				"instance's own public interface at no cost), or set %s.assume_egress: true to\n"+
				"declare that the subnets you place tasks in route 0.0.0.0/0 to a NAT gateway you manage\n"+
				"yourself. meroku does not create one: this VPC is public-subnet-only by design.",
			key, key, key)
	}

	return nil
}

// computeCheckAMIFamily is rules 4, 5 and 10.
//
// The instance type's architecture and whether it carries a GPU are read from
// its NAME, because generation makes no AWS calls (it must work with no
// credentials). computeInstanceShape is therefore deliberately unsure of
// itself: an unrecognised family produces no refusal at all, so a family AWS
// ships next month cannot make a working config stop generating.
func computeCheckAMIFamily(p computePoolView) error {
	key := p.key()

	switch p.amiFamily {
	case computeAMIFamilyAL2023GPU:
		// Rule 4 — a GPU AMI with nothing to drive.
		sawUnknown := false
		for _, t := range p.instanceTypes {
			s := computeInstanceShape(t)
			if !s.gpuKnown {
				sawUnknown = true
				continue
			}
			if s.gpu {
				return computeCheckArch(p, computeArchX86)
			}
		}
		if sawUnknown || len(p.instanceTypes) == 0 {
			return computeCheckArch(p, computeArchX86)
		}
		return fmt.Errorf(
			"%s.ami_family is \"al2023_gpu\" but none of %s.instance_types has a GPU:\n"+
				"  %s\n\n"+
				"The GPU-optimized ECS AMI ships the NVIDIA driver and the container runtime hook, and it is\n"+
				"the only reason to choose that family. On a CPU-only instance it boots and registers with the\n"+
				"cluster, so nothing looks wrong — but any task declaring a GPU resource requirement stays in\n"+
				"PROVISIONING forever, and every instance in the pool costs more for a driver nothing uses.\n\n"+
				"Either set %s.ami_family: al2023 (or al2023_arm64), or list a GPU instance type\n"+
				"such as g5.xlarge in %s.instance_types.",
			key, key, strings.Join(p.instanceTypes, ", "), key, key)

	case computeAMIFamilyAL2023ARM64:
		return computeCheckArch(p, computeArchARM64)

	default: // al2023, and anything the schema has not defined
		return computeCheckArch(p, computeArchX86)
	}
}

// computeCheckArch is rule 5 (arm64 family, x86 instance) and rule 10, its
// mirror (x86 family, arm64 instance — C-11).
func computeCheckArch(p computePoolView, want string) error {
	key := p.key()

	for _, t := range p.instanceTypes {
		s := computeInstanceShape(t)
		if s.arch == "" || s.arch == want {
			continue
		}

		// The second fix is only offered when it exists: there is no arm64 GPU
		// family in this schema, so an arm64 type in an al2023_gpu pool can
		// only be removed.
		secondFix := fmt.Sprintf(", or set %s.ami_family: %s", key, computeAMIFamilyAL2023ARM64)
		if want == computeArchARM64 {
			secondFix = fmt.Sprintf(", or set %s.ami_family: %s", key, computeAMIFamilyAL2023)
		} else if p.amiFamily == computeAMIFamilyAL2023GPU {
			secondFix = ""
		}

		return fmt.Errorf(
			"%s.ami_family is %q but %s.instance_types contains %q, which is %s.\n\n"+
				"An AMI only boots on the architecture it was built for. The Auto Scaling group launches the\n"+
				"instance, EC2 rejects it with \"The architecture of the specified instance type does not match\n"+
				"the architecture of the specified AMI\", and the failure is recorded as a scaling activity —\n"+
				"not as a Terraform error and not as an ECS event. The pool simply never gains capacity, and\n"+
				"the tasks placed on it wait in PROVISIONING.\n\n"+
				"Remove %q from %s.instance_types%s.",
			key, p.amiFamily, key, t, s.arch, t, key, secondFix)
	}
	return nil
}

// computeCheckUnits runs every rule that needs a workload and its pool
// together: rules 9, 1, 8, 6 and 12.
func computeCheckUnits(units []computeUnitView, pools []computePoolView) error {
	byName := make(map[string]computePoolView, len(pools))
	for _, p := range pools {
		if _, exists := byName[p.name]; !exists {
			byName[p.name] = p
		}
	}

	for _, u := range units {
		if !u.isEC2() {
			continue
		}

		// Rule 9 — an EC2 unit that names no pool at all. Synthesis has
		// already run, so if we are here there ARE pools and meroku will not
		// guess which one was meant (DEV-9).
		if u.pool == "" {
			return fmt.Errorf(
				"%s is \"ec2\" but %s is empty.\n\n"+
					"%s\n\n"+
					"A pool is only synthesized when the environment defines none at all, so meroku will not\n"+
					"choose one of yours on your behalf. Left as it is, the module indexes its pool map with an\n"+
					"empty key and terraform plan fails with `Error: Invalid index`, naming a locals expression\n"+
					"that is not in this file.\n\n"+
					"Either set %s to one of those, or set %s: fargate.",
				u.runtimeKey, u.poolKey, computeDefinedPoolsSentence(pools), u.poolKey, u.runtimeKey)
		}

		pool, defined := byName[u.pool]
		// Rule 1 — a reference to a pool that does not exist (AC-43).
		if !defined {
			return fmt.Errorf(
				"%s is %q, but compute.pools defines no pool with that name.\n\n"+
					"%s\n\n"+
					"Terraform builds its pool map from compute.pools and then indexes it with this value, so a\n"+
					"name that is not in the map fails the plan with `Error: Invalid index` against a locals\n"+
					"expression rather than against this file.\n\n"+
					"Either set %s to one of those, or add a pool named %q to compute.pools.",
				u.poolKey, u.pool, computeDefinedPoolsSentence(pools), u.poolKey, u.pool)
		}

		// Rule 8 — a reference to a pool that is defined but disabled
		// (DEV-9 / C-7). This is rule 1's failure from the inside: the pool is
		// in the YAML, so it reads as correct, and the module has already
		// filtered it out by the time anything indexes it.
		if !pool.enabled {
			return fmt.Errorf(
				"%s is %q, but %s.enabled is false.\n\n"+
					"A disabled pool is filtered out of the module's pool map before any resource indexes it, so\n"+
					"terraform plan fails with\n\n"+
					"  Error: Invalid index\n"+
					"    aws_ecs_capacity_provider.pool[%q]\n\n"+
					"which names a locals expression the user has never seen and no line of this YAML.\n\n"+
					"Either set %s.enabled: true, or move %s to an enabled pool — or back to\n"+
					"%s: fargate.",
				u.poolKey, u.pool, pool.key(), u.pool, pool.key(), u.label, u.runtimeKey)
		}

		// Rule 6 — a task that no instance in the pool can hold.
		if err := computeCheckMemoryFits(u, pool); err != nil {
			return err
		}

		// Rule 12 — X-Ray cannot work under bridge, in either direction
		// (C-1 ripple, DEV-30).
		if u.xray && pool.networkMode == computeNetworkBridge {
			return fmt.Errorf(
				"%s is true, but %s runs on %s, whose network_mode is \"bridge\".\n\n"+
					"X-Ray is broken under bridge in both directions. The backend's ADOT sidecar declares fixed\n"+
					"host ports (2000, 4317, 4318, 55681), so under bridge it binds the instance's ports and a\n"+
					"second task carrying the same sidecar cannot place on that instance — which surfaces as a\n"+
					"task stuck in PROVISIONING with a resource-constraint event, mentioning nothing about\n"+
					"X-Ray. A service's sidecar omits the host ports and so does not collide, but its\n"+
					"application container sits in a separate network namespace from the daemon, so\n"+
					"AWS_XRAY_DAEMON_ADDRESS's default of 127.0.0.1:2000 never resolves and every trace is\n"+
					"dropped in silence.\n\n"+
					"Either set %s: false, or move %s to a pool with\n"+
					"network_mode: awsvpc and assume_egress: true.",
				u.xrayKey, u.label, pool.key(), u.xrayKey, u.label)
		}
	}

	return nil
}

// computeCheckMemoryFits is rule 6: a task reservation larger than the biggest
// instance the pool can launch never places, and ECS reports it as a task stuck
// in PROVISIONING with no error attached to anything.
//
// It is skipped whenever a single instance type in the pool has no known
// memory, because "the largest instance" is then unknown and a refusal would be
// a guess. computeInstanceMemoryMiB is generous by construction for the same
// reason — it may over-state a size, never under-state one, so this rule can
// only ever fail to fire, not fire wrongly.
func computeCheckMemoryFits(u computeUnitView, pool computePoolView) error {
	if !u.hasMemory || u.memoryMiB <= 0 || len(pool.instanceTypes) == 0 {
		return nil
	}

	largest := 0
	largestType := ""
	for _, t := range pool.instanceTypes {
		mib, known := computeInstanceMemoryMiB(t)
		if !known {
			return nil
		}
		if mib > largest {
			largest, largestType = mib, t
		}
	}
	if largest == 0 || u.memoryMiB <= largest {
		return nil
	}

	return fmt.Errorf(
		"%s is %d MiB, but the largest instance in %s is %s with %d MiB.\n\n"+
			"An ECS task is placed on one instance, never spread across several, so a reservation bigger\n"+
			"than the biggest instance in the pool can never be satisfied. The task stays in PROVISIONING\n"+
			"and neither the service events nor the plan say why. In practice the usable figure is lower\n"+
			"still — the ECS agent and the operating system take their share of every instance.\n\n"+
			"Either lower %s, or add a larger instance type to\n"+
			"%s.instance_types.",
		u.memoryKey, u.memoryMiB, pool.key(), largestType, largest, u.memoryKey, pool.key())
}

// computeDefinedPoolsSentence lists the pools that do exist, so that a refusal
// about a name is answerable without opening another file.
func computeDefinedPoolsSentence(pools []computePoolView) string {
	if len(pools) == 0 {
		return "compute.pools is empty."
	}
	names := make([]string, 0, len(pools))
	for _, p := range pools {
		label := fmt.Sprintf("%q", p.name)
		if !p.enabled {
			label += " (disabled)"
		}
		names = append(names, label)
	}
	return "Defined pools: " + strings.Join(names, ", ") + "."
}

// computeWarnings is FR-53: things worth saying that are not worth refusing.
// They are returned rather than printed so that a test can read them.
func computeWarnings(envMap map[string]interface{}) []string {
	pools := computePoolViews(envMap)
	if len(pools) == 0 {
		return nil
	}
	units := computeUnitViews(envMap)

	albEnabled := false
	if alb, ok := yamlSubMap(envMap, "alb"); ok {
		albEnabled, _ = alb["enabled"].(bool)
	}

	referenced := make(map[string][]string, len(pools))
	for _, u := range units {
		if u.isEC2() && u.pool != "" {
			referenced[u.pool] = append(referenced[u.pool], u.label)
		}
	}

	var out []string
	for _, p := range pools {
		users := referenced[p.name]

		// A pool nothing runs on still costs money the moment min_size is
		// above zero.
		if len(users) == 0 {
			out = append(out, fmt.Sprintf(
				"%s is referenced by no workload. Nothing will be placed on it, and it still runs "+
					"min_size instances. Set workload.backend_compute_pool or a service's compute_pool to "+
					"%q, or delete the pool.",
				p.key(), p.name))
			continue
		}

		// Pure spot behind an ALB: an interruption can take the whole pool at
		// once, because every instance in it is interruptible.
		if p.capacityType == "spot" && p.onDemandBase == nil && albEnabled {
			out = append(out, fmt.Sprintf(
				"%s.capacity_type is \"spot\" with no on-demand base, and %s serves traffic through the "+
					"ALB. A capacity rebalance can reclaim every instance in the pool at once, and the target "+
					"group has nothing left to route to. Consider capacity_type: spot_with_base with "+
					"on_demand_base: 1.",
				p.key(), strings.Join(users, ", ")))
		}

		// The one path in this design no Fargate environment exercises: an
		// EC2 service reached through API Gateway resolves via Cloud Map SRV
		// records, because under bridge the host port is assigned dynamically.
		if p.networkMode == computeNetworkBridge && !albEnabled {
			out = append(out, fmt.Sprintf(
				"%s uses network_mode: bridge and this environment has no ALB, so %s is reached through "+
					"API Gateway over Cloud Map SRV records rather than A records — the dynamic host port "+
					"makes SRV the only option. It is supported; it is called out because no Fargate "+
					"environment exercises it.",
				p.key(), strings.Join(users, ", ")))
		}
	}
	return out
}

func (p computePoolView) key() string {
	return fmt.Sprintf("compute.pools[%q]", p.name)
}

// computePoolViews reads compute.pools out of the raw map, normalising the
// defaults every rule below would otherwise have to remember.
func computePoolViews(envMap map[string]interface{}) []computePoolView {
	compute, ok := yamlSubMap(envMap, "compute")
	if !ok {
		return nil
	}

	raw := computeMapList(compute, "pools")
	out := make([]computePoolView, 0, len(raw))
	for i, node := range raw {
		p := computePoolView{
			index:         i + 1,
			name:          computeStringField(node, "name"),
			enabled:       true,
			instanceTypes: computeStringListField(node, "instance_types"),
			capacityType:  computeStringField(node, "capacity_type"),
			onDemandBase:  computeIntField(node, "on_demand_base"),
			minSize:       computeIntField(node, "min_size"),
			maxSize:       computeIntField(node, "max_size"),
			networkMode:   computeStringField(node, "network_mode"),
			amiFamily:     computeStringField(node, "ami_family"),
		}
		if enabled, present := computeBoolField(node, "enabled"); present {
			p.enabled = enabled
		}
		// assume_egress absent and false mean the same thing to every rule:
		// the operator has not asserted an egress path (D-7).
		p.assumeEgress, _ = computeBoolField(node, "assume_egress")

		// Absence and the default are the same value everywhere downstream —
		// Go, the template, and variables.tf all default alike (D-6).
		if p.networkMode == "" {
			p.networkMode = computeNetworkBridge
		}
		if p.amiFamily == "" {
			p.amiFamily = computeAMIFamilyAL2023
		}
		if p.capacityType == "" {
			p.capacityType = "on_demand"
		}
		out = append(out, p)
	}
	return out
}

// computeUnitViews reads the backend and every (already filtered) service.
//
// Scheduled tasks and event-processor tasks are absent on purpose: schema v26
// gives neither a runtime nor a compute_pool, so neither can name a pool and
// neither can be wrong about one.
func computeUnitViews(envMap map[string]interface{}) []computeUnitView {
	var out []computeUnitView

	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		u := computeUnitView{
			label:      "the backend",
			runtimeKey: "workload.backend_runtime",
			poolKey:    "workload.backend_compute_pool",
			xrayKey:    "workload.xray_enabled",
			memoryKey:  "workload.backend_memory",
			runtime:    computeStringField(workload, "backend_runtime"),
			pool:       computeStringField(workload, "backend_compute_pool"),
		}
		u.xray, _ = computeBoolField(workload, "xray_enabled")
		// backend_memory is a string of MiB in this schema, unlike a service's
		// int. computeIntField reads both.
		if mem := computeIntField(workload, "backend_memory"); mem != nil {
			u.memoryMiB, u.hasMemory = *mem, true
		}
		out = append(out, u)
	}

	for _, svc := range computeMapList(envMap, "services") {
		name := computeStringField(svc, "name")
		u := computeUnitView{
			label:      fmt.Sprintf("services[%q]", name),
			runtimeKey: fmt.Sprintf("services[%q].runtime", name),
			poolKey:    fmt.Sprintf("services[%q].compute_pool", name),
			xrayKey:    fmt.Sprintf("services[%q].xray_enabled", name),
			memoryKey:  fmt.Sprintf("services[%q].memory", name),
			runtime:    computeStringField(svc, "runtime"),
			pool:       computeStringField(svc, "compute_pool"),
		}
		u.xray, _ = computeBoolField(svc, "xray_enabled")
		if mem := computeIntField(svc, "memory"); mem != nil {
			u.memoryMiB, u.hasMemory = *mem, true
		}
		out = append(out, u)
	}

	return out
}

// --- accessors ---------------------------------------------------------
//
// yaml.v2 gives map[interface{}]interface{} for every nested node
// (app/migrations.go:234, :463) while the generate path converts to
// map[string]interface{}. Everything below accepts either, so no check can be
// bypassed by whichever loader a caller used (CON-4).

// computeNodeMap normalises one YAML node to a string-keyed map.
func computeNodeMap(raw interface{}) (map[string]interface{}, bool) {
	switch m := raw.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			out[fmt.Sprintf("%v", k)] = v
		}
		return out, true
	default:
		return nil, false
	}
}

// computeMapList reads a list of mappings — compute.pools, services — skipping
// any element that is not a mapping rather than asserting on it.
func computeMapList(container map[string]interface{}, key string) []map[string]interface{} {
	raw, exists := container[key]
	if !exists || raw == nil {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := computeNodeMap(item); ok {
			out = append(out, m)
		}
	}
	return out
}

func computeStringField(node map[string]interface{}, key string) string {
	v, exists := node[key]
	if !exists || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func computeStringListField(node map[string]interface{}, key string) []string {
	v, exists := node[key]
	if !exists || v == nil {
		return nil
	}
	switch list := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return list
	default:
		return nil
	}
}

// computeBoolField reports the value and whether the key was present at all.
// Absent, false and true are three states the messages tell apart (D-7).
func computeBoolField(node map[string]interface{}, key string) (bool, bool) {
	v, exists := node[key]
	if !exists || v == nil {
		return false, false
	}
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(b))
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}

// computeIntField returns nil for an absent key, so that min_size: 0 —
// meaningful, CON-7 — is never confused with min_size missing. Strings are
// parsed because workload.backend_memory is one.
func computeIntField(node map[string]interface{}, key string) *int {
	v, exists := node[key]
	if !exists || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		return &n
	case int32:
		out := int(n)
		return &out
	case int64:
		out := int(n)
		return &out
	case float64:
		out := int(n)
		return &out
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

// --- what a name can tell you about an instance type -------------------

// computeInstanceFacts is what an instance type's NAME reveals. Generation
// makes no AWS calls, so this is all a refusal may be built on — and every
// field carries a "known" flag, because a family this parser has never seen
// must produce silence rather than a wrong refusal.
type computeInstanceFacts struct {
	family string // the letters before the generation digit: "m", "c", "g", "im"
	arch   string // computeArchX86 | computeArchARM64 | "" when unknown

	gpu      bool
	gpuKnown bool
}

// computeInstanceShape parses "m7i-flex.large" into family "m", generation 7,
// attributes "i-flex", size "large".
//
// Architecture comes from the attribute letters: a Graviton type always carries
// a "g" there (m7g, c7gn, im4gn, x2gd, g5g), and nothing else does. a1 is the
// one Graviton family that predates the convention and is special-cased. A name
// that does not parse — or a family whose architecture the convention does not
// settle, like mac — reports "".
func computeInstanceShape(name string) computeInstanceFacts {
	facts := computeInstanceFacts{}

	prefix := name
	if dot := strings.Index(name, "."); dot >= 0 {
		prefix = name[:dot]
	}
	prefix = strings.ToLower(strings.TrimSpace(prefix))

	i := 0
	for i < len(prefix) && prefix[i] >= 'a' && prefix[i] <= 'z' {
		i++
	}
	j := i
	for j < len(prefix) && prefix[j] >= '0' && prefix[j] <= '9' {
		j++
	}
	if i == 0 || j == i {
		return facts // no family letters, or no generation digit: not a type we can read
	}

	facts.family = prefix[:i]
	attrs := prefix[j:]
	if dash := strings.Index(attrs, "-"); dash >= 0 {
		attrs = attrs[:dash] // "i-flex" -> "i"
	}

	switch facts.family {
	case "mac":
		// mac1 is x86, mac2 is arm64, and neither runs ECS tasks. Say nothing.
	case "a":
		facts.arch = computeArchARM64
	default:
		if strings.ContainsRune(attrs, 'g') {
			facts.arch = computeArchARM64
		} else {
			facts.arch = computeArchX86
		}
	}

	switch facts.family {
	case "g", "gr", "p":
		facts.gpu, facts.gpuKnown = true, true
	case "t", "m", "c", "r", "x", "z", "i", "d", "h", "u", "a", "hpc", "im", "is":
		facts.gpu, facts.gpuKnown = false, true
	default:
		// inf, trn, dl, f, vt and anything AWS ships next: accelerators of some
		// kind, or unknown. Neither answer is safe to refuse on.
	}

	return facts
}

// computeInstanceMemoryMiB answers "how much memory does one of these have"
// without asking AWS, for rule 6 only.
//
// The fallback catalogue is consulted first because it is measured data. Beyond
// its dozen records the figure is derived from the naming convention: within a
// family, memory doubles with each size step, so one number per family class
// and one factor per size covers every current-generation type. Families whose
// sizes do not follow it — i3 at 15.25 GiB per large, x1e, p, the accelerators
// — are deliberately absent, and an absent family means rule 6 does not fire.
//
// Where the derivation is imprecise it errs HIGH (g5g.xlarge really has 8 GiB,
// not the 16 this returns), so the rule can only fail to catch a real problem,
// never invent one.
func computeInstanceMemoryMiB(name string) (int, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, rec := range pricingpkg.GetFallbackCatalog().InstanceTypes {
		if strings.EqualFold(rec.InstanceType, name) && rec.MemoryMiB > 0 {
			return rec.MemoryMiB, true
		}
	}

	dot := strings.Index(name, ".")
	if dot < 0 {
		return 0, false
	}
	facts := computeInstanceShape(name)
	if facts.family == "" {
		return 0, false
	}

	// GiB of memory in one <family>.large, the unit every other size is a
	// multiple of.
	perLarge := map[string]int{
		"t": 8, "m": 8, "a": 4, "c": 4, "r": 16, "z": 16, "g": 8,
	}[facts.family]
	if perLarge == 0 {
		return 0, false
	}

	factor, known := map[string]float64{
		"nano": 0.0625, "micro": 0.125, "small": 0.25, "medium": 0.5,
		"large": 1, "xlarge": 2, "2xlarge": 4, "3xlarge": 6, "4xlarge": 8,
		"6xlarge": 12, "8xlarge": 16, "9xlarge": 18, "12xlarge": 24,
		"16xlarge": 32, "18xlarge": 36, "24xlarge": 48, "32xlarge": 64,
		"48xlarge": 96,
	}[name[dot+1:]]
	if !known {
		return 0, false // metal, and anything else off the ladder
	}

	return int(float64(perLarge) * factor * 1024), true
}
