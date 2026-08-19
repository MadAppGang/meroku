package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

// AWSPreflightCheck is the entry point for call sites that hold no context of
// their own (app/terrafrom.go). Anything that owns one should call
// AWSPreflightCheckContext instead: step 8 spends up to 45 seconds on sequential
// AWS reads, and a caller whose work has already been abandoned should not have
// to wait out that budget before finding out.
func AWSPreflightCheck(env Env) error {
	return AWSPreflightCheckContext(context.Background(), env)
}

// AWSPreflightCheckContext performs comprehensive AWS setup validation before terraform operations
// Returns nil if everything is ready, error with recovery suggestions otherwise
func AWSPreflightCheckContext(ctx context.Context, env Env) error {
	fmt.Println("\n🔍 Running AWS pre-flight checks...")

	// Step 1: Validate AWS_PROFILE is set
	awsProfile := os.Getenv("AWS_PROFILE")
	if awsProfile == "" && env.AWSProfile != "" {
		fmt.Printf("⚠️  AWS_PROFILE not set, using profile from config: %s\n", env.AWSProfile)
		os.Setenv("AWS_PROFILE", env.AWSProfile)
		awsProfile = env.AWSProfile
	}

	if awsProfile == "" {
		return fmt.Errorf(`❌ AWS_PROFILE not set

Recovery steps:
1. Set AWS profile in your YAML config (aws_profile field)
2. Or run: export AWS_PROFILE=your-profile-name
3. Or select a profile when prompted by meroku`)
	}

	fmt.Printf("✅ AWS_PROFILE set to: %s\n", awsProfile)

	// Step 2: Check AWS CLI version
	fmt.Println("🔧 Checking AWS CLI version...")
	if err := checkAWSCLIVersion(); err != nil {
		return fmt.Errorf(`❌ AWS CLI check failed: %v

Recovery steps:
1. Install AWS CLI v2: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
2. macOS: brew install awscli
3. Linux: curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && unzip awscliv2.zip && sudo ./aws/install
4. Windows: Download installer from AWS website
5. Verify installation: aws --version`, err)
	}

	// Step 3: Check Terraform version
	fmt.Println("🔧 Checking Terraform version...")
	if err := checkTerraformVersion(); err != nil {
		return fmt.Errorf(`❌ Terraform check failed: %v

Recovery steps:
1. Install Terraform: https://developer.hashicorp.com/terraform/install
2. macOS (recommended):
   brew tap hashicorp/tap
   brew install hashicorp/tap/terraform
3. Linux: Download from https://releases.hashicorp.com/terraform/
4. Windows: Download installer from HashiCorp website
5. Verify installation: terraform version`, err)
	}

	// Step 4: Validate AWS credentials work
	if err := validateAWSCredentials(env.Region); err != nil {
		return fmt.Errorf(`❌ AWS credentials validation failed: %v

Recovery steps:
1. Check if your AWS profile exists: aws configure list-profiles
2. For SSO: Run 'aws sso login --profile %s'
3. For IAM keys: Run 'aws configure --profile %s'
4. Verify credentials: aws sts get-caller-identity --profile %s`, err, awsProfile, awsProfile, awsProfile)
	}

	// Step 5: Check git repository status vs remote
	fmt.Println("📦 Checking git repository status...")
	if err := checkGitRepositoryStatus(); err != nil {
		// Non-fatal warning - we don't exit, just warn
		fmt.Printf("⚠️  %v\n", err)
	}

	// Step 6: Ensure S3 state bucket exists
	fmt.Printf("🪣  Checking S3 state bucket: %s\n", env.StateBucket)
	if err := checkBucketStateForEnv(env); err != nil {
		// If SSO token expired, try to refresh
		if strings.Contains(err.Error(), "SSO") || strings.Contains(err.Error(), "expired") {
			fmt.Println("⚠️  SSO token appears expired, attempting to refresh...")
			if err := refreshSSOToken(awsProfile); err != nil {
				return fmt.Errorf(`❌ Failed to refresh SSO token: %v

Recovery steps:
1. Run: aws sso login --profile %s
2. Then try again`, err, awsProfile)
			}

			// Retry bucket check after SSO refresh
			fmt.Println("🔄 Retrying S3 bucket check after SSO refresh...")
			if err := checkBucketStateForEnv(env); err != nil {
				return fmt.Errorf(`❌ S3 bucket check failed: %v

Recovery steps:
1. Verify bucket name is valid: %s
2. Check region is correct: %s
3. Ensure you have S3 permissions
4. Try creating bucket manually: aws s3 mb s3://%s --region %s`,
					err, env.StateBucket, env.Region, env.StateBucket, env.Region)
			}
		} else {
			return fmt.Errorf(`❌ S3 bucket check failed: %v

Recovery steps:
1. Verify bucket name is valid: %s
2. Check region is correct: %s
3. Ensure you have S3 permissions
4. Try creating bucket manually: aws s3 mb s3://%s --region %s`,
				err, env.StateBucket, env.Region, env.StateBucket, env.Region)
		}
	}

	// Step 7: Ensure DynamoDB state lock table exists (if configured)
	if env.StateLockTable != "" {
		fmt.Printf("🔒 Checking DynamoDB lock table: %s\n", env.StateLockTable)
		if err := checkDynamoDBTableForEnv(env); err != nil {
			// If SSO token expired, try to refresh
			if strings.Contains(err.Error(), "SSO") || strings.Contains(err.Error(), "expired") {
				fmt.Println("⚠️  SSO token appears expired, attempting to refresh...")
				if err := refreshSSOToken(awsProfile); err != nil {
					return fmt.Errorf(`❌ Failed to refresh SSO token: %v

Recovery steps:
1. Run: aws sso login --profile %s
2. Then try again`, err, awsProfile)
				}

				// Retry DynamoDB check after SSO refresh
				fmt.Println("🔄 Retrying DynamoDB table check after SSO refresh...")
				if err := checkDynamoDBTableForEnv(env); err != nil {
					return fmt.Errorf(`❌ DynamoDB table check failed: %v

Recovery steps:
1. Verify table name is valid: %s
2. Check region is correct: %s
3. Ensure you have DynamoDB permissions
4. Try creating table manually:
   aws dynamodb create-table \
     --table-name %s \
     --attribute-definitions AttributeName=LockID,AttributeType=S \
     --key-schema AttributeName=LockID,KeyType=HASH \
     --billing-mode PAY_PER_REQUEST \
     --region %s`,
						err, env.StateLockTable, env.Region, env.StateLockTable, env.Region)
				}
			} else {
				return fmt.Errorf(`❌ DynamoDB table check failed: %v

Recovery steps:
1. Verify table name is valid: %s
2. Check region is correct: %s
3. Ensure you have DynamoDB permissions
4. Try creating table manually:
   aws dynamodb create-table \
     --table-name %s \
     --attribute-definitions AttributeName=LockID,AttributeType=S \
     --key-schema AttributeName=LockID,KeyType=HASH \
     --billing-mode PAY_PER_REQUEST \
     --region %s`,
					err, env.StateLockTable, env.Region, env.StateLockTable, env.Region)
			}
		}
	}

	// Step 8: EC2 compute pool checks (FR-59). Advisory only, and silent for an
	// environment with no pools — which is every environment created before
	// schema v26. It runs last, after the blocking checks have all passed, so a
	// warning here is the last thing on screen before the deploy proceeds.
	runComputePoolPreflight(ctx, env)

	fmt.Println("✅ All AWS pre-flight checks passed!")
	return nil
}

// validateAWSCredentials checks if AWS credentials are valid and working
func validateAWSCredentials(region string) error {
	return validateAWSCredentialsWithRetry(region, false)
}

func validateAWSCredentialsWithRetry(region string, isRetry bool) error {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to load AWS configuration: %v", err)
	}

	// Use STS GetCallerIdentity to validate credentials
	stsClient := sts.NewFromConfig(cfg)
	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		// Check if SSO token expired
		if !isRetry && (strings.Contains(err.Error(), "SSO") || strings.Contains(err.Error(), "expired")) {
			awsProfile := os.Getenv("AWS_PROFILE")
			fmt.Printf("⚠️  SSO token expired for profile: %s\n", awsProfile)
			if err := refreshSSOToken(awsProfile); err != nil {
				return fmt.Errorf("SSO token refresh failed: %v", err)
			}
			// Retry once after SSO refresh
			return validateAWSCredentialsWithRetry(region, true)
		}
		return fmt.Errorf("failed to validate credentials: %v", err)
	}

	fmt.Printf("✅ AWS credentials valid - Account: %s, User: %s\n",
		*result.Account, *result.Arn)

	return nil
}

// refreshSSOToken attempts to refresh SSO token by running aws sso login
func refreshSSOToken(profile string) error {
	fmt.Printf("🔄 Refreshing SSO token for profile: %s\n", profile)

	args := []string{"sso", "login"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	output, err := runCommandWithOutput("aws", args...)
	if err != nil {
		return fmt.Errorf("aws sso login failed: %v\nOutput: %s", err, output)
	}

	fmt.Println("✅ SSO token refreshed successfully")
	return nil
}

// checkAWSCLIVersion validates that AWS CLI is installed and meets minimum version requirement
func checkAWSCLIVersion() error {
	const minVersion = "2.31.20"

	output, err := runCommandWithOutput("aws", "--version")
	if err != nil {
		return fmt.Errorf("AWS CLI not found - please install AWS CLI v2 (minimum version %s)", minVersion)
	}

	version := parseAWSCLIVersion(output)
	if version == "" {
		return fmt.Errorf("could not parse AWS CLI version from output: %s", output)
	}

	if !isVersionAtLeast(version, minVersion) {
		return fmt.Errorf("AWS CLI version %s is installed, but minimum required version is %s", version, minVersion)
	}

	fmt.Printf("✅ AWS CLI version %s (meets minimum requirement %s)\n", version, minVersion)
	return nil
}

// checkTerraformVersion validates that Terraform is installed and meets minimum version requirement
func checkTerraformVersion() error {
	const minVersion = "1.13.4"

	output, err := runCommandWithOutput("terraform", "version")
	if err != nil {
		return fmt.Errorf("Terraform not found - please install Terraform (minimum version %s)", minVersion)
	}

	version := parseTerraformVersion(output)
	if version == "" {
		return fmt.Errorf("could not parse Terraform version from output: %s", output)
	}

	if !isVersionAtLeast(version, minVersion) {
		return fmt.Errorf("Terraform version %s is installed, but minimum required version is %s", version, minVersion)
	}

	fmt.Printf("✅ Terraform version %s (meets minimum requirement %s)\n", version, minVersion)
	return nil
}

// parseAWSCLIVersion extracts version number from AWS CLI output
// Example input: "aws-cli/2.31.20 Python/3.11.6 Darwin/24.0.0 source/arm64"
// Returns: "2.31.20"
func parseAWSCLIVersion(output string) string {
	// AWS CLI version format: "aws-cli/X.Y.Z ..."
	parts := strings.Fields(output)
	if len(parts) == 0 {
		return ""
	}

	// First field should be "aws-cli/X.Y.Z"
	versionPart := parts[0]
	if !strings.HasPrefix(versionPart, "aws-cli/") {
		return ""
	}

	version := strings.TrimPrefix(versionPart, "aws-cli/")
	return version
}

// parseTerraformVersion extracts version number from Terraform output
// Example input: "Terraform v1.13.4\non darwin_arm64\n..."
// Returns: "1.13.4"
func parseTerraformVersion(output string) string {
	// Terraform version format: "Terraform vX.Y.Z"
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return ""
	}

	// First line should contain version
	firstLine := strings.TrimSpace(lines[0])
	parts := strings.Fields(firstLine)
	if len(parts) < 2 {
		return ""
	}

	// Second field should be "vX.Y.Z"
	versionPart := parts[1]
	if !strings.HasPrefix(versionPart, "v") {
		return ""
	}

	version := strings.TrimPrefix(versionPart, "v")
	return version
}

// isVersionAtLeast checks if current version meets or exceeds minimum version requirement
// Uses semantic versioning comparison (major.minor.patch)
func isVersionAtLeast(current, minimum string) bool {
	currentParts := parseVersionParts(current)
	minimumParts := parseVersionParts(minimum)

	// Compare each part (major, minor, patch)
	for i := 0; i < 3; i++ {
		currentVal := 0
		minimumVal := 0

		if i < len(currentParts) {
			currentVal = currentParts[i]
		}
		if i < len(minimumParts) {
			minimumVal = minimumParts[i]
		}

		if currentVal > minimumVal {
			return true
		}
		if currentVal < minimumVal {
			return false
		}
		// If equal, continue to next part
	}

	// All parts equal means version meets requirement
	return true
}

// parseVersionParts splits a version string into integer parts
// Example: "2.31.20" -> [2, 31, 20]
func parseVersionParts(version string) []int {
	parts := strings.Split(version, ".")
	result := make([]int, 0, len(parts))

	for _, part := range parts {
		// Handle cases like "1.13.4-dev" by taking only the numeric part
		numericPart := strings.Split(part, "-")[0]
		if num, err := strconv.Atoi(numericPart); err == nil {
			result = append(result, num)
		}
	}

	return result
}

// checkGitRepositoryStatus checks if the local repository is behind the remote
// Returns a warning message if local is behind, nil if up-to-date or if not a git repo
func checkGitRepositoryStatus() error {
	// Check if this is a git repository
	if _, err := runCommandWithOutput("git", "rev-parse", "--git-dir"); err != nil {
		// Not a git repository, skip check
		return nil
	}

	// Get current branch name
	branchOutput, err := runCommandWithOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// Can't determine branch, skip check
		return nil
	}
	currentBranch := strings.TrimSpace(branchOutput)

	// Fetch latest from remote (quietly, don't show output to user)
	_, err = runCommandWithOutput("git", "fetch", "origin", currentBranch, "--quiet")
	if err != nil {
		// Network issues or no remote, skip check
		return nil
	}

	// Get local HEAD commit
	localCommit, err := runCommandWithOutput("git", "rev-parse", "HEAD")
	if err != nil {
		return nil
	}
	localCommit = strings.TrimSpace(localCommit)

	// Get remote HEAD commit
	remoteCommit, err := runCommandWithOutput("git", "rev-parse", fmt.Sprintf("origin/%s", currentBranch))
	if err != nil {
		// Remote branch doesn't exist, skip check
		return nil
	}
	remoteCommit = strings.TrimSpace(remoteCommit)

	// Compare commits
	if localCommit == remoteCommit {
		fmt.Println("✅ Git repository is up-to-date with remote")
		return nil
	}

	// Check if local is behind remote
	mergeBase, err := runCommandWithOutput("git", "merge-base", "HEAD", fmt.Sprintf("origin/%s", currentBranch))
	if err != nil {
		return nil
	}
	mergeBase = strings.TrimSpace(mergeBase)

	if mergeBase == localCommit {
		// Local is behind remote
		// Count commits behind
		commitsOutput, _ := runCommandWithOutput("git", "rev-list", "--count", fmt.Sprintf("HEAD..origin/%s", currentBranch))
		commitsBehind := strings.TrimSpace(commitsOutput)

		return fmt.Errorf(`Git repository is %s commit(s) behind origin/%s

⚠️  WARNING: You are deploying with outdated code!

Recommended actions:
1. Pull latest changes: git pull origin %s
2. Review changes: git log HEAD..origin/%s --oneline
3. Re-run deployment after updating

To continue anyway, proceed with deployment (not recommended)`, commitsBehind, currentBranch, currentBranch, currentBranch)
	}

	// Local has diverged (has commits not on remote)
	fmt.Printf("ℹ️  Local branch has unpushed commits (different from remote)\n")
	return nil
}

// ---------------------------------------------------------------------------
// EC2 compute pool preflight (FR-59)
// ---------------------------------------------------------------------------
//
// Three checks, all of them read-only or DryRun, all of them gated on the
// environment actually having an enabled compute pool. An environment with no
// pools — which is every environment that existed before schema v26 — prints
// nothing at all from this block, so the Fargate deploy experience is unchanged.
//
// None of them can fail a deploy. They are warnings in the sense step 5's git
// check is a warning: printed, and then the deploy carries on. A check that
// could not run (no credentials, a denied read, a timeout) says so and is
// skipped — refusing to deploy because a diagnostic could not be gathered would
// be strictly worse than deploying without the diagnostic.

const (
	// autoScalingServiceLinkedRole is created by AWS on the first Auto Scaling
	// group in an account. Its absence is normally self-healing, but it is
	// worth naming here because of the failure it can produce — see
	// checkAutoScalingServiceLinkedRole.
	autoScalingServiceLinkedRole = "AWSServiceRoleForAutoScaling"

	// maxComputeLaunchProbes bounds the RunInstances dry runs one preflight
	// will issue. Each is a round trip, and a pathological config listing
	// dozens of instance types across several pools would otherwise put tens of
	// seconds in front of every deploy. Real pools carry one to four types.
	maxComputeLaunchProbes = 20

	// computePreflightTimeout bounds the whole EC2 block. On expiry the
	// remaining checks report as skipped; nothing is retried and nothing fails.
	computePreflightTimeout = 45 * time.Second
)

// computePoolAMIParams maps a pool's ami_family onto the public SSM parameter
// holding the current ECS-optimized AMI for it. These are the same three
// parameters modules/workloads/ec2_capacity.tf:50-52 resolves, which is the
// point: the dry run probes the exact image the Auto Scaling group will launch,
// not a stand-in an SCP might treat differently.
var computePoolAMIParams = map[string]string{
	"al2023":       "/aws/service/ecs/optimized-ami/amazon-linux-2023/recommended/image_id",
	"al2023_arm64": "/aws/service/ecs/optimized-ami/amazon-linux-2023/arm64/recommended/image_id",
	"al2023_gpu":   "/aws/service/ecs/optimized-ami/amazon-linux-2023/gpu/recommended/image_id",
}

// computeLaunchProbe is one ec2:RunInstances dry run: one image, one instance
// type, and the pools that asked for the combination. Pools are plural because
// two pools sharing a family and a type produce identical answers, and issuing
// the call twice would only slow the preflight down.
type computeLaunchProbe struct {
	InstanceType string
	AMIFamily    string // resolved through computePoolAMIParams; empty when AMIID is set
	AMIID        string // the pool's explicit ami_id, which overrides the family
	Pools        []string
}

// describe names the probe the way the warnings do: instance type first,
// because that is what an SCP conditions on and what the reader has to change.
func (p computeLaunchProbe) describe() string {
	if len(p.Pools) == 1 {
		return fmt.Sprintf("%s (pool %q)", p.InstanceType, p.Pools[0])
	}
	quoted := make([]string, 0, len(p.Pools))
	for _, name := range p.Pools {
		quoted = append(quoted, strconv.Quote(name))
	}
	return fmt.Sprintf("%s (pools %s)", p.InstanceType, strings.Join(quoted, ", "))
}

// computePreflightPlan is the credential-free half of the EC2 preflight: what
// to check, derived from the environment alone. Splitting it out is what makes
// the gating and the de-duplication testable without an AWS account.
type computePreflightPlan struct {
	// PoolNames are the enabled pools, in YAML order. Empty means the whole
	// block is skipped and nothing is printed.
	PoolNames []string
	// AWSVPCPools are the enabled pools that set network_mode: awsvpc. Under
	// bridge — the default, D-6 — there are no task ENIs to trunk, so the
	// awsvpcTrunking check is not merely uninteresting but inapplicable, and
	// raising it would be noise. Empty means the check does not run.
	AWSVPCPools []string
	// Probes are the de-duplicated RunInstances dry runs, in YAML order.
	Probes []computeLaunchProbe
	// ProbesDropped counts combinations left unprobed by maxComputeLaunchProbes.
	ProbesDropped int
}

// planComputePreflight derives the plan from the environment. Pure: no clock,
// no network, no AWS.
func planComputePreflight(env Env) computePreflightPlan {
	var plan computePreflightPlan

	// index maps a probe key onto its position in plan.Probes, so a repeated
	// (image, instance type) pair appends a pool name instead of a second call.
	index := map[string]int{}

	for _, pool := range env.Compute.Pools {
		// Matches modules/workloads/ec2_capacity.tf:27 and api_pricing.go:540:
		// enabled is optional(bool, true), so an absent key means enabled.
		if pool.Enabled != nil && !*pool.Enabled {
			continue
		}
		plan.PoolNames = append(plan.PoolNames, pool.Name)

		if pool.NetworkMode == "awsvpc" {
			plan.AWSVPCPools = append(plan.AWSVPCPools, pool.Name)
		}

		family := pool.AMIFamily
		if family == "" {
			family = "al2023" // variables.tf's optional(string, "al2023")
		}

		for _, instanceType := range pool.InstanceTypes {
			instanceType = strings.TrimSpace(instanceType)
			if instanceType == "" {
				continue
			}
			key := "family:" + family + "|" + instanceType
			if pool.AMIID != "" {
				key = "id:" + pool.AMIID + "|" + instanceType
			}
			if at, ok := index[key]; ok {
				plan.Probes[at].Pools = appendUniqueName(plan.Probes[at].Pools, pool.Name)
				continue
			}
			if len(plan.Probes) >= maxComputeLaunchProbes {
				plan.ProbesDropped++
				continue
			}
			probe := computeLaunchProbe{InstanceType: instanceType, Pools: []string{pool.Name}}
			if pool.AMIID != "" {
				probe.AMIID = pool.AMIID
			} else {
				probe.AMIFamily = family
			}
			index[key] = len(plan.Probes)
			plan.Probes = append(plan.Probes, probe)
		}
	}

	return plan
}

func appendUniqueName(names []string, name string) []string {
	for _, existing := range names {
		if existing == name {
			return names
		}
	}
	return append(names, name)
}

// The AWS seam. Four narrow interfaces rather than one wide one, following
// app/compute_catalog.go: a fake for the trunking check has no business being
// able to launch an instance, dry run or not.

type ecsAccountSettingsAPI interface {
	ListAccountSettings(context.Context, *ecs.ListAccountSettingsInput, ...func(*ecs.Options)) (*ecs.ListAccountSettingsOutput, error)
}

type ec2DryRunAPI interface {
	RunInstances(context.Context, *ec2.RunInstancesInput, ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
}

type ssmParameterAPI interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

type iamGetRoleAPI interface {
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
}

type computePreflightClients struct {
	ECS ecsAccountSettingsAPI
	EC2 ec2DryRunAPI
	SSM ssmParameterAPI
	IAM iamGetRoleAPI
}

// runComputePoolPreflight is the entry point AWSPreflightCheck calls. It
// returns nothing: every outcome is advisory.
//
// The caller's context is the parent of the 45-second budget, not
// context.Background(). Twenty sequential RunInstances dry runs plus an SSM and
// an IAM read is the worst case, and none of it is worth doing once the deploy
// that asked for it has been abandoned — a timeout is a ceiling on how long the
// checks may take, never a floor on how long they must.
func runComputePoolPreflight(ctx context.Context, env Env) {
	plan := planComputePreflight(env)
	if len(plan.PoolNames) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, computePreflightTimeout)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(env.Region))
	if err != nil {
		// Not reachable in the normal flow — step 4 already validated
		// credentials — but a diagnostic that cannot load a config must say so
		// rather than take the deploy down with it.
		for _, line := range computePreflightHeader(plan) {
			fmt.Println(line)
		}
		fmt.Printf("ℹ️  Skipped the EC2 compute pool checks: could not load AWS configuration (%v)\n", err)
		return
	}

	clients := computePreflightClients{
		ECS: ecs.NewFromConfig(cfg),
		EC2: ec2.NewFromConfig(cfg),
		SSM: ssm.NewFromConfig(cfg),
		IAM: iam.NewFromConfig(cfg),
	}

	for _, line := range checkComputePools(ctx, plan, clients) {
		fmt.Println(line)
	}
}

// checkComputePools runs the three checks and returns the lines to print.
// Returning lines rather than printing them is what lets the tests assert on
// the exact wording, which for check 2 is most of the value of the check.
func checkComputePools(ctx context.Context, plan computePreflightPlan, c computePreflightClients) []string {
	if len(plan.PoolNames) == 0 {
		return nil
	}

	lines := computePreflightHeader(plan)
	lines = append(lines, checkAWSVPCTrunking(ctx, plan, c.ECS)...)
	lines = append(lines, checkInstanceTypesLaunchable(ctx, plan, c.EC2, c.SSM)...)
	lines = append(lines, checkAutoScalingServiceLinkedRole(ctx, c.IAM)...)
	return lines
}

func computePreflightHeader(plan computePreflightPlan) []string {
	noun := "pools"
	if len(plan.PoolNames) == 1 {
		noun = "pool"
	}
	return []string{fmt.Sprintf("🖥️  Checking EC2 compute %s: %s",
		noun, strings.Join(plan.PoolNames, ", "))}
}

// checkAWSVPCTrunking reports the account-level awsvpcTrunking setting, and
// only for a pool that explicitly opted into network_mode: awsvpc.
//
// D-6 made bridge the default, and under bridge a task has no ENI of its own —
// it shares the instance's primary interface — so trunking changes nothing.
// Reporting it for a bridge pool would be a warning the reader can neither act
// on nor safely ignore, which is the worst kind.
func checkAWSVPCTrunking(ctx context.Context, plan computePreflightPlan, api ecsAccountSettingsAPI) []string {
	if len(plan.AWSVPCPools) == 0 {
		return nil
	}

	out, err := api.ListAccountSettings(ctx, &ecs.ListAccountSettingsInput{
		Name:              ecstypes.SettingNameAwsvpcTrunking,
		EffectiveSettings: true,
	})
	if err != nil {
		return []string{fmt.Sprintf(
			"ℹ️  Skipped the ENI trunking check: could not read the awsvpcTrunking account setting (%s)",
			summarizeAWSError(err))}
	}

	value := ""
	for _, setting := range out.Settings {
		if setting.Name == ecstypes.SettingNameAwsvpcTrunking && setting.Value != nil {
			value = *setting.Value
			break
		}
	}

	pools := strings.Join(plan.AWSVPCPools, ", ")
	switch value {
	case "enabled":
		return []string{fmt.Sprintf(
			"✅ ENI trunking (awsvpcTrunking) is enabled — awsvpc pool(s) %s get the full task density", pools)}
	case "":
		return []string{"ℹ️  Skipped the ENI trunking check: the account returned no awsvpcTrunking setting"}
	default:
		return []string{
			fmt.Sprintf("⚠️  ENI trunking (awsvpcTrunking) is %s on this account.", strings.ToUpper(value)),
			fmt.Sprintf("   Pool(s) %s set network_mode: awsvpc, so every task consumes an ENI of its own.", pools),
			"   Without trunking an m7i-flex.large holds roughly 2 tasks instead of roughly 10, which is",
			"   most of the cost case for EC2 capacity. Enable it (account-wide, one call, free):",
			"     aws ecs put-account-setting --name awsvpcTrunking --value enabled",
			"   Instances already running keep their old limit; the setting applies at registration.",
		}
	}
}

// checkInstanceTypesLaunchable dry-runs ec2:RunInstances for every instance
// type the pools name, and reports the real reason a launch would be refused.
//
// This exists because of a live failure (D-10). The apply got as far as the
// Auto Scaling group and then said:
//
//	AccessDenied: You are not authorized to use launch template: lt-...
//
// which is true and useless. It sends the reader to IAM and to the launch
// template, both of which were fine. The actual cause was an AWS Organizations
// service control policy denying ec2:RunInstances for every instance type
// except t2.micro. CreateAutoScalingGroup cannot surface that — Auto Scaling
// attempts the launch on the caller's behalf, is denied, and can only report
// that it could not use the template.
//
// A DryRun RunInstances returns the denial verbatim, SCP ARN included, creates
// nothing and costs nothing. It turns a 40-minute dead end into one line at
// preflight.
//
// What it does not catch, stated so nobody over-reads a pass: the probe sends
// the image and the instance type, not the subnet, security groups, tags or
// instance profile the real launch will carry. An SCP conditioned on one of
// those still passes here and fails at apply. The common shape — a condition on
// ec2:InstanceType — is exactly what this catches.
func checkInstanceTypesLaunchable(ctx context.Context, plan computePreflightPlan, api ec2DryRunAPI, ssmAPI ssmParameterAPI) []string {
	if len(plan.Probes) == 0 {
		return nil
	}

	var lines []string
	amiCache := map[string]string{}  // family -> ami id
	amiFailed := map[string]string{} // family -> why resolution failed

	for _, probe := range plan.Probes {
		if err := ctx.Err(); err != nil {
			lines = append(lines, fmt.Sprintf(
				"ℹ️  Skipped the remaining ec2:RunInstances dry runs: %s", summarizeAWSError(err)))
			break
		}

		imageID := probe.AMIID
		if imageID == "" {
			var ok bool
			imageID, ok = amiCache[probe.AMIFamily]
			if !ok {
				if reason, failed := amiFailed[probe.AMIFamily]; failed {
					lines = append(lines, fmt.Sprintf(
						"ℹ️  Skipped the ec2:RunInstances dry run for %s: %s", probe.describe(), reason))
					continue
				}
				resolved, err := resolveECSOptimizedAMI(ctx, ssmAPI, probe.AMIFamily)
				if err != nil {
					reason := fmt.Sprintf("could not resolve the ECS-optimized AMI for ami_family %q (%s)",
						probe.AMIFamily, summarizeAWSError(err))
					amiFailed[probe.AMIFamily] = reason
					lines = append(lines, fmt.Sprintf(
						"ℹ️  Skipped the ec2:RunInstances dry run for %s: %s", probe.describe(), reason))
					continue
				}
				amiCache[probe.AMIFamily] = resolved
				imageID = resolved
			}
		}

		lines = append(lines, dryRunInstanceType(ctx, api, probe, imageID)...)
	}

	if plan.ProbesDropped > 0 {
		lines = append(lines, fmt.Sprintf(
			"ℹ️  %d further instance type(s) were not dry-run: the preflight probes at most %d.",
			plan.ProbesDropped, maxComputeLaunchProbes))
	}

	return lines
}

// dryRunInstanceType issues exactly one DryRun RunInstances. A dry run creates
// nothing: AWS performs the authorization check and then returns
// DryRunOperation ("Request would have succeeded") instead of launching.
func dryRunInstanceType(ctx context.Context, api ec2DryRunAPI, probe computeLaunchProbe, imageID string) []string {
	_, err := api.RunInstances(ctx, &ec2.RunInstancesInput{
		DryRun:       aws.Bool(true),
		ImageId:      aws.String(imageID),
		InstanceType: ec2types.InstanceType(probe.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
	})

	// The success path IS an error. AWS signals "this would have worked" by
	// returning DryRunOperation, so nil would mean an instance was launched —
	// which the DryRun flag makes impossible, but treating nil as success keeps
	// the branch honest either way.
	if err == nil || awsErrorCode(err) == "DryRunOperation" {
		return []string{fmt.Sprintf("✅ ec2:RunInstances dry run passed for %s", probe.describe())}
	}

	reason := summarizeAWSError(err)
	lines := []string{
		fmt.Sprintf("⚠️  ec2:RunInstances would be REFUSED for %s.", probe.describe()),
		fmt.Sprintf("   AWS says: %s", reason),
	}

	// The SCP ARN is the answer, and AWS buries it ~300 characters into a single
	// line. Lifting it onto its own line is the difference between a reader
	// seeing the cause and skimming past it.
	if arn := extractSCPARN(reason); arn != "" {
		lines = append(lines,
			fmt.Sprintf("   Denied by service control policy: %s", arn),
			"   That is an AWS Organizations policy, not an IAM one — it can only be changed from the",
			"   organization's management account. Either have the policy allow this instance type,",
			"   or set the pool's instance_types to one it already permits.")
	}

	lines = append(lines,
		"   Left unfixed, terraform apply creates every other resource and then fails on the Auto",
		"   Scaling group with \"AccessDenied: You are not authorized to use launch template\", which",
		"   names neither the instance type nor the policy. The line above is the real reason.")

	return lines
}

// scpDenialMarker is the phrase AWS uses immediately before the policy ARN in
// an Organizations denial. Matching the text is the only option available: the
// SDK models this as a plain UnauthorizedOperation with no structured field for
// the policy, so the ARN exists only inside the message.
const scpDenialMarker = "explicit deny in a service control policy: "

// extractSCPARN pulls the policy ARN out of an Organizations denial, or returns
// "" when the message is not one.
func extractSCPARN(reason string) string {
	at := strings.Index(reason, scpDenialMarker)
	if at < 0 {
		return ""
	}
	arn := strings.Fields(reason[at+len(scpDenialMarker):])
	if len(arn) == 0 {
		return ""
	}
	return strings.TrimRight(arn[0], ".,;")
}

// resolveECSOptimizedAMI reads the public SSM parameter for a pool's AMI
// family — the same parameter modules/workloads/ec2_capacity.tf resolves, so
// the dry run probes the image the ASG will actually launch.
func resolveECSOptimizedAMI(ctx context.Context, api ssmParameterAPI, family string) (string, error) {
	param, ok := computePoolAMIParams[family]
	if !ok {
		return "", fmt.Errorf("unknown ami_family %q", family)
	}
	out, err := api.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(param)})
	if err != nil {
		return "", err
	}
	if out.Parameter == nil || out.Parameter.Value == nil || *out.Parameter.Value == "" {
		return "", fmt.Errorf("SSM parameter %s is empty", param)
	}
	return *out.Parameter.Value, nil
}

// checkAutoScalingServiceLinkedRole reports whether AWSServiceRoleForAutoScaling
// exists. AWS creates it as a side effect of the first CreateAutoScalingGroup in
// an account, so its absence is usually self-healing — but the failure it can
// produce in the meantime is the same unreadable "not authorized to use launch
// template" that D-10 chased for forty minutes, so it belongs beside the other
// EC2 checks rather than in a runbook nobody reads until it is too late.
func checkAutoScalingServiceLinkedRole(ctx context.Context, api iamGetRoleAPI) []string {
	_, err := api.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(autoScalingServiceLinkedRole)})
	if err == nil {
		return []string{fmt.Sprintf("✅ Auto Scaling service-linked role %s exists", autoScalingServiceLinkedRole)}
	}

	var notFound *iamtypes.NoSuchEntityException
	if errors.As(err, &notFound) || awsErrorCode(err) == "NoSuchEntity" {
		return []string{
			fmt.Sprintf("⚠️  The Auto Scaling service-linked role %s does not exist in this account.", autoScalingServiceLinkedRole),
			"   AWS normally creates it on the first Auto Scaling group, so this often resolves itself.",
			"   If the apply fails on the ASG with a denial naming the launch template, create it and re-run:",
			"     aws iam create-service-linked-role --aws-service-name autoscaling.amazonaws.com",
		}
	}

	return []string{fmt.Sprintf(
		"ℹ️  Skipped the Auto Scaling service-linked role check: could not read %s (%s)",
		autoScalingServiceLinkedRole, summarizeAWSError(err))}
}

// awsErrorCode returns the smithy error code, or "" when the error is not an
// AWS API error. Matching on the code rather than on a concrete exception type
// follows classifyComputeError in app/compute_catalog.go: EC2, ECS, SSM and IAM
// model these differently and four type switches would be four chances to miss
// one.
func awsErrorCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

// encodedAuthMessageMarker prefixes the ~1 KB base64 blob AWS appends to an
// UnauthorizedOperation. It is only readable via sts:DecodeAuthorizationMessage
// and says nothing the plain text does not — the SCP ARN is already in the plain
// text — so it is cut before the message is shown.
const encodedAuthMessageMarker = "Encoded authorization failure message:"

// summarizeAWSError renders an AWS error as one readable line: the SDK's
// operation wrapper and the encoded blob removed, whitespace collapsed.
func summarizeAWSError(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if at := strings.Index(msg, encodedAuthMessageMarker); at >= 0 {
		msg = msg[:at]
	}

	// The SDK wraps as: `operation error EC2: RunInstances, https response
	// error StatusCode: 403, RequestID: ..., api error UnauthorizedOperation:
	// <the part a human wants>`. Keep the code and the sentence after it.
	if at := strings.Index(msg, "api error "); at >= 0 {
		msg = msg[at+len("api error "):]
	}

	msg = strings.TrimSpace(msg)
	msg = strings.TrimRight(msg, ".,; ")
	return strings.Join(strings.Fields(msg), " ")
}
