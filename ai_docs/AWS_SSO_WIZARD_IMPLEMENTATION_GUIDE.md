# AWS SSO Setup Wizard - Implementation Guide

## Quick Start for Developers

This guide provides practical implementation guidance for building the AWS SSO Setup Wizard.

## File Structure

```
app/
├── aws_sso_profile_inspector.go    # Profile analysis and detection
├── aws_sso_setup_wizard_tui.go     # Bubble Tea interactive wizard
├── aws_config_writer.go            # Safe config file writer
├── aws_sso_auto_login.go           # Login automation and validation
└── aws_sso_setup_wizard_test.go    # Tests for all components
```

---

## Part 1: Profile Inspector

### Purpose
Analyze existing AWS configuration and identify what's missing.

### Key Functions

```go
// app/aws_sso_profile_inspector.go

package main

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// ProfileAnalysis contains the result of analyzing a profile
type ProfileAnalysis struct {
    ProfileName      string
    Exists           bool
    IsComplete       bool
    MissingFields    []string

    // Current values (empty if missing)
    SSOSessionName   string
    SSOStartURL      string
    SSORegion        string
    SSOAccountID     string
    RoleName         string
    Region           string

    // Suggested values from YAML
    SuggestedRegion    string
    SuggestedAccountID string
}

// SSOSessionInfo contains SSO session details
type SSOSessionInfo struct {
    Name               string
    StartURL           string
    Region             string
    RegistrationScopes string
    Exists             bool
}

// ProfileInspector analyzes AWS profiles
type ProfileInspector struct {
    ConfigPath string
}

// NewProfileInspector creates a new inspector
func NewProfileInspector() (*ProfileInspector, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("failed to get home directory: %w", err)
    }

    return &ProfileInspector{
        ConfigPath: filepath.Join(homeDir, ".aws", "config"),
    }, nil
}

// InspectProfile analyzes a single profile
func (pi *ProfileInspector) InspectProfile(profileName string) (ProfileAnalysis, error) {
    analysis := ProfileAnalysis{
        ProfileName: profileName,
        Exists:      false,
        IsComplete:  false,
    }

    // Read config file
    content, err := os.ReadFile(pi.ConfigPath)
    if err != nil {
        if os.IsNotExist(err) {
            return analysis, nil // Config doesn't exist yet
        }
        return analysis, fmt.Errorf("failed to read config: %w", err)
    }

    // Parse config file
    lines := strings.Split(string(content), "\n")
    inProfile := false

    for _, line := range lines {
        line = strings.TrimSpace(line)

        // Check for profile section
        if strings.HasPrefix(line, "[profile ") && strings.HasSuffix(line, "]") {
            profile := strings.TrimPrefix(line, "[profile ")
            profile = strings.TrimSuffix(profile, "]")

            if profile == profileName {
                inProfile = true
                analysis.Exists = true
            } else {
                inProfile = false
            }
            continue
        }

        // Parse fields within profile
        if inProfile && strings.Contains(line, "=") {
            parts := strings.SplitN(line, "=", 2)
            if len(parts) != 2 {
                continue
            }

            key := strings.TrimSpace(parts[0])
            value := strings.TrimSpace(parts[1])

            switch key {
            case "sso_session":
                analysis.SSOSessionName = value
            case "sso_account_id":
                analysis.SSOAccountID = value
            case "sso_role_name":
                analysis.RoleName = value
            case "region":
                analysis.Region = value
            }
        }
    }

    // Check completeness
    if analysis.Exists {
        analysis.IsComplete = analysis.SSOSessionName != "" &&
            analysis.SSOAccountID != "" &&
            analysis.RoleName != "" &&
            analysis.Region != ""

        // Collect missing fields
        if analysis.SSOSessionName == "" {
            analysis.MissingFields = append(analysis.MissingFields, "sso_session")
        }
        if analysis.SSOAccountID == "" {
            analysis.MissingFields = append(analysis.MissingFields, "sso_account_id")
        }
        if analysis.RoleName == "" {
            analysis.MissingFields = append(analysis.MissingFields, "sso_role_name")
        }
        if analysis.Region == "" {
            analysis.MissingFields = append(analysis.MissingFields, "region")
        }
    }

    return analysis, nil
}

// ListSSOSessions returns all SSO sessions in config
func (pi *ProfileInspector) ListSSOSessions() ([]SSOSessionInfo, error) {
    content, err := os.ReadFile(pi.ConfigPath)
    if err != nil {
        if os.IsNotExist(err) {
            return []SSOSessionInfo{}, nil
        }
        return nil, fmt.Errorf("failed to read config: %w", err)
    }

    var sessions []SSOSessionInfo
    var currentSession *SSOSessionInfo

    lines := strings.Split(string(content), "\n")

    for _, line := range lines {
        line = strings.TrimSpace(line)

        // Check for SSO session section
        if strings.HasPrefix(line, "[sso-session ") && strings.HasSuffix(line, "]") {
            // Save previous session
            if currentSession != nil {
                sessions = append(sessions, *currentSession)
            }

            // Start new session
            sessionName := strings.TrimPrefix(line, "[sso-session ")
            sessionName = strings.TrimSuffix(sessionName, "]")
            currentSession = &SSOSessionInfo{
                Name:   sessionName,
                Exists: true,
            }
            continue
        }

        // Parse session fields
        if currentSession != nil && strings.Contains(line, "=") {
            parts := strings.SplitN(line, "=", 2)
            if len(parts) != 2 {
                continue
            }

            key := strings.TrimSpace(parts[0])
            value := strings.TrimSpace(parts[1])

            switch key {
            case "sso_start_url":
                currentSession.StartURL = value
            case "sso_region":
                currentSession.Region = value
            case "sso_registration_scopes":
                currentSession.RegistrationScopes = value
            }
        }

        // End of section
        if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[sso-session ") {
            if currentSession != nil {
                sessions = append(sessions, *currentSession)
                currentSession = nil
            }
        }
    }

    // Save last session
    if currentSession != nil {
        sessions = append(sessions, *currentSession)
    }

    return sessions, nil
}

// InspectEnvironmentProfiles analyzes all profiles for project environments
func (pi *ProfileInspector) InspectEnvironmentProfiles(yamlConfigs map[string]Env) map[string]ProfileAnalysis {
    results := make(map[string]ProfileAnalysis)

    for envName, envConfig := range yamlConfigs {
        // Determine profile name (use aws_profile from YAML if set, else use env name)
        profileName := envConfig.AWSProfile
        if profileName == "" {
            profileName = envName
        }

        analysis, err := pi.InspectProfile(profileName)
        if err != nil {
            // Log error but continue
            fmt.Printf("Warning: failed to inspect profile %s: %v\n", profileName, err)
            continue
        }

        // Add suggestions from YAML
        analysis.SuggestedRegion = envConfig.Region
        analysis.SuggestedAccountID = envConfig.AccountID

        results[envName] = analysis
    }

    return results
}
```

### Testing

```go
// app/aws_sso_profile_inspector_test.go

func TestProfileInspector_InspectProfile_NotExists(t *testing.T) {
    // Test case: profile doesn't exist
}

func TestProfileInspector_InspectProfile_Complete(t *testing.T) {
    // Test case: profile exists and is complete
}

func TestProfileInspector_InspectProfile_Incomplete(t *testing.T) {
    // Test case: profile exists but missing fields
}

func TestProfileInspector_ListSSOSessions(t *testing.T) {
    // Test case: parse SSO sessions from config
}
```

---

## Part 2: Config Writer

### Purpose
Safely write profiles and SSO sessions to `~/.aws/config` with backup.

### Key Functions

```go
// app/aws_config_writer.go

package main

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// ConfigWriter safely writes to AWS config
type ConfigWriter struct {
    ConfigPath string
}

// ProfileConfig represents a profile to write
type ProfileConfig struct {
    ProfileName     string
    SSOSessionName  string
    SSOAccountID    string
    SSORoleName     string
    Region          string
}

// SSOSessionConfig represents an SSO session to write
type SSOSessionConfig struct {
    Name               string
    StartURL           string
    Region             string
    RegistrationScopes string
}

// NewConfigWriter creates a new config writer
func NewConfigWriter() (*ConfigWriter, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("failed to get home directory: %w", err)
    }

    configPath := filepath.Join(homeDir, ".aws", "config")

    // Ensure .aws directory exists
    awsDir := filepath.Dir(configPath)
    if err := os.MkdirAll(awsDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create .aws directory: %w", err)
    }

    return &ConfigWriter{
        ConfigPath: configPath,
    }, nil
}

// BackupConfig creates a timestamped backup
func (cw *ConfigWriter) BackupConfig() (string, error) {
    // Check if config exists
    if _, err := os.Stat(cw.ConfigPath); os.IsNotExist(err) {
        return "", nil // No config to backup
    }

    // Create backup path with timestamp
    timestamp := time.Now().Format("20060102_150405")
    backupPath := fmt.Sprintf("%s.backup_%s", cw.ConfigPath, timestamp)

    // Copy file
    input, err := os.ReadFile(cw.ConfigPath)
    if err != nil {
        return "", fmt.Errorf("failed to read config: %w", err)
    }

    if err := os.WriteFile(backupPath, input, 0644); err != nil {
        return "", fmt.Errorf("failed to write backup: %w", err)
    }

    return backupPath, nil
}

// CreateOrUpdateSSOSession writes or updates an SSO session
func (cw *ConfigWriter) CreateOrUpdateSSOSession(session SSOSessionConfig) error {
    // Backup first
    backupPath, err := cw.BackupConfig()
    if err != nil {
        return fmt.Errorf("failed to backup config: %w", err)
    }
    if backupPath != "" {
        fmt.Printf("✅ Backup created: %s\n", backupPath)
    }

    // Read existing config
    content := ""
    if _, err := os.Stat(cw.ConfigPath); err == nil {
        data, err := os.ReadFile(cw.ConfigPath)
        if err != nil {
            return fmt.Errorf("failed to read config: %w", err)
        }
        content = string(data)
    }

    // Check if session already exists
    sectionHeader := fmt.Sprintf("[sso-session %s]", session.Name)

    if strings.Contains(content, sectionHeader) {
        // Update existing session
        return cw.updateSSOSession(session)
    }

    // Append new session
    sessionBlock := fmt.Sprintf(`
[sso-session %s]
sso_start_url = %s
sso_region = %s
sso_registration_scopes = %s
`,
        session.Name,
        session.StartURL,
        session.Region,
        session.RegistrationScopes,
    )

    file, err := os.OpenFile(cw.ConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open config: %w", err)
    }
    defer file.Close()

    if _, err := file.WriteString(sessionBlock); err != nil {
        return fmt.Errorf("failed to write session: %w", err)
    }

    return nil
}

// CreateOrUpdateProfile writes or updates a profile
func (cw *ConfigWriter) CreateOrUpdateProfile(profile ProfileConfig) error {
    // Backup first (if not already done)

    // Read existing config
    content := ""
    if _, err := os.Stat(cw.ConfigPath); err == nil {
        data, err := os.ReadFile(cw.ConfigPath)
        if err != nil {
            return fmt.Errorf("failed to read config: %w", err)
        }
        content = string(data)
    }

    // Check if profile already exists
    sectionHeader := fmt.Sprintf("[profile %s]", profile.ProfileName)

    if strings.Contains(content, sectionHeader) {
        // Update existing profile
        return cw.updateProfile(profile)
    }

    // Append new profile
    profileBlock := fmt.Sprintf(`
[profile %s]
credential_process = aws configure export-credentials --profile %s
sso_session = %s
sso_account_id = %s
sso_role_name = %s
region = %s
`,
        profile.ProfileName,
        profile.ProfileName,
        profile.SSOSessionName,
        profile.SSOAccountID,
        profile.SSORoleName,
        profile.Region,
    )

    file, err := os.OpenFile(cw.ConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open config: %w", err)
    }
    defer file.Close()

    if _, err := file.WriteString(profileBlock); err != nil {
        return fmt.Errorf("failed to write profile: %w", err)
    }

    return nil
}

// updateProfile updates an existing profile (helper)
func (cw *ConfigWriter) updateProfile(profile ProfileConfig) error {
    // Read config
    data, err := os.ReadFile(cw.ConfigPath)
    if err != nil {
        return fmt.Errorf("failed to read config: %w", err)
    }

    lines := strings.Split(string(data), "\n")
    newLines := []string{}

    inProfile := false
    profileHeader := fmt.Sprintf("[profile %s]", profile.ProfileName)

    for i, line := range lines {
        trimmed := strings.TrimSpace(line)

        // Check for profile section
        if trimmed == profileHeader {
            inProfile = true
            newLines = append(newLines, line)

            // Replace all fields in this section
            // Skip old fields and add new ones
            j := i + 1
            for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
                j++
            }

            // Add updated fields
            newLines = append(newLines,
                fmt.Sprintf("credential_process = aws configure export-credentials --profile %s", profile.ProfileName),
                fmt.Sprintf("sso_session = %s", profile.SSOSessionName),
                fmt.Sprintf("sso_account_id = %s", profile.SSOAccountID),
                fmt.Sprintf("sso_role_name = %s", profile.SSORoleName),
                fmt.Sprintf("region = %s", profile.Region),
            )

            // Skip old section content
            continue
        }

        if inProfile && strings.HasPrefix(trimmed, "[") {
            inProfile = false
        }

        if !inProfile {
            newLines = append(newLines, line)
        }
    }

    // Write back
    newContent := strings.Join(newLines, "\n")
    if err := os.WriteFile(cw.ConfigPath, []byte(newContent), 0644); err != nil {
        return fmt.Errorf("failed to write config: %w", err)
    }

    return nil
}

// Similar updateSSOSession helper...
```

---

## Part 3: Auto-Login & Validation

### Purpose
Automate SSO login and validate credentials work.

### Key Functions

```go
// app/aws_sso_auto_login.go

package main

import (
    "context"
    "fmt"
    "os"
    "os/exec"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/sts"
)

// PerformSSOLogin runs 'aws sso login' for a profile
func PerformSSOLogin(profileName string) error {
    fmt.Printf("🔐 Running SSO login for profile: %s\n", profileName)

    cmd := exec.Command("aws", "sso", "login", "--profile", profileName)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("SSO login failed: %w", err)
    }

    fmt.Println("✅ SSO login successful")
    return nil
}

// ValidateProfileCredentials tests if credentials work
func ValidateProfileCredentials(profileName string) (*sts.GetCallerIdentityOutput, error) {
    // Temporarily set profile
    oldProfile := os.Getenv("AWS_PROFILE")
    os.Setenv("AWS_PROFILE", profileName)
    defer os.Setenv("AWS_PROFILE", oldProfile)

    ctx := context.Background()
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to load config: %w", err)
    }

    stsClient := sts.NewFromConfig(cfg)
    result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
    if err != nil {
        return nil, fmt.Errorf("failed to get caller identity: %w", err)
    }

    fmt.Printf("✅ Credentials validated - Account: %s\n", *result.Account)
    return result, nil
}

// UpdateYAMLWithProfile updates environment YAML with profile info
func UpdateYAMLWithProfile(envName, profileName, accountID string) error {
    // Load env
    env, err := loadEnv(envName)
    if err != nil {
        return fmt.Errorf("failed to load env: %w", err)
    }

    // Update fields
    env.AWSProfile = profileName
    env.AccountID = accountID

    // Save
    if err := saveEnvToFile(env, envName+".yaml"); err != nil {
        return fmt.Errorf("failed to save env: %w", err)
    }

    fmt.Printf("✅ %s.yaml updated with profile and account ID\n", envName)
    return nil
}
```

---

## Part 4: Wizard TUI

### Purpose
Interactive multi-step wizard using Bubble Tea.

### Key Structure

```go
// app/aws_sso_setup_wizard_tui.go

package main

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/huh"
)

type wizardStep int

const (
    stepWelcome wizardStep = iota
    stepSSOSession
    stepEnvironments
    stepConfirmation
    stepWriting
    stepAuthentication
    stepValidation
    stepComplete
)

type SetupWizardModel struct {
    step              wizardStep
    environments      []string
    currentEnvIndex   int

    // Analysis results
    profileAnalysis   map[string]ProfileAnalysis
    ssoSessions       []SSOSessionInfo

    // User inputs
    ssoSessionName    string
    ssoStartURL       string
    ssoRegion         string
    envInputs         map[string]map[string]string // env -> field -> value

    // Components
    inspector         *ProfileInspector
    writer            *ConfigWriter

    // State
    error             error
    complete          bool
}

// RunSSOSetupWizard is the main entry point
func RunSSOSetupWizard(environments []string) error {
    // Initialize components
    inspector, err := NewProfileInspector()
    if err != nil {
        return err
    }

    writer, err := NewConfigWriter()
    if err != nil {
        return err
    }

    // Create model
    model := SetupWizardModel{
        step:         stepWelcome,
        environments: environments,
        inspector:    inspector,
        writer:       writer,
        envInputs:    make(map[string]map[string]string),
    }

    // Run Bubble Tea program
    p := tea.NewProgram(&model)
    if _, err := p.Run(); err != nil {
        return err
    }

    return model.error
}

// Implement tea.Model interface
func (m SetupWizardModel) Init() tea.Cmd {
    return m.inspectConfiguration()
}

func (m SetupWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "q":
            return m, tea.Quit
        }

    case inspectionCompleteMsg:
        // Move to next step based on analysis
        m.profileAnalysis = msg.profiles
        m.ssoSessions = msg.sessions
        return m, m.nextStep()
    }

    return m, nil
}

func (m SetupWizardModel) View() string {
    // Render based on current step
    switch m.step {
    case stepWelcome:
        return m.viewWelcome()
    case stepSSOSession:
        return m.viewSSOSessionSetup()
    case stepEnvironments:
        return m.viewEnvironmentSetup()
    // ... other steps
    }

    return ""
}

// Helper commands
type inspectionCompleteMsg struct {
    profiles map[string]ProfileAnalysis
    sessions []SSOSessionInfo
}

func (m *SetupWizardModel) inspectConfiguration() tea.Cmd {
    return func() tea.Msg {
        // Inspect profiles
        // ...

        return inspectionCompleteMsg{
            profiles: profiles,
            sessions: sessions,
        }
    }
}
```

### Huh Forms Integration

```go
// Use huh for interactive prompts

func (m *SetupWizardModel) promptSSOSession() error {
    var startURL, region, sessionName string

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewInput().
                Title("SSO Start URL").
                Description("e.g., https://mycompany.awsapps.com/start").
                Value(&startURL).
                Validate(func(str string) error {
                    if !strings.HasPrefix(str, "https://") {
                        return fmt.Errorf("must start with https://")
                    }
                    return nil
                }),

            huh.NewSelect[string]().
                Title("SSO Region").
                Options(getRegionOptions()...).
                Value(&region),

            huh.NewInput().
                Title("Session Name").
                Value(&sessionName).
                Placeholder("mycompany"),
        ),
    )

    if err := form.Run(); err != nil {
        return err
    }

    m.ssoStartURL = startURL
    m.ssoRegion = region
    m.ssoSessionName = sessionName

    return nil
}

func (m *SetupWizardModel) promptEnvironment(envName string) error {
    analysis := m.profileAnalysis[envName]

    var accountID, roleName, region string

    // Pre-fill from suggestions
    accountID = analysis.SuggestedAccountID
    region = analysis.SuggestedRegion
    roleName = "AdministratorAccess"

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewNote().
                Title(fmt.Sprintf("Environment: %s", envName)).
                Description(fmt.Sprintf("Region: %s (from YAML)", region)),

            huh.NewInput().
                Title("AWS Account ID").
                Description("12-digit account ID").
                Value(&accountID).
                Validate(func(str string) error {
                    if len(str) != 12 {
                        return fmt.Errorf("must be 12 digits")
                    }
                    return nil
                }),

            huh.NewInput().
                Title("Role Name").
                Value(&roleName).
                Placeholder("AdministratorAccess"),

            huh.NewInput().
                Title("Region").
                Value(&region),
        ),
    )

    if err := form.Run(); err != nil {
        return err
    }

    // Store inputs
    m.envInputs[envName] = map[string]string{
        "account_id": accountID,
        "role_name":  roleName,
        "region":     region,
    }

    return nil
}
```

---

## Part 5: Integration

### Main Menu Integration

```go
// app/main.go

func mainMenu() {
    choices := []string{
        "Deploy Infrastructure",
        "Plan Infrastructure Changes",
        "⚙️  AWS SSO Setup Wizard",  // NEW
        "🔐 Select AWS Profile",
        // ... rest of menu
    }

    // Handle selection
    switch selectedChoice {
    case "⚙️  AWS SSO Setup Wizard":
        // Load all environments
        envs := getEnvironments()
        if err := RunSSOSetupWizard(envs); err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    // ... other cases
    }
}
```

### Pre-Flight Check Integration

```go
// app/aws_preflight.go

func AWSPreflightCheck(env Env) error {
    fmt.Println("\n🔍 Running AWS pre-flight checks...")

    // Validate profile
    awsProfile := os.Getenv("AWS_PROFILE")
    if awsProfile == "" && env.AWSProfile != "" {
        awsProfile = env.AWSProfile
        os.Setenv("AWS_PROFILE", awsProfile)
    }

    if awsProfile == "" {
        // Offer to run wizard
        var runWizard bool
        huh.NewConfirm().
            Title("No AWS profile configured. Set up now?").
            Value(&runWizard).
            Run()

        if runWizard {
            if err := RunSSOSetupWizard([]string{env.Env}); err != nil {
                return err
            }
            // Retry pre-flight check after wizard
            return AWSPreflightCheck(env)
        }

        return fmt.Errorf("AWS profile required")
    }

    // Continue with validation...
}
```

---

## Testing Strategy

### Unit Tests

```go
// Test each component independently

func TestProfileInspector(t *testing.T) {
    // Create temp config file
    // Test parsing
    // Verify analysis results
}

func TestConfigWriter(t *testing.T) {
    // Create temp config
    // Test writing profiles
    // Test backup creation
    // Verify file contents
}

func TestAutoLogin(t *testing.T) {
    // Mock AWS SDK calls
    // Test validation logic
    // Test YAML updates
}
```

### Integration Tests

```go
// Test full wizard flow

func TestWizardFlow_NewSetup(t *testing.T) {
    // Start with no config
    // Run wizard
    // Verify config created
    // Verify YAML updated
}

func TestWizardFlow_UpdateExisting(t *testing.T) {
    // Start with partial config
    // Run wizard
    // Verify config updated
    // Verify backup created
}
```

### Manual Testing Checklist

- [ ] Fresh install (no ~/.aws directory)
- [ ] Existing SSO session, no profiles
- [ ] Existing profiles, incomplete fields
- [ ] Multiple environments
- [ ] SSO login failure recovery
- [ ] Account ID validation
- [ ] Region mismatch handling
- [ ] Backup creation
- [ ] YAML file updates

---

## Common Patterns

### Error Handling

```go
// Always provide recovery options

if err := someOperation(); err != nil {
    fmt.Printf("❌ Operation failed: %v\n", err)

    var retry bool
    huh.NewConfirm().
        Title("Would you like to retry?").
        Value(&retry).
        Run()

    if retry {
        return someOperation() // Retry
    }

    return err // Give up
}
```

### Progress Indicators

```go
// Show progress during long operations

fmt.Println("🔄 Writing configuration...")
fmt.Println("  ✓ Backup created")
fmt.Println("  ✓ SSO session written")
fmt.Println("  ✓ Profile written")
fmt.Println("  ✓ Config validated")
fmt.Println("✅ Configuration complete")
```

### Pre-filling Forms

```go
// Always pre-fill when possible

func promptWithDefaults(env Env) {
    // Use YAML values as defaults
    accountID := env.AccountID    // Pre-fill if available
    region := env.Region           // Pre-fill from YAML
    roleName := "AdministratorAccess" // Sensible default

    // User can just press Enter to accept defaults
}
```

---

## Debugging Tips

### Logging

```go
// Add verbose logging option

var verbose bool // Set from CLI flag

func debugLog(format string, args ...interface{}) {
    if verbose {
        fmt.Printf("[DEBUG] "+format+"\n", args...)
    }
}

// Use throughout code
debugLog("Inspecting profile: %s", profileName)
debugLog("Found SSO sessions: %v", sessions)
```

### Config File Inspection

```go
// Helper to dump config for debugging

func dumpConfig() {
    homeDir, _ := os.UserHomeDir()
    configPath := filepath.Join(homeDir, ".aws", "config")

    content, err := os.ReadFile(configPath)
    if err != nil {
        fmt.Printf("Error reading config: %v\n", err)
        return
    }

    fmt.Println("=== AWS Config ===")
    fmt.Println(string(content))
    fmt.Println("==================")
}
```

---

## Deployment Checklist

Before releasing the wizard:

- [ ] All unit tests passing
- [ ] Integration tests passing
- [ ] Manual testing complete
- [ ] Documentation updated
- [ ] Error messages clear and helpful
- [ ] Recovery paths tested
- [ ] Backup system working
- [ ] Multi-environment flow tested
- [ ] Performance acceptable (< 5s for wizard)
- [ ] CLI help text added
- [ ] Examples documented

---

## Future Enhancements

After initial release, consider:

1. **Profile Templates** - Save common configurations
2. **Organization Detection** - Auto-detect SSO URL from email
3. **Batch Operations** - Set up all environments at once
4. **Profile Validation** - Check permissions before deployment
5. **Cloud Formation** - Generate delegation roles automatically
6. **Team Sharing** - Export/import configs (sanitized)

---

## Summary

The wizard follows these principles:

1. **Detect First** - Analyze before asking
2. **Smart Defaults** - Pre-fill everything possible
3. **Safe Operations** - Backup before modifying
4. **Clear Feedback** - Show progress and results
5. **Easy Recovery** - Handle errors gracefully

This creates a **frictionless setup experience** that takes users from zero to ready in minutes.
