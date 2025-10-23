package main

import (
	"fmt"
	"strings"
)

const awsSSODocumentation = `═══════════════════════════════════════════════════════════════════
COMPREHENSIVE AWS SSO CONFIGURATION GUIDE
═══════════════════════════════════════════════════════════════════

AWS SSO (Single Sign-On) uses IAM Identity Center for centralized authentication.

TWO CONFIGURATION STYLES:

1. MODERN SSO (RECOMMENDED - AWS CLI v2+)
   Uses separate [sso-session] section for shared configuration:

   [sso-session my-sso]
   sso_start_url = https://my-company.awsapps.com/start
   sso_region = us-east-1
   sso_registration_scopes = sso:account:access

   [profile dev]
   sso_session = my-sso
   sso_account_id = 123456789012
   sso_role_name = AdministratorAccess
   region = us-east-1
   output = json

   BENEFITS:
   - One sso-session shared by multiple profiles
   - Login once, use all profiles
   - Easier to maintain
   - Required for AWS CLI v2.0+

2. LEGACY SSO (OLD STYLE - Still supported but not recommended)
   All SSO config in profile section:

   [profile dev]
   sso_start_url = https://my-company.awsapps.com/start
   sso_region = us-east-1
   sso_account_id = 123456789012
   sso_role_name = AdministratorAccess
   region = us-east-1
   output = json

   DRAWBACKS:
   - Duplicate config for multiple profiles
   - Must login separately for each profile
   - More error-prone

REQUIRED FIELDS (Modern SSO):
- sso-session section:
  * sso_start_url: HTTPS URL (https://*.awsapps.com/start or https://*.aws.amazon.com/...)
  * sso_region: AWS region where IAM Identity Center is hosted (usually us-east-1)
  * sso_registration_scopes: Usually "sso:account:access" (for API access)

- profile section:
  * sso_session: Name of the sso-session to use
  * sso_account_id: 12-digit AWS account ID
  * sso_role_name: IAM role name (e.g., AdministratorAccess, PowerUserAccess)
  * region: Default AWS region for this profile
  * output: Output format (json, yaml, text, table)

VALIDATION RULES:
- sso_start_url must start with https://
- sso_start_url typically contains .awsapps.com or .aws.amazon.com
- sso_region must be a valid AWS region (e.g., us-east-1, eu-west-1)
- sso_account_id must be exactly 12 digits
- sso_role_name must be valid IAM role name (alphanumeric, +, =, ., @, -, _)
- region must be a valid AWS region

COMMON ROLE NAMES:
- AdministratorAccess: Full access to all AWS services
- PowerUserAccess: Full access except IAM management
- ReadOnlyAccess: Read-only access to all AWS services
- ViewOnlyAccess: Similar to ReadOnly
- SecurityAudit: Audit and compliance access
- DatabaseAdministrator: Database management
- NetworkAdministrator: Network infrastructure management
- SystemAdministrator: System administration tasks
`

const commonSSOIssues = `═══════════════════════════════════════════════════════════════════
COMMON AWS SSO ISSUES AND SOLUTIONS
═══════════════════════════════════════════════════════════════════

ISSUE 1: "Error loading SSO Token"
CAUSE: No SSO session cached or session expired
SOLUTION:
  1. Run: aws sso login --profile {profile}
  2. Browser will open for authentication
  3. Complete authentication in browser
  4. Return to CLI, credentials cached

ISSUE 2: "Profile not found"
CAUSE: AWS config file missing or profile not defined
SOLUTION:
  1. Check if ~/.aws/config exists
  2. Verify profile section exists: [profile {name}]
  3. If missing, create config file with proper permissions (600)
  4. Write profile configuration

ISSUE 3: "Invalid sso_region"
CAUSE: Region format wrong or unsupported region
SOLUTION:
  1. Use standard AWS region format: us-east-1 (not US-EAST-1)
  2. sso_region is where IAM Identity Center is hosted (usually us-east-1)
  3. This is different from the default region where resources run

ISSUE 4: "SSO authentication failed"
CAUSE: Invalid SSO start URL or wrong credentials
SOLUTION:
  1. Verify SSO start URL is correct (ask user to confirm)
  2. Check URL format: https://*.awsapps.com/start
  3. Ensure user has access to the SSO portal
  4. Try login again after correcting URL

ISSUE 5: "Account ID mismatch"
CAUSE: YAML config has different account ID than AWS returns
SOLUTION:
  1. Run: aws sts get-caller-identity --profile {profile}
  2. Compare returned Account ID with YAML config
  3. Ask user which is correct
  4. Update YAML or AWS config accordingly

ISSUE 6: "Permission denied"
CAUSE: Wrong role selected or insufficient permissions
SOLUTION:
  1. Verify role name is correct (case-sensitive)
  2. Check user has permission to assume this role in SSO portal
  3. Try a different role (e.g., ReadOnlyAccess instead of AdministratorAccess)

ISSUE 7: "sso-session not found"
CAUSE: Profile references non-existent sso-session
SOLUTION:
  1. Check [sso-session {name}] section exists in config
  2. Verify sso_session value in profile matches section name exactly
  3. Create missing sso-session section if needed

ISSUE 8: "AWS CLI version too old"
CAUSE: AWS CLI v1.x doesn't support modern SSO
SOLUTION:
  1. Check version: aws --version
  2. If v1.x, ask user to upgrade to v2+
  3. Provide installation instructions for their OS

TROUBLESHOOTING WORKFLOW:
1. CHECK: Does AWS CLI exist and is v2+?
2. CHECK: Does ~/.aws/config file exist?
3. CHECK: Does profile section exist in config?
4. CHECK: If modern SSO, does sso-session section exist?
5. CHECK: Are all required fields present?
6. CHECK: Do field values pass validation?
7. TRY: Run aws sso login
8. TRY: Run aws sts get-caller-identity
9. VERIFY: Account ID matches expected value
10. SUCCESS: Profile works!
`

const awsSSOTools = `═══════════════════════════════════════════════════════════════════
YOUR AVAILABLE TOOLS
═══════════════════════════════════════════════════════════════════

1. THINK - Analyze the situation and reason about next steps
   Format: THINK: <your reasoning>
   Use to: Show your logical process to the user

2. read_aws_config - Read AWS configuration file
   Format: read_aws_config: ~/.aws/config
   Use to:
   - See current configuration structure
   - List existing profiles
   - Find sso-session sections
   - Understand what needs to be fixed

3. write_aws_config - Write complete AWS configuration
   Format: write_aws_config: <full config content>
   Use to:
   - Create new configuration
   - Update existing configuration
   - Fix configuration issues
   Note: Always creates backup before writing

4. read_yaml - Read project YAML configuration
   Format: read_yaml: project/dev.yaml
   Use to:
   - Get account_id from YAML
   - Get region from YAML
   - Understand project configuration

5. write_yaml - Update YAML configuration
   Format: write_yaml: FILE:project/dev.yaml|OLD:old_text|NEW:new_text
   Use to:
   - Sync account_id between YAML and AWS
   - Update profile name in YAML

6. ask_choice - Ask user to select from options
   Format: ask_choice: QUESTION:Which role?|OPTIONS:Admin,Power,ReadOnly
   Use to:
   - Get user preference from predefined choices
   - Select IAM role
   - Choose AWS region
   - Pick configuration style

7. ask_confirm - Ask user yes/no question
   Format: ask_confirm: Replace existing SSO config?
   Use to:
   - Confirm before destructive actions
   - Verify user intent
   - Get permission to proceed

8. ask_input - Ask user for text input with validation
   Format: ask_input: QUESTION:SSO URL?|VALIDATOR:url|PLACEHOLDER:https://...
   Use to:
   - Get SSO start URL
   - Get account ID
   - Get any user-provided value
   Validators: url, region, account_id, role_name, none

9. web_search - Search for AWS documentation
   Format: web_search: AWS SSO InvalidParameterException
   Use to:
   - Research unknown errors
   - Find AWS documentation
   - Learn about new issues
   - Get best practices

10. aws_validate - Validate AWS configuration
    Format: aws_validate: sso_login|profile:dev
    Format: aws_validate: credentials|profile:dev|account:123456789012
    Format: aws_validate: cli_version
    Use to:
    - Test SSO login after config
    - Verify credentials work
    - Check AWS CLI version

11. EXEC - Execute AWS CLI or shell commands
    Format: EXEC: <command>
    Examples:
    - EXEC: aws sso login --profile dev
    - EXEC: aws sts get-caller-identity --profile dev
    - EXEC: aws --version
    Use to:
    - Run direct AWS CLI commands
    - Execute validation commands
    - Test configuration

12. COMPLETE - Mark setup as complete
    Format: COMPLETE: <summary message>
    Use when: AWS SSO is fully configured and validated

TOOL SELECTION STRATEGY:
- Start with read_aws_config to understand current state
- Use read_yaml to get account/region from project config
- Ask user for missing critical info (SSO URL, role name)
- Write configuration once you have all required fields
- Validate with aws_validate after writing
- If validation fails, use web_search to research errors
- Mark COMPLETE only after successful validation
`

// BuildEnhancedSystemPrompt constructs the comprehensive system prompt
func BuildEnhancedSystemPrompt(ctx *SSOAgentContext) string {
	var b strings.Builder

	// Header
	b.WriteString("You are an AWS SSO configuration expert. Your goal is to set up AWS SSO profiles correctly and efficiently.\n\n")

	// Documentation sections
	b.WriteString(awsSSODocumentation)
	b.WriteString("\n\n")
	b.WriteString(commonSSOIssues)
	b.WriteString("\n\n")
	b.WriteString(awsSSOTools)
	b.WriteString("\n\n")

	// Current situation
	b.WriteString("═══════════════════════════════════════════════════════════════════\n")
	b.WriteString("CURRENT SITUATION\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════\n\n")
	b.WriteString(fmt.Sprintf("Profile Name: %s\n", ctx.ProfileName))
	b.WriteString(fmt.Sprintf("Config File: %s\n", ctx.ConfigPath))
	b.WriteString(fmt.Sprintf("Config Exists: %v\n\n", ctx.ConfigExists))

	// Profile status
	b.WriteString("Profile Status:\n")
	if ctx.ProfileInfo == nil {
		b.WriteString("Profile not analyzed yet\n")
	} else if !ctx.ProfileInfo.Exists {
		b.WriteString("Profile does not exist - needs to be created\n")
	} else if ctx.ProfileInfo.Complete {
		b.WriteString(fmt.Sprintf("Profile exists and is complete (Type: %s)\n", ctx.ProfileInfo.Type))
	} else {
		b.WriteString(fmt.Sprintf("Profile exists but incomplete (Type: %s, Missing: %s)\n",
			ctx.ProfileInfo.Type, strings.Join(ctx.ProfileInfo.MissingFields, ", ")))
	}
	b.WriteString("\n")

	// Available information
	b.WriteString("Available Information:\n")
	if ctx.YAMLEnv != nil {
		b.WriteString(fmt.Sprintf("- From YAML: AccountID=%s, Region=%s\n",
			ctx.YAMLEnv.AccountID, ctx.YAMLEnv.Region))
	}
	b.WriteString(fmt.Sprintf("- Collected: SSOStartURL=%s, SSORegion=%s, AccountID=%s, RoleName=%s, Region=%s\n\n",
		ctx.SSOStartURL, ctx.SSORegion, ctx.AccountID, ctx.RoleName, ctx.Region))

	// AWS Config content if available
	if ctx.AWSConfigContent != "" {
		b.WriteString("Current AWS Config Content:\n")
		b.WriteString("```\n")
		b.WriteString(ctx.AWSConfigContent)
		b.WriteString("\n```\n\n")
	}

	// Strategy
	b.WriteString("═══════════════════════════════════════════════════════════════════\n")
	b.WriteString("STRATEGY\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════\n\n")
	b.WriteString("1. Analyze what information is missing\n")
	b.WriteString("2. Ask user for missing critical information (SSO start URL is most important)\n")
	b.WriteString("3. Use information from YAML when available (don't ask for what you have)\n")
	b.WriteString("4. Write configuration once you have all required fields\n")
	b.WriteString("5. Execute \"aws sso login\" to authenticate\n")
	b.WriteString("6. Validate with \"aws sts get-caller-identity\"\n")
	b.WriteString("7. Verify account ID matches expected value\n")
	b.WriteString("8. Mark as COMPLETE when everything works\n\n")

	b.WriteString("Ask questions in priority order:\n")
	b.WriteString("1. SSO Start URL (critical, can't proceed without it)\n")
	b.WriteString("2. Role Name (has good defaults like AdministratorAccess)\n")
	b.WriteString("3. SSO Region (defaults to us-east-1)\n")
	b.WriteString("4. Other optional fields\n\n")

	b.WriteString("Don't ask for information you already have from YAML or previous answers!\n\n")

	// Action history
	b.WriteString("═══════════════════════════════════════════════════════════════════\n")
	b.WriteString("ACTION HISTORY (Recent actions taken)\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════\n\n")

	if len(ctx.ActionHistory) == 0 {
		b.WriteString("No actions taken yet\n")
	} else {
		// Show last 5 actions
		start := len(ctx.ActionHistory) - 5
		if start < 0 {
			start = 0
		}

		for i := start; i < len(ctx.ActionHistory); i++ {
			action := ctx.ActionHistory[i]
			b.WriteString(fmt.Sprintf("%d. %s", i+1, action.Type))
			if action.Description != "" {
				b.WriteString(fmt.Sprintf(": %s", action.Description))
			}
			if action.Question != "" {
				b.WriteString(fmt.Sprintf(" | Question: %s | Answer: %s", action.Question, action.Answer))
			}
			if action.Result != "" {
				// Truncate long results
				result := action.Result
				if len(result) > 200 {
					result = result[:200] + "..."
				}
				b.WriteString(fmt.Sprintf(" | Result: %s", result))
			}
			if action.Error != nil {
				b.WriteString(fmt.Sprintf(" | Error: %v", action.Error))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Final instructions
	b.WriteString("═══════════════════════════════════════════════════════════════════\n")
	b.WriteString("WHAT SHOULD YOU DO NEXT?\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════\n\n")

	b.WriteString("CRITICAL: You MUST respond with BOTH a THINK line AND an ACTION line.\n\n")

	b.WriteString("RESPONSE FORMAT (REQUIRED):\n")
	b.WriteString("Line 1: THINK: <brief reasoning about what to do next>\n")
	b.WriteString("Line 2: <ACTION>: <command>\n\n")

	b.WriteString("EXAMPLES OF CORRECT RESPONSES:\n\n")

	b.WriteString("Example 1 - Reading config:\n")
	b.WriteString("THINK: Need to see what's currently in the AWS config file\n")
	b.WriteString("read_aws_config: ~/.aws/config\n\n")

	b.WriteString("Example 2 - Asking for SSO URL:\n")
	b.WriteString("THINK: Missing SSO start URL, must ask user\n")
	b.WriteString("ask_input: QUESTION:What is your SSO start URL?|VALIDATOR:url|PLACEHOLDER:https://mycompany.awsapps.com/start\n\n")

	b.WriteString("Example 3 - Writing config:\n")
	b.WriteString("THINK: Have all required info, writing modern SSO config\n")
	b.WriteString("write_aws_config: [sso-session my-sso]\nsso_start_url = https://example.awsapps.com/start\n...\n\n")

	b.WriteString("Example 4 - Validating:\n")
	b.WriteString("THINK: Config written, now test SSO login\n")
	b.WriteString("aws_validate: sso_login|profile:dev\n\n")

	b.WriteString("IMPORTANT RULES:\n")
	b.WriteString("- NEVER output just THINK alone - you MUST follow with an ACTION\n")
	b.WriteString("- THINK line explains WHY you're doing something\n")
	b.WriteString("- ACTION line specifies WHAT tool to use and HOW\n")
	b.WriteString("- If you don't know what action to take, use read_aws_config first\n")
	b.WriteString("- Ask ONE question at a time (use ask_input for missing info)\n")
	b.WriteString("- Use YAML data to avoid redundant questions\n")
	b.WriteString("- Be concise and helpful\n\n")

	b.WriteString("NEXT STEP LOGIC:\n")
	if len(ctx.ActionHistory) == 0 {
		b.WriteString("This is your FIRST action - start by reading the AWS config file:\n")
		b.WriteString("THINK: Need to see current configuration\n")
		b.WriteString("read_aws_config: ~/.aws/config\n")
	} else {
		lastAction := ctx.ActionHistory[len(ctx.ActionHistory)-1]
		b.WriteString(fmt.Sprintf("Your LAST action was: %s\n", lastAction.Type))
		if lastAction.Type == "think" {
			b.WriteString("⚠️  WARNING: You just did THINK without an action! You MUST take an action now.\n")
			if ctx.AWSConfigContent == "" {
				b.WriteString("→ Read the AWS config file first: read_aws_config: ~/.aws/config\n")
			} else if ctx.SSOStartURL == "" {
				b.WriteString("→ Ask for SSO start URL: ask_input: QUESTION:What is your SSO start URL?|VALIDATOR:url|PLACEHOLDER:https://mycompany.awsapps.com/start\n")
			}
		}
	}

	return b.String()
}
