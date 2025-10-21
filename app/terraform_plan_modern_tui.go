package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const AppVersion = "v3.5.6" // Should match version.txt

type viewMode int

const (
	dashboardView viewMode = iota
	applyView
	fullScreenDiffView
	aiHelpView
)

// AI visualization messages
type aiSuccessMsg struct{}
type aiErrorMsg struct {
	err error
}

// AI help messages
type aiHelpReadyMsg struct {
	problem  string
	commands []string
	errors   []string
}
type aiHelpErrorMsg struct {
	err error
}

// Type definitions from the original implementation
type PlannedResource struct {
	Address      string                 `json:"address"`
	Mode         string                 `json:"mode"`
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	ProviderName string                 `json:"provider_name"`
	Values       map[string]interface{} `json:"values"`
}

type ResourceChange struct {
	Address      string `json:"address"`
	Mode         string `json:"mode"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	ProviderName string `json:"provider_config_key"`
	Change       struct {
		Actions []string               `json:"actions"`
		Before  map[string]interface{} `json:"before"`
		After   map[string]interface{} `json:"after"`
	} `json:"change"`
}

type TerraformPlanVisual struct {
	FormatVersion    string                 `json:"format_version"`
	TerraformVersion string                 `json:"terraform_version"`
	Variables        map[string]interface{} `json:"variables"`
	PlannedValues    struct {
		RootModule struct {
			Resources []PlannedResource `json:"resources"`
		} `json:"root_module"`
	} `json:"planned_values"`
	ResourceChanges []ResourceChange `json:"resource_changes"`
	OutputChanges   map[string]struct {
		Actions []string `json:"actions"`
	} `json:"output_changes"`
}

type changeGroups struct {
	creates  []ResourceChange
	updates  []ResourceChange
	deletes  []ResourceChange
	replaces []ResourceChange
	reads    []ResourceChange
}

type changeStats struct {
	totalChanges int
	byType       map[string]int
	byAction     map[string]int
}

type serviceGroup struct {
	name      string
	icon      string
	resources []ResourceChange
	expanded  bool
}

type providerGroup struct {
	name      string
	icon      string
	services  []serviceGroup
	expanded  bool
}

type modernPlanModel struct {
	plan           TerraformPlanVisual
	providers      []providerGroup
	selectedProvider int
	selectedService  int
	selectedResource int
	stats          changeStats
	currentView    viewMode
	detailViewport viewport.Model
	treeViewport   viewport.Model
	logViewport    viewport.Model
	diffViewport   viewport.Model  // For full-screen diff view
	width          int
	height         int
	help           help.Model
	keys           modernKeyMap
	showHelp       bool
	progress       progress.Model
	applyProgress  float64
	applyState     *applyState
	program        *tea.Program
	// Resource replacement tracking
	markedForReplace map[string]bool
	showReplaceMode  bool
	// Full-screen diff state
	diffResource   *ResourceChange  // Resource being viewed in full-screen
	// AI error display
	aiError        string
	showAIError    bool
	aiLoading      bool
	// AI help view
	aiHelpProblem   string
	aiHelpCommands  []string
	aiHelpErrors    []string
	aiHelpViewport  viewport.Model
	aiHelpLoading   bool
}

type modernKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Enter    key.Binding
	Back     key.Binding
	Space    key.Binding
	Tab      key.Binding
	Apply    key.Binding
	AskAI    key.Binding
	Copy     key.Binding
	Quit     key.Binding
	Help     key.Binding
	Replace  key.Binding
	Import   key.Binding
}

func (k modernKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Enter, k.Copy, k.Apply, k.Quit}
}

func (k modernKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Tab, k.Enter, k.Space, k.Replace},
		{k.Copy, k.Apply, k.AskAI},
		{k.Help, k.Quit},
	}
}

var modernKeys = modernKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "expand"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("Esc", "back"),
	),
	Space: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("Space", "expand/collapse"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("Tab", "navigate"),
	),
	Apply: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "apply"),
	),
	AskAI: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "ask AI to explain"),
	),
	Copy: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "copy changes"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Replace: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "mark for replace"),
	),
	Import: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "import existing"),
	),
}

// Modern color palette
var (
	bgColor        = lipgloss.Color("#0a0a0a")
	fgColor        = lipgloss.Color("#ffffff")
	borderColor    = lipgloss.Color("#333333")
	primaryColor   = lipgloss.Color("#7c3aed")
	successColor   = lipgloss.Color("#10b981")
	warningColor   = lipgloss.Color("#f59e0b")
	dangerColor    = lipgloss.Color("#ef4444")
	mutedColor     = lipgloss.Color("#6b7280")
	accentColor    = lipgloss.Color("#3b82f6")
	dimColor       = lipgloss.Color("#9ca3af")
	
	// Styles
	baseStyle = lipgloss.NewStyle().
		Background(bgColor).
		Foreground(fgColor)
		
	headerStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1a1a")).
		Foreground(fgColor).
		Bold(true).
		Padding(0, 2)
		
	titleStyle = lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true)
		
	boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
		
	selectedBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(0, 1)
		
	treeItemStyle = lipgloss.NewStyle().
		PaddingLeft(2)
		
	selectedItemStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#374151")).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)
		
	createIconStyle = lipgloss.NewStyle().Foreground(successColor)
	updateIconStyle = lipgloss.NewStyle().Foreground(warningColor)
	deleteIconStyle = lipgloss.NewStyle().Foreground(dangerColor)
	
	labelStyle = lipgloss.NewStyle().
		Foreground(mutedColor)
		
	valueStyle = lipgloss.NewStyle().
		Foreground(fgColor)
		
	typeStyle = lipgloss.NewStyle().
		Foreground(accentColor).
		Italic(true)
		
	dimStyle = lipgloss.NewStyle().
		Foreground(dimColor)
)

// Provider icons
var providerIcons = map[string]string{
	"aws":        "󰡨",
	"azurerm":    "󰡨",
	"google":     "󱇶",
	"kubernetes": "󱃾",
	"helm":       "⎈",
	"docker":     "󰡨",
	"local":      "󰋊",
	"null":       "∅",
	"random":     "󰒲",
	"template":   "󰈙",
	"terraform":  "󱁢",
}

func getProviderIcon(provider string) string {
	if icon, ok := providerIcons[provider]; ok {
		return icon
	}
	return "󰒓"
}

func getActionIcon(action string) string {
	switch action {
	case "create":
		return "✚"
	case "update":
		return "~"
	case "delete":
		return "✗"
	case "replace":
		return "↻"
	default:
		return "─"
	}
}

func getActionSymbol(action string) string {
	switch action {
	case "create":
		return "+"
	case "update":
		return "~"
	case "delete":
		return "-"
	case "replace":
		return "±"
	default:
		return " "
	}
}

// Resource type icons
var resourceIcons = map[string]string{
	"aws_instance":                     "🖥️ ",
	"aws_ecs_service":                  "🐳",
	"aws_ecs_task_definition":          "📋",
	"aws_ecs_cluster":                  "🎯",
	"aws_db_instance":                  "🗄️ ",
	"aws_rds_cluster":                  "🗃️ ",
	"aws_s3_bucket":                    "🪣",
	"aws_lambda_function":              "⚡",
	"aws_api_gateway_rest_api":         "🌐",
	"aws_cloudfront_distribution":      "☁️ ",
	"aws_route53_record":               "🔤",
	"aws_route53_zone":                 "🌍",
	"aws_security_group":               "🔒",
	"aws_security_group_rule":          "🔐",
	"aws_iam_role":                     "👤",
	"aws_iam_policy":                   "📜",
	"aws_iam_role_policy_attachment":   "🔗",
	"aws_vpc":                          "🏗️ ",
	"aws_subnet":                       "🕸️ ",
	"aws_internet_gateway":             "🚪",
	"aws_nat_gateway":                  "🔀",
	"aws_elasticache_cluster":          "💾",
	"aws_alb":                          "⚖️ ",
	"aws_lb":                           "⚖️ ",
	"aws_lb_target_group":              "🎯",
	"aws_autoscaling_group":            "📊",
	"aws_cloudwatch_log_group":         "📝",
	"aws_sqs_queue":                    "📬",
	"aws_sns_topic":                    "📢",
	"aws_dynamodb_table":               "🗂️ ",
	"aws_ecr_repository":               "📦",
	"aws_eks_cluster":                  "☸️ ",
	"aws_cognito_user_pool":            "👥",
	"aws_secretsmanager_secret":        "🔑",
	"aws_ssm_parameter":                "⚙️ ",
	"aws_eventbridge_rule":             "📅",
	"aws_service_discovery_service":    "🔍",
	"aws_appsync_graphql_api":          "🕸️ ",
	"aws_ses_domain_identity":          "✉️ ",
	"aws_acm_certificate":              "🎖️ ",
	"aws_wafv2_web_acl":                "🛡️ ",
	"module":                           "📦",
	"null_resource":                    "⚪",
	"random_password":                  "🎲",
	"time_sleep":                       "⏰",
}

func getResourceIcon(resourceType string) string {
	if icon, ok := resourceIcons[resourceType]; ok {
		return icon
	}
	// Default icon for unknown resource types
	if strings.HasPrefix(resourceType, "aws_") {
		return "☁️ "
	}
	return "📄"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getServiceFromResourceType(resourceType string) string {
	// Remove provider prefix (e.g., aws_)
	parts := strings.Split(resourceType, "_")
	if len(parts) < 2 {
		return "general"
	}
	
	// Map common AWS services
	serviceMap := map[string]string{
		"instance":             "EC2",
		"security_group":       "EC2",
		"security_group_rule":  "EC2",
		"eip":                  "EC2",
		"ami":                  "EC2",
		"key_pair":             "EC2",
		"launch_template":      "EC2",
		"s3":                   "S3",
		"bucket":               "S3",
		"db":                   "RDS",
		"rds":                  "RDS",
		"vpc":                  "VPC",
		"subnet":               "VPC",
		"internet_gateway":     "VPC",
		"nat_gateway":          "VPC",
		"route_table":          "VPC",
		"route":                "VPC",
		"vpc_endpoint":         "VPC",
		"lambda":               "Lambda",
		"iam":                  "IAM",
		"role":                 "IAM",
		"policy":               "IAM",
		"ecs":                  "ECS",
		"eks":                  "EKS",
		"ecr":                  "ECR",
		"alb":                  "ELB",
		"lb":                   "ELB",
		"elb":                  "ELB",
		"target_group":         "ELB",
		"cloudfront":           "CloudFront",
		"route53":              "Route53",
		"cloudwatch":           "CloudWatch",
		"sns":                  "SNS",
		"sqs":                  "SQS",
		"dynamodb":             "DynamoDB",
		"elasticache":          "ElastiCache",
		"cognito":              "Cognito",
		"api_gateway":          "API Gateway",
		"appsync":              "AppSync",
		"secretsmanager":       "Secrets Manager",
		"ssm":                  "Systems Manager",
		"eventbridge":          "EventBridge",
		"ses":                  "SES",
		"acm":                  "ACM",
		"waf":                  "WAF",
		"autoscaling":          "Auto Scaling",
		"service_discovery":    "Service Discovery",
	}
	
	// Check each part of the resource type
	for _, part := range parts[1:] {
		if service, ok := serviceMap[part]; ok {
			return service
		}
	}
	
	// Check for compound names
	typeWithoutProvider := strings.Join(parts[1:], "_")
	for key, service := range serviceMap {
		if strings.Contains(typeWithoutProvider, key) {
			return service
		}
	}
	
	// Default to first meaningful part
	if len(parts) > 1 && parts[1] != "" {
		return strings.Title(parts[1])
	}
	
	return "Other"
}

func getServiceIcon(service string) string {
	serviceIcons := map[string]string{
		"EC2":               "🖥️",
		"S3":                "🪣",
		"RDS":               "🗄️",
		"VPC":               "🌐",
		"Lambda":            "⚡",
		"IAM":               "🔐",
		"ECS":               "🐳",
		"EKS":               "☸️",
		"ECR":               "📦",
		"ELB":               "⚖️",
		"CloudFront":        "☁️",
		"Route53":           "🌍",
		"CloudWatch":        "📊",
		"SNS":               "📢",
		"SQS":               "📬",
		"DynamoDB":          "🗂️",
		"ElastiCache":       "💾",
		"Cognito":           "👥",
		"API Gateway":       "🚪",
		"AppSync":           "🕸️",
		"Secrets Manager":   "🔑",
		"Systems Manager":   "⚙️",
		"EventBridge":       "📅",
		"SES":               "✉️",
		"ACM":               "🎖️",
		"WAF":               "🛡️",
		"Auto Scaling":      "📈",
		"Service Discovery": "🔍",
	}
	
	if icon, ok := serviceIcons[service]; ok {
		return icon
	}
	return "📁"
}

func initModernTerraformPlanTUI(planJSON string) (tea.Model, error) {
	var plan TerraformPlanVisual
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse terraform plan JSON: %w", err)
	}

	groups := groupResourceChanges(plan.ResourceChanges)
	stats := calculateStatistics(groups)
	
	// Group resources by provider and service
	providerMap := make(map[string]map[string][]ResourceChange)
	for _, change := range plan.ResourceChanges {
		// Skip no-op changes
		if len(change.Change.Actions) == 0 || 
		   (len(change.Change.Actions) == 1 && (change.Change.Actions[0] == "no-op" || change.Change.Actions[0] == "read")) {
			continue
		}
		
		parts := strings.Split(change.ProviderName, ".")
		provider := parts[len(parts)-1]
		if strings.Contains(provider, "/") {
			provider = strings.Split(provider, "/")[1]
		}
		
		// Extract service from resource type
		service := getServiceFromResourceType(change.Type)
		
		if providerMap[provider] == nil {
			providerMap[provider] = make(map[string][]ResourceChange)
		}
		
		// Handle replace operations specially - split into delete and create
		if len(change.Change.Actions) == 2 && 
		   change.Change.Actions[0] == "delete" && 
		   change.Change.Actions[1] == "create" {
			// Create a delete entry
			deleteChange := change
			deleteChange.Change.Actions = []string{"delete"}
			deleteChange.Change.After = nil  // Delete only has before state
			
			// Create a create entry
			createChange := change
			createChange.Change.Actions = []string{"create"}
			createChange.Change.Before = nil  // Create only has after state
			
			// Append both with modified addresses to distinguish them
			deleteChange.Address = change.Address + " (destroy)"
			createChange.Address = change.Address + " (create)"
			
			providerMap[provider][service] = append(providerMap[provider][service], deleteChange)
			providerMap[provider][service] = append(providerMap[provider][service], createChange)
		} else {
			// Regular change - just append as-is
			providerMap[provider][service] = append(providerMap[provider][service], change)
		}
	}
	
	// Create provider groups with service subgroups
	var providers []providerGroup
	for providerName, serviceMap := range providerMap {
		var services []serviceGroup
		
		// Create service groups
		for serviceName, resources := range serviceMap {
			// Sort resources within service
			sort.Slice(resources, func(i, j int) bool {
				return resources[i].Address < resources[j].Address
			})
			
			services = append(services, serviceGroup{
				name:      serviceName,
				icon:      getServiceIcon(serviceName),
				resources: resources,
				expanded:  false,
			})
		}
		
		// Sort services by name
		sort.Slice(services, func(i, j int) bool {
			return services[i].name < services[j].name
		})
		
		providers = append(providers, providerGroup{
			name:     providerName,
			icon:     getProviderIcon(providerName),
			services: services,
			expanded: false,
		})
	}
	
	// Sort providers by name
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].name < providers[j].name
	})
	
	// Expand first provider and its first service
	if len(providers) > 0 {
		providers[0].expanded = true
		if len(providers[0].services) > 0 {
			providers[0].services[0].expanded = true
		}
	}
	
	// Create viewports
	detailVp := viewport.New(0, 0)
	treeVp := viewport.New(0, 0)
	logVp := viewport.New(0, 0)
	diffVp := viewport.New(0, 0)
	
	// Create progress bar
	prog := progress.New(progress.WithDefaultGradient())
	
	return &modernPlanModel{
		plan:           plan,
		providers:      providers,
		stats:          stats,
		currentView:    dashboardView,
		detailViewport: detailVp,
		treeViewport:   treeVp,
		logViewport:    logVp,
		diffViewport:   diffVp,
		help:           help.New(),
		keys:           modernKeys,
		progress:       prog,
		markedForReplace: make(map[string]bool),
		showReplaceMode:  false,
		diffResource:   nil,
	}, nil
}

func (m *modernPlanModel) Init() tea.Cmd {
	m.updateTreeViewport()
	m.updateDetailViewport()
	return nil
}

func (m *modernPlanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case aiSuccessMsg:
		m.aiLoading = false
		m.showAIError = false
		// Clear screen and force redraw
		return m, tea.Batch(tea.ClearScreen, func() tea.Msg {
			return tea.WindowSizeMsg{Width: m.width, Height: m.height}
		})

	case aiErrorMsg:
		m.aiLoading = false
		m.aiError = msg.err.Error()
		m.showAIError = true
		return m, nil

	case aiHelpReadyMsg:
		// AI help fetch completed successfully
		m.aiHelpLoading = false
		m.aiHelpProblem = msg.problem
		m.aiHelpCommands = msg.commands
		m.aiHelpErrors = msg.errors
		// Update viewport with content
		m.aiHelpViewport.SetContent(m.buildAIHelpContent())
		return m, nil

	case aiHelpErrorMsg:
		// AI help fetch failed
		m.aiHelpLoading = false
		m.aiError = msg.err.Error()
		m.showAIError = true
		m.currentView = applyView // Go back to apply view
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		// Update viewport sizes based on view
		switch m.currentView {
		case dashboardView:
			// Split pane layout
			treeWidth := m.width / 2 - 2
			detailWidth := m.width / 2 - 2
			// Calculate height accounting for:
			// Header: 1 line
			// Plan summary: 3 lines (with padding)
			// Split pane borders: 2 lines (top and bottom)
			// Change summary: 3 lines
			// Footer: 1 line
			// Total overhead: ~10 lines
			contentHeight := m.height - 10
			
			m.treeViewport.Width = treeWidth
			m.treeViewport.Height = contentHeight
			
			m.detailViewport.Width = detailWidth
			m.detailViewport.Height = contentHeight
			
		case fullScreenDiffView:
			// Full-screen diff view
			m.diffViewport.Width = m.width - 4
			m.diffViewport.Height = m.height - 6 // Leave room for header and footer
			m.updateDiffViewport()
			
		case applyView:
			// Recalculate fixed layout on resize
			m.calculateApplyLayout(m.height)
			// Set viewport width and height based on calculated layout
			m.logViewport.Width = m.width - 4
			if m.applyState != nil {
				// logsHeight is screen height, viewport needs content height minus title line
				// Screen height - 2 (borders) - 1 (title) = content height for viewport
				viewportHeight := m.applyState.logsHeight - 3
				if viewportHeight < 2 {
					viewportHeight = 2
				}
				m.logViewport.Height = viewportHeight
				m.updateApplyLogViewport()
			}
		}
		
		m.updateTreeViewport()
		m.updateDetailViewport()
		
	case tea.KeyMsg:
		// If showing AI error, any key dismisses it
		if m.showAIError {
			m.showAIError = false
			m.aiError = ""
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			// Escape from full-screen diff goes back to dashboard
			if m.currentView == fullScreenDiffView {
				m.currentView = dashboardView
				return m, nil
			}
			// Escape from AI help view goes back to apply view
			if m.currentView == aiHelpView {
				m.currentView = applyView
				return m, nil
			}
			return m, tea.Quit
			
		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			
		case key.Matches(msg, m.keys.Back):
			if m.currentView != dashboardView {
				m.currentView = dashboardView
			}
			
		case key.Matches(msg, m.keys.AskAI):
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				m.aiLoading = true
				m.showAIError = false
				m.aiError = ""
				return m, m.askAIToExplainCmd()
			} else {
				m.aiError = "ANTHROPIC_API_KEY environment variable not set. Set it with: export ANTHROPIC_API_KEY=your_key_here"
				m.showAIError = true
				return m, nil
			}
			
		case key.Matches(msg, m.keys.Copy):
			if m.currentView == dashboardView {
				m.copyChangesToClipboard()
			}
			
		case key.Matches(msg, m.keys.Apply):
			// Don't apply again if we're already in apply view with completed deployment
			if m.currentView == applyView && m.applyState != nil && m.applyState.applyComplete {
				// If we have errors and AI is available, show AI help view
				if m.applyState.hasErrors && isAIHelperAvailable() {
					// Immediately switch to AI help view with loading state
					m.currentView = aiHelpView
					m.aiHelpLoading = true
					m.aiHelpViewport = viewport.New(m.width-4, m.height-6)
					// Launch async fetch and start animation
					return m, tea.Batch(m.fetchAIHelp(), m.tickCmd())
				}
				// Otherwise, just ignore the key press
				return m, nil
			}

			// No need to check for replacements - the startTerraformApply function
			// already handles -replace flags properly
			m.currentView = applyView
			m.initApplyState()
			// Set viewport dimensions
			m.logViewport.Width = m.width - 4
			m.logViewport.Height = m.height / 3
			m.updateApplyLogViewport() // Show initial logs
			return m, m.startTerraformApply()
			
		case key.Matches(msg, m.keys.Tab):
			// Tab navigation for apply view
			if m.currentView == applyView && m.applyState != nil {
				m.applyState.selectedSection = (m.applyState.selectedSection + 1) % 3
			}
			
		case key.Matches(msg, m.keys.Space):
			if m.currentView == dashboardView && len(m.providers) > 0 {
				// Space toggles expand/collapse
				if m.selectedService == -1 {
					// Toggle provider
					m.providers[m.selectedProvider].expanded = !m.providers[m.selectedProvider].expanded
					if !m.providers[m.selectedProvider].expanded {
						m.selectedService = -1
						m.selectedResource = -1
					}
				} else if m.selectedResource == -1 {
					// Toggle service
					provider := &m.providers[m.selectedProvider]
					if provider.expanded && m.selectedService < len(provider.services) {
						provider.services[m.selectedService].expanded = !provider.services[m.selectedService].expanded
						if !provider.services[m.selectedService].expanded {
							m.selectedResource = -1
						}
					}
				}
				m.updateTreeViewport()
			}
			
		case key.Matches(msg, m.keys.Enter):
			if m.currentView == dashboardView && len(m.providers) > 0 {
				// Enter opens full-screen diff for resources
				if m.selectedResource >= 0 {
					resource := m.getSelectedResource()
					if resource != nil {
						m.diffResource = resource
						m.currentView = fullScreenDiffView
						m.updateDiffViewport()
					}
				} else if m.selectedService == -1 {
					// Toggle provider if no resource selected
					m.providers[m.selectedProvider].expanded = !m.providers[m.selectedProvider].expanded
					if !m.providers[m.selectedProvider].expanded {
						m.selectedService = -1
						m.selectedResource = -1
					}
					m.updateTreeViewport()
				} else if m.selectedResource == -1 {
					// Toggle service if no resource selected
					provider := &m.providers[m.selectedProvider]
					if provider.expanded && m.selectedService < len(provider.services) {
						provider.services[m.selectedService].expanded = !provider.services[m.selectedService].expanded
						if !provider.services[m.selectedService].expanded {
							m.selectedResource = -1
						}
					}
					m.updateTreeViewport()
				}
			} else if m.currentView == fullScreenDiffView {
				// Enter or Escape in full-screen diff goes back to dashboard
				m.currentView = dashboardView
			}
			
		case key.Matches(msg, m.keys.Down):
			if m.currentView == dashboardView {
				m.navigateDown()
				m.updateTreeViewport()
				m.updateDetailViewport()
			} else if m.currentView == fullScreenDiffView {
				// Scroll down in full-screen diff view
				m.diffViewport.LineDown(1)
			} else if m.currentView == aiHelpView {
				// Scroll down in AI help view
				m.aiHelpViewport.LineDown(1)
			} else if m.currentView == applyView && m.applyState != nil {
				// Only scroll logs if logs section is selected
				if m.applyState.selectedSection == 2 {
					m.logViewport.LineDown(1)
				}
			}

		case key.Matches(msg, m.keys.Up):
			if m.currentView == dashboardView {
				m.navigateUp()
				m.updateTreeViewport()
				m.updateDetailViewport()
			} else if m.currentView == fullScreenDiffView {
				// Scroll up in full-screen diff view
				m.diffViewport.LineUp(1)
			} else if m.currentView == aiHelpView {
				// Scroll up in AI help view
				m.aiHelpViewport.LineUp(1)
			} else if m.currentView == applyView && m.applyState != nil {
				// Only scroll logs if logs section is selected
				if m.applyState.selectedSection == 2 {
					m.logViewport.LineUp(1)
				}
			}
			
		case key.Matches(msg, m.keys.Replace):
			// Toggle replace mode or mark resource for replacement
			if m.currentView == dashboardView {
				resource := m.getSelectedResource()
				if resource != nil {
					// Toggle the replacement mark for this resource
					if m.markedForReplace[resource.Address] {
						delete(m.markedForReplace, resource.Address)
					} else {
						m.markedForReplace[resource.Address] = true
					}
					m.updateTreeViewport()
					m.updateDetailViewport()
				} else {
					// Toggle replace mode if no specific resource selected
					m.showReplaceMode = !m.showReplaceMode
				}
			}
			
		case key.Matches(msg, m.keys.Import):
			// Show import help for the selected resource
			if m.currentView == dashboardView {
				resource := m.getSelectedResource()
				if resource != nil {
					m.showImportHelp(resource)
					// Force a full redraw when returning
					return m, tea.Batch(
						tea.ClearScreen,
						func() tea.Msg {
							return tea.WindowSizeMsg{
								Width:  m.width,
								Height: m.height,
							}
						},
					)
				}
			}
			
		case msg.String() == "s":
			// Stop apply
			if m.currentView == applyView && m.applyState != nil && m.applyState.isApplying {
				if m.applyState.cmd != nil {
					m.applyState.cmd.Process.Kill()
				}
				m.applyState.isApplying = false
				m.applyState.hasErrors = true
			}
			
		case msg.String() == "l":
			// Toggle log view size
			if m.currentView == applyView && m.applyState != nil {
				m.applyState.showFullLogs = !m.applyState.showFullLogs
				// Update viewport size
				if m.applyState.showFullLogs {
					m.logViewport.Height = m.height * 2 / 3
				} else {
					m.logViewport.Height = m.height / 3 // Consistent with window resize
				}
				m.updateApplyLogViewport()
			}

		// Removed: details view (key 'd') and error details view (key 'x')
		// All information is shown in logs with proper wrapping

		case tea.KeyMsg(msg).String() == "pgup":
			// Page up in full-screen diff view
			if m.currentView == fullScreenDiffView {
				m.diffViewport.HalfViewUp()
			}
			
		case tea.KeyMsg(msg).String() == "pgdown":
			// Page down in full-screen diff view
			if m.currentView == fullScreenDiffView {
				m.diffViewport.HalfViewDown()
			}
		}
		
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
		
	// Apply-specific messages
	case applyStartMsg:
		if m.applyState != nil {
			m.applyState.isApplying = true
			m.updateApplyLogViewport() // Update viewport to show initial logs
		}
		// Start animation ticker
		return m, m.tickCmd()
		
	case applyCompleteMsg:
		if m.applyState != nil {
			m.applyState.isApplying = false
			m.applyState.applyComplete = true
		}
		
	case applyErrorMsg:
		if m.applyState != nil {
			m.applyState.isApplying = false
			m.applyState.applyComplete = true
			m.applyState.hasErrors = true

			// Cancel all pending operations when deployment fails
			for _, pending := range m.applyState.pending {
				m.applyState.completed = append(m.applyState.completed, completedResource{
					Address:   pending.Address,
					Action:    "cancelled",
					Duration:  0,
					Timestamp: time.Now(),
					Success:   false,
					Error:     "Deployment cancelled due to previous errors",
				})
			}
			// Clear pending list
			m.applyState.pending = []pendingResource{}
		}

	case resourceStartMsg:
		if m.applyState != nil {
			m.applyState.currentOp = &currentOperation{
				Address:   msg.Address,
				Action:    msg.Action,
				StartTime: time.Now(),
				Status:    "Starting...",
			}
			// Update log viewport
			m.updateApplyLogViewport()
		}
		
	case resourceProgressMsg:
		if m.applyState != nil && m.applyState.currentOp != nil {
			m.applyState.currentOp.Progress = msg.Progress
			m.applyState.currentOp.Status = msg.Status
		}
		
	case resourceCompleteMsg:
		if m.applyState != nil {
			// Get action from pending list before removing
			action := "update"
			for _, p := range m.applyState.pending {
				if p.Address == msg.Address {
					action = p.Action
					break
				}
			}
			
			// Move from pending to completed
			for i, p := range m.applyState.pending {
				if p.Address == msg.Address {
					m.applyState.pending = append(m.applyState.pending[:i], m.applyState.pending[i+1:]...)
					break
				}
			}
			
			// Get duration safely
			var duration time.Duration
			if m.applyState.currentOp != nil && m.applyState.currentOp.Address == msg.Address {
				action = m.applyState.currentOp.Action
				duration = time.Since(m.applyState.currentOp.StartTime)
			} else if msg.Duration > 0 {
				duration = msg.Duration
			}
			
			m.applyState.completed = append(m.applyState.completed, completedResource{
				Address:      msg.Address,
				Action:       action,
				Duration:     duration,
				Timestamp:    time.Now(),
				Success:      msg.Success,
				Error:        msg.Error,
				ErrorSummary: msg.ErrorSummary,
				ErrorDetail:  msg.ErrorDetail,
			})

			if !msg.Success {
				// Only count as error if it's not a cascading cancellation
				isCascadingFailure := action == "cancelled" ||
					strings.Contains(strings.ToLower(msg.Error), "cancelled due to previous errors") ||
					strings.Contains(strings.ToLower(msg.Error), "deployment cancelled")

				if !isCascadingFailure {
					m.applyState.errorCount++
					m.applyState.hasErrors = true
				}
			}
			
			m.applyState.currentOp = nil
			m.updateApplyLogViewport()
		}
		
	case logMsg:
		if m.applyState != nil {
			m.applyState.logs = append(m.applyState.logs, logEntry{
				Timestamp: time.Now(),
				Level:     msg.Level,
				Message:   msg.Message,
				Resource:  msg.Resource,
			})
			
			if msg.Level == "warning" {
				m.applyState.warningCount++
			}
			// Note: error count is incremented in resourceCompleteMsg handler to avoid duplicates

			m.updateApplyLogViewport()
		}
		
	case applyTickMsg:
		// Update animation frame and continue ticking if still applying
		if m.applyState != nil && m.applyState.isApplying {
			m.applyState.animationFrame++
			return m, m.tickCmd()
		}
		// Also tick if we're loading AI help (for spinner animation)
		if m.currentView == aiHelpView && m.aiHelpLoading {
			return m, m.tickCmd()
		}
	}
	
	// Update viewports
	var cmds []tea.Cmd
	switch m.currentView {
	case dashboardView:
		var cmd tea.Cmd
		m.treeViewport, cmd = m.treeViewport.Update(msg)
		cmds = append(cmds, cmd)
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		cmds = append(cmds, cmd)
		
		
	case applyView:
		var cmd tea.Cmd
		m.logViewport, cmd = m.logViewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	
	return m, tea.Batch(cmds...)
}

func (m *modernPlanModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Show AI loading indicator
	if m.aiLoading {
		return m.renderAILoading()
	}

	// Show AI error overlay
	if m.showAIError {
		return m.renderAIError()
	}

	switch m.currentView {
	case dashboardView:
		return m.renderDashboard()
	case applyView:
		return m.renderApplyView()
	case fullScreenDiffView:
		return m.renderFullScreenDiff()
	case aiHelpView:
		return m.renderAIHelpView()
	default:
		return m.renderDashboard()
	}
}


func (m *modernPlanModel) renderDashboard() string {
	// Header
	header := m.renderHeader()
	
	// Plan summary
	summary := m.renderPlanSummary()
	
	// Main content area with split panes
	content := m.renderSplitPanes()
	
	// Change summary
	changeSummary := m.renderChangeSummary()
	
	// Footer
	footer := m.renderFooter()
	
	// Combine all parts
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		summary,
		content,
		changeSummary,
		footer,
	)
}

func (m *modernPlanModel) renderHeader() string {
	left := titleStyle.Render(fmt.Sprintf("🚀 Meroku %s", AppVersion))
	env := "Development"
	if m.stats.totalChanges > 50 {
		env = "Production"
	}
	right := fmt.Sprintf("📋 %s │ ⚡ Terraform %s", env, m.plan.TerraformVersion)
	
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 0 {
		gap = 0
	}
	
	return headerStyle.Width(m.width).Render(
		left + strings.Repeat(" ", gap) + right,
	)
}

func (m *modernPlanModel) renderPlanSummary() string {
	summary := fmt.Sprintf("Plan: %d to add, %d to change, %d to destroy",
		m.stats.byAction["create"],
		m.stats.byAction["update"],
		m.stats.byAction["delete"],
	)
	
	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(summary)
}

func (m *modernPlanModel) renderSplitPanes() string {
	treeWidth := m.width / 2 - 2
	detailWidth := m.width / 2 - 2
	
	// Resources tree
	treeTitle := "┌─ Resources " + strings.Repeat("─", treeWidth-14) + "┐"
	treeContent := m.treeViewport.View()
	treeBottom := "└" + strings.Repeat("─", treeWidth-2) + "┘"
	
	tree := lipgloss.JoinVertical(
		lipgloss.Left,
		treeTitle,
		treeContent,
		treeBottom,
	)
	
	// Details pane
	detailTitle := "┌─ Details " + strings.Repeat("─", detailWidth-12) + "┐"
	detailContent := m.detailViewport.View()
	detailBottom := "└" + strings.Repeat("─", detailWidth-2) + "┘"
	
	details := lipgloss.JoinVertical(
		lipgloss.Left,
		detailTitle,
		detailContent,
		detailBottom,
	)
	
	// Join horizontally
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		tree,
		" ",
		details,
	)
}

func (m *modernPlanModel) renderChangeSummary() string {
	width := m.width - 8
	
	// Calculate percentages
	total := float64(m.stats.totalChanges)
	if total == 0 {
		total = 1
	}
	
	createPct := float64(m.stats.byAction["create"]) / total
	updatePct := float64(m.stats.byAction["update"]) / total
	deletePct := float64(m.stats.byAction["delete"]) / total
	
	// Create progress bars
	createBar := m.renderProgressBar("additions", createPct, successColor, m.stats.byAction["create"])
	updateBar := m.renderProgressBar("changes", updatePct, warningColor, m.stats.byAction["update"])
	deleteBar := m.renderProgressBar("deletions", deletePct, dangerColor, m.stats.byAction["delete"])
	
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		createBar,
		updateBar,
		deleteBar,
	)
	
	return boxStyle.Width(width).Render(content)
}

func (m *modernPlanModel) renderProgressBar(label string, percent float64, color lipgloss.Color, count int) string {
	barWidth := 40
	filled := int(percent * float64(barWidth))
	
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(borderColor).Render(strings.Repeat("░", barWidth-filled))
	
	pctStr := fmt.Sprintf("%d %s (%.0f%%)", count, label, percent*100)
	
	return fmt.Sprintf("%s %s", bar, pctStr)
}

func (m *modernPlanModel) renderFooter() string {
	help := "[↑↓] Navigate  [Space/Enter] Expand  [r] Replace  [i] Import  [c] Copy  "
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		help += "[e] Ask AI  "
	}
	help += "[a] Apply  [?] Help  [q] Quit"
	
	// Add indicator for marked resources
	if len(m.markedForReplace) > 0 {
		help += fmt.Sprintf("  |  %d resources marked for replacement", len(m.markedForReplace))
	}
	
	return lipgloss.NewStyle().
		Foreground(mutedColor).
		Padding(0, 1).
		Render(help)
}

func (m *modernPlanModel) updateTreeViewport() {
	content := m.renderTreeContent()
	m.treeViewport.SetContent(content)
}

func (m *modernPlanModel) renderTreeContent() string {
	var b strings.Builder
	
	// Calculate available width for the tree (half screen minus borders and padding)
	treeWidth := m.width / 2 - 4
	
	for i, provider := range m.providers {
		isProviderSelected := i == m.selectedProvider
		
		// Count total resources
		totalResources := 0
		for _, service := range provider.services {
			totalResources += len(service.resources)
		}
		
		// Provider header
		chevron := "▶"
		if provider.expanded {
			chevron = "▼"
		}
		
		providerLine := fmt.Sprintf("%s %s %s (%d resources)",
			chevron,
			provider.icon,
			strings.Title(provider.name),
			totalResources,
		)
		
		if isProviderSelected && m.selectedService == -1 && m.selectedResource == -1 {
			// Pad the line to full width for consistent highlighting
			paddedLine := providerLine + strings.Repeat(" ", max(0, treeWidth-lipgloss.Width(providerLine)))
			b.WriteString(selectedItemStyle.Render(paddedLine))
		} else {
			b.WriteString(providerLine)
		}
		b.WriteString("\n")
		
		// Services
		if provider.expanded {
			for j, service := range provider.services {
				isServiceSelected := isProviderSelected && j == m.selectedService
				
				// Service header
				serviceChevron := "▶"
				if service.expanded {
					serviceChevron = "▼"
				}
				
				serviceLine := fmt.Sprintf("  %s %s %s (%d)",
					serviceChevron,
					service.icon,
					service.name,
					len(service.resources),
				)
				
				if isServiceSelected && m.selectedResource == -1 {
					// Pad the line to full width for consistent highlighting
					paddedLine := serviceLine + strings.Repeat(" ", max(0, treeWidth-lipgloss.Width(serviceLine)))
					b.WriteString(selectedItemStyle.Render(paddedLine))
				} else {
					b.WriteString(serviceLine)
				}
				b.WriteString("\n")
				
				// Resources
				if service.expanded {
					for k, resource := range service.resources {
						isResourceSelected := isServiceSelected && k == m.selectedResource
						
						// Get action icon and style
						action := resource.Change.Actions[0]
						icon := getActionIcon(action)
						
						var iconStyle lipgloss.Style
						switch action {
						case "create":
							iconStyle = createIconStyle
						case "update":
							iconStyle = updateIconStyle
						case "delete":
							iconStyle = deleteIconStyle
						default:
							iconStyle = lipgloss.NewStyle()
						}
						
						// Resource name - handle replacement suffixes specially
						displayName := resource.Address
						isReplacement := false
						replacementType := ""
						
						if strings.HasSuffix(resource.Address, " (destroy)") {
							displayName = strings.TrimSuffix(resource.Address, " (destroy)")
							isReplacement = true
							replacementType = " [DELETE]"
						} else if strings.HasSuffix(resource.Address, " (create)") {
							displayName = strings.TrimSuffix(resource.Address, " (create)")
							isReplacement = true
							replacementType = " [CREATE]"
						}
						
						// Extract just the resource name from the full address
						// Handle array notation like module.domain.aws_route53_record.api_domain["api.dev.sava-p.com"]
						name := displayName
						
						// If it contains array notation, extract the key
						arrayKey := ""
						if idx := strings.Index(displayName, "["); idx != -1 {
							// Extract the array key
							if endIdx := strings.Index(displayName, "]"); endIdx > idx {
								arrayKey = displayName[idx+1:endIdx]
								// Remove quotes if present
								arrayKey = strings.Trim(arrayKey, "\"'")
								// Use the base name without array notation
								displayName = displayName[:idx]
							}
						}
						
						// Now extract the resource name
						parts := strings.Split(displayName, ".")
						if len(parts) > 1 {
							// Take last two parts for better context (e.g., "aws_route53_record.api_domain")
							name = parts[len(parts)-2] + "." + parts[len(parts)-1]
						} else {
							name = parts[len(parts)-1]
						}
						
						// Add array key if present
						if arrayKey != "" {
							// Shorten long array keys for display
							if len(arrayKey) > 30 {
								arrayKey = arrayKey[:27] + "..."
							}
							name = name + "[" + arrayKey + "]"
						}
						
						// Add replacement indicator to the name
						if isReplacement {
							name = name + replacementType
						}
						
						connector := "├"
						if k == len(service.resources)-1 {
							connector = "└"
						}
						
						// Check if resource is marked for replacement (using clean address)
						replaceMarker := ""
						if m.markedForReplace[displayName] {
							replaceMarker = " ↻"
						}
						
						if isResourceSelected {
							// When selected, don't apply icon styles that would override the background
							resourceLine := fmt.Sprintf("    %s %s %s%s",
								connector,
								icon,
								name,
								replaceMarker,
							)
							// Pad the line to full width for consistent highlighting
							paddedLine := resourceLine + strings.Repeat(" ", max(0, treeWidth-lipgloss.Width(resourceLine)))
							b.WriteString(selectedItemStyle.Render(paddedLine))
						} else {
							// When not selected, apply icon styles
							nameWithMarker := name
							if replaceMarker != "" {
								nameWithMarker = name + lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Render(replaceMarker)
							}
							resourceLine := fmt.Sprintf("    %s %s %s",
								connector,
								iconStyle.Render(icon),
								nameWithMarker,
							)
							b.WriteString(resourceLine)
						}
						b.WriteString("\n")
					}
				}
			}
			b.WriteString("\n")
		}
	}
	
	return b.String()
}

func (m *modernPlanModel) updateDetailViewport() {
	resource := m.getSelectedResource()
	if resource == nil {
		m.detailViewport.SetContent("Select a resource to view details")
		return
	}
	
	content := m.renderResourceDetails(resource)
	m.detailViewport.SetContent(content)
}

func (m *modernPlanModel) updateDiffViewport() {
	if m.diffResource == nil {
		m.diffViewport.SetContent("No resource selected")
		return
	}
	
	content := m.renderFullDiffContent(m.diffResource)
	m.diffViewport.SetContent(content)
}

// renderFullScreenDiff renders the full-screen diff view for a selected resource
func (m *modernPlanModel) renderFullScreenDiff() string {
	if m.diffResource == nil {
		return "No resource selected"
	}
	
	// Create header with resource info
	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1a2e")).
		Foreground(lipgloss.Color("#ffffff")).
		Padding(1, 2).
		Width(m.width)
	
	// Clean up address for display
	displayAddress := m.diffResource.Address
	displayAddress = strings.TrimSuffix(displayAddress, " (destroy)")
	displayAddress = strings.TrimSuffix(displayAddress, " (create)")
	
	// Get action color and text
	action := ""
	actionColor := mutedColor
	if len(m.diffResource.Change.Actions) > 0 {
		switch m.diffResource.Change.Actions[0] {
		case "create":
			action = "CREATE"
			actionColor = successColor
		case "update":
			action = "UPDATE"
			actionColor = warningColor
		case "delete":
			action = "DELETE"
			actionColor = dangerColor
		case "read":
			action = "READ"
			actionColor = accentColor
		}
	}
	
	actionBadge := lipgloss.NewStyle().
		Background(actionColor).
		Foreground(lipgloss.Color("#ffffff")).
		Padding(0, 1).
		Bold(true).
		Render(action)
	
	icon := getResourceIcon(m.diffResource.Type)
	header := headerStyle.Render(fmt.Sprintf("%s %s  %s  %s", 
		icon, m.diffResource.Type, actionBadge, displayAddress))
	
	// Help text at bottom
	helpStyle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Padding(0, 2)
	
	helpText := helpStyle.Render("Press Enter or Esc to return • ↑/↓ to scroll • j/k for vim navigation")
	
	// Calculate available height for viewport
	availableHeight := m.height - lipgloss.Height(header) - lipgloss.Height(helpText) - 1
	
	// Update viewport dimensions if needed
	if m.diffViewport.Height != availableHeight {
		m.diffViewport.Height = availableHeight
		m.diffViewport.Width = m.width
	}
	
	// Combine all parts
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.diffViewport.View(),
		helpText,
	)
}

// renderFullDiffContent generates the detailed diff content for a resource
func (m *modernPlanModel) renderFullDiffContent(resource *ResourceChange) string {
	if resource == nil {
		return "No resource selected"
	}
	
	var b strings.Builder
	
	// Section style
	sectionStyle := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true).
		MarginTop(1).
		MarginBottom(1)
	
	// Determine the type of change
	if len(resource.Change.Actions) > 0 {
		switch resource.Change.Actions[0] {
		case "delete":
			// Deletion - show what's being removed
			b.WriteString(sectionStyle.Render("🗑️  Resource Deletion") + "\n\n")
			
			warningStyle := lipgloss.NewStyle().
				Foreground(dangerColor).
				Bold(true)
			b.WriteString(warningStyle.Render("⚠️  This resource will be permanently deleted") + "\n\n")
			
			if resource.Change.Before != nil {
				b.WriteString(sectionStyle.Render("Current Configuration (to be deleted):") + "\n")
				b.WriteString(m.renderExpandedAttributesWithPrefix(resource.Change.Before, lipgloss.NewStyle().Foreground(dangerColor), "- "))
			}
			
		case "create":
			// Creation - show what's being added
			b.WriteString(sectionStyle.Render("✨ New Resource Creation") + "\n\n")
			
			if resource.Change.After != nil {
				b.WriteString(sectionStyle.Render("Configuration:") + "\n")
				b.WriteString(m.renderExpandedAttributes(resource.Change.After, lipgloss.NewStyle().Foreground(successColor), ""))
			}
			
		case "update":
			// Update - show the diff
			b.WriteString(sectionStyle.Render("📝 Resource Update") + "\n\n")
			
			// Show changed attributes
			b.WriteString(sectionStyle.Render("Changes:") + "\n")
			b.WriteString(m.renderExpandedUpdateDiff(resource.Change.Before, resource.Change.After))
			
			// Show unchanged attributes if there are any
			unchanged := m.getUnchangedAttributes(resource.Change.Before, resource.Change.After)
			if len(unchanged) > 0 && len(unchanged) < 20 { // Only show if not too many
				b.WriteString("\n" + sectionStyle.Render("Unchanged Attributes:") + "\n")
				b.WriteString(m.renderExpandedAttributes(unchanged, lipgloss.NewStyle().Foreground(mutedColor).Faint(true), "  "))
			}
			
		case "read":
			// Read operation
			b.WriteString(sectionStyle.Render("🔍 Resource Read") + "\n\n")
			b.WriteString("This resource will be read from the provider to update the state.\n")
		}
	}
	
	// Add metadata section if we have provider info
	if resource.ProviderName != "" || resource.Type != "" || resource.Name != "" {
		b.WriteString("\n" + sectionStyle.Render("Metadata:") + "\n")
		
		metaStyle := lipgloss.NewStyle().Foreground(mutedColor)
		keyStyle := lipgloss.NewStyle().Foreground(mutedColor)
		
		// Use consistent formatting for metadata
		if resource.ProviderName != "" {
			b.WriteString("  ")
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-10s", "Provider:")))
			b.WriteString(" ")
			b.WriteString(metaStyle.Render(resource.ProviderName))
			b.WriteString("\n")
		}
		if resource.Mode != "" {
			b.WriteString("  ")
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-10s", "Mode:")))
			b.WriteString(" ")
			b.WriteString(metaStyle.Render(resource.Mode))
			b.WriteString("\n")
		}
		if resource.Type != "" {
			b.WriteString("  ")
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-10s", "Type:")))
			b.WriteString(" ")
			b.WriteString(metaStyle.Render(resource.Type))
			b.WriteString("\n")
		}
		if resource.Name != "" {
			b.WriteString("  ")
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-10s", "Name:")))
			b.WriteString(" ")
			b.WriteString(metaStyle.Render(resource.Name))
			b.WriteString("\n")
		}
	}
	
	return b.String()
}

// renderUpdateDiff renders a detailed diff for updates with syntax highlighting
func (m *modernPlanModel) renderUpdateDiff(before, after map[string]interface{}) string {
	var b strings.Builder
	
	// Styles for diff
	addedStyle := lipgloss.NewStyle().Foreground(successColor)
	removedStyle := lipgloss.NewStyle().Foreground(dangerColor)
	modifiedStyle := lipgloss.NewStyle().Foreground(warningColor)
	keyStyle := lipgloss.NewStyle().Foreground(accentColor)
	
	// Collect all keys
	allKeys := make(map[string]bool)
	for k := range before {
		allKeys[k] = true
	}
	for k := range after {
		allKeys[k] = true
	}
	
	// Sort keys for consistent display
	var keys []string
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, key := range keys {
		beforeVal, beforeExists := before[key]
		afterVal, afterExists := after[key]
		
		if !beforeExists && afterExists {
			// Added attribute
			b.WriteString(addedStyle.Render("+ "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
			b.WriteString(addedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(afterVal))))
		} else if beforeExists && !afterExists {
			// Removed attribute
			b.WriteString(removedStyle.Render("- "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
			b.WriteString(removedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(beforeVal))))
		} else if !m.valuesEqual(beforeVal, afterVal) {
			// Modified attribute
			b.WriteString(modifiedStyle.Render("~ "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
			b.WriteString(removedStyle.Render(fmt.Sprintf(" %v", m.formatValue(beforeVal))))
			b.WriteString(modifiedStyle.Render(" → "))
			b.WriteString(addedStyle.Render(fmt.Sprintf("%v\n", m.formatValue(afterVal))))
		}
	}
	
	if b.Len() == 0 {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("No changes detected")
	}
	
	return b.String()
}

// renderUnchangedAttributes renders attributes that haven't changed
func (m *modernPlanModel) renderUnchangedAttributes(attributes map[string]interface{}) string {
	var b strings.Builder
	
	keyStyle := lipgloss.NewStyle().Foreground(mutedColor)
	valueStyle := lipgloss.NewStyle().Foreground(mutedColor).Faint(true)
	
	// Sort keys for consistent display
	var keys []string
	for k := range attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, key := range keys {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
		b.WriteString(valueStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(attributes[key]))))
	}
	
	return b.String()
}

// getUnchangedAttributes returns attributes that are the same in before and after
func (m *modernPlanModel) getUnchangedAttributes(before, after map[string]interface{}) map[string]interface{} {
	unchanged := make(map[string]interface{})
	
	for key, beforeVal := range before {
		if afterVal, exists := after[key]; exists && m.valuesEqual(beforeVal, afterVal) {
			unchanged[key] = beforeVal
		}
	}
	
	return unchanged
}

// renderExpandedAttributes renders attributes with full expansion of nested structures
func (m *modernPlanModel) renderExpandedAttributesWithPrefix(attributes map[string]interface{}, style lipgloss.Style, prefix string) string {
	var b strings.Builder
	
	keyStyle := lipgloss.NewStyle().Foreground(accentColor)
	
	// Sort keys for consistent display
	var keys []string
	for k := range attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, key := range keys {
		value := attributes[key]
		b.WriteString(style.Render(prefix))
		
		b.WriteString(keyStyle.Render(fmt.Sprintf("%-30s", key+":")))
		b.WriteString(" ")
		
		// Always expand arrays and maps in full-screen view
		switch v := value.(type) {
		case []interface{}:
			if len(v) == 0 {
				b.WriteString(style.Render("[]"))
			} else {
				b.WriteString(m.renderExpandedArray(v, style, prefix+"  "))
			}
		case map[string]interface{}:
			if len(v) == 0 {
				b.WriteString(style.Render("{}"))
			} else {
				b.WriteString(m.renderExpandedMap(v, style, prefix+"  "))
			}
		default:
			b.WriteString(m.renderExpandedValue(value, style, prefix+"  "))
		}
		b.WriteString("\n")
	}
	
	return b.String()
}

func (m *modernPlanModel) renderExpandedAttributes(attributes map[string]interface{}, style lipgloss.Style, prefix string) string {
	var b strings.Builder
	
	keyStyle := lipgloss.NewStyle().Foreground(accentColor)
	
	// Sort keys for consistent display
	var keys []string
	for k := range attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, key := range keys {
		value := attributes[key]
		b.WriteString(prefix)
		
		if prefix == "" {
			// For added items
			b.WriteString(style.Render("+ "))
		}
		
		b.WriteString(keyStyle.Render(fmt.Sprintf("%-30s", key+":")))
		b.WriteString(" ")
		
		// Always expand arrays and maps in full-screen view
		switch v := value.(type) {
		case []interface{}:
			if len(v) == 0 {
				b.WriteString(style.Render("[]"))
			} else {
				b.WriteString(m.renderExpandedArray(v, style, prefix+"  "))
			}
		case map[string]interface{}:
			if len(v) == 0 {
				b.WriteString(style.Render("{}"))
			} else {
				b.WriteString(m.renderExpandedMap(v, style, prefix+"  "))
			}
		default:
			b.WriteString(m.renderExpandedValue(value, style, prefix+"  "))
		}
		b.WriteString("\n")
	}
	
	return b.String()
}

// renderExpandedArray always expands arrays to show their full content
func (m *modernPlanModel) renderExpandedArray(arr []interface{}, style lipgloss.Style, indent string) string {
	// Check if all items are simple
	allSimple := true
	for _, item := range arr {
		switch item.(type) {
		case string, bool, float64, nil:
			// simple
		default:
			allSimple = false
			break
		}
	}
	
	// Simple arrays can be inline
	if allSimple {
		items := make([]string, len(arr))
		for i, item := range arr {
			items[i] = fmt.Sprintf("%v", item)
		}
		return style.Render("[" + strings.Join(items, ", ") + "]")
	}
	
	// For single item arrays with a map, try to show inline if simple
	if len(arr) == 1 {
		if mp, ok := arr[0].(map[string]interface{}); ok && len(mp) <= 4 && isSimpleMap(mp) {
			// Show simple single map inline
			items := make([]string, 0, len(mp))
			for k, v := range mp {
				items = append(items, fmt.Sprintf("%s: %v", k, v))
			}
			sort.Strings(items)
			return style.Render("[{" + strings.Join(items, ", ") + "}]")
		}
	}
	
	// Complex arrays need full expansion
	var b strings.Builder
	b.WriteString(style.Render("["))
	b.WriteString("\n")
	
	for i, item := range arr {
		b.WriteString(indent + "    ")
		
		// Render each item with full expansion
		switch v := item.(type) {
		case map[string]interface{}:
			// Expand maps within arrays properly
			b.WriteString(m.renderExpandedMap(v, style, indent+"    "))
		case []interface{}:
			// Nested arrays
			b.WriteString(m.renderExpandedArray(v, style, indent+"    "))
		default:
			// Simple values
			b.WriteString(style.Render(m.formatSimpleValue(item)))
		}
		
		if i < len(arr)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "  " + style.Render("]"))
	return b.String()
}

// renderExpandedMap always expands maps to show their full content
func (m *modernPlanModel) renderExpandedMap(mp map[string]interface{}, style lipgloss.Style, indent string) string {
	// Check if it's a simple map that can be shown inline
	if len(mp) <= 3 && isSimpleMap(mp) {
		items := make([]string, 0, len(mp))
		for k, v := range mp {
			items = append(items, fmt.Sprintf("%s: %v", k, v))
		}
		sort.Strings(items)
		result := "{" + strings.Join(items, ", ") + "}"
		if len(result) <= 80 {
			return style.Render(result)
		}
	}
	
	// Complex maps need expansion
	var b strings.Builder
	b.WriteString(style.Render("{"))
	
	sortedKeys := make([]string, 0, len(mp))
	for k := range mp {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	
	// Always start on new line for complex objects
	b.WriteString("\n")
	
	for i, k := range sortedKeys {
		b.WriteString(indent + "    ")
		b.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render(k + ": "))
		
		// Recursively render the value
		val := mp[k]
		switch v := val.(type) {
		case []interface{}:
			// Arrays get their own handling
			result := m.renderExpandedArray(v, style, indent+"    ")
			b.WriteString(result)
		case map[string]interface{}:
			// Nested maps
			result := m.renderExpandedMap(v, style, indent+"    ")
			b.WriteString(result)
		default:
			// Simple values
			b.WriteString(style.Render(m.formatSimpleValue(val)))
		}
		
		if i < len(sortedKeys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "  " + style.Render("}"))
	return b.String()
}

// formatSimpleValue formats a simple value for display
func (m *modernPlanModel) formatSimpleValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "(empty)"
		}
		// Check if it's a JSON string and parse it
		if strings.HasPrefix(strings.TrimSpace(v), "[") || strings.HasPrefix(strings.TrimSpace(v), "{") {
			var parsed interface{}
			if err := json.Unmarshal([]byte(v), &parsed); err == nil {
				// Successfully parsed JSON, format it nicely
				return m.renderParsedJSON(parsed, "", lipgloss.NewStyle().Foreground(fgColor))
			}
		}
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case nil:
		return "null"
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// renderInlineMap renders a map, attempting to show full details when possible
func (m *modernPlanModel) renderInlineMap(mp map[string]interface{}, style lipgloss.Style) string {
	items := make([]string, 0, len(mp))
	
	// Sort keys for consistent output
	sortedKeys := make([]string, 0, len(mp))
	for k := range mp {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	
	for _, k := range sortedKeys {
		v := mp[k]
		valStr := ""
		
		// Render values with more detail
		switch val := v.(type) {
		case string:
			valStr = val
		case bool:
			valStr = fmt.Sprintf("%t", val)
		case float64:
			if val == float64(int64(val)) {
				valStr = fmt.Sprintf("%d", int64(val))
			} else {
				valStr = fmt.Sprintf("%.2f", val)
			}
		case nil:
			valStr = "null"
		case []interface{}:
			if len(val) == 0 {
				valStr = "[]"
			} else {
				// Try to render simple arrays inline
				allSimple := true
				for _, item := range val {
					switch item.(type) {
					case string, bool, float64, nil:
						// continue
					default:
						allSimple = false
						break
					}
				}
				if allSimple && len(val) <= 3 {
					simpleItems := make([]string, len(val))
					for i, item := range val {
						simpleItems[i] = fmt.Sprintf("%v", item)
					}
					valStr = "[" + strings.Join(simpleItems, ", ") + "]"
				} else {
					// For complex arrays, we need to expand them
					valStr = fmt.Sprintf("[%d items]", len(val))
				}
			}
		case map[string]interface{}:
			if len(val) == 0 {
				valStr = "{}"
			} else {
				// Try to render simple maps inline
				if len(val) <= 2 && isSimpleMap(val) {
					subItems := make([]string, 0, len(val))
					for sk, sv := range val {
						subItems = append(subItems, fmt.Sprintf("%s: %v", sk, sv))
					}
					sort.Strings(subItems)
					valStr = "{" + strings.Join(subItems, ", ") + "}"
				} else {
					valStr = fmt.Sprintf("{%d keys}", len(val))
				}
			}
		default:
			valStr = fmt.Sprintf("%v", v)
		}
		
		items = append(items, fmt.Sprintf("%s: %s", k, valStr))
	}
	
	return style.Render("{" + strings.Join(items, ", ") + "}")
}

// renderParsedJSON renders parsed JSON data with proper formatting
func (m *modernPlanModel) renderParsedJSON(data interface{}, indent string, style lipgloss.Style) string {
	switch v := data.(type) {
	case []interface{}:
		if len(v) == 0 {
			return style.Render("[]")
		}
		var b strings.Builder
		b.WriteString(style.Render("[\n"))
		for i, item := range v {
			b.WriteString(indent + "  ")
			b.WriteString(m.renderParsedJSON(item, indent+"  ", style))
			if i < len(v)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + style.Render("]"))
		return b.String()
		
	case map[string]interface{}:
		if len(v) == 0 {
			return style.Render("{}")
		}
		var b strings.Builder
		b.WriteString(style.Render("{\n"))
		
		// Sort keys for consistent output
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		for i, k := range keys {
			b.WriteString(indent + "  ")
			b.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render(k + ": "))
			b.WriteString(m.renderParsedJSON(v[k], indent+"  ", style))
			if i < len(keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + style.Render("}"))
		return b.String()
		
	case string:
		// Quote strings in JSON output
		return style.Render(fmt.Sprintf("%q", v))
		
	case float64:
		if v == float64(int64(v)) {
			return style.Render(fmt.Sprintf("%d", int64(v)))
		}
		return style.Render(fmt.Sprintf("%.2f", v))
		
	case bool:
		return style.Render(fmt.Sprintf("%t", v))
		
	case nil:
		return style.Render("null")
		
	default:
		return style.Render(fmt.Sprintf("%v", v))
	}
}

// renderExpandedValue renders a value with full expansion of nested structures
func (m *modernPlanModel) renderExpandedValue(value interface{}, style lipgloss.Style, indent string) string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return style.Render("(empty)")
		}
		// Check if it's a JSON string and parse it
		if strings.HasPrefix(strings.TrimSpace(v), "[") || strings.HasPrefix(strings.TrimSpace(v), "{") {
			var parsed interface{}
			if err := json.Unmarshal([]byte(v), &parsed); err == nil {
				// Successfully parsed JSON, format it nicely
				return m.renderParsedJSON(parsed, indent, style)
			}
		}
		return style.Render(v)
	case bool:
		return style.Render(fmt.Sprintf("%t", v))
	case nil:
		return style.Render("null")
	case float64:
		if v == float64(int64(v)) {
			return style.Render(fmt.Sprintf("%d", int64(v)))
		}
		return style.Render(fmt.Sprintf("%.2f", v))
	case []interface{}:
		if len(v) == 0 {
			return style.Render("[]")
		}
		
		// Check if it's a simple array or array of maps
		allMaps := true
		allSimple := true
		for _, item := range v {
			if _, isMap := item.(map[string]interface{}); !isMap {
				allMaps = false
			}
			switch item.(type) {
			case string, bool, float64, nil:
				// simple type
			default:
				allSimple = false
			}
		}
		
		// For arrays of simple values, always show inline
		if allSimple {
			items := make([]string, len(v))
			for i, item := range v {
				items[i] = fmt.Sprintf("%v", item)
			}
			return style.Render("[" + strings.Join(items, ", ") + "]")
		}
		
		// For single map in array, show more compact
		if len(v) == 1 && allMaps {
			if m, ok := v[0].(map[string]interface{}); ok && isSimpleMap(m) {
				// Single simple map, show inline
				items := make([]string, 0, len(m))
				for k, val := range m {
					items = append(items, fmt.Sprintf("%s: %v", k, val))
				}
				sort.Strings(items)
				return style.Render("[{" + strings.Join(items, ", ") + "}]")
			}
		}
		
		// For complex arrays or multiple items, show expanded but more compact
		var b strings.Builder
		b.WriteString(style.Render("["))
		
		// If it's an array of maps, use compact formatting for small simple maps
		if allMaps && len(v) <= 2 {
			allSimpleMaps := true
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					if !isSimpleMap(m) || len(m) > 4 {
						allSimpleMaps = false
						break
					}
				}
			}
			
			if allSimpleMaps {
				// Inline format for simple maps
				for i, item := range v {
					if i > 0 {
						b.WriteString(", ")
					}
					if m, ok := item.(map[string]interface{}); ok {
						items := make([]string, 0, len(m))
						for k, val := range m {
							// Use actual values, not formatCompactValue
							valStr := fmt.Sprintf("%v", val)
							if s, ok := val.(string); ok && s != "" {
								valStr = s
							}
							items = append(items, fmt.Sprintf("%s: %s", k, valStr))
						}
						sort.Strings(items)
						b.WriteString("{" + strings.Join(items, ", ") + "}")
					}
				}
				b.WriteString(style.Render("]"))
				return b.String()
			}
		}
		
		// Full expansion for complex structures
		b.WriteString("\n")
		for i, item := range v {
			b.WriteString(indent + "  ")
			b.WriteString(m.renderExpandedValue(item, style, indent+"  "))
			if i < len(v)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + style.Render("]"))
		return b.String()
		
	case map[string]interface{}:
		if len(v) == 0 {
			return style.Render("{}")
		}
		
		// For tags and other simple maps, always show inline
		if _, hasName := v["Name"]; hasName && len(v) <= 3 {
			// Likely tags, show inline
			items := make([]string, 0, len(v))
			for key, val := range v {
				items = append(items, fmt.Sprintf("%s: %v", key, formatCompactValue(val)))
			}
			sort.Strings(items)
			return style.Render("{" + strings.Join(items, ", ") + "}")
		}
		
		// For small simple maps, show inline
		if len(v) <= 4 && isSimpleMap(v) {
			items := make([]string, 0, len(v))
			for k, val := range v {
				items = append(items, fmt.Sprintf("%s: %v", k, formatCompactValue(val)))
			}
			sort.Strings(items)
			result := "{" + strings.Join(items, ", ") + "}"
			// If it's not too long, show inline
			if len(result) <= 80 {
				return style.Render(result)
			}
		}
		
		// For complex maps, show expanded with proper indentation
		var b strings.Builder
		b.WriteString(style.Render("{"))
		
		sortedKeys := make([]string, 0, len(v))
		for k := range v {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		
		// Check if we can show in compact form
		canBeCompact := true
		for _, k := range sortedKeys {
			switch v[k].(type) {
			case []interface{}, map[string]interface{}:
				if !isSimpleValue(v[k]) {
					canBeCompact = false
					break
				}
			}
		}
		
		if canBeCompact && len(v) <= 4 {
			// Compact inline form
			items := make([]string, 0, len(v))
			for _, k := range sortedKeys {
				items = append(items, fmt.Sprintf("%s: %v", k, formatCompactValue(v[k])))
			}
			return style.Render("{" + strings.Join(items, ", ") + "}")
		}
		
		// Full expansion
		b.WriteString("\n")
		for i, k := range sortedKeys {
			b.WriteString(indent + "  ")
			b.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render(k + ": "))
			b.WriteString(m.renderExpandedValue(v[k], style, indent+"  "))
			if i < len(sortedKeys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indent + style.Render("}"))
		return b.String()
		
	default:
		return style.Render(fmt.Sprintf("%v", v))
	}
}

// formatCompactValue formats a value in compact form (used for inline display)
func formatCompactValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "(empty)"
		}
		if len(v) > 30 {
			return v[:27] + "..."
		}
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case nil:
		return "null"
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%.2f", v)
	case []interface{}:
		if len(v) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", len(v))
	case map[string]interface{}:
		if len(v) == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d keys}", len(v))
	default:
		str := fmt.Sprintf("%v", v)
		if len(str) > 30 {
			return str[:27] + "..."
		}
		return str
	}
}

// isSimpleValue checks if a value is simple enough for inline display
func isSimpleValue(value interface{}) bool {
	switch v := value.(type) {
	case string, bool, float64, nil:
		return true
	case []interface{}:
		return len(v) == 0 || (len(v) <= 3 && isSimpleArray(v))
	case map[string]interface{}:
		return len(v) == 0 || (len(v) <= 2 && isSimpleMap(v))
	default:
		return false
	}
}

// renderExpandedUpdateDiff renders an expanded diff for updates
func (m *modernPlanModel) renderExpandedUpdateDiff(before, after map[string]interface{}) string {
	var b strings.Builder
	
	// Styles for diff
	addedStyle := lipgloss.NewStyle().Foreground(successColor)
	removedStyle := lipgloss.NewStyle().Foreground(dangerColor)
	modifiedStyle := lipgloss.NewStyle().Foreground(warningColor)
	keyStyle := lipgloss.NewStyle().Foreground(accentColor)
	
	// Collect all keys
	allKeys := make(map[string]bool)
	for k := range before {
		allKeys[k] = true
	}
	for k := range after {
		allKeys[k] = true
	}
	
	// Sort keys for consistent display
	var keys []string
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, key := range keys {
		beforeVal, beforeExists := before[key]
		afterVal, afterExists := after[key]
		
		if !beforeExists && afterExists {
			// Added attribute
			b.WriteString(addedStyle.Render("+ "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-30s", key+":")))
			b.WriteString(" ")
			b.WriteString(m.renderExpandedValue(afterVal, addedStyle, "  "))
			b.WriteString("\n")
		} else if beforeExists && !afterExists {
			// Removed attribute
			b.WriteString(removedStyle.Render("- "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-30s", key+":")))
			b.WriteString(" ")
			b.WriteString(m.renderExpandedValue(beforeVal, removedStyle, "  "))
			b.WriteString("\n")
		} else if !m.valuesEqual(beforeVal, afterVal) {
			// Modified attribute
			b.WriteString(modifiedStyle.Render("~ "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-30s", key+":")))
			b.WriteString("\n")
			b.WriteString(removedStyle.Render("    - "))
			b.WriteString(m.renderExpandedValue(beforeVal, removedStyle, "      "))
			b.WriteString("\n")
			b.WriteString(addedStyle.Render("    + "))
			b.WriteString(m.renderExpandedValue(afterVal, addedStyle, "      "))
			b.WriteString("\n")
		}
	}
	
	if b.Len() == 0 {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("No changes detected")
	}
	
	return b.String()
}

// Helper functions to determine if values are simple
func isSimpleArray(arr []interface{}) bool {
	for _, item := range arr {
		switch item.(type) {
		case string, bool, float64, nil:
			continue
		default:
			return false
		}
	}
	return true
}

func isSimpleMap(m map[string]interface{}) bool {
	for _, val := range m {
		switch val.(type) {
		case string, bool, float64, nil:
			continue
		default:
			return false
		}
	}
	return true
}

// valuesEqual compares two values for equality
func (m *modernPlanModel) valuesEqual(a, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	
	// Use formatValue to normalize and compare
	return m.formatValue(a) == m.formatValue(b)
}

// formatValue formats a value for display
func (m *modernPlanModel) formatValue(value interface{}) string {
	return formatValue(value)
}

// renderAttributesDiff renders attributes in a diff format
func (m *modernPlanModel) renderAttributesDiff(before, after map[string]interface{}, showUnchanged bool) string {
	var b strings.Builder
	
	// Styles
	addedStyle := lipgloss.NewStyle().Foreground(successColor)
	removedStyle := lipgloss.NewStyle().Foreground(dangerColor)
	keyStyle := lipgloss.NewStyle().Foreground(accentColor)
	unchangedStyle := lipgloss.NewStyle().Foreground(mutedColor)
	
	// If only showing deletions (before != nil, after == nil)
	if before != nil && after == nil {
		var keys []string
		for k := range before {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		for _, key := range keys {
			b.WriteString(removedStyle.Render("- "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
			b.WriteString(removedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(before[key]))))
		}
		return b.String()
	}
	
	// If only showing additions (before == nil, after != nil)
	if before == nil && after != nil {
		var keys []string
		for k := range after {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		for _, key := range keys {
			b.WriteString(addedStyle.Render("+ "))
			b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
			b.WriteString(addedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(after[key]))))
		}
		return b.String()
	}
	
	// If comparing before and after
	if before != nil && after != nil {
		allKeys := make(map[string]bool)
		for k := range before {
			allKeys[k] = true
		}
		for k := range after {
			allKeys[k] = true
		}
		
		var keys []string
		for k := range allKeys {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		for _, key := range keys {
			beforeVal, beforeExists := before[key]
			afterVal, afterExists := after[key]
			
			if !beforeExists && afterExists {
				// Added
				b.WriteString(addedStyle.Render("+ "))
				b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
				b.WriteString(addedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(afterVal))))
			} else if beforeExists && !afterExists {
				// Removed
				b.WriteString(removedStyle.Render("- "))
				b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
				b.WriteString(removedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(beforeVal))))
			} else if showUnchanged || !m.valuesEqual(beforeVal, afterVal) {
				if m.valuesEqual(beforeVal, afterVal) {
					// Unchanged
					b.WriteString(unchangedStyle.Render("  "))
					b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
					b.WriteString(unchangedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(afterVal))))
				} else {
					// Modified
					b.WriteString(removedStyle.Render("- "))
					b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
					b.WriteString(removedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(beforeVal))))
					b.WriteString(addedStyle.Render("+ "))
					b.WriteString(keyStyle.Render(fmt.Sprintf("%-28s", key+":")))
					b.WriteString(addedStyle.Render(fmt.Sprintf(" %v\n", m.formatValue(afterVal))))
				}
			}
		}
	}
	
	return b.String()
}

func (m *modernPlanModel) renderResourceDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Clean up the address for display (remove suffixes we added)
	displayAddress := resource.Address
	displayAddress = strings.TrimSuffix(displayAddress, " (destroy)")
	displayAddress = strings.TrimSuffix(displayAddress, " (create)")
	
	// Resource header with icon
	icon := getResourceIcon(resource.Type)
	actionStyle := getActionStyle(resource.Change.Actions[0])
	
	b.WriteString(fmt.Sprintf("%s %s\n\n", icon, titleStyle.Render(displayAddress)))
	
	// Action badge - show special indicator for replacement parts
	actionText := strings.ToUpper(resource.Change.Actions[0])
	if strings.HasSuffix(resource.Address, " (destroy)") {
		actionText = "DELETE (REPLACEMENT)"
	} else if strings.HasSuffix(resource.Address, " (create)") {
		actionText = "CREATE (REPLACEMENT)"
	}
	
	actionBadge := actionStyle.Padding(0, 1).Render(actionText)
	replaceBadge := ""
	if m.markedForReplace[displayAddress] {
		replaceBadge = "  " + lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Padding(0, 1).
			Render("MARKED FOR REPLACE")
	}
	b.WriteString(fmt.Sprintf("%s  %s%s\n\n", actionBadge, typeStyle.Render(resource.Type), replaceBadge))
	
	// Render based on action type
	switch resource.Change.Actions[0] {
	case "create":
		if strings.HasSuffix(resource.Address, " (create)") {
			// This is the create part of a replacement
			b.WriteString(m.renderReplacementCreateDetails(resource))
		} else {
			b.WriteString(m.renderCreateDetails(resource))
		}
	case "update":
		b.WriteString(m.renderUpdateDetails(resource))
	case "delete":
		if strings.HasSuffix(resource.Address, " (destroy)") {
			// This is the delete part of a replacement
			b.WriteString(m.renderReplacementDeleteDetails(resource))
		} else {
			b.WriteString(m.renderDeleteDetails(resource))
		}
	case "replace":
		b.WriteString(m.renderReplaceDetails(resource))
	}
	
	return b.String()
}

// renderReplacementDeleteDetails shows the delete part of a replacement
func (m *modernPlanModel) renderReplacementDeleteDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Simple header - no box, just text
	warningStyle := lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	b.WriteString(warningStyle.Render("🔄 Delete phase (replacement)") + "\n\n")
	
	if resource.Change.Before != nil {
		b.WriteString(m.renderFormattedAttributes(resource.Change.Before, lipgloss.NewStyle().Foreground(dangerColor)))
	}
	
	return b.String()
}

// renderReplacementCreateDetails shows the create part of a replacement
func (m *modernPlanModel) renderReplacementCreateDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Simple header - no box, just text
	createStyle := lipgloss.NewStyle().Foreground(successColor).Bold(true)
	b.WriteString(createStyle.Render("🔄 Create phase (replacement)") + "\n\n")
	
	if resource.Change.After != nil {
		b.WriteString(m.renderFormattedAttributes(resource.Change.After, lipgloss.NewStyle().Foreground(successColor)))
	}
	
	return b.String()
}

func getActionStyle(action string) lipgloss.Style {
	switch action {
	case "create":
		return lipgloss.NewStyle().Background(successColor).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	case "update":
		return lipgloss.NewStyle().Background(warningColor).Foreground(lipgloss.Color("#000000")).Bold(true)
	case "delete":
		return lipgloss.NewStyle().Background(dangerColor).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	case "replace":
		return lipgloss.NewStyle().Background(primaryColor).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	default:
		return lipgloss.NewStyle()
	}
}

func (m *modernPlanModel) renderCreateDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	if resource.Change.After != nil {
		b.WriteString(m.renderFormattedAttributes(resource.Change.After, lipgloss.NewStyle().Foreground(fgColor)))
	}
	
	return b.String()
}

func (m *modernPlanModel) renderUpdateDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Find changed attributes
	changes := m.findChangedAttributes(resource.Change.Before, resource.Change.After)
	
	if len(changes) > 0 {
		changesHeader := lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)
		
		b.WriteString(changesHeader.Render("━━━ Changes ━━━") + "\n\n")
		
		for i, change := range changes {
			b.WriteString(m.renderSimpleChange(change))
			if i < len(changes)-1 {
				b.WriteString("\n")
			}
		}
	}
	
	// Show unchanged important attributes (only if there are few changes)
	if len(changes) < 3 {
		unchanged := m.findUnchangedImportantAttributes(resource.Type, resource.Change.Before, resource.Change.After)
		if len(unchanged) > 0 && len(unchanged) < 5 {
			b.WriteString("\n\n")
			
			unchangedHeader := lipgloss.NewStyle().
				Foreground(dimColor)
			
			b.WriteString(unchangedHeader.Render("━━━ Unchanged ━━━") + "\n\n")
			
			for _, key := range sortedKeys(unchanged) {
				b.WriteString(fmt.Sprintf("  %-28s %s\n",
					lipgloss.NewStyle().Foreground(mutedColor).Render(key + ":"),
					lipgloss.NewStyle().Foreground(dimColor).Render(formatValue(unchanged[key]))))
			}
		}
	}
	
	return b.String()
}

// renderSimpleChange renders a change in a compact format
func (m *modernPlanModel) renderSimpleChange(change attributeChange) string {
	keyStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)
	
	if change.isNew {
		return fmt.Sprintf("  %s %-20s %s",
			createIconStyle.Render("+"),
			keyStyle.Render(change.key + ":"),
			createIconStyle.Render(formatValue(change.after)))
	}
	
	if change.isRemoved {
		return fmt.Sprintf("  %s %-20s %s",
			deleteIconStyle.Render("-"),
			keyStyle.Render(change.key + ":"),
			deleteIconStyle.Render(formatValue(change.before)))
	}
	
	// Changed value - show inline
	return fmt.Sprintf("  %s %-20s %s → %s",
		updateIconStyle.Render("~"),
		keyStyle.Render(change.key + ":"),
		deleteIconStyle.Render(formatValue(change.before)),
		createIconStyle.Render(formatValue(change.after)))
}

// renderFormattedChange renders a single change with better formatting
func (m *modernPlanModel) renderFormattedChange(change attributeChange) string {
	keyStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)
	
	if change.isNew {
		// New attribute
		box := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(successColor).
			Padding(0, 1)
		
		content := fmt.Sprintf("%s %s\n%s",
			createIconStyle.Render("ADD"),
			keyStyle.Render(change.key),
			createIconStyle.Render(formatValue(change.after)))
		
		return box.Render(content)
	}
	
	if change.isRemoved {
		// Removed attribute
		box := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(dangerColor).
			Padding(0, 1)
		
		content := fmt.Sprintf("%s %s\n%s",
			deleteIconStyle.Render("REMOVE"),
			keyStyle.Render(change.key),
			deleteIconStyle.Render(formatValue(change.before)))
		
		return box.Render(content)
	}
	
	// Modified attribute - show diff
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(warningColor).
		Padding(0, 1)
	
	content := fmt.Sprintf("%s %s\n%s %s\n%s %s",
		updateIconStyle.Render("MODIFY"),
		keyStyle.Render(change.key),
		deleteIconStyle.Render("- "),
		formatValue(change.before),
		createIconStyle.Render("+ "),
		formatValue(change.after))
	
	return box.Render(content)
}

func (m *modernPlanModel) renderDeleteDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Simple, compact warning message
	warningStyle := lipgloss.NewStyle().
		Foreground(dangerColor).
		Bold(true)
	
	b.WriteString(warningStyle.Render("⚠️  This resource will be permanently deleted") + "\n\n")
	
	if resource.Change.Before != nil {
		b.WriteString(m.renderFormattedAttributes(resource.Change.Before, lipgloss.NewStyle().Foreground(fgColor)))
	}
	
	return b.String()
}

func (m *modernPlanModel) renderReplaceDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Header with clear replacement indication
	headerStyle := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true).
		Padding(0, 1).
		Border(lipgloss.ThickBorder()).
		BorderForeground(primaryColor)
	
	b.WriteString(headerStyle.Render("⚠️  RESOURCE REPLACEMENT") + "\n\n")
	
	// Explain what replacement means
	warningBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#2a1a2a")).
		Foreground(lipgloss.Color("#ff9999")).
		Padding(0, 1).
		Width(60)
	
	b.WriteString(warningBox.Render("⚠️  Resource will be destroyed and recreated.\n⚠️  Potential data loss - ensure backups exist.") + "\n\n")
	
	// Show what's forcing the replacement
	if resource.Change.Before != nil && resource.Change.After != nil {
		forceNew := m.findForceNewAttributes(resource.Change.Before, resource.Change.After)
		if len(forceNew) > 0 {
			reasonHeader := lipgloss.NewStyle().
				Foreground(dangerColor).
				Bold(true).
				MarginBottom(1)
			
			b.WriteString(reasonHeader.Render("━━━ Attributes Forcing Replacement ━━━") + "\n\n")
			
			for _, attr := range forceNew {
				attrBox := lipgloss.NewStyle().
					Border(lipgloss.NormalBorder()).
					BorderForeground(dangerColor).
					Padding(0, 1).
					Foreground(dangerColor)
				
				b.WriteString(attrBox.Render("⚡ " + attr) + "\n")
			}
			b.WriteString("\n")
		}
	}
	
	// Show side-by-side comparison if both exist
	if resource.Change.Before != nil && resource.Change.After != nil {
		// Current state section
		deleteHeader := lipgloss.NewStyle().
			Foreground(dangerColor).
			Bold(true).
			MarginBottom(1)
		
		b.WriteString(deleteHeader.Render("━━━ Current State (to be destroyed) ━━━") + "\n\n")
		b.WriteString(m.renderFormattedAttributes(resource.Change.Before, lipgloss.NewStyle().Foreground(dangerColor)))
		b.WriteString("\n")
		
		// New state section
		createHeader := lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true).
			MarginBottom(1)
		
		b.WriteString(createHeader.Render("━━━ New State (to be created) ━━━") + "\n\n")
		b.WriteString(m.renderFormattedAttributes(resource.Change.After, lipgloss.NewStyle().Foreground(successColor)))
	}
	
	return b.String()
}

func (m *modernPlanModel) renderAttributeTree(title string, attrs map[string]interface{}, style lipgloss.Style) string {
	var b strings.Builder
	
	// Title with underline (only if title is provided)
	if title != "" {
		titleStyle := lipgloss.NewStyle().Bold(true).Underline(true)
		b.WriteString(titleStyle.Render(title) + "\n\n")
	}
	
	// Use the new formatted attributes renderer
	b.WriteString(m.renderFormattedAttributes(attrs, style))
	
	return b.String()
}

// renderFormattedAttributes renders attributes in a clean, formatted way
func (m *modernPlanModel) renderFormattedAttributes(attrs map[string]interface{}, style lipgloss.Style) string {
	var b strings.Builder
	
	// Group and sort attributes
	important, others := m.categorizeAttributes(attrs)
	
	// Create a table-like layout for better readability
	if len(important) > 0 {
		// Important attributes section header
		sectionStyle := lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)
		
		b.WriteString(sectionStyle.Render("━━━ Key Attributes ━━━") + "\n\n")
		
		for _, key := range sortedKeys(important) {
			b.WriteString(m.renderFormattedAttribute(key, important[key], style))
			b.WriteString("\n")
		}
	}
	
	// Other attributes
	if len(others) > 0 {
		if len(important) > 0 {
			b.WriteString("\n")
		}
		
		sectionStyle := lipgloss.NewStyle().
			Foreground(mutedColor)
		
		b.WriteString(sectionStyle.Render("━━━ Other Attributes ━━━") + "\n\n")
		
		for _, key := range sortedKeys(others) {
			b.WriteString(m.renderFormattedAttribute(key, others[key], lipgloss.NewStyle().Foreground(fgColor)))
			b.WriteString("\n")
		}
	}
	
	return b.String()
}

// renderFormattedAttribute renders a single attribute with proper formatting
func (m *modernPlanModel) renderFormattedAttribute(key string, value interface{}, style lipgloss.Style) string {
	// Format the value based on its type
	var formattedValue string
	switch v := value.(type) {
	case map[string]interface{}:
		// For nested objects, show them indented on next line
		formattedValue = m.renderNestedObject(v, 1)
		return fmt.Sprintf("  %s: %s", 
			lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(key),
			style.Render(formattedValue))
	case []interface{}:
		// Use compact format for dashboard view
		formattedValue = m.renderArrayCompact(v)
	default:
		formattedValue = formatValue(value)
	}
	
	// Create a properly formatted row with consistent spacing
	// Use fixed-width formatting for better alignment
	keyPart := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Render(fmt.Sprintf("  %-28s", key + ":"))
	
	valuePart := style.Render(formattedValue)
	
	return fmt.Sprintf("%s %s", keyPart, valuePart)
}

// renderNestedObject renders nested objects with proper indentation
func (m *modernPlanModel) renderNestedObject(obj map[string]interface{}, indent int) string {
	if len(obj) == 0 {
		return "{}"
	}
	
	// For small objects, render inline
	if len(obj) <= 2 {
		var parts []string
		for _, k := range sortedKeys(obj) {
			parts = append(parts, fmt.Sprintf("%s: %s", k, formatValue(obj[k])))
		}
		return fmt.Sprintf("{ %s }", strings.Join(parts, ", "))
	}
	
	// For larger objects, render with indentation
	var b strings.Builder
	b.WriteString("\n")
	indentStr := strings.Repeat("  ", indent+1)
	for _, k := range sortedKeys(obj) {
		b.WriteString(fmt.Sprintf("%s%s: %s\n", indentStr, k, formatValue(obj[k])))
	}
	return b.String()
}

// renderArrayCompact renders arrays in a compact format for dashboard view
func (m *modernPlanModel) renderArrayCompact(arr []interface{}) string {
	if len(arr) == 0 {
		return "[]"
	}
	
	// Check if it's an array of complex objects
	isComplex := false
	for _, item := range arr {
		switch item.(type) {
		case map[string]interface{}, []interface{}:
			isComplex = true
			break
		}
		if isComplex {
			break
		}
	}
	
	if isComplex {
		// For arrays of objects in compact view, just show count
		return fmt.Sprintf("[%d objects]", len(arr))
	}
	
	// For simple arrays, show items inline
	if len(arr) <= 3 {
		var items []string
		for _, item := range arr {
			items = append(items, formatValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	}
	
	// Show first 2 items and count for longer arrays
	var items []string
	for i := 0; i < min(2, len(arr)); i++ {
		items = append(items, formatValue(arr[i]))
	}
	return fmt.Sprintf("[%s, ... +%d more]", strings.Join(items, ", "), len(arr)-2)
}

// renderArray renders arrays in a detailed format for full-screen view
func (m *modernPlanModel) renderArray(arr []interface{}) string {
	if len(arr) == 0 {
		return "[]"
	}
	
	// Check if it's an array of simple values or complex objects
	isComplex := false
	for _, item := range arr {
		switch item.(type) {
		case map[string]interface{}, []interface{}:
			isComplex = true
			break
		}
		if isComplex {
			break
		}
	}
	
	if isComplex {
		// For arrays of objects, render them properly formatted
		var b strings.Builder
		b.WriteString("[\n")
		for i, item := range arr {
			switch v := item.(type) {
			case map[string]interface{}:
				// Render each object on its own lines
				b.WriteString("    {\n")
				keys := make([]string, 0, len(v))
				for k := range v {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for j, k := range keys {
					b.WriteString(fmt.Sprintf("      %s: %v", k, formatValue(v[k])))
					if j < len(keys)-1 {
						b.WriteString(",")
					}
					b.WriteString("\n")
				}
				b.WriteString("    }")
			default:
				b.WriteString(fmt.Sprintf("    %v", formatValue(item)))
			}
			if i < len(arr)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("  ]")
		return b.String()
	}
	
	if len(arr) <= 3 {
		// Show all items if 3 or fewer
		var items []string
		for _, item := range arr {
			items = append(items, formatValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	}
	
	// Show first 3 items and count for longer arrays
	var items []string
	for i := 0; i < min(3, len(arr)); i++ {
		items = append(items, formatValue(arr[i]))
	}
	if len(arr) > 3 {
		return fmt.Sprintf("[%s, ... +%d more]", strings.Join(items, ", "), len(arr)-3)
	}
	return fmt.Sprintf("[%s]", strings.Join(items, ", "))
}

// Removed renderAttributeWithStyle - using new formatting functions instead

type attributeChange struct {
	key    string
	before interface{}
	after  interface{}
	isNew  bool
	isRemoved bool
}

func (m *modernPlanModel) findChangedAttributes(before, after map[string]interface{}) []attributeChange {
	changes := []attributeChange{}
	allKeys := make(map[string]bool)
	
	for k := range before {
		allKeys[k] = true
	}
	for k := range after {
		allKeys[k] = true
	}
	
	for key := range allKeys {
		if isComputedAttribute(key) {
			continue
		}
		
		beforeVal, beforeExists := before[key]
		afterVal, afterExists := after[key]
		
		if !beforeExists && afterExists {
			changes = append(changes, attributeChange{
				key:   key,
				after: afterVal,
				isNew: true,
			})
		} else if beforeExists && !afterExists {
			changes = append(changes, attributeChange{
				key:       key,
				before:    beforeVal,
				isRemoved: true,
			})
		} else if beforeExists && afterExists && fmt.Sprintf("%v", beforeVal) != fmt.Sprintf("%v", afterVal) {
			changes = append(changes, attributeChange{
				key:    key,
				before: beforeVal,
				after:  afterVal,
			})
		}
	}
	
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].key < changes[j].key
	})
	
	return changes
}

func (m *modernPlanModel) renderAttributeChange(change attributeChange) string {
	// Style for keys with proper padding
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	
	if change.isNew {
		return fmt.Sprintf("  %s %-20s %s",
			createIconStyle.Render("✚"),
			keyStyle.Render(change.key + ":"),
			createIconStyle.Render(formatValue(change.after)))
	}
	
	if change.isRemoved {
		return fmt.Sprintf("  %s %-20s %s",
			deleteIconStyle.Render("✖"),
			keyStyle.Render(change.key + ":"),
			deleteIconStyle.Render(formatValue(change.before)))
	}
	
	// Changed value - show as a diff with better formatting
	changeBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(warningColor).
		Padding(0, 1).
		Width(70)
	
	var changeContent strings.Builder
	changeContent.WriteString(fmt.Sprintf("%s %s\n",
		updateIconStyle.Render("⟳"),
		keyStyle.Render(change.key)))
	changeContent.WriteString(fmt.Sprintf("  %s %s\n",
		deleteIconStyle.Render("FROM:"),
		deleteIconStyle.Render(formatValue(change.before))))
	changeContent.WriteString(fmt.Sprintf("  %s %s",
		createIconStyle.Render("  TO:"),
		createIconStyle.Render(formatValue(change.after))))
	
	return changeBox.Render(changeContent.String())
}

func (m *modernPlanModel) categorizeAttributes(attrs map[string]interface{}) (important, others map[string]interface{}) {
	important = make(map[string]interface{})
	others = make(map[string]interface{})
	
	importantKeys := map[string]bool{
		// Core identifiers
		"name": true, "id": true, "arn": true, "domain_name": true,
		// Instance/compute
		"instance_type": true, "ami": true, "image": true,
		"desired_count": true, "min_size": true, "max_size": true,
		"cpu": true, "memory": true, 
		// Lambda/functions
		"runtime": true, "handler": true, "function_name": true,
		// Database
		"engine": true, "engine_version": true, "instance_class": true,
		"allocated_storage": true, "database_name": true,
		// Storage
		"bucket": true, "key": true, "acl": true,
		// Network
		"vpc_id": true, "subnet_id": true, "availability_zone": true,
		"port": true, "protocol": true, "cidr_blocks": true,
		// SSL/TLS
		"certificate_body": true, "certificate_chain": true,
		"validation_method": true, "key_algorithm": true,
	}
	
	for key, value := range attrs {
		if isComputedAttribute(key) {
			continue
		}
		if importantKeys[key] {
			important[key] = value
		} else {
			others[key] = value
		}
	}
	
	return
}

func (m *modernPlanModel) findUnchangedImportantAttributes(resourceType string, before, after map[string]interface{}) map[string]interface{} {
	unchanged := make(map[string]interface{})
	
	importantKeys := []string{"vpc_id", "subnet_id", "availability_zone", "region"}
	
	for _, key := range importantKeys {
		if beforeVal, exists := before[key]; exists {
			if afterVal, afterExists := after[key]; afterExists && fmt.Sprintf("%v", beforeVal) == fmt.Sprintf("%v", afterVal) {
				unchanged[key] = beforeVal
			}
		}
	}
	
	return unchanged
}

func (m *modernPlanModel) findForceNewAttributes(before, after map[string]interface{}) []string {
	forceNew := []string{}
	
	// Common attributes that force replacement
	forceNewKeys := []string{"ami", "instance_type", "availability_zone", "subnet_id", "engine", "engine_version"}
	
	for _, key := range forceNewKeys {
		beforeVal, beforeExists := before[key]
		afterVal, afterExists := after[key]
		
		if beforeExists && afterExists && fmt.Sprintf("%v", beforeVal) != fmt.Sprintf("%v", afterVal) {
			forceNew = append(forceNew, fmt.Sprintf("%s: %v → %v", key, formatValue(beforeVal), formatValue(afterVal)))
		}
	}
	
	return forceNew
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// Handle empty strings
		if v == "" {
			return "(empty)"
		}
		// Handle multi-line strings
		if strings.Contains(v, "\n") {
			lines := strings.Split(v, "\n")
			if len(lines) > 3 {
				return fmt.Sprintf("(multiline: %d lines)", len(lines))
			}
			return strings.Join(lines, " ")
		}
		// Truncate long strings but show more content
		if len(v) > 60 {
			return v[:57] + "..."
		}
		// Return string as-is for readability
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	case float64:
		// Format numbers nicely
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%.2f", v)
	case []interface{}:
		if len(v) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", len(v))
	case map[string]interface{}:
		if len(v) == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d keys}", len(v))
	default:
		str := fmt.Sprintf("%v", v)
		if len(str) > 60 {
			return str[:57] + "..."
		}
		return str
	}
}


func (m *modernPlanModel) getSelectedResource() *ResourceChange {
	if m.selectedProvider < 0 || m.selectedProvider >= len(m.providers) {
		return nil
	}
	
	provider := &m.providers[m.selectedProvider]
	if !provider.expanded || m.selectedService < 0 || m.selectedService >= len(provider.services) {
		return nil
	}
	
	service := &provider.services[m.selectedService]
	if !service.expanded || m.selectedResource < 0 || m.selectedResource >= len(service.resources) {
		return nil
	}
	
	return &service.resources[m.selectedResource]
}

func (m *modernPlanModel) navigateDown() {
	if len(m.providers) == 0 {
		return
	}
	
	provider := &m.providers[m.selectedProvider]
	
	// If on provider level
	if m.selectedService == -1 {
		if provider.expanded && len(provider.services) > 0 {
			m.selectedService = 0
			m.selectedResource = -1
		} else if m.selectedProvider < len(m.providers)-1 {
			m.selectedProvider++
			m.selectedService = -1
			m.selectedResource = -1
		}
		return
	}
	
	// If on service level
	if m.selectedResource == -1 {
		service := &provider.services[m.selectedService]
		if service.expanded && len(service.resources) > 0 {
			m.selectedResource = 0
		} else if m.selectedService < len(provider.services)-1 {
			m.selectedService++
			m.selectedResource = -1
		} else if m.selectedProvider < len(m.providers)-1 {
			m.selectedProvider++
			m.selectedService = -1
			m.selectedResource = -1
		}
		return
	}
	
	// If on resource level
	service := &provider.services[m.selectedService]
	if m.selectedResource < len(service.resources)-1 {
		m.selectedResource++
	} else if m.selectedService < len(provider.services)-1 {
		m.selectedService++
		m.selectedResource = -1
	} else if m.selectedProvider < len(m.providers)-1 {
		m.selectedProvider++
		m.selectedService = -1
		m.selectedResource = -1
	}
}

func (m *modernPlanModel) navigateUp() {
	if len(m.providers) == 0 {
		return
	}
	
	// If on resource level
	if m.selectedResource > 0 {
		m.selectedResource--
		return
	} else if m.selectedResource == 0 {
		m.selectedResource = -1
		return
	}
	
	// If on service level
	if m.selectedService > 0 {
		m.selectedService--
		provider := &m.providers[m.selectedProvider]
		service := &provider.services[m.selectedService]
		if service.expanded && len(service.resources) > 0 {
			m.selectedResource = len(service.resources) - 1
		}
		return
	} else if m.selectedService == 0 {
		m.selectedService = -1
		return
	}
	
	// If on provider level
	if m.selectedProvider > 0 {
		m.selectedProvider--
		provider := &m.providers[m.selectedProvider]
		if provider.expanded && len(provider.services) > 0 {
			m.selectedService = len(provider.services) - 1
			service := &provider.services[m.selectedService]
			if service.expanded && len(service.resources) > 0 {
				m.selectedResource = len(service.resources) - 1
			}
		}
	}
}


func (m *modernPlanModel) copyChangesToClipboard() {
	// Create a minimal structure with only the changes
	planExport := struct {
		TerraformVersion string           `json:"terraform_version"`
		ResourceChanges  []ResourceChange `json:"resource_changes"`
		Summary          struct {
			Total   int `json:"total"`
			Create  int `json:"create"`
			Update  int `json:"update"`
			Delete  int `json:"delete"`
			Replace int `json:"replace"`
		} `json:"summary"`
	}{
		TerraformVersion: m.plan.TerraformVersion,
		ResourceChanges:  []ResourceChange{},
	}
	
	// Collect all actual changes (already filtered for no-ops)
	for _, provider := range m.providers {
		for _, service := range provider.services {
			planExport.ResourceChanges = append(planExport.ResourceChanges, service.resources...)
		}
	}
	
	// Set summary
	planExport.Summary.Total = m.stats.totalChanges
	planExport.Summary.Create = m.stats.byAction["create"]
	planExport.Summary.Update = m.stats.byAction["update"]
	planExport.Summary.Delete = m.stats.byAction["delete"]
	planExport.Summary.Replace = m.stats.byAction["replace"]
	
	// Convert to JSON
	jsonData, err := json.MarshalIndent(planExport, "", "  ")
	if err != nil {
		return
	}
	
	// Copy to clipboard
	err = clipboard.WriteAll(string(jsonData))
	if err == nil {
		// Show success message (would be better with a toast notification)
		// For now, we can't show in TUI mode, so this won't be visible
	}
}

func (m *modernPlanModel) askAIToExplainCmd() tea.Cmd {
	return func() tea.Msg {
		// Get only the changes (not no-ops)
		changes := []ResourceChange{}
		for _, change := range m.plan.ResourceChanges {
			if len(change.Change.Actions) > 0 && change.Change.Actions[0] != "no-op" && change.Change.Actions[0] != "read" {
				changes = append(changes, change)
			}
		}

		// Prepare the data for Anthropic
		planData := struct {
			TerraformVersion string           `json:"terraform_version"`
			ResourceChanges  []ResourceChange `json:"resource_changes"`
			Summary          struct {
				Total   int `json:"total"`
				Create  int `json:"create"`
				Update  int `json:"update"`
				Delete  int `json:"delete"`
				Replace int `json:"replace"`
			} `json:"summary"`
		}{
			TerraformVersion: m.plan.TerraformVersion,
			ResourceChanges:  changes,
		}

		// Calculate summary
		for _, change := range changes {
			if len(change.Change.Actions) > 0 {
				switch change.Change.Actions[0] {
				case "create":
					planData.Summary.Create++
				case "update":
					planData.Summary.Update++
				case "delete":
					planData.Summary.Delete++
				case "replace":
					planData.Summary.Replace++
				}
			}
		}
		planData.Summary.Total = len(changes)

		// Call Anthropic API - this will handle its own output
		err := callAnthropicForVisualizationWithProgress(planData)
		if err != nil {
			return aiErrorMsg{err: err}
		}
		return aiSuccessMsg{}
	}
}

func (m *modernPlanModel) showImportHelp(resource *ResourceChange) {
	// Generate import ID based on resource type
	importID := m.getImportIDHint(resource.Type, resource.Address)
	
	// For certain resources, try to get the name from the resource configuration
	if resource.Change.After != nil {
		switch resource.Type {
		case "aws_iam_role":
			if nameValue, exists := resource.Change.After["name"]; exists {
				if name, ok := nameValue.(string); ok && name != "" {
					importID = name
				}
			}
		case "aws_ecr_repository":
			if nameValue, exists := resource.Change.After["name"]; exists {
				if name, ok := nameValue.(string); ok && name != "" {
					importID = name
				}
			}
		case "aws_ssm_parameter":
			if nameValue, exists := resource.Change.After["name"]; exists {
				if name, ok := nameValue.(string); ok && name != "" {
					importID = name
				}
			}
		case "aws_ecs_service":
			// ECS services need cluster/service format
			var clusterName, serviceName string
			if clusterValue, exists := resource.Change.After["cluster"]; exists {
				if c, ok := clusterValue.(string); ok && c != "" {
					clusterName = c
				}
			}
			if nameValue, exists := resource.Change.After["name"]; exists {
				if n, ok := nameValue.(string); ok && n != "" {
					serviceName = n
				}
			}
			if clusterName != "" && serviceName != "" {
				importID = fmt.Sprintf("%s/%s", clusterName, serviceName)
			}
		}
	}
	
	// Clear screen
	fmt.Print("\033[H\033[2J")
	
	// Check if this resource requires runtime IDs that can't be predicted
	if importID == "@REQUIRES_RUNTIME_ID@" {
		fmt.Printf("\n⚠️  %s Cannot Be Imported\n", resource.Type)
		fmt.Printf("Resource: %s\n\n", resource.Address)
		fmt.Printf("This resource type requires IDs that don't exist until the parent resource is created.\n\n")
		
		// Provide specific guidance based on resource type
		switch resource.Type {
		case "aws_apigatewayv2_integration", "aws_apigatewayv2_route":
			fmt.Printf("Recommendation: Let Terraform create this resource naturally.\n")
			fmt.Printf("The API Gateway API must exist first before integrations/routes can be added.\n\n")
			fmt.Printf("If the parent API already exists, you can find its ID with:\n")
			fmt.Printf("  aws apigatewayv2 get-apis --query \"Items[?Name=='<api-name>'].ApiId\"\n")
		default:
			fmt.Printf("This resource will be created automatically once its dependencies exist.\n")
		}
		
		fmt.Printf("\nReturning to plan view in 2 seconds...")
		time.Sleep(2 * time.Second)
		return
	}
	
	// Check if this resource requires looking up the ID
	if importID == "@REQUIRES_LOOKUP@" {
		// Extract name from address
		parts := strings.Split(resource.Address, ".")
		name := parts[len(parts)-1]
		if strings.Contains(name, "[") {
			name = strings.TrimSuffix(strings.Split(name, "[")[1], "]")
			name = strings.Trim(name, `"'`)
		}
		
		fmt.Printf("\n🔍 Looking up %s ID\n", resource.Type)
		fmt.Printf("Resource: %s\n", resource.Address)
		
		// Perform automatic lookup based on resource type
		switch resource.Type {
		case "aws_security_group":
			// First try to get the security group name from configuration
			var sgName string
			if resource.Change.After != nil {
				if nameValue, exists := resource.Change.After["name"]; exists {
					if n, ok := nameValue.(string); ok && n != "" {
						sgName = n
					}
				}
			}
			
			// If we don't have a name from config, extract from address
			if sgName == "" {
				sgName = name
			}
			
			fmt.Printf("Looking up security group ID for name: %s\n\n", sgName)
			
			// Look up security group by name
			lookupCmd := exec.Command("aws", "ec2", "describe-security-groups",
				"--filters", fmt.Sprintf("Name=group-name,Values=%s", sgName),
				"--query", "SecurityGroups[0].GroupId",
				"--output", "text")
			
			output, err := lookupCmd.Output()
			sgID := strings.TrimSpace(string(output))
			
			if err != nil || sgID == "" || sgID == "None" {
				fmt.Printf("❌ Could not find security group with name: %s\n", sgName)
				fmt.Printf("\nYou'll need to find the security group ID manually and run:\n")
				fmt.Printf("  terraform import %s <sg-xxxxx>\n", resource.Address)
				fmt.Print("\nReturning to plan view in 3 seconds...")
				time.Sleep(3 * time.Second)
				return
			}
			
			fmt.Printf("✅ Found security group ID: %s\n", sgID)
			importID = sgID
			
		case "aws_iam_policy":
			// First try to get the policy name from configuration
			var policyName string
			if resource.Change.After != nil {
				if nameValue, exists := resource.Change.After["name"]; exists {
					if n, ok := nameValue.(string); ok && n != "" {
						policyName = n
					}
				}
			}
			
			// If we don't have a name from config, extract from address
			if policyName == "" {
				policyName = name
			}
			
			fmt.Printf("Looking up IAM policy ARN for name: %s\n\n", policyName)
			
			// Get current AWS account ID
			accountCmd := exec.Command("aws", "sts", "get-caller-identity", "--query", "Account", "--output", "text")
			accountOutput, err := accountCmd.Output()
			if err != nil {
				fmt.Printf("❌ Could not get AWS account ID: %v\n", err)
				fmt.Print("\nReturning to plan view in 3 seconds...")
				time.Sleep(3 * time.Second)
				return
			}
			accountID := strings.TrimSpace(string(accountOutput))
			
			// Construct the ARN
			policyARN := fmt.Sprintf("arn:aws:iam::%s:policy/%s", accountID, policyName)
			
			// Verify the policy exists
			checkCmd := exec.Command("aws", "iam", "get-policy", "--policy-arn", policyARN)
			_, err = checkCmd.Output()
			
			if err != nil {
				fmt.Printf("❌ Could not find IAM policy with ARN: %s\n", policyARN)
				fmt.Printf("\nYou'll need to find the policy ARN manually and run:\n")
				fmt.Printf("  terraform import %s <policy-arn>\n", resource.Address)
				fmt.Print("\nReturning to plan view in 3 seconds...")
				time.Sleep(3 * time.Second)
				return
			}
			
			fmt.Printf("✅ Found IAM policy ARN: %s\n", policyARN)
			importID = policyARN
			
		case "aws_iam_role_policy_attachment":
			// Role policy attachments need role name and policy ARN
			// Extract both from the configuration
			var roleName, policyArn string
			
			if resource.Change.After != nil {
				if roleValue, exists := resource.Change.After["role"]; exists {
					if r, ok := roleValue.(string); ok {
						roleName = r
					}
				}
				if policyValue, exists := resource.Change.After["policy_arn"]; exists {
					if p, ok := policyValue.(string); ok {
						policyArn = p
					}
				}
			}
			
			if roleName == "" || policyArn == "" {
				fmt.Printf("❌ Could not determine role name or policy ARN from configuration\n")
				fmt.Printf("\nRole policy attachments need to be imported as: role-name/policy-arn\n")
				fmt.Printf("\nYou'll need to run:\n")
				fmt.Printf("  terraform import %s <role-name>/<policy-arn>\n", resource.Address)
				fmt.Print("\nReturning to plan view in 3 seconds...")
				time.Sleep(3 * time.Second)
				return
			}
			
			// Construct the import ID
			importID = fmt.Sprintf("%s/%s", roleName, policyArn)
			fmt.Printf("Using import ID: %s\n", importID)
			
		case "aws_service_discovery_service":
			// Get service name from configuration
			var serviceName string
			if resource.Change.After != nil {
				if nameValue, exists := resource.Change.After["name"]; exists {
					if n, ok := nameValue.(string); ok && n != "" {
						serviceName = n
					}
				}
			}
			
			// If we don't have a name from config, extract from address
			if serviceName == "" {
				serviceName = name
			}
			
			fmt.Printf("Looking up Service Discovery service ID for name: %s\n\n", serviceName)
			
			// List all namespaces to find the service
			// First, let's try the default namespace pattern
			namespaceCmd := exec.Command("aws", "servicediscovery", "list-namespaces",
				"--query", "Namespaces[?contains(Name, 'moreai')].Id",
				"--output", "text")
			
			namespaceOutput, err := namespaceCmd.Output()
			if err != nil {
				fmt.Printf("❌ Could not list Service Discovery namespaces: %v\n", err)
				fmt.Print("\nReturning to plan view in 3 seconds...")
				time.Sleep(3 * time.Second)
				return
			}
			
			namespaceIDs := strings.Fields(strings.TrimSpace(string(namespaceOutput)))
			var serviceID string
			
			// Search for the service in each namespace
			for _, nsID := range namespaceIDs {
				listCmd := exec.Command("aws", "servicediscovery", "list-services",
					"--filters", fmt.Sprintf("Name=NAMESPACE_ID,Values=%s", nsID),
					"--query", fmt.Sprintf("Services[?Name=='%s'].Id", serviceName),
					"--output", "text")
				
				output, err := listCmd.Output()
				if err == nil {
					sid := strings.TrimSpace(string(output))
					if sid != "" && sid != "None" {
						serviceID = sid
						break
					}
				}
			}
			
			if serviceID == "" {
				fmt.Printf("❌ Could not find Service Discovery service with name: %s\n", serviceName)
				fmt.Printf("\nYou'll need to find the service ID manually and run:\n")
				fmt.Printf("  terraform import %s <service-id>\n", resource.Address)
				fmt.Print("\nReturning to plan view in 3 seconds...")
				time.Sleep(3 * time.Second)
				return
			}
			
			fmt.Printf("✅ Found Service Discovery service ID: %s\n", serviceID)
			importID = serviceID
			
		case "aws_lb_target_group":
			fmt.Printf("Searching for target group...\n\n")
			
			// Look up target group ARN by name
			lookupCmd := exec.Command("aws", "elbv2", "describe-target-groups",
				"--names", fmt.Sprintf("moreai-dev-%s-tg", name),
				"--query", "TargetGroups[0].TargetGroupArn",
				"--output", "text")
			
			output, err := lookupCmd.Output()
			tgARN := strings.TrimSpace(string(output))
			
			if err != nil || tgARN == "" || tgARN == "None" {
				fmt.Printf("❌ Could not find target group with name: moreai-dev-%s-tg\n", name)
				fmt.Printf("\nYou'll need to find the target group ARN manually and run:\n")
				fmt.Printf("  terraform import %s <target-group-arn>\n", resource.Address)
				fmt.Print("\nPress ENTER to continue...")
				bufio.NewReader(os.Stdin).ReadBytes('\n')
				return
			}
			
			// Found the ARN, proceed with import
			fmt.Printf("✅ Found target group ARN: %s\n", tgARN)
			importID = tgARN
		}
		
		// Clear the lookup message before proceeding to import
		fmt.Print("\033[H\033[2J")
	}
	
	// Check if we need user input (legacy placeholder format)
	needsUserInput := strings.Contains(importID, "<") && strings.Contains(importID, ">")
	
	// If we need user input, ask for it
	if needsUserInput {
		fmt.Printf("\n🔍 Import %s\n", resource.Type)
		fmt.Printf("Resource: %s\n", resource.Address)
		fmt.Printf("\nEnter the resource ID (%s): ", importID)
		
		reader := bufio.NewReader(os.Stdin)
		userInput, _ := reader.ReadString('\n')
		userInput = strings.TrimSpace(userInput)
		
		if userInput == "" {
			// User cancelled, return immediately
			return
		}
		
		importID = userInput
		// Clear screen again after input
		fmt.Print("\033[H\033[2J")
	}
	
	// Show importing progress immediately
	fmt.Printf("\n⏳ Importing %s\n", resource.Address)
	fmt.Printf("   ID: %s\n\n", importID)
	
	// Execute the import command with real-time output
	cmd := exec.Command("terraform", "import", resource.Address, importID)
	
	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("❌ Failed to create stdout pipe: %v\n", err)
		fmt.Print("\nReturning to plan view in 2 seconds...")
		time.Sleep(2 * time.Second)
		return
	}
	
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("❌ Failed to create stderr pipe: %v\n", err)
		fmt.Print("\nReturning to plan view in 2 seconds...")
		time.Sleep(2 * time.Second)
		return
	}
	
	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Printf("❌ Failed to start import: %v\n", err)
		fmt.Print("\nReturning to plan view in 2 seconds...")
		time.Sleep(2 * time.Second)
		return
	}
	
	// Read output in real-time
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()
	
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()
	
	// Wait for command to complete
	err = cmd.Wait()
	
	fmt.Println() // Add a blank line
	
	if err != nil {
		fmt.Printf("❌ Import failed: %v\n", err)
		fmt.Print("\nReturning to plan view in 3 seconds...")
		time.Sleep(3 * time.Second)
	} else {
		fmt.Println("✅ Import successful!")
		fmt.Print("\nReturning to plan view in 2 seconds...")
		time.Sleep(2 * time.Second)
	}
}

func (m *modernPlanModel) getImportIDHint(resourceType, address string) string {
	// Extract name from address if possible
	parts := strings.Split(address, ".")
	name := parts[len(parts)-1]
	if strings.Contains(name, "[") {
		// Handle indexed resources like services["aichat"]
		name = strings.TrimSuffix(strings.Split(name, "[")[1], "]")
		name = strings.Trim(name, `"'`)
	}
	
	// Provide hints based on resource type
	switch resourceType {
	case "aws_cloudwatch_log_group":
		// CloudWatch log groups typically follow a pattern
		if strings.Contains(address, "service") && name != "" {
			// Use the exact pattern from the error message
			return fmt.Sprintf("moreai_service_%s_dev", name)
		}
		return "<log-group-name>"
	case "aws_security_group":
		// Security groups must be imported by ID (sg-xxxxx)
		// We need to look up the ID from the name
		return "@REQUIRES_LOOKUP@"
	case "aws_iam_role":
		// IAM roles are imported by their name directly
		return "<role-name>"
	case "aws_iam_policy":
		// IAM policies need to be imported by ARN
		// We need to look up the ARN from the name
		return "@REQUIRES_LOOKUP@"
	case "aws_s3_bucket":
		return name
	case "aws_ecr_repository":
		// ECR repositories are imported by their name only
		return name
	case "aws_service_discovery_service":
		// Service discovery services need their service ID
		// We need to look it up
		return "@REQUIRES_LOOKUP@"
	case "aws_apigatewayv2_integration":
		// API Gateway integrations need API ID and integration ID
		// These cannot be predicted from names alone
		// Return a special marker that will trigger different handling
		return "@REQUIRES_RUNTIME_ID@"
	case "aws_apigatewayv2_route":
		// Routes need API ID and route ID
		// These cannot be predicted from names alone
		return "@REQUIRES_RUNTIME_ID@"
	case "aws_apigatewayv2_api":
		// API Gateway APIs use name
		if name != "" {
			return name
		}
		return "<api-name>"
	case "aws_ecs_service":
		// ECS services use cluster-name/service-name
		if name != "" {
			return fmt.Sprintf("moreai-dev-cluster/%s", name)
		}
		return "<cluster-name>/<service-name>"
	case "aws_lb_target_group":
		// Target groups need ARN for import, not name
		return "@REQUIRES_LOOKUP@"
	case "aws_lb", "aws_alb":
		// Load balancers use name
		if name != "" {
			return name
		}
		return "<load-balancer-name>"
	case "aws_cloudwatch_log_stream":
		// Log streams need log group name and stream name
		return "<log-group-name>/<log-stream-name>"
	case "aws_route53_record":
		// Route53 records need zone ID and record name
		return "<zone-id>/<record-name>/<record-type>"
	case "aws_acm_certificate":
		// Certificates can be imported by domain name
		if name != "" {
			return name
		}
		return "<domain-name>"
	case "aws_iam_role_policy_attachment":
		// Role policy attachments need special handling
		// Import format is: role-name/policy-arn
		return "@REQUIRES_LOOKUP@"
	case "aws_ssm_parameter":
		// SSM parameters need their full path
		// For service parameters, construct the full path
		if strings.Contains(address, "services_env[") && name != "" {
			// Assuming pattern like /dev/moreai/aichat/...
			return fmt.Sprintf("/dev/moreai/%s", name)
		}
		return "<parameter-path>"
	default:
		// For any unhandled resource, try to use the name
		if name != "" {
			return name
		}
		return "<resource-name>"
	}
}


func (m *modernPlanModel) renderApplyView() string {
	if m.applyState == nil {
		return "Initializing apply..."
	}
	
	// Calculate elapsed time
	elapsed := time.Since(m.applyState.startTime)
	elapsedStr := fmt.Sprintf("%02d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	
	// Header
	header := m.renderApplyHeader(elapsedStr)

	// Build sections conditionally based on calculated heights
	var sections []string

	// Overall progress (only if height > 0)
	if m.applyState.progressHeight > 0 {
		sections = append(sections, m.renderApplyOverallProgress())
	}

	// Current operation (only if height > 0)
	if m.applyState.currentOpHeight > 0 {
		sections = append(sections, m.renderApplyCurrentOperation())
	}

	// Error summary (only if height > 0)
	if m.applyState.errorSummaryHeight > 0 {
		sections = append(sections, m.renderApplyErrorSummary())
	}

	// Columns (always show, but with calculated height)
	sections = append(sections, m.renderApplyColumns())

	// Logs (always show, but with calculated height)
	sections = append(sections, m.renderApplyLogs())

	// Join all visible sections
	mainContent := lipgloss.JoinVertical(lipgloss.Left, sections...)
	
	// Footer
	footer := m.renderApplyFooter()
	
	// Compose everything
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		mainContent,
		footer,
	)
}

func (m *modernPlanModel) renderApplyHeader(elapsed string) string {
	title := "🚧 Applying Changes..."
	if m.applyState.applyComplete {
		if m.applyState.hasErrors {
			title = "❌ Apply Failed"
		} else {
			title = "✅ Apply Complete"
		}
	}
	
	// Build status indicators
	var status []string
	if m.applyState.errorCount > 0 {
		status = append(status, deleteIconStyle.Render(fmt.Sprintf("❌ %d errors", m.applyState.errorCount)))
	}
	if m.applyState.warningCount > 0 {
		status = append(status, updateIconStyle.Render(fmt.Sprintf("⚠️  %d warnings", m.applyState.warningCount)))
	}
	status = append(status, fmt.Sprintf("⏱ %s", elapsed))
	
	right := strings.Join(status, "  ")
	
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(right) - 4
	if gap < 0 {
		gap = 0
	}
	
	return headerStyle.Width(m.width).Render(
		title + strings.Repeat(" ", gap) + right,
	)
}

func (m *modernPlanModel) renderApplyOverallProgress() string {
	if m.applyState.totalResources == 0 {
		return ""
	}

	completed := len(m.applyState.completed)
	percent := float64(completed) / float64(m.applyState.totalResources)

	// If deployment has failed and stopped, consider it 100% complete (failed state)
	statusText := "Overall Progress"
	if m.applyState.hasErrors && !m.applyState.isApplying {
		percent = 1.0
		statusText = "Deployment Failed"
	}

	progressBar := m.progress.ViewAs(percent)
	stats := fmt.Sprintf("%d/%d", completed, m.applyState.totalResources)

	progressLine := fmt.Sprintf("%s: %s %s (%d%%)",
		statusText, progressBar, stats, int(percent*100))

	return boxStyle.Width(m.width - 4).Render(progressLine)
}

func (m *modernPlanModel) renderApplyCurrentOperation() string {
	// Convert screen height to content height (subtract 2 for borders)
	screenHeight := 8 // default fallback
	if m.applyState.currentOpHeight > 0 {
		screenHeight = m.applyState.currentOpHeight
	}
	contentHeight := screenHeight - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	if m.applyState.currentOp == nil {
		// Show empty state with fixed height
		box := boxStyle.Copy().
			BorderForeground(dimColor).
			Width(m.width - 4).
			Height(contentHeight)
		return box.Render(titleStyle.Render("Currently Updating") + "\n" + dimStyle.Render("No active operations"))
	}

	op := m.applyState.currentOp
	icon := "🔄"
	actionStyle := dimStyle

	switch op.Action {
	case "create":
		actionStyle = createIconStyle
		icon = "✚"
	case "update":
		actionStyle = updateIconStyle
		icon = "~"
	case "delete":
		actionStyle = deleteIconStyle
		icon = "✗"
	}
	
	// Progress bar for current operation
	opProgress := ""
	elapsedDisplay := ""
	
	if op.Progress > 0 {
		opProgress = m.progress.ViewAs(op.Progress)
	} else {
		// Show infinite progress animation using animation frame
		totalBars := 20
		windowSize := 5
		// Use animation frame for smooth animation
		position := (m.applyState.animationFrame / 2) % (totalBars + windowSize)
		
		bar := ""
		for i := 0; i < totalBars; i++ {
			if i >= position-windowSize && i < position {
				bar += "█"
			} else {
				bar += "░"
			}
		}
		opProgress = bar
		
		// Use elapsed time from Terraform if available, otherwise calculate
		if op.ElapsedTime != "" {
			elapsedDisplay = fmt.Sprintf(" [%s elapsed]", op.ElapsedTime)
		} else {
			elapsed := time.Since(op.StartTime)
			elapsedDisplay = fmt.Sprintf(" [%ds elapsed]", int(elapsed.Seconds()))
		}
	}
	
	content := fmt.Sprintf(
		"%s %s %s\n%s%s\n\nStatus: %s",
		icon,
		actionStyle.Render(strings.Title(op.Action)+"ing"),
		op.Address,
		opProgress,
		elapsedDisplay,
		op.Status,
	)

	box := boxStyle.Copy().
		BorderForeground(primaryColor).
		Width(m.width - 4).
		Height(contentHeight)

	return box.Render(titleStyle.Render("Currently Updating") + "\n" + content)
}

func (m *modernPlanModel) renderApplyColumns() string {
	// Calculate width for each column
	// We want to use the full available width
	// Each box will get half the width minus the gap between them
	gap := 2
	halfWidth := (m.width - gap) / 2
	
	// Completed column
	completedBox := m.renderApplyCompleted(halfWidth)
	
	// Pending column
	pendingBox := m.renderApplyPending(halfWidth)
	
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		completedBox,
		"  ",
		pendingBox,
	)
}

func (m *modernPlanModel) renderApplyCompleted(width int) string {
	content := titleStyle.Render("Completed") + "\n"
	
	if len(m.applyState.completed) == 0 {
		content += dimStyle.Render("No resources completed yet")
	} else {
		// Show last 6 completed resources (to fit in box)
		displayCount := 6
		start := 0
		if len(m.applyState.completed) > displayCount {
			start = len(m.applyState.completed) - displayCount
			content += dimStyle.Render(fmt.Sprintf("↑ %d earlier completed", start)) + "\n"
		}
		
		for _, res := range m.applyState.completed[start:] {
			icon := "✅"
			actionStyle := dimStyle

			// Handle cancelled operations specially
			if res.Action == "cancelled" {
				icon = "🚫"
				actionStyle = dimStyle.Faint(true)
			} else if !res.Success {
				icon = "❌"
				actionStyle = deleteIconStyle.Bold(true)
			} else {
				switch res.Action {
				case "create":
					actionStyle = createIconStyle
				case "update":
					actionStyle = updateIconStyle
				case "delete":
					actionStyle = deleteIconStyle
				}
			}
			
			// Truncate long addresses
			addr := res.Address
			// The width passed is the outer box width (including border and padding)
			// boxStyle has: border = 2 chars (left+right), padding(0,1) = 2 chars
			// We also need space for icon + space = 2 chars
			// Total overhead = 2 + 2 + 2 = 6 characters
			maxLen := width - 6
			
			if len(addr) > maxLen && maxLen > 10 {
				// Simple character-based truncation for consistency
				addr = addr[:maxLen-3] + "..."
			}
			
			line := fmt.Sprintf("%s %s", icon, addr)

			// Simplified: Just show the icon and address, no inline errors
			// Full error details are shown in the logs with proper wrapping
			if res.Action == "cancelled" {
				content += actionStyle.Render(line) + "\n"
			} else if !res.Success {
				// For failed resources, show with error background
				errorStyle := actionStyle.Background(lipgloss.Color("#3D0000"))
				content += errorStyle.Render(line) + "\n"
			} else {
				content += actionStyle.Render(line) + "\n"
			}
		}
	}
	
	// Apply highlight if selected and use calculated height
	// Convert screen height to content height (subtract 2 for borders)
	screenHeight := 10 // default fallback
	if m.applyState.columnsHeight > 0 {
		screenHeight = m.applyState.columnsHeight
	}
	contentHeight := screenHeight - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	box := boxStyle.Width(width).Height(contentHeight)
	if m.applyState.selectedSection == 0 {
		box = box.BorderForeground(primaryColor)
	}

	return box.Render(content)
}

func (m *modernPlanModel) renderApplyPending(width int) string {
	content := titleStyle.Render("Pending") + "\n"
	
	if len(m.applyState.pending) == 0 {
		content += dimStyle.Render("No pending resources")
	} else {
		// Show next 6 pending resources (to fit in box)
		displayCount := 6
		end := displayCount
		if len(m.applyState.pending) < displayCount {
			end = len(m.applyState.pending)
		}
		
		for _, res := range m.applyState.pending[:end] {
			icon := "⏳"
			actionStyle := dimStyle
			switch res.Action {
			case "create":
				actionStyle = createIconStyle
				icon = "✚"
			case "update":
				actionStyle = updateIconStyle
				icon = "~"
			case "delete":
				actionStyle = deleteIconStyle
				icon = "✗"
			}
			
			// Truncate long addresses
			addr := res.Address
			// The width passed is the outer box width (including border and padding)
			// boxStyle has: border = 2 chars (left+right), padding(0,1) = 2 chars
			// We also need space for icon + space = 2 chars
			// Total overhead = 2 + 2 + 2 = 6 characters
			maxLen := width - 6
			
			if len(addr) > maxLen && maxLen > 10 {
				// Simple character-based truncation for consistency
				addr = addr[:maxLen-3] + "..."
			}
			
			line := fmt.Sprintf("%s %s", icon, addr)
			content += actionStyle.Render(line) + "\n"
		}
		
		if len(m.applyState.pending) > displayCount {
			content += dimStyle.Render(fmt.Sprintf("↓ %d more pending", len(m.applyState.pending)-displayCount))
		}
	}
	
	// Apply highlight if selected and use calculated height
	// Convert screen height to content height (subtract 2 for borders)
	screenHeight := 10 // default fallback
	if m.applyState.columnsHeight > 0 {
		screenHeight = m.applyState.columnsHeight
	}
	contentHeight := screenHeight - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	box := boxStyle.Width(width).Height(contentHeight)
	if m.applyState.selectedSection == 1 {
		box = box.BorderForeground(primaryColor)
	}

	return box.Render(content)
}

func (m *modernPlanModel) renderApplyErrorSummary() string {
	// Always show error summary box to maintain fixed layout
	var content string

	if m.applyState.errorCount > 0 {
		// Has errors - show error content
		content = deleteIconStyle.Bold(true).Render("⚠️  Errors Detected") + "\n"

		// Collect error summaries from failed resources (more informative)
		var errorMessages []string
		for i := len(m.applyState.completed) - 1; i >= 0 && len(errorMessages) < 2; i-- {
			res := m.applyState.completed[i]
			if !res.Success {
				// Prefer diagnostic summary over raw error message
				msg := ""
				if res.ErrorSummary != "" {
					msg = fmt.Sprintf("%s: %s", res.Address, res.ErrorSummary)
				} else if res.Error != "" {
					// Extract just the important part from error
					parts := strings.SplitN(res.Error, ":", 2)
					if len(parts) > 1 {
						msg = fmt.Sprintf("%s:%s", res.Address, parts[1])
					} else {
						msg = fmt.Sprintf("%s: %s", res.Address, res.Error)
					}
				} else {
					msg = fmt.Sprintf("%s: Operation failed", res.Address)
				}

				// Truncate if still too long
				if len(msg) > 100 {
					msg = msg[:97] + "..."
				}
				errorMessages = append(errorMessages, fmt.Sprintf("• %s", msg))
			}
		}

		// Show in reverse order (oldest first)
		for i := len(errorMessages) - 1; i >= 0; i-- {
			content += deleteIconStyle.Render(errorMessages[i]) + "\n"
		}

		if len(errorMessages) == 0 {
			content += deleteIconStyle.Render("Check logs for error details") + "\n"
		}
	} else {
		// No errors - show empty placeholder to reserve space
		content = dimStyle.Render("No errors detected") + "\n"
	}

	// Convert screen height to content height (subtract 2 for borders)
	screenHeight := 5 // default fallback
	if m.applyState.errorSummaryHeight > 0 {
		screenHeight = m.applyState.errorSummaryHeight
	}
	contentHeight := screenHeight - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	borderColor := dimColor
	if m.applyState.errorCount > 0 {
		borderColor = dangerColor
	}

	box := boxStyle.Copy().
		BorderForeground(borderColor).
		Width(m.width - 4).
		Height(contentHeight).
		Padding(0, 1)

	return box.Render(content)
}

func (m *modernPlanModel) renderApplyLogs() string {
	title := titleStyle.Render("Logs")

	// Convert screen height to content height (subtract 2 for borders)
	screenHeight := 10 // default fallback
	if m.applyState != nil && m.applyState.logsHeight > 0 {
		screenHeight = m.applyState.logsHeight
	}
	contentHeight := screenHeight - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Apply highlight if selected
	box := boxStyle.Width(m.width - 4).Height(contentHeight)
	if m.applyState != nil && m.applyState.selectedSection == 2 {
		box = box.BorderForeground(primaryColor)
	}

	return box.Render(title + "\n" + m.logViewport.View())
}

func (m *modernPlanModel) renderApplyFooter() string {
	help := ""
	
	if !m.applyState.applyComplete && m.applyState.isApplying {
		help += "[s] Stop  "
	}
	
	if m.applyState.showFullLogs {
		help += "[l] Normal View  "
	} else {
		help += "[l] Full Logs  "
	}

	help += "[Tab] Switch Section  "
	
	if m.applyState.selectedSection == 2 {
		help += "[↑↓] Scroll Logs  "
	}
	
	if m.applyState.applyComplete {
		if m.applyState.hasErrors && isAIHelperAvailable() {
			help += "[a] AI Help • "
		}
		help += "[Enter] Continue  "
	}

	help += "[Ctrl+C] Force Stop"

	return dimStyle.Render(help)
}

func (m *modernPlanModel) updateApplyLogViewport() {
	if m.applyState == nil {
		return
	}
	
	var content strings.Builder
	
	// Collect non-debug logs first if not in full log mode
	var logsToShow []logEntry
	if !m.applyState.showFullLogs {
		// Filter out debug messages
		for _, log := range m.applyState.logs {
			if log.Level != "debug" {
				logsToShow = append(logsToShow, log)
			}
		}
	} else {
		logsToShow = m.applyState.logs
	}
	
	// Ensure we show at least something if there are logs
	if len(logsToShow) == 0 && len(m.applyState.logs) > 0 {
		content.WriteString(dimStyle.Render("No non-debug logs yet. Press 'l' to show all logs.\n"))
	}

	// Show all logs - viewport handles scrolling
	for _, log := range logsToShow {

		timestamp := log.Timestamp.Format("15:04:05")

		var icon string
		var style lipgloss.Style
		var levelStr string
		switch log.Level {
		case "error":
			icon = "❌"
			style = deleteIconStyle.Bold(true)
			levelStr = style.Render("[ERROR]")
		case "warning":
			icon = "⚠️"
			style = updateIconStyle
			levelStr = style.Render("[WARN] ")
		case "info":
			icon = "ℹ️"
			style = dimStyle
			levelStr = dimStyle.Render("[INFO] ")
		case "debug":
			icon = "🔍"
			style = dimStyle.Faint(true)
			levelStr = style.Render("[DEBUG]")
		default:
			icon = "•"
			style = dimStyle
			levelStr = dimStyle.Render("[INFO] ")
		}

		// Format the log line - let viewport handle wrapping naturally
		logLine := fmt.Sprintf("%s %s %s %s", timestamp, levelStr, icon, log.Message)

		// For errors, highlight the entire line with background color
		if log.Level == "error" {
			errorStyle := style.Background(lipgloss.Color("#3D0000"))
			content.WriteString(errorStyle.Render(logLine) + "\n")
		} else {
			content.WriteString(style.Render(logLine) + "\n")
		}
	}

	m.logViewport.SetContent(content.String())

	// Auto-scroll to bottom to show latest logs (unless user has manually scrolled up)
	// Only auto-scroll if we're near the bottom or if this is a new log entry
	if m.applyState.isApplying || m.logViewport.AtBottom() {
		m.logViewport.GotoBottom()
	}
}

func (m *modernPlanModel) renderApplyDetailsView(header, elapsed string) string {
	if m.applyState == nil {
		return "No apply state available"
	}
	
	// Details content
	var content strings.Builder
	content.WriteString(titleStyle.Render("📋 Apply Details") + "\n\n")
	
	// Show current operation if any
	if m.applyState.currentOp != nil {
		op := m.applyState.currentOp
		content.WriteString(titleStyle.Render("Current Operation") + "\n")
		content.WriteString(fmt.Sprintf("Address: %s\n", op.Address))
		content.WriteString(fmt.Sprintf("Action: %s\n", op.Action))
		content.WriteString(fmt.Sprintf("Status: %s\n", op.Status))
		content.WriteString(fmt.Sprintf("Progress: %d%%\n", int(op.Progress*100)))
		content.WriteString(fmt.Sprintf("Duration: %s\n\n", time.Since(op.StartTime).Round(time.Second)))
	} else {
		content.WriteString(dimStyle.Render("No operation currently in progress\n\n"))
	}
	
	// Show recent completed operations
	content.WriteString(titleStyle.Render("Recent Operations") + "\n")
	if len(m.applyState.completed) > 0 {
		// Show last 10 completed operations
		start := 0
		if len(m.applyState.completed) > 10 {
			start = len(m.applyState.completed) - 10
		}
		
		for _, res := range m.applyState.completed[start:] {
			icon := "✅"
			if !res.Success {
				icon = "❌"
			}
			content.WriteString(fmt.Sprintf("%s %s %s - %s (%v)\n", 
				icon, 
				res.Timestamp.Format("15:04:05"),
				res.Action,
				res.Address,
				res.Duration.Round(time.Millisecond)))
			if !res.Success && res.Error != "" {
				content.WriteString(deleteIconStyle.Render(fmt.Sprintf("   └ %s\n", res.Error)))
			}
		}
	} else {
		content.WriteString(dimStyle.Render("No completed operations yet\n"))
	}
	
	// Show pending operations count
	content.WriteString("\n" + titleStyle.Render("Pending Operations") + "\n")
	content.WriteString(fmt.Sprintf("Total pending: %d\n", len(m.applyState.pending)))
	
	// Show error/warning summary
	if m.applyState.errorCount > 0 || m.applyState.warningCount > 0 {
		content.WriteString("\n" + titleStyle.Render("Summary") + "\n")
		if m.applyState.errorCount > 0 {
			content.WriteString(deleteIconStyle.Render(fmt.Sprintf("Errors: %d\n", m.applyState.errorCount)))
		}
		if m.applyState.warningCount > 0 {
			content.WriteString(updateIconStyle.Render(fmt.Sprintf("Warnings: %d\n", m.applyState.warningCount)))
		}
	}
	
	detailBox := boxStyle.Width(m.width - 2).Height(m.height - 6).Render(content.String())
	
	footer := "[d] Back to overview  [x] Show error details  [q] Quit"
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		detailBox,
		dimStyle.Render(footer),
	)
}

func groupResourceChanges(changes []ResourceChange) changeGroups {
	groups := changeGroups{}

	for _, change := range changes {
		if len(change.Change.Actions) == 0 {
			continue
		}
		
		// Skip no-op changes
		if len(change.Change.Actions) == 1 && change.Change.Actions[0] == "no-op" {
			continue
		}

		switch change.Change.Actions[0] {
		case "create":
			groups.creates = append(groups.creates, change)
		case "update":
			groups.updates = append(groups.updates, change)
		case "delete":
			groups.deletes = append(groups.deletes, change)
		case "replace":
			groups.replaces = append(groups.replaces, change)
		case "read":
			// Skip read-only operations
			continue
		}
	}

	// Sort each group by resource address
	sortChanges := func(changes []ResourceChange) {
		sort.Slice(changes, func(i, j int) bool {
			return changes[i].Address < changes[j].Address
		})
	}

	sortChanges(groups.creates)
	sortChanges(groups.updates)
	sortChanges(groups.deletes)
	sortChanges(groups.replaces)
	sortChanges(groups.reads)

	return groups
}

func calculateStatistics(groups changeGroups) changeStats {
	stats := changeStats{
		byType:   make(map[string]int),
		byAction: make(map[string]int),
	}

	countChanges := func(changes []ResourceChange, action string) {
		stats.byAction[action] = len(changes)
		for _, change := range changes {
			stats.byType[change.Type]++
			stats.totalChanges++
		}
	}

	countChanges(groups.creates, "create")
	countChanges(groups.updates, "update")
	countChanges(groups.deletes, "delete")
	countChanges(groups.replaces, "replace")
	countChanges(groups.reads, "read")

	return stats
}

func isComputedAttribute(key string) bool {
	computedAttrs := []string{"id", "arn", "created_at", "updated_at", "tags_all", "owner_id", "default_route_table_id"}
	for _, attr := range computedAttrs {
		if key == attr {
			return true
		}
	}
	return false
}

func showModernTerraformPlanTUI(planJSON string) error {
	model, err := initModernTerraformPlanTUI(planJSON)
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	// Store program reference for sending messages during apply
	if m, ok := model.(*modernPlanModel); ok {
		m.program = p
	}

	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	// No longer prompt for AI help after exiting - users can press 'a' in the TUI for AI help
	// This old prompting behavior has been replaced with in-TUI AI help view

	return nil
}

// offerAIHelpForApplyErrors offers AI assistance after apply errors
func offerAIHelpForApplyErrors(m *modernPlanModel) {
	// Collect error messages
	var errorMessages []string
	for _, res := range m.applyState.completed {
		if !res.Success && res.Error != "" {
			errorMessages = append(errorMessages, fmt.Sprintf("Resource: %s\nAction: %s\nError: %s",
				res.Address, res.Action, res.Error))
		}
	}

	if len(errorMessages) == 0 {
		return
	}

	// Get current directory
	workingDir, _ := os.Getwd()

	// Get AWS profile from environment or use default
	awsProfile := os.Getenv("AWS_PROFILE")
	if awsProfile == "" {
		awsProfile = "default"
	}

	// Get region from environment variable or use default
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = os.Getenv("AWS_DEFAULT_REGION")
	}
	if awsRegion == "" {
		awsRegion = "us-east-1" // fallback
	}

	// Extract environment from working directory (e.g., env/dev)
	envName := "unknown"
	if strings.Contains(workingDir, "/env/") {
		parts := strings.Split(workingDir, "/env/")
		if len(parts) > 1 {
			envParts := strings.Split(parts[1], "/")
			if len(envParts) > 0 {
				envName = envParts[0]
			}
		}
	}

	ctx := ErrorContext{
		Operation:   "apply",
		Environment: envName,
		AWSProfile:  awsProfile,
		AWSRegion:   awsRegion,
		Errors:      errorMessages,
		WorkingDir:  workingDir,
	}

	offerAIHelp(ctx)
}

func (m *modernPlanModel) renderApplyErrorDetailsView(header, elapsed string) string {
	// Collect all failed resources (but exclude cascading cancellations)
	var failedResources []completedResource
	for _, res := range m.applyState.completed {
		if !res.Success {
			// Skip resources that were just cancelled due to previous errors
			// These aren't real errors, just cascading failures
			isCascadingFailure := res.Action == "cancelled" ||
				strings.Contains(strings.ToLower(res.Error), "cancelled due to previous errors") ||
				strings.Contains(strings.ToLower(res.Error), "deployment cancelled")

			if !isCascadingFailure {
				failedResources = append(failedResources, res)
			}
		}
	}
	
	if len(failedResources) == 0 {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			"\n\nNo errors to display.",
			m.renderApplyFooter(),
		)
	}
	
	// Create error details content
	var content strings.Builder
	content.WriteString(titleStyle.Render("🔴 Error Details") + "\n\n")
	content.WriteString(fmt.Sprintf("Total Errors: %d\n\n", len(failedResources)))
	
	// Show each failed resource
	for i, res := range failedResources {
		// Resource header
		content.WriteString(deleteIconStyle.Bold(true).Render(fmt.Sprintf("Error %d: %s", i+1, res.Address)) + "\n")
		content.WriteString(strings.Repeat("─", m.width-10) + "\n")
		
		// Action attempted
		content.WriteString(fmt.Sprintf("Action: %s\n", res.Action))
		
		// Duration before failure
		if res.Duration > 0 {
			content.WriteString(fmt.Sprintf("Failed after: %v\n", res.Duration))
		}

		// Error Summary (if available from diagnostic)
		if res.ErrorSummary != "" {
			content.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(dangerColor).Render("Error Summary:") + "\n")
			summaryStyle := lipgloss.NewStyle().
				Foreground(dangerColor).
				PaddingLeft(2)
			maxWidth := m.width - 10
			wrapped := wordWrap(res.ErrorSummary, maxWidth)
			content.WriteString(summaryStyle.Render(wrapped) + "\n")
		}

		// Error Detail (full diagnostic detail)
		if res.ErrorDetail != "" {
			content.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(dangerColor).Render("Detailed Error Information:") + "\n")
			detailStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6B6B")).
				PaddingLeft(2)
			maxWidth := m.width - 10
			wrapped := wordWrap(res.ErrorDetail, maxWidth)
			content.WriteString(detailStyle.Render(wrapped) + "\n")
		}

		// Raw error message (fallback if no diagnostic available)
		if res.ErrorSummary == "" && res.ErrorDetail == "" {
			content.WriteString("\nError Message:\n")
			errorStyle := lipgloss.NewStyle().
				Foreground(dangerColor).
				PaddingLeft(2)

			// Wrap long error messages
			errorMsg := res.Error
			if errorMsg == "" {
				errorMsg = "No error message available"
			}

			// Simple word wrapping
			maxWidth := m.width - 10
			wrapped := wordWrap(errorMsg, maxWidth)
			content.WriteString(errorStyle.Render(wrapped) + "\n")
		}
		
		// Add spacing between errors
		if i < len(failedResources)-1 {
			content.WriteString("\n")
		}
	}
	
	// Create scrollable viewport for error details
	vp := viewport.New(m.width-4, m.height-10)
	vp.SetContent(content.String())
	
	// Build the view
	box := boxStyle.Width(m.width-2).Height(m.height-8)
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		box.Render(vp.View()),
		m.renderApplyFooter(),
	)
}

// Simple word wrap helper
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	
	var result strings.Builder
	lines := strings.Split(text, "\n")
	
	for _, line := range lines {
		if len(line) <= width {
			result.WriteString(line + "\n")
			continue
		}
		
		// Wrap long lines
		for len(line) > width {
			// Find last space before width
			cutPoint := width
			for i := width - 1; i >= 0; i-- {
				if line[i] == ' ' {
					cutPoint = i
					break
				}
			}
			
			// If no space found, just cut at width
			if cutPoint == width {
				result.WriteString(line[:width] + "\n")
				line = line[width:]
			} else {
				result.WriteString(line[:cutPoint] + "\n")
				line = line[cutPoint+1:] // Skip the space
			}
		}
		
		if len(line) > 0 {
			result.WriteString(line + "\n")
		}
	}
	
	return strings.TrimSuffix(result.String(), "\n")
}

// tickCmd returns a command that sends a tick message after a short delay
func (m *modernPlanModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return applyTickMsg{}
	})
}

// renderAILoading shows a loading indicator while AI is processing
func (m *modernPlanModel) renderAILoading() string {
	loadingStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Padding(2, 4)

	content := "🤖 Asking AI to explain your infrastructure changes...\n\n"
	content += "This may take a moment. The browser will open automatically when ready.\n\n"
	content += "Press Ctrl+C to cancel"

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		loadingStyle.Render(content),
	)
}

// renderAIError shows an error message overlay
func (m *modernPlanModel) renderAIError() string {
	// Create a full-screen background overlay
	backgroundStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("0")).
		Width(m.width).
		Height(m.height)

	errorBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Background(lipgloss.Color("235")).
		Padding(1, 2).
		Width(m.width - 10)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Padding(0, 0, 1, 0)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("251")).
		Padding(0, 0, 1, 0)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)

	content := titleStyle.Render("❌ AI Visualization Error") + "\n\n"
	content += errorStyle.Render(m.aiError) + "\n\n"
	content += helpStyle.Render("Press any key to continue...")

	errorBox := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		errorBoxStyle.Render(content),
	)

	// Render the background with the error box on top
	return backgroundStyle.Render(errorBox)
}
// fetchAIHelpAndShowView fetches AI suggestions and switches to AI help view
func (m *modernPlanModel) fetchAIHelp() tea.Cmd {
	return func() tea.Msg {
		// Collect error messages from completed resources
		var errorMessages []string
		for _, res := range m.applyState.completed {
			if !res.Success {
				// Build a complete error message using all available error information
				var errorParts []string
				errorParts = append(errorParts, fmt.Sprintf("Resource: %s", res.Address))
				errorParts = append(errorParts, fmt.Sprintf("Action: %s", res.Action))

				// Use ErrorDetail if available (has the full AWS error), otherwise fallback to ErrorSummary, then Error
				if res.ErrorDetail != "" {
					errorParts = append(errorParts, fmt.Sprintf("Error: %s", res.ErrorDetail))
				} else if res.ErrorSummary != "" {
					errorParts = append(errorParts, fmt.Sprintf("Error: %s", res.ErrorSummary))
				} else if res.Error != "" {
					errorParts = append(errorParts, fmt.Sprintf("Error: %s", res.Error))
				}

				errorMessages = append(errorMessages, strings.Join(errorParts, "\n"))
			}
		}

		if len(errorMessages) == 0 {
			return aiHelpErrorMsg{err: fmt.Errorf("no errors to analyze")}
		}

		// Get current directory
		workingDir, _ := os.Getwd()

		// Get AWS profile from environment or use default
		awsProfile := os.Getenv("AWS_PROFILE")
		if awsProfile == "" {
			awsProfile = "default"
		}

		// Get region from environment variable or use default
		awsRegion := os.Getenv("AWS_REGION")
		if awsRegion == "" {
			awsRegion = os.Getenv("AWS_DEFAULT_REGION")
		}
		if awsRegion == "" {
			awsRegion = "us-east-1" // fallback
		}

		// Extract environment from working directory (e.g., env/dev)
		envName := "unknown"
		if strings.Contains(workingDir, "/env/") {
			parts := strings.Split(workingDir, "/env/")
			if len(parts) > 1 {
				envParts := strings.Split(parts[1], "/")
				if len(envParts) > 0 {
					envName = envParts[0]
				}
			}
		}

		ctx := ErrorContext{
			Operation:   "apply",
			Environment: envName,
			AWSProfile:  awsProfile,
			AWSRegion:   awsRegion,
			Errors:      errorMessages,
			WorkingDir:  workingDir,
		}

		// Get AI suggestions
		problem, commands, err := getAIErrorSuggestions(ctx)
		if err != nil {
			// Return error message
			return aiHelpErrorMsg{err: err}
		}

		// Return success message with data
		return aiHelpReadyMsg{
			problem:  problem,
			commands: commands,
			errors:   errorMessages,
		}
	}
}

// buildAIHelpContent builds the content string for the AI help viewport
func (m *modernPlanModel) buildAIHelpContent() string {
	var content strings.Builder
	divider := strings.Repeat("─", m.width-8)

	// Original Error Section
	content.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		Render("❌ Original Error"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n\n")

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	for _, err := range m.aiHelpErrors {
		content.WriteString(errorStyle.Render(err))
		content.WriteString("\n\n")
	}

	// AI Analysis Section
	content.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true).
		Render("💡 AI Analysis"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n\n")

	problemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	content.WriteString(problemStyle.Render(m.aiHelpProblem))
	content.WriteString("\n\n")

	// Suggested Fix Section
	content.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true).
		Render("📋 Suggested Fix"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n\n")
	content.WriteString("Run these commands:\n\n")

	commandStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))

	for _, cmd := range m.aiHelpCommands {
		content.WriteString(commandStyle.Render("  " + cmd))
		content.WriteString("\n")
	}

	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n\n")

	disclaimerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)
	content.WriteString(disclaimerStyle.Render("⚠️  AI-generated suggestions - please review before running"))

	return content.String()
}

// renderAIHelpView renders the AI help view with original error, AI analysis, and suggested fixes
func (m *modernPlanModel) renderAIHelpView() string {
	// Header
	header := headerStyle.Width(m.width).Render("🤖 AI Error Help")

	// Show loading state if still fetching
	if m.aiHelpLoading {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[int(time.Now().UnixMilli()/100)%len(spinnerFrames)]

		loadingStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Align(lipgloss.Center, lipgloss.Center)

		loadingContent := fmt.Sprintf("%s Analyzing errors with AI...\n\nPlease wait while Claude analyzes the error and suggests fixes.", frame)

		loadingBox := boxStyle.
			Width(m.width - 2).
			Height(m.height - 4).
			Render(loadingStyle.Render(loadingContent))

		footerText := dimStyle.Render("[ESC] Cancel")

		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			loadingBox,
			footerText,
		)
	}

	// Viewport with AI help content
	viewportBox := boxStyle.
		Width(m.width - 2).
		Height(m.height - 4).
		Render(m.aiHelpViewport.View())

	// Footer with help text
	footerText := dimStyle.Render("[↑↓] Scroll • [ESC] Back to Apply View")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		viewportBox,
		footerText,
	)
}
