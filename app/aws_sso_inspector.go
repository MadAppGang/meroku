package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/ini.v1"
)

// Custom AWS config path (for testing)
var customAWSConfigPath string

// ProfileInspector analyzes AWS configuration and detects profile completeness
type ProfileInspector struct {
	configPath string
	config     *ini.File
}

// ProfileInfo contains analyzed profile information
type ProfileInfo struct {
	Name             string
	Exists           bool
	Type             ProfileType
	Complete         bool
	MissingFields    []string
	SSOStartURL      string
	SSORegion        string
	SSOAccountID     string
	SSORoleName      string
	SSOSession       string // For modern SSO
	Region           string
	Output           string
	SSOSessionInfo   *SSOSessionInfo // For modern SSO
}

// SSOSessionInfo contains sso-session section details
type SSOSessionInfo struct {
	Name              string
	Exists            bool
	SSOStartURL       string
	SSORegion         string
	RegistrationScopes string
	Complete          bool
	MissingFields     []string
}

// ProfileType indicates the configuration style
type ProfileType string

const (
	ProfileTypeUnknown     ProfileType = "unknown"
	ProfileTypeModernSSO   ProfileType = "modern_sso"   // Uses sso-session
	ProfileTypeLegacySSO   ProfileType = "legacy_sso"   // Direct SSO fields
	ProfileTypeStaticKeys  ProfileType = "static_keys"  // Access key/secret
	ProfileTypeAssumeRole  ProfileType = "assume_role"  // Role assumption
)

// NewProfileInspector creates a new profile inspector
func NewProfileInspector() (*ProfileInspector, error) {
	configPath := getAWSConfigPath()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &ProfileInspector{
			configPath: configPath,
			config:     nil, // Will be created on write
		}, nil
	}

	// Load existing config
	cfg, err := ini.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AWS config: %w", err)
	}

	return &ProfileInspector{
		configPath: configPath,
		config:     cfg,
	}, nil
}

// InspectProfile analyzes a specific profile
func (pi *ProfileInspector) InspectProfile(profileName string) (*ProfileInfo, error) {
	info := &ProfileInfo{
		Name:          profileName,
		MissingFields: []string{},
	}

	// Check if config file exists
	if pi.config == nil {
		info.Exists = false
		info.Complete = false
		info.MissingFields = []string{"config_file_missing"}
		return info, nil
	}

	// Get section name (default vs named profile)
	sectionName := getSectionName(profileName)

	// Check if profile exists
	if !pi.config.HasSection(sectionName) {
		info.Exists = false
		info.Complete = false
		info.MissingFields = []string{"profile_not_found"}
		return info, nil
	}

	info.Exists = true
	section := pi.config.Section(sectionName)

	// Determine profile type and validate
	info.Type = pi.detectProfileType(section)

	switch info.Type {
	case ProfileTypeModernSSO:
		pi.validateModernSSO(section, info)
	case ProfileTypeLegacySSO:
		pi.validateLegacySSO(section, info)
	case ProfileTypeStaticKeys:
		// Static keys are handled by AWS SDK, we don't validate them
		info.Complete = true
	case ProfileTypeAssumeRole:
		// Assume role profiles need source_profile
		info.Complete = section.HasKey("source_profile")
		if !info.Complete {
			info.MissingFields = append(info.MissingFields, "source_profile")
		}
	default:
		info.Complete = false
		info.MissingFields = append(info.MissingFields, "unknown_profile_type")
	}

	return info, nil
}

// detectProfileType determines what kind of profile this is
func (pi *ProfileInspector) detectProfileType(section *ini.Section) ProfileType {
	// Check for modern SSO (uses sso_session reference)
	if section.HasKey("sso_session") {
		return ProfileTypeModernSSO
	}

	// Check for legacy SSO (has sso_start_url directly)
	if section.HasKey("sso_start_url") {
		return ProfileTypeLegacySSO
	}

	// Check for incomplete SSO profile (has SSO fields but missing session/start_url)
	// Note: sso_region in profile section indicates legacy SSO, not modern SSO
	// In modern SSO, sso_region should be in the sso-session section
	if section.HasKey("sso_account_id") || section.HasKey("sso_role_name") {
		return ProfileTypeModernSSO
	}

	// Check for static keys
	if section.HasKey("aws_access_key_id") {
		return ProfileTypeStaticKeys
	}

	// Check for assume role
	if section.HasKey("source_profile") || section.HasKey("role_arn") {
		return ProfileTypeAssumeRole
	}

	return ProfileTypeUnknown
}

// validateModernSSO validates modern SSO configuration (recommended)
func (pi *ProfileInspector) validateModernSSO(section *ini.Section, info *ProfileInfo) {
	// Profile section required fields
	requiredProfileFields := map[string]*string{
		"sso_session":    &info.SSOSession,
		"sso_account_id": &info.SSOAccountID,
		"sso_role_name":  &info.SSORoleName,
	}

	for field, target := range requiredProfileFields {
		if section.HasKey(field) {
			*target = section.Key(field).String()
		} else {
			info.MissingFields = append(info.MissingFields, field)
		}
	}

	// Optional fields
	if section.HasKey("region") {
		info.Region = section.Key("region").String()
	}
	if section.HasKey("output") {
		info.Output = section.Key("output").String()
	}

	// Validate sso-session section if referenced
	if info.SSOSession != "" {
		info.SSOSessionInfo = pi.validateSSOSession(info.SSOSession)
		if !info.SSOSessionInfo.Complete {
			// Add specific missing fields from sso-session instead of generic error
			for _, field := range info.SSOSessionInfo.MissingFields {
				// Prefix field name to show it's from sso-session section
				info.MissingFields = append(info.MissingFields, fmt.Sprintf("%s (in sso-session '%s')", field, info.SSOSession))
			}
		}
	}

	// Profile is complete if no missing fields and sso-session is valid
	info.Complete = len(info.MissingFields) == 0 &&
		(info.SSOSessionInfo == nil || info.SSOSessionInfo.Complete)
}

// validateLegacySSO validates legacy SSO configuration (not recommended)
func (pi *ProfileInspector) validateLegacySSO(section *ini.Section, info *ProfileInfo) {
	// Legacy SSO required fields (all in profile section)
	requiredFields := map[string]*string{
		"sso_start_url":  &info.SSOStartURL,
		"sso_region":     &info.SSORegion,
		"sso_account_id": &info.SSOAccountID,
		"sso_role_name":  &info.SSORoleName,
	}

	for field, target := range requiredFields {
		if section.HasKey(field) {
			*target = section.Key(field).String()
		} else {
			info.MissingFields = append(info.MissingFields, field)
		}
	}

	// Optional fields
	if section.HasKey("region") {
		info.Region = section.Key("region").String()
	}
	if section.HasKey("output") {
		info.Output = section.Key("output").String()
	}

	info.Complete = len(info.MissingFields) == 0
}

// validateSSOSession validates an sso-session section
func (pi *ProfileInspector) validateSSOSession(sessionName string) *SSOSessionInfo {
	ssoInfo := &SSOSessionInfo{
		Name:          sessionName,
		MissingFields: []string{},
	}

	sectionName := fmt.Sprintf("sso-session %s", sessionName)

	if !pi.config.HasSection(sectionName) {
		ssoInfo.Exists = false
		ssoInfo.MissingFields = append(ssoInfo.MissingFields, "sso_session_not_found")
		ssoInfo.Complete = false
		return ssoInfo
	}

	ssoInfo.Exists = true
	section := pi.config.Section(sectionName)

	// Required fields for sso-session
	requiredFields := map[string]*string{
		"sso_start_url": &ssoInfo.SSOStartURL,
		"sso_region":    &ssoInfo.SSORegion,
	}

	for field, target := range requiredFields {
		if section.HasKey(field) {
			*target = section.Key(field).String()
		} else {
			ssoInfo.MissingFields = append(ssoInfo.MissingFields, field)
		}
	}

	// Optional field
	if section.HasKey("sso_registration_scopes") {
		ssoInfo.RegistrationScopes = section.Key("sso_registration_scopes").String()
	}

	ssoInfo.Complete = len(ssoInfo.MissingFields) == 0
	return ssoInfo
}

// ValidateField validates individual field formats
func (pi *ProfileInspector) ValidateField(fieldName, value string) error {
	switch fieldName {
	case "sso_start_url":
		return validateSSOStartURL(value)
	case "sso_region", "region":
		return validateAWSRegion(value)
	case "sso_account_id":
		return validateAccountID(value)
	case "sso_role_name":
		return validateRoleName(value)
	case "output":
		return validateOutputFormat(value)
	default:
		return nil // Unknown fields are allowed
	}
}

// CheckAWSCLI verifies AWS CLI is installed and meets minimum version
func (pi *ProfileInspector) CheckAWSCLI() error {
	// Check if aws command exists
	_, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("AWS CLI not found in PATH\n\nInstall AWS CLI v2:\n  macOS:   brew install awscli\n  Linux:   curl \"https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip\" -o \"awscliv2.zip\"\n  Windows: Download from https://aws.amazon.com/cli/")
	}

	// Check version
	cmd := exec.Command("aws", "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get AWS CLI version: %w", err)
	}

	// Parse version (e.g., "aws-cli/2.13.0 Python/3.11.4")
	versionStr := string(output)
	versionRegex := regexp.MustCompile(`aws-cli/(\d+)\.(\d+)\.(\d+)`)
	matches := versionRegex.FindStringSubmatch(versionStr)

	if len(matches) < 2 {
		return fmt.Errorf("could not parse AWS CLI version from: %s", versionStr)
	}

	majorVersion := matches[1]
	if majorVersion == "1" {
		return fmt.Errorf("AWS CLI v1.x found, but v2+ is required\n\nUpgrade to AWS CLI v2: https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html")
	}

	return nil
}

// getAWSConfigPath returns the path to AWS config file
func getAWSConfigPath() string {
	// Check custom path first (for testing)
	if customAWSConfigPath != "" {
		return customAWSConfigPath
	}

	// Check environment variable
	if configPath := os.Getenv("AWS_CONFIG_FILE"); configPath != "" {
		return configPath
	}

	// Default location
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

// SetCustomAWSConfigPath sets a custom AWS config path (for testing)
func SetCustomAWSConfigPath(path string) {
	customAWSConfigPath = path
	if path != "" {
		fmt.Printf("🔧 Using custom AWS config: %s\n", path)
	}
}

// getSectionName returns the INI section name for a profile
func getSectionName(profileName string) string {
	if profileName == "default" {
		return "default"
	}
	return fmt.Sprintf("profile %s", profileName)
}

// Validation helper functions

func validateSSOStartURL(url string) error {
	if url == "" {
		return fmt.Errorf("SSO start URL cannot be empty")
	}
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("SSO start URL must start with https://")
	}
	// Common pattern: https://*.awsapps.com/start
	if !strings.Contains(url, ".awsapps.com") && !strings.Contains(url, ".aws.amazon.com") {
		return fmt.Errorf("SSO start URL should be an AWS SSO portal URL (e.g., https://mycompany.awsapps.com/start)")
	}
	return nil
}

func validateAWSRegion(region string) error {
	if region == "" {
		return fmt.Errorf("region cannot be empty")
	}
	// Pattern: us-east-1, eu-west-2, ap-southeast-1, etc.
	regionRegex := regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`)
	if !regionRegex.MatchString(region) {
		return fmt.Errorf("invalid AWS region format: %s (expected format: us-east-1)", region)
	}
	return nil
}

func validateAccountID(accountID string) error {
	if accountID == "" {
		return fmt.Errorf("account ID cannot be empty")
	}
	// Must be exactly 12 digits
	accountRegex := regexp.MustCompile(`^\d{12}$`)
	if !accountRegex.MatchString(accountID) {
		return fmt.Errorf("invalid AWS account ID: %s (must be 12 digits)", accountID)
	}
	return nil
}

func validateRoleName(roleName string) error {
	if roleName == "" {
		return fmt.Errorf("role name cannot be empty")
	}
	// Valid IAM role name characters: alphanumeric, plus, equals, comma, period, at sign, underscore, hyphen
	roleRegex := regexp.MustCompile(`^[\w+=,.@-]+$`)
	if !roleRegex.MatchString(roleName) {
		return fmt.Errorf("invalid IAM role name: %s", roleName)
	}
	return nil
}

func validateOutputFormat(output string) error {
	validFormats := map[string]bool{
		"json":  true,
		"yaml":  true,
		"text":  true,
		"table": true,
	}
	if !validFormats[output] {
		return fmt.Errorf("invalid output format: %s (valid: json, yaml, text, table)", output)
	}
	return nil
}
