package main

import (
	"context"
	"fmt"
	"os"
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

	// Step 2: Validate AWS credentials work
	if err := validateAWSCredentials(env.Region); err != nil {
		return fmt.Errorf(`❌ AWS credentials validation failed: %v

Recovery steps:
1. Check if your AWS profile exists: aws configure list-profiles
2. For SSO: Run 'aws sso login --profile %s'
3. For IAM keys: Run 'aws configure --profile %s'
4. Verify credentials: aws sts get-caller-identity --profile %s`, err, awsProfile, awsProfile, awsProfile)
	}

	// Step 3: Ensure S3 state bucket exists
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
