package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// preflightAPIError is a smithy.APIError whose Error() carries the whole
// message, the way a real SDK error does. stubAPIError in state_reconnect_test.go
// returns only the code, which is not enough to test summarizeAWSError.
type preflightAPIError struct {
	code    string
	message string
}

func (e *preflightAPIError) Error() string {
	return "operation error EC2: RunInstances, https response error StatusCode: 403, " +
		"RequestID: 00000000-0000-0000-0000-000000000000, api error " + e.code + ": " + e.message
}
func (e *preflightAPIError) ErrorCode() string             { return e.code }
func (e *preflightAPIError) ErrorMessage() string          { return e.message }
func (e *preflightAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

// The real SCP denial from D-10, verbatim apart from the account numbers, which
// are synthetic. It is used as-is because the exact shape of this string is what
// the check has to survive: an SCP ARN worth showing, followed by a kilobyte of
// base64 that is not.
const scpDenialMessage = "You are not authorized to perform this operation. " +
	"User: arn:aws:sts::000000000000:assumed-role/Admin/Jack is not authorized to perform: " +
	"ec2:RunInstances on resource: arn:aws:ec2:ap-southeast-2:000000000000:instance/* " +
	"with an explicit deny in a service control policy: " +
	"arn:aws:organizations::000000000001:policy/o-fake0000/service_control_policy/p-fake0000. " +
	"Encoded authorization failure message: WRWZPoc2Hiv1Ls2Ra9rQ5YM1xR1iSc9BiIC3ZQWJHsMMVsj6fK2fTRRvnKx7Kb"

type preflightFakeECS struct {
	calls int
	value string
	err   error
}

func (f *preflightFakeECS) ListAccountSettings(_ context.Context, _ *ecs.ListAccountSettingsInput, _ ...func(*ecs.Options)) (*ecs.ListAccountSettingsOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.value == "" {
		return &ecs.ListAccountSettingsOutput{}, nil
	}
	return &ecs.ListAccountSettingsOutput{Settings: []ecstypes.Setting{{
		Name:  ecstypes.SettingNameAwsvpcTrunking,
		Value: aws.String(f.value),
	}}}, nil
}

type preflightFakeEC2 struct {
	// byType decides the answer per instance type. A missing entry means
	// DryRunOperation, i.e. the launch would have succeeded.
	byType map[string]error
	seen   []*ec2.RunInstancesInput
}

func (f *preflightFakeEC2) RunInstances(_ context.Context, in *ec2.RunInstancesInput, _ ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	f.seen = append(f.seen, in)
	if err, ok := f.byType[string(in.InstanceType)]; ok {
		return nil, err
	}
	return nil, &preflightAPIError{
		code:    "DryRunOperation",
		message: "Request would have succeeded, but DryRun flag is set",
	}
}

type preflightFakeSSM struct {
	names []string
	value string
	err   error
}

func (f *preflightFakeSSM) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	f.names = append(f.names, aws.ToString(in.Name))
	if f.err != nil {
		return nil, f.err
	}
	return &ssm.GetParameterOutput{Parameter: &ssmtypes.Parameter{Value: aws.String(f.value)}}, nil
}

type preflightFakeIAM struct {
	calls int
	err   error
}

func (f *preflightFakeIAM) GetRole(_ context.Context, _ *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::000000000000:role/x")}}, nil
}

// happyClients wires four fakes that all answer successfully.
func happyClients() (computePreflightClients, *preflightFakeECS, *preflightFakeEC2, *preflightFakeSSM, *preflightFakeIAM) {
	ecsFake := &preflightFakeECS{value: "enabled"}
	ec2Fake := &preflightFakeEC2{}
	ssmFake := &preflightFakeSSM{value: "ami-08fc1510075b003e4"}
	iamFake := &preflightFakeIAM{}
	return computePreflightClients{ECS: ecsFake, EC2: ec2Fake, SSM: ssmFake, IAM: iamFake},
		ecsFake, ec2Fake, ssmFake, iamFake
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

// ---------------------------------------------------------------------------
// Planning — the gate, and the de-duplication
// ---------------------------------------------------------------------------

func TestPlanComputePreflight_NoPoolsPlansNothing(t *testing.T) {
	// This is the property the whole feature is gated on: every environment
	// created before schema v26 has no compute block at all, and must produce
	// no plan, hence no output and no AWS calls.
	plan := planComputePreflight(Env{Region: "ap-southeast-2"})

	if len(plan.PoolNames) != 0 {
		t.Fatalf("an environment with no compute block must plan no pools, got %v", plan.PoolNames)
	}
	if len(plan.Probes) != 0 || len(plan.AWSVPCPools) != 0 {
		t.Fatalf("an environment with no compute block must plan no checks, got %+v", plan)
	}
}

func TestPlanComputePreflight_DisabledPoolIsIgnored(t *testing.T) {
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "off", Enabled: boolPtr(false), InstanceTypes: []string{"m7i-flex.large"}},
		{Name: "on", InstanceTypes: []string{"t3.small"}}, // absent enabled == enabled
	}}})

	if got := plan.PoolNames; len(got) != 1 || got[0] != "on" {
		t.Fatalf("expected only the enabled pool, got %v", got)
	}
	if len(plan.Probes) != 1 || plan.Probes[0].InstanceType != "t3.small" {
		t.Fatalf("a disabled pool must contribute no probes, got %+v", plan.Probes)
	}
}

func TestPlanComputePreflight_TrunkingOnlyForAWSVPCPools(t *testing.T) {
	tests := []struct {
		name        string
		networkMode string
		wantChecked bool
	}{
		{"absent means bridge", "", false},
		{"explicit bridge", "bridge", false},
		{"awsvpc opts in", "awsvpc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
				{Name: "general", NetworkMode: tt.networkMode, InstanceTypes: []string{"m7i-flex.large"}},
			}}})
			if got := len(plan.AWSVPCPools) > 0; got != tt.wantChecked {
				t.Fatalf("network_mode %q: trunking checked = %v, want %v", tt.networkMode, got, tt.wantChecked)
			}
		})
	}
}

func TestPlanComputePreflight_DedupesProbesAcrossPools(t *testing.T) {
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "a", InstanceTypes: []string{"m7i-flex.large", "t3.small"}},
		{Name: "b", InstanceTypes: []string{"m7i-flex.large"}}, // same family, same type
	}}})

	if len(plan.Probes) != 2 {
		t.Fatalf("identical (family, type) pairs must collapse into one probe, got %d: %+v",
			len(plan.Probes), plan.Probes)
	}
	shared := plan.Probes[0]
	if shared.InstanceType != "m7i-flex.large" {
		t.Fatalf("probes must keep YAML order, got %q first", shared.InstanceType)
	}
	if len(shared.Pools) != 2 || shared.Pools[0] != "a" || shared.Pools[1] != "b" {
		t.Fatalf("a collapsed probe must name every pool that asked for it, got %v", shared.Pools)
	}
	if got := shared.describe(); !strings.Contains(got, `"a"`) || !strings.Contains(got, `"b"`) {
		t.Fatalf("describe() must name both pools, got %q", got)
	}
}

func TestPlanComputePreflight_DifferentAMIFamiliesDoNotCollapse(t *testing.T) {
	// The same instance type under a different image is a different launch, and
	// an SCP or an AMI-architecture mismatch can answer differently.
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "x86", AMIFamily: "al2023", InstanceTypes: []string{"m7i-flex.large"}},
		{Name: "gpu", AMIFamily: "al2023_gpu", InstanceTypes: []string{"m7i-flex.large"}},
	}}})

	if len(plan.Probes) != 2 {
		t.Fatalf("different ami_family must probe separately, got %+v", plan.Probes)
	}
}

func TestPlanComputePreflight_ExplicitAMIIDOverridesFamily(t *testing.T) {
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "pinned", AMIFamily: "al2023", AMIID: "ami-0f00", InstanceTypes: []string{"t3.micro"}},
	}}})

	if len(plan.Probes) != 1 {
		t.Fatalf("expected one probe, got %+v", plan.Probes)
	}
	if plan.Probes[0].AMIID != "ami-0f00" || plan.Probes[0].AMIFamily != "" {
		t.Fatalf("ami_id must win over ami_family, got %+v", plan.Probes[0])
	}
}

func TestPlanComputePreflight_DefaultsAMIFamilyToAL2023(t *testing.T) {
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", InstanceTypes: []string{"m7i-flex.large"}},
	}}})

	if plan.Probes[0].AMIFamily != "al2023" {
		t.Fatalf("an absent ami_family must default to al2023 like variables.tf does, got %q",
			plan.Probes[0].AMIFamily)
	}
}

func TestPlanComputePreflight_CapsProbeCount(t *testing.T) {
	types := make([]string, 0, maxComputeLaunchProbes+3)
	for i := 0; i < maxComputeLaunchProbes+3; i++ {
		types = append(types, string(rune('a'+i))+".large")
	}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "wide", InstanceTypes: types},
	}}})

	if len(plan.Probes) != maxComputeLaunchProbes {
		t.Fatalf("probes must be capped at %d, got %d", maxComputeLaunchProbes, len(plan.Probes))
	}
	if plan.ProbesDropped != 3 {
		t.Fatalf("the cap must be reported, want 3 dropped, got %d", plan.ProbesDropped)
	}
}

func TestPlanComputePreflight_SkipsBlankInstanceTypes(t *testing.T) {
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", InstanceTypes: []string{"", "  ", " t3.small "}},
	}}})

	if len(plan.Probes) != 1 || plan.Probes[0].InstanceType != "t3.small" {
		t.Fatalf("blank entries must be dropped and surrounding space trimmed, got %+v", plan.Probes)
	}
}

// ---------------------------------------------------------------------------
// The gate, end to end
// ---------------------------------------------------------------------------

func TestCheckComputePools_SilentAndCallsNothingWithoutPools(t *testing.T) {
	clients, ecsFake, ec2Fake, ssmFake, iamFake := happyClients()

	lines := checkComputePools(context.Background(), planComputePreflight(Env{}), clients)

	if len(lines) != 0 {
		t.Fatalf("an environment with no pools must print nothing, got %q", joined(lines))
	}
	if ecsFake.calls+len(ec2Fake.seen)+len(ssmFake.names)+iamFake.calls != 0 {
		t.Fatalf("an environment with no pools must make no AWS calls, got ecs=%d ec2=%d ssm=%d iam=%d",
			ecsFake.calls, len(ec2Fake.seen), len(ssmFake.names), iamFake.calls)
	}
}

func TestCheckComputePools_BridgePoolSkipsTrunkingSilently(t *testing.T) {
	clients, ecsFake, _, _, _ := happyClients()
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", InstanceTypes: []string{"m7i-flex.large"}}, // bridge by default
	}}})

	lines := checkComputePools(context.Background(), plan, clients)

	if ecsFake.calls != 0 {
		t.Fatalf("bridge pools must not query the awsvpcTrunking setting, got %d calls", ecsFake.calls)
	}
	if strings.Contains(joined(lines), "trunking") {
		t.Fatalf("bridge pools must not mention trunking at all, got:\n%s", joined(lines))
	}
}

func TestCheckComputePools_RunsAllThreeChecksForAnAWSVPCPool(t *testing.T) {
	clients, ecsFake, ec2Fake, _, iamFake := happyClients()
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", NetworkMode: "awsvpc", InstanceTypes: []string{"m7i-flex.large"}},
	}}})

	out := joined(checkComputePools(context.Background(), plan, clients))

	if ecsFake.calls != 1 || len(ec2Fake.seen) != 1 || iamFake.calls != 1 {
		t.Fatalf("expected one call each, got ecs=%d ec2=%d iam=%d", ecsFake.calls, len(ec2Fake.seen), iamFake.calls)
	}
	for _, want := range []string{"trunking", "ec2:RunInstances", autoScalingServiceLinkedRole} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in the output, got:\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------------
// Check 1 — awsvpcTrunking
// ---------------------------------------------------------------------------

func awsvpcPlan() computePreflightPlan {
	return planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", NetworkMode: "awsvpc", InstanceTypes: []string{"m7i-flex.large"}},
	}}})
}

func TestCheckAWSVPCTrunking_DisabledWarnsWithTheDensityCostAndTheFix(t *testing.T) {
	fake := &preflightFakeECS{value: "disabled"}

	out := joined(checkAWSVPCTrunking(context.Background(), awsvpcPlan(), fake))

	if !strings.HasPrefix(out, "⚠️") {
		t.Fatalf("a disabled setting is a warning, not a failure; got:\n%s", out)
	}
	// The three things the warning is for: what is wrong, what it costs, how to
	// fix it. Without the cost sentence the reader has no way to judge whether
	// to act, which is how a warning becomes noise.
	for _, want := range []string{
		"awsvpcTrunking",
		"general",
		"m7i-flex.large",
		"2 tasks",
		"10",
		"aws ecs put-account-setting --name awsvpcTrunking --value enabled",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in the trunking warning, got:\n%s", want, out)
		}
	}
}

func TestCheckAWSVPCTrunking_EnabledIsOneReassuringLine(t *testing.T) {
	fake := &preflightFakeECS{value: "enabled"}

	lines := checkAWSVPCTrunking(context.Background(), awsvpcPlan(), fake)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "✅") {
		t.Fatalf("an enabled setting is one success line, got:\n%s", joined(lines))
	}
}

func TestCheckAWSVPCTrunking_ReadFailureIsSkippedNotFailed(t *testing.T) {
	fake := &preflightFakeECS{err: &preflightAPIError{
		code: "AccessDeniedException", message: "not authorized to perform: ecs:ListAccountSettings"}}

	lines := checkAWSVPCTrunking(context.Background(), awsvpcPlan(), fake)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "ℹ️") {
		t.Fatalf("a check that could not run must say it was skipped, got:\n%s", joined(lines))
	}
	if !strings.Contains(lines[0], "ecs:ListAccountSettings") {
		t.Fatalf("the skip must name why, got %q", lines[0])
	}
}

func TestCheckAWSVPCTrunking_EmptyResponseIsSkipped(t *testing.T) {
	lines := checkAWSVPCTrunking(context.Background(), awsvpcPlan(), &preflightFakeECS{})

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "ℹ️") {
		t.Fatalf("an account that returns no setting must be reported as unknown, got:\n%s", joined(lines))
	}
}

// ---------------------------------------------------------------------------
// Check 2 — instance-type launchability. This is the D-10 check.
// ---------------------------------------------------------------------------

func TestDryRunInstanceType_DryRunOperationMeansSuccess(t *testing.T) {
	fake := &preflightFakeEC2{}
	probe := computeLaunchProbe{InstanceType: "m7i-flex.large", Pools: []string{"general"}}

	lines := dryRunInstanceType(context.Background(), fake, probe, "ami-0f00")

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "✅") {
		t.Fatalf("DryRunOperation is AWS saying the request would have succeeded; got:\n%s", joined(lines))
	}
	if !strings.Contains(lines[0], "m7i-flex.large") || !strings.Contains(lines[0], `"general"`) {
		t.Fatalf("even the success line names the type and the pool, got %q", lines[0])
	}
}

func TestDryRunInstanceType_SCPDenialNamesTypePoolAndReason(t *testing.T) {
	// The whole point of P12's second check: turn D-10's 40-minute dead end
	// into one preflight line that names the real cause.
	fake := &preflightFakeEC2{byType: map[string]error{
		"m6i.large": &preflightAPIError{code: "UnauthorizedOperation", message: scpDenialMessage},
	}}
	probe := computeLaunchProbe{InstanceType: "m6i.large", Pools: []string{"general"}}

	out := joined(dryRunInstanceType(context.Background(), fake, probe, "ami-0f00"))

	if !strings.HasPrefix(out, "⚠️") {
		t.Fatalf("a refusal is a warning, not a blocker; got:\n%s", out)
	}
	for _, want := range []string{
		"m6i.large",             // the type
		`"general"`,             // the pool
		"UnauthorizedOperation", // the AWS code
		"ec2:RunInstances",      // the action actually denied
		// The SCP ARN is the answer, and it must appear on a line of its own
		// rather than only at character 300 of the quoted AWS message.
		"Denied by service control policy: arn:aws:organizations::000000000001:policy/o-fake0000/service_control_policy/p-fake0000",
		"management account", // where the reader has to go
		"launch template",    // the misleading message this pre-empts
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in the denial, got:\n%s", want, out)
		}
	}
}

func TestDryRunInstanceType_DropsTheEncodedAuthorizationBlob(t *testing.T) {
	// The base64 tail is ~1 KB, is only readable through
	// sts:DecodeAuthorizationMessage, and says nothing the plain text does not.
	// Printing it buries the SCP ARN that is the actual answer.
	fake := &preflightFakeEC2{byType: map[string]error{
		"m6i.large": &preflightAPIError{code: "UnauthorizedOperation", message: scpDenialMessage},
	}}
	probe := computeLaunchProbe{InstanceType: "m6i.large", Pools: []string{"general"}}

	out := joined(dryRunInstanceType(context.Background(), fake, probe, "ami-0f00"))

	if strings.Contains(out, encodedAuthMessageMarker) || strings.Contains(out, "WRWZPoc2Hiv") {
		t.Fatalf("the encoded authorization blob must be cut, got:\n%s", out)
	}
}

func TestDryRunInstanceType_NonSCPDenialOmitsTheOrganizationsAdvice(t *testing.T) {
	fake := &preflightFakeEC2{byType: map[string]error{
		"m6i.large": &preflightAPIError{
			code:    "UnauthorizedOperation",
			message: "You are not authorized to perform this operation.",
		},
	}}
	probe := computeLaunchProbe{InstanceType: "m6i.large", Pools: []string{"general"}}

	out := joined(dryRunInstanceType(context.Background(), fake, probe, "ami-0f00"))

	if strings.Contains(out, "management account") {
		t.Fatalf("plain IAM denials must not be blamed on Organizations, got:\n%s", out)
	}
}

func TestDryRunInstanceType_IsAlwaysADryRunOfExactlyOneInstance(t *testing.T) {
	// The safety property of this whole check. If DryRun were ever false, a
	// preflight would launch a billable instance on every deploy.
	fake := &preflightFakeEC2{}
	probe := computeLaunchProbe{InstanceType: "m7i-flex.large", Pools: []string{"general"}}

	dryRunInstanceType(context.Background(), fake, probe, "ami-0f00")

	if len(fake.seen) != 1 {
		t.Fatalf("expected exactly one RunInstances call, got %d", len(fake.seen))
	}
	in := fake.seen[0]
	if !aws.ToBool(in.DryRun) {
		t.Fatal("DryRun must be true — without it this check launches a real instance")
	}
	if aws.ToInt32(in.MinCount) != 1 || aws.ToInt32(in.MaxCount) != 1 {
		t.Fatalf("expected a single-instance request, got min=%d max=%d",
			aws.ToInt32(in.MinCount), aws.ToInt32(in.MaxCount))
	}
	if aws.ToString(in.ImageId) != "ami-0f00" || string(in.InstanceType) != "m7i-flex.large" {
		t.Fatalf("the probe must send the resolved image and the pool's type, got %+v", in)
	}
}

func TestCheckInstanceTypesLaunchable_ReportsEveryTypeSeparately(t *testing.T) {
	fake := &preflightFakeEC2{byType: map[string]error{
		"m6i.large": &preflightAPIError{code: "UnauthorizedOperation", message: scpDenialMessage},
	}}
	ssmFake := &preflightFakeSSM{value: "ami-0f00"}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", InstanceTypes: []string{"m7i-flex.large", "m6i.large", "t3.small"}},
	}}})

	out := joined(checkInstanceTypesLaunchable(context.Background(), plan, fake, ssmFake))

	if len(fake.seen) != 3 {
		t.Fatalf("expected one dry run per instance type, got %d", len(fake.seen))
	}
	if strings.Count(out, "✅ ec2:RunInstances dry run passed") != 2 {
		t.Fatalf("expected two passes, got:\n%s", out)
	}
	if !strings.Contains(out, "REFUSED for m6i.large") {
		t.Fatalf("the one refused type must be named, got:\n%s", out)
	}
}

func TestCheckInstanceTypesLaunchable_ResolvesEachAMIFamilyOnce(t *testing.T) {
	fake := &preflightFakeEC2{}
	ssmFake := &preflightFakeSSM{value: "ami-0f00"}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "a", InstanceTypes: []string{"m7i-flex.large", "t3.small", "t3.micro"}},
		{Name: "b", AMIFamily: "al2023_arm64", InstanceTypes: []string{"m7g.large"}},
	}}})

	checkInstanceTypesLaunchable(context.Background(), plan, fake, ssmFake)

	if len(ssmFake.names) != 2 {
		t.Fatalf("expected one SSM lookup per family, got %d: %v", len(ssmFake.names), ssmFake.names)
	}
	if ssmFake.names[0] != computePoolAMIParams["al2023"] || ssmFake.names[1] != computePoolAMIParams["al2023_arm64"] {
		t.Fatalf("the lookups must be the same parameters ec2_capacity.tf resolves, got %v", ssmFake.names)
	}
}

func TestCheckInstanceTypesLaunchable_ExplicitAMIIDNeedsNoSSM(t *testing.T) {
	fake := &preflightFakeEC2{}
	ssmFake := &preflightFakeSSM{value: "ami-should-not-be-used"}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "pinned", AMIID: "ami-0pinned", InstanceTypes: []string{"t3.micro"}},
	}}})

	checkInstanceTypesLaunchable(context.Background(), plan, fake, ssmFake)

	if len(ssmFake.names) != 0 {
		t.Fatalf("a pool with ami_id must not touch SSM, got %v", ssmFake.names)
	}
	if aws.ToString(fake.seen[0].ImageId) != "ami-0pinned" {
		t.Fatalf("the pinned image must be probed, got %q", aws.ToString(fake.seen[0].ImageId))
	}
}

func TestCheckInstanceTypesLaunchable_AMIResolutionFailureSkipsWithoutFailing(t *testing.T) {
	fake := &preflightFakeEC2{}
	ssmFake := &preflightFakeSSM{err: &preflightAPIError{
		code: "AccessDeniedException", message: "not authorized to perform: ssm:GetParameter"}}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", InstanceTypes: []string{"m7i-flex.large", "t3.small"}},
	}}})

	lines := checkInstanceTypesLaunchable(context.Background(), plan, fake, ssmFake)
	out := joined(lines)

	if len(fake.seen) != 0 {
		t.Fatal("no image means no dry run; the check must skip rather than guess an AMI")
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "ℹ️") {
			t.Fatalf("an unresolvable AMI is a skip, never a warning about the pool; got:\n%s", out)
		}
	}
	// One failed lookup per family, not one per instance type: the failure is
	// cached exactly as the success is.
	if len(ssmFake.names) != 1 {
		t.Fatalf("a failed lookup must not be retried per type, got %d calls", len(ssmFake.names))
	}
	if !strings.Contains(out, "ssm:GetParameter") {
		t.Fatalf("the skip must name why, got:\n%s", out)
	}
}

func TestCheckInstanceTypesLaunchable_UnknownAMIFamilySkips(t *testing.T) {
	fake := &preflightFakeEC2{}
	ssmFake := &preflightFakeSSM{value: "ami-0f00"}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", AMIFamily: "windows2022", InstanceTypes: []string{"t3.small"}},
	}}})

	out := joined(checkInstanceTypesLaunchable(context.Background(), plan, fake, ssmFake))

	if len(ssmFake.names) != 0 || len(fake.seen) != 0 {
		t.Fatal("an unknown ami_family must not reach AWS at all")
	}
	if !strings.Contains(out, `unknown ami_family "windows2022"`) {
		t.Fatalf("the skip must name the family, got:\n%s", out)
	}
}

func TestCheckInstanceTypesLaunchable_CancelledContextStopsQuietly(t *testing.T) {
	fake := &preflightFakeEC2{}
	ssmFake := &preflightFakeSSM{value: "ami-0f00"}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{
		{Name: "general", InstanceTypes: []string{"m7i-flex.large", "t3.small"}},
	}}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lines := checkInstanceTypesLaunchable(ctx, plan, fake, ssmFake)

	if len(fake.seen) != 0 {
		t.Fatal("an expired budget must stop the probes")
	}
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "ℹ️") {
		t.Fatalf("a timeout is a skip line, got:\n%s", joined(lines))
	}
}

func TestCheckInstanceTypesLaunchable_ReportsTheProbeCap(t *testing.T) {
	types := make([]string, 0, maxComputeLaunchProbes+2)
	for i := 0; i < maxComputeLaunchProbes+2; i++ {
		types = append(types, string(rune('a'+i))+".large")
	}
	plan := planComputePreflight(Env{Compute: Compute{Pools: []ComputePool{{Name: "wide", InstanceTypes: types}}}})

	out := joined(checkInstanceTypesLaunchable(context.Background(), plan,
		&preflightFakeEC2{}, &preflightFakeSSM{value: "ami-0f00"}))

	if !strings.Contains(out, "2 further instance type(s) were not dry-run") {
		t.Fatalf("truncation must be admitted rather than hidden, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Check 3 — the Auto Scaling service-linked role
// ---------------------------------------------------------------------------

func TestCheckAutoScalingServiceLinkedRole_PresentIsOneLine(t *testing.T) {
	lines := checkAutoScalingServiceLinkedRole(context.Background(), &preflightFakeIAM{})

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "✅") {
		t.Fatalf("an existing role is one success line, got:\n%s", joined(lines))
	}
}

func TestCheckAutoScalingServiceLinkedRole_AbsentWarnsWithTheCreateCommand(t *testing.T) {
	fake := &preflightFakeIAM{err: &iamtypes.NoSuchEntityException{
		Message: aws.String("The role with name AWSServiceRoleForAutoScaling cannot be found."),
	}}

	out := joined(checkAutoScalingServiceLinkedRole(context.Background(), fake))

	if !strings.HasPrefix(out, "⚠️") {
		t.Fatalf("an absent role is a warning — AWS usually creates it — not a blocker; got:\n%s", out)
	}
	for _, want := range []string{
		autoScalingServiceLinkedRole,
		"aws iam create-service-linked-role --aws-service-name autoscaling.amazonaws.com",
		"launch template", // the misleading error this pre-empts
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in the warning, got:\n%s", want, out)
		}
	}
}

func TestCheckAutoScalingServiceLinkedRole_DeniedReadIsSkipped(t *testing.T) {
	fake := &preflightFakeIAM{err: &preflightAPIError{
		code: "AccessDenied", message: "not authorized to perform: iam:GetRole"}}

	lines := checkAutoScalingServiceLinkedRole(context.Background(), fake)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "ℹ️") {
		t.Fatalf("a profile that cannot read IAM must skip, not warn about a role it cannot see; got:\n%s",
			joined(lines))
	}
}

// ---------------------------------------------------------------------------
// Error rendering
// ---------------------------------------------------------------------------

func TestSummarizeAWSError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil",
			err:  nil,
			want: "",
		},
		{
			name: "plain error passes through",
			err:  errors.New("context deadline exceeded"),
			want: "context deadline exceeded",
		},
		{
			name: "the SDK operation wrapper is dropped",
			err:  &preflightAPIError{code: "DryRunOperation", message: "Request would have succeeded, but DryRun flag is set"},
			want: "DryRunOperation: Request would have succeeded, but DryRun flag is set",
		},
		{
			name: "newlines collapse to single spaces",
			err:  errors.New("first line\n   second line"),
			want: "first line second line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeAWSError(tt.err); got != tt.want {
				t.Errorf("summarizeAWSError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeAWSError_KeepsTheSCPARNAndDropsTheBlob(t *testing.T) {
	got := summarizeAWSError(&preflightAPIError{code: "UnauthorizedOperation", message: scpDenialMessage})

	if !strings.Contains(got, "service_control_policy/p-fake0000") {
		t.Fatalf("the SCP ARN is the answer and must survive, got %q", got)
	}
	if strings.Contains(got, "WRWZPoc2Hiv") {
		t.Fatalf("the encoded blob must not survive, got %q", got)
	}
	if strings.Contains(got, "https response error StatusCode") {
		t.Fatalf("the SDK wrapper must not survive, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Live
// ---------------------------------------------------------------------------

// TestComputePoolPreflight_Live drives the real AWS path, following the
// convention of TestCheckDNSPreflight_Live: skipped unless MEROKU_E2E_EC2=1, so
// ordinary `go test` runs stay hermetic.
//
//	MEROKU_E2E_EC2=1 \
//	MEROKU_E2E_PROFILE=meroku2 \
//	MEROKU_E2E_REGION=ap-southeast-2 \
//	MEROKU_E2E_ALLOWED_TYPE=m7i-flex.large \
//	MEROKU_E2E_DENIED_TYPE=m6i.large \
//	go test -run TestComputePoolPreflight_Live -v ./app
//
// Every call it makes is read-only or DryRun; it creates nothing. The two type
// env vars are what make it a real test of the D-10 check rather than a print:
// on an account with an SCP restricting ec2:RunInstances by instance type, the
// allowed one must pass and the denied one must be refused.
func TestComputePoolPreflight_Live(t *testing.T) {
	if os.Getenv("MEROKU_E2E_EC2") != "1" {
		t.Skip("set MEROKU_E2E_EC2=1 to run the live EC2 compute pool preflight")
	}

	profile := os.Getenv("MEROKU_E2E_PROFILE")
	region := os.Getenv("MEROKU_E2E_REGION")
	allowed := os.Getenv("MEROKU_E2E_ALLOWED_TYPE")
	denied := os.Getenv("MEROKU_E2E_DENIED_TYPE")
	if profile == "" || region == "" || allowed == "" || denied == "" {
		t.Fatal("MEROKU_E2E_PROFILE, MEROKU_E2E_REGION, MEROKU_E2E_ALLOWED_TYPE and MEROKU_E2E_DENIED_TYPE are all required")
	}
	t.Setenv("AWS_PROFILE", profile)

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		t.Fatalf("could not load AWS config: %v", err)
	}
	clients := computePreflightClients{
		ECS: ecs.NewFromConfig(cfg),
		EC2: ec2.NewFromConfig(cfg),
		SSM: ssm.NewFromConfig(cfg),
		IAM: iam.NewFromConfig(cfg),
	}

	t.Run("an environment with no pools stays silent against a real account", func(t *testing.T) {
		lines := checkComputePools(ctx, planComputePreflight(Env{Region: region}), clients)
		if len(lines) != 0 {
			t.Fatalf("expected no output, got:\n%s", joined(lines))
		}
	})

	t.Run("the awsvpc pool is checked end to end", func(t *testing.T) {
		plan := planComputePreflight(Env{Region: region, Compute: Compute{Pools: []ComputePool{
			{Name: "general", NetworkMode: "awsvpc", InstanceTypes: []string{allowed, denied}},
			{Name: "off", Enabled: boolPtr(false), InstanceTypes: []string{"c5.24xlarge"}},
		}}})

		lines := checkComputePools(ctx, plan, clients)
		out := joined(lines)
		t.Logf("live preflight output:\n%s", out)

		if !strings.Contains(out, "✅ ec2:RunInstances dry run passed for "+allowed) {
			t.Errorf("expected %s to pass the dry run, got:\n%s", allowed, out)
		}
		if !strings.Contains(out, "⚠️  ec2:RunInstances would be REFUSED for "+denied) {
			t.Errorf("expected %s to be refused, got:\n%s", denied, out)
		}
		if strings.Contains(out, "c5.24xlarge") {
			t.Errorf("a disabled pool must never be probed, got:\n%s", out)
		}
		if strings.Contains(out, encodedAuthMessageMarker) {
			t.Errorf("the encoded authorization blob must never be printed, got:\n%s", out)
		}
		// The trunking check must have run: the pool asked for awsvpc.
		if !strings.Contains(out, "awsvpcTrunking") {
			t.Errorf("expected the trunking check to run for an awsvpc pool, got:\n%s", out)
		}
	})
}

func TestExtractSCPARN(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{
			name:   "the real D-10 denial",
			reason: summarizeAWSError(&preflightAPIError{code: "UnauthorizedOperation", message: scpDenialMessage}),
			want:   "arn:aws:organizations::000000000001:policy/o-fake0000/service_control_policy/p-fake0000",
		},
		{
			name:   "a plain IAM denial carries no policy ARN",
			reason: "UnauthorizedOperation: You are not authorized to perform this operation",
			want:   "",
		},
		{
			name:   "trailing punctuation is trimmed",
			reason: "... explicit deny in a service control policy: arn:aws:organizations::1:policy/p-x.",
			want:   "arn:aws:organizations::1:policy/p-x",
		},
		{
			name:   "a truncated message yields nothing rather than a panic",
			reason: "... explicit deny in a service control policy: ",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSCPARN(tt.reason); got != tt.want {
				t.Errorf("extractSCPARN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAwsErrorCode(t *testing.T) {
	if got := awsErrorCode(&preflightAPIError{code: "DryRunOperation"}); got != "DryRunOperation" {
		t.Errorf("awsErrorCode() = %q, want DryRunOperation", got)
	}
	if got := awsErrorCode(errors.New("network unreachable")); got != "" {
		t.Errorf("a non-API error has no code, got %q", got)
	}
	if got := awsErrorCode(nil); got != "" {
		t.Errorf("nil has no code, got %q", got)
	}
}
