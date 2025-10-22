package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AutoLogin handles AWS SSO authentication and validation
type AutoLogin struct {
	profile string
}

// NewAutoLogin creates a new auto-login handler
func NewAutoLogin(profile string) *AutoLogin {
	return &AutoLogin{
		profile: profile,
	}
}

// Login performs SSO login for the profile
func (al *AutoLogin) Login() error {
	fmt.Printf("🔐 Logging in to AWS SSO (profile: %s)...\n", al.profile)

	args := []string{"sso", "login", "--profile", al.profile}

	// Run aws sso login command
	output, err := runCommandWithOutput("aws", args...)
	if err != nil {
		return fmt.Errorf("AWS SSO login failed: %w\n\nOutput: %s\n\nTroubleshooting:\n1. Ensure your SSO start URL is correct\n2. Check that your browser can access the SSO portal\n3. Try logging in manually: aws sso login --profile %s", err, output, al.profile)
	}

	fmt.Println("✅ Successfully authenticated with AWS SSO!")
	return nil
}

// ValidateCredentials checks if credentials work by calling STS GetCallerIdentity
func (al *AutoLogin) ValidateCredentials(expectedAccountID, expectedRegion string) (*ValidationResult, error) {
	fmt.Printf("🔍 Validating AWS credentials (profile: %s)...\n", al.profile)

	ctx := context.Background()

	// Load AWS config with the profile
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(al.profile),
		config.WithRegion(expectedRegion),
	)
	if err != nil {
		// Check if SSO token expired
		if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "sso") {
			return nil, fmt.Errorf("SSO token expired or invalid. Please run: aws sso login --profile %s", al.profile)
		}
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	// Call STS GetCallerIdentity
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		// Parse error for helpful messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "token") || strings.Contains(errMsg, "expired") {
			return nil, fmt.Errorf("SSO token expired. Run: aws sso login --profile %s", al.profile)
		}
		if strings.Contains(errMsg, "UnrecognizedClientException") {
			return nil, fmt.Errorf("invalid AWS credentials. Check your AWS configuration")
		}
		if strings.Contains(errMsg, "network") || strings.Contains(errMsg, "connection") {
			return nil, fmt.Errorf("network error. Check your internet connection")
		}
		return nil, fmt.Errorf("AWS API call failed: %w", err)
	}

	// Build validation result
	result := &ValidationResult{
		Profile:   al.profile,
		AccountID: aws.ToString(identity.Account),
		ARN:       aws.ToString(identity.Arn),
		UserID:    aws.ToString(identity.UserId),
		Success:   true,
	}

	// Check account ID if expected value provided
	if expectedAccountID != "" && result.AccountID != expectedAccountID {
		result.Success = false
		result.AccountIDMismatch = true
		result.ExpectedAccountID = expectedAccountID
		return result, fmt.Errorf("account ID mismatch: expected %s, got %s", expectedAccountID, result.AccountID)
	}

	fmt.Printf("✅ Credentials validated successfully!\n")
	fmt.Printf("   Account: %s\n", result.AccountID)
	fmt.Printf("   User: %s\n", result.ARN)

	return result, nil
}

// LoginAndValidate performs both login and validation in one call
func (al *AutoLogin) LoginAndValidate(expectedAccountID, expectedRegion string) (*ValidationResult, error) {
	// First, try to validate without logging in (in case already logged in)
	result, err := al.ValidateCredentials(expectedAccountID, expectedRegion)
	if err == nil {
		// Already logged in and valid
		return result, nil
	}

	// Check if error is due to expired token
	if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "sso") {
		// Try to login
		if loginErr := al.Login(); loginErr != nil {
			return nil, loginErr
		}

		// Retry validation after login
		return al.ValidateCredentials(expectedAccountID, expectedRegion)
	}

	// Some other error occurred
	return nil, err
}

// ValidationResult contains the result of credential validation
type ValidationResult struct {
	Profile            string
	AccountID          string
	ARN                string
	UserID             string
	Success            bool
	AccountIDMismatch  bool
	ExpectedAccountID  string
}

// String returns a formatted string representation
func (vr *ValidationResult) String() string {
	if vr.Success {
		return fmt.Sprintf("✅ Profile '%s' validated\n   Account: %s\n   User: %s",
			vr.Profile, vr.AccountID, vr.ARN)
	}

	if vr.AccountIDMismatch {
		return fmt.Sprintf("❌ Profile '%s' - Account ID mismatch\n   Expected: %s\n   Actual: %s",
			vr.Profile, vr.ExpectedAccountID, vr.AccountID)
	}

	return fmt.Sprintf("❌ Profile '%s' validation failed", vr.Profile)
}

// CheckSSOTokenStatus checks if SSO token exists and is valid
func CheckSSOTokenStatus(profile string) (bool, error) {
	// Try a quick STS call without interactive login
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return false, err
	}

	stsClient := sts.NewFromConfig(cfg)
	_, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err == nil {
		return true, nil // Token is valid
	}

	// Check if error is due to expired/missing token
	errMsg := err.Error()
	if strings.Contains(errMsg, "token") || strings.Contains(errMsg, "expired") || strings.Contains(errMsg, "sso") {
		return false, nil // Token expired/missing
	}

	// Some other error
	return false, err
}
