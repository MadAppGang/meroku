package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AWSPreflightCheck performs comprehensive AWS setup validation before terraform operations
// Returns nil if everything is ready, error with recovery suggestions otherwise
func AWSPreflightCheck(env Env) error {
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
2. macOS: brew install terraform
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
