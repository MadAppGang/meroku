package main

import (
	"encoding/json"
	"fmt"
	"os"
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

const AppVersion = "v2.2.5"

type viewMode int

const (
	dashboardView viewMode = iota
	applyView
)

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
	width          int
	height         int
	help           help.Model
	keys           modernKeyMap
	showHelp       bool
	progress       progress.Model
	applyProgress  float64
	applyState     *applyState
	program        *tea.Program
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
}

func (k modernKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Enter, k.Copy, k.Apply, k.Quit}
}

func (k modernKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Tab, k.Enter, k.Space},
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
		Background(lipgloss.Color("#1f2937")).
		Bold(true)
		
	createIconStyle = lipgloss.NewStyle().Foreground(successColor)
	updateIconStyle = lipgloss.NewStyle().Foreground(warningColor)
	deleteIconStyle = lipgloss.NewStyle().Foreground(dangerColor)
	
	labelStyle = lipgloss.NewStyle().
		Foreground(mutedColor).
		Width(15)
		
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
		providerMap[provider][service] = append(providerMap[provider][service], change)
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
		help:           help.New(),
		keys:           modernKeys,
		progress:       prog,
	}, nil
}

func (m *modernPlanModel) Init() tea.Cmd {
	m.updateTreeViewport()
	m.updateDetailViewport()
	return nil
}

func (m *modernPlanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
			contentHeight := m.height - 10 // Header + footer
			
			m.treeViewport.Width = treeWidth
			m.treeViewport.Height = contentHeight - 10
			
			m.detailViewport.Width = detailWidth
			m.detailViewport.Height = contentHeight - 10
			
			
		case applyView:
			m.logViewport.Width = m.width - 4
			m.logViewport.Height = m.height / 3
		}
		
		m.updateTreeViewport()
		m.updateDetailViewport()
		
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
			
		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			
		case key.Matches(msg, m.keys.Back):
			if m.currentView != dashboardView {
				m.currentView = dashboardView
			}
			
		case key.Matches(msg, m.keys.AskAI):
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				m.askAIToExplain()
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
			
		case key.Matches(msg, m.keys.Copy):
			if m.currentView == dashboardView {
				m.copyChangesToClipboard()
			}
			
		case key.Matches(msg, m.keys.Apply):
			m.currentView = applyView
			m.initApplyState()
			return m, m.startTerraformApply()
			
		case key.Matches(msg, m.keys.Tab):
			// Tab navigation logic
			
		case key.Matches(msg, m.keys.Space), key.Matches(msg, m.keys.Enter):
			if m.currentView == dashboardView && len(m.providers) > 0 {
				// Handle provider/service/resource hierarchy
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
			
		case key.Matches(msg, m.keys.Down):
			if m.currentView == dashboardView {
				m.navigateDown()
				m.updateTreeViewport()
				m.updateDetailViewport()
			} else if m.currentView == applyView {
				// Scroll logs down
				m.logViewport.LineDown(1)
			}
			
		case key.Matches(msg, m.keys.Up):
			if m.currentView == dashboardView {
				m.navigateUp()
				m.updateTreeViewport()
				m.updateDetailViewport()
			} else if m.currentView == applyView {
				// Scroll logs up
				m.logViewport.LineUp(1)
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
					m.logViewport.Height = m.height / 3
				}
				m.updateApplyLogViewport()
			}
			
		case msg.String() == "d":
			// Toggle details view
			if m.currentView == applyView && m.applyState != nil {
				m.applyState.showDetails = !m.applyState.showDetails
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
		}
		
	case applyCompleteMsg:
		if m.applyState != nil {
			m.applyState.isApplying = false
			m.applyState.applyComplete = true
		}
		
	case applyErrorMsg:
		if m.applyState != nil {
			m.applyState.isApplying = false
			m.applyState.hasErrors = true
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
			// Move from pending to completed
			for i, p := range m.applyState.pending {
				if p.Address == msg.Address {
					m.applyState.pending = append(m.applyState.pending[:i], m.applyState.pending[i+1:]...)
					break
				}
			}
			
			m.applyState.completed = append(m.applyState.completed, completedResource{
				Address:   msg.Address,
				Action:    m.applyState.currentOp.Action,
				Duration:  time.Since(m.applyState.currentOp.StartTime),
				Timestamp: time.Now(),
				Success:   msg.Success,
				Error:     msg.Error,
			})
			
			if !msg.Success {
				m.applyState.errorCount++
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
			} else if msg.Level == "error" {
				m.applyState.errorCount++
			}
			
			m.updateApplyLogViewport()
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
	
	switch m.currentView {
	case dashboardView:
		return m.renderDashboard()
	case applyView:
		return m.renderApplyView()
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
	help := "[↑↓] Navigate  [Space/Enter] Expand  [c] Copy  "
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		help += "[e] Ask AI  "
	}
	help += "[a] Apply  [?] Help  [q] Quit"
	
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
			b.WriteString(selectedItemStyle.Render(providerLine))
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
					b.WriteString(selectedItemStyle.Render(serviceLine))
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
						
						// Resource name
						parts := strings.Split(resource.Address, ".")
						name := parts[len(parts)-1]
						if len(parts) > 1 {
							name = parts[len(parts)-2] + "." + name
						}
						
						connector := "├"
						if k == len(service.resources)-1 {
							connector = "└"
						}
						
						resourceLine := fmt.Sprintf("    %s %s %s",
							connector,
							iconStyle.Render(icon),
							name,
						)
						
						if isResourceSelected {
							b.WriteString(selectedItemStyle.Render(resourceLine))
						} else {
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

func (m *modernPlanModel) renderResourceDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	// Resource header with icon
	icon := getResourceIcon(resource.Type)
	actionStyle := getActionStyle(resource.Change.Actions[0])
	
	b.WriteString(fmt.Sprintf("%s %s\n", icon, titleStyle.Render(resource.Address)))
	b.WriteString(strings.Repeat("─", 60) + "\n\n")
	
	// Action badge
	actionBadge := actionStyle.Padding(0, 1).Render(strings.ToUpper(resource.Change.Actions[0]))
	b.WriteString(fmt.Sprintf("%s  %s\n\n", actionBadge, typeStyle.Render(resource.Type)))
	
	// Render based on action type
	switch resource.Change.Actions[0] {
	case "create":
		b.WriteString(m.renderCreateDetails(resource))
	case "update":
		b.WriteString(m.renderUpdateDetails(resource))
	case "delete":
		b.WriteString(m.renderDeleteDetails(resource))
	case "replace":
		b.WriteString(m.renderReplaceDetails(resource))
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
	
	b.WriteString(createIconStyle.Bold(true).Render("➕ Creating new resource") + "\n\n")
	
	if resource.Change.After != nil {
		b.WriteString(m.renderAttributeTree("Configuration", resource.Change.After, createIconStyle))
	}
	
	return b.String()
}

func (m *modernPlanModel) renderUpdateDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	b.WriteString(updateIconStyle.Bold(true).Render("📝 Updating resource") + "\n\n")
	
	// Find changed attributes
	changes := m.findChangedAttributes(resource.Change.Before, resource.Change.After)
	
	if len(changes) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Changes:") + "\n\n")
		for _, change := range changes {
			b.WriteString(m.renderAttributeChange(change))
			b.WriteString("\n")
		}
	}
	
	// Show unchanged important attributes
	unchanged := m.findUnchangedImportantAttributes(resource.Type, resource.Change.Before, resource.Change.After)
	if len(unchanged) > 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(mutedColor).Bold(true).Render("Unchanged:") + "\n\n")
		for key, value := range unchanged {
			b.WriteString(fmt.Sprintf("  %s %s\n", 
				labelStyle.Render(key+":"),
				valueStyle.Render(formatValue(value))))
		}
	}
	
	return b.String()
}

func (m *modernPlanModel) renderDeleteDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	b.WriteString(deleteIconStyle.Bold(true).Render("🗑️  Deleting resource") + "\n\n")
	
	if resource.Change.Before != nil {
		b.WriteString(m.renderAttributeTree("Current Configuration", resource.Change.Before, deleteIconStyle))
	}
	
	b.WriteString("\n" + lipgloss.NewStyle().
		Background(dangerColor).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		Padding(0, 1).
		Render("⚠️  This resource will be permanently deleted"))
	
	return b.String()
}

func (m *modernPlanModel) renderReplaceDetails(resource *ResourceChange) string {
	var b strings.Builder
	
	b.WriteString(lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Render("🔄 Resource will be replaced (recreated)") + "\n\n")
	
	// Show what's forcing the replacement
	if resource.Change.Before != nil && resource.Change.After != nil {
		forceNew := m.findForceNewAttributes(resource.Change.Before, resource.Change.After)
		if len(forceNew) > 0 {
			b.WriteString(lipgloss.NewStyle().Bold(true).Render("Forces replacement:") + "\n")
			for _, attr := range forceNew {
				b.WriteString(fmt.Sprintf("  %s %s\n", 
					lipgloss.NewStyle().Foreground(dangerColor).Render("!"),
					attr))
			}
			b.WriteString("\n")
		}
	}
	
	// Show the new configuration
	if resource.Change.After != nil {
		b.WriteString(m.renderAttributeTree("New Configuration", resource.Change.After, createIconStyle))
	}
	
	return b.String()
}

func (m *modernPlanModel) renderAttributeTree(title string, attrs map[string]interface{}, style lipgloss.Style) string {
	var b strings.Builder
	
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title) + "\n")
	
	// Group and sort attributes
	important, others := m.categorizeAttributes(attrs)
	
	// Render important attributes first
	if len(important) > 0 {
		for _, key := range sortedKeys(important) {
			b.WriteString(m.renderAttributeWithStyle(key, important[key], 1, style))
		}
	}
	
	// Render other attributes
	if len(others) > 0 && len(important) > 0 {
		b.WriteString("\n")
	}
	
	for _, key := range sortedKeys(others) {
		b.WriteString(m.renderAttributeWithStyle(key, others[key], 1, lipgloss.NewStyle()))
	}
	
	return b.String()
}

func (m *modernPlanModel) renderAttributeWithStyle(key string, value interface{}, indent int, style lipgloss.Style) string {
	prefix := strings.Repeat("  ", indent)
	
	switch v := value.(type) {
	case map[string]interface{}:
		result := fmt.Sprintf("%s%s %s:\n", prefix, style.Render("▸"), labelStyle.Render(key))
		for _, k := range sortedKeys(v) {
			result += m.renderAttributeWithStyle(k, v[k], indent+1, style)
		}
		return result
		
	case []interface{}:
		if len(v) == 0 {
			return fmt.Sprintf("%s%s %s: %s\n", prefix, style.Render("•"), labelStyle.Render(key), valueStyle.Render("[]"))
		}
		result := fmt.Sprintf("%s%s %s: (%d items)\n", prefix, style.Render("▸"), labelStyle.Render(key), len(v))
		for i, item := range v {
			if i >= 3 { // Limit array display
				result += fmt.Sprintf("%s  %s\n", prefix, lipgloss.NewStyle().Foreground(mutedColor).Render("..."))
				break
			}
			result += m.renderAttributeWithStyle(fmt.Sprintf("[%d]", i), item, indent+1, style)
		}
		return result
		
	default:
		valueStr := formatValue(value)
		return fmt.Sprintf("%s%s %s: %s\n", prefix, style.Render("•"), labelStyle.Render(key), valueStyle.Render(valueStr))
	}
}

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
	if change.isNew {
		return fmt.Sprintf("  %s %s: %s",
			createIconStyle.Render("+"),
			labelStyle.Render(change.key),
			createIconStyle.Render(formatValue(change.after)))
	}
	
	if change.isRemoved {
		return fmt.Sprintf("  %s %s: %s",
			deleteIconStyle.Render("-"),
			labelStyle.Render(change.key),
			deleteIconStyle.Render(formatValue(change.before)))
	}
	
	// Changed value
	return fmt.Sprintf("  %s %s:\n    %s %s\n    %s %s",
		updateIconStyle.Render("~"),
		labelStyle.Render(change.key),
		deleteIconStyle.Render("-"),
		deleteIconStyle.Render(formatValue(change.before)),
		createIconStyle.Render("+"),
		createIconStyle.Render(formatValue(change.after)))
}

func (m *modernPlanModel) categorizeAttributes(attrs map[string]interface{}) (important, others map[string]interface{}) {
	important = make(map[string]interface{})
	others = make(map[string]interface{})
	
	importantKeys := map[string]bool{
		"name": true, "instance_type": true, "ami": true, "image": true,
		"desired_count": true, "min_size": true, "max_size": true,
		"cpu": true, "memory": true, "runtime": true, "handler": true,
		"engine": true, "engine_version": true, "instance_class": true,
		"allocated_storage": true, "bucket": true, "key": true,
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
		if len(v) > 60 {
			return v[:57] + "..."
		}
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	case []interface{}:
		return fmt.Sprintf("[%d items]", len(v))
	case map[string]interface{}:
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

func (m *modernPlanModel) askAIToExplain() {
	// Exit TUI temporarily and clear screen
	fmt.Print("\033[H\033[2J") // Clear screen
	fmt.Print("\033[0;0H")     // Move cursor to top
	
	// Get only the changes (not no-ops)
	changes := []ResourceChange{}
	for _, change := range m.plan.ResourceChanges {
		if len(change.Change.Actions) > 0 && change.Change.Actions[0] != "no-op" && change.Change.Actions[0] != "read" {
			changes = append(changes, change)
		}
	}
	
	// Debug: Print first few resources to see what we're sending
	fmt.Printf("\n📋 Debug: Found %d resources to send\n", len(changes))
	if len(changes) > 0 {
		fmt.Printf("   First resource: %s (%s) - Action: %v\n", 
			changes[0].Address, 
			changes[0].Type,
			changes[0].Change.Actions)
		fmt.Printf("     Before: %v fields, After: %v fields\n",
			len(changes[0].Change.Before),
			len(changes[0].Change.After))
		if len(changes) > 1 {
			fmt.Printf("   Second resource: %s (%s) - Action: %v\n", 
				changes[1].Address, 
				changes[1].Type,
				changes[1].Change.Actions)
			fmt.Printf("     Before: %v fields, After: %v fields\n",
				len(changes[1].Change.Before),
				len(changes[1].Change.After))
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
	
	// Call Anthropic API with loading indicator
	err := callAnthropicForVisualizationWithProgress(planData)
	if err != nil {
		fmt.Printf("\n❌ Error creating visual view: %v\n", err)
		fmt.Println("\nPress ENTER to return to the TUI...")
		fmt.Scanln()
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
	
	// If showing details view
	if m.applyState.showDetails && m.applyState.currentOp != nil {
		return m.renderApplyDetailsView(header, elapsedStr)
	}
	
	// Overall progress
	overallProgress := m.renderApplyOverallProgress()
	
	// Current operation
	currentOp := m.renderApplyCurrentOperation()
	
	// Adjust layout based on log view size
	var mainContent string
	if m.applyState.showFullLogs {
		// Show only logs when in full log mode
		mainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			overallProgress,
			currentOp,
			m.renderApplyLogs(),
		)
	} else {
		// Normal view with columns and logs
		columns := m.renderApplyColumns()
		logsSection := m.renderApplyLogs()
		
		mainContent = lipgloss.JoinVertical(
			lipgloss.Left,
			overallProgress,
			currentOp,
			columns,
			logsSection,
		)
	}
	
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
	
	right := fmt.Sprintf("⏱ %s elapsed", elapsed)
	
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
	progressBar := m.progress.ViewAs(percent)
	stats := fmt.Sprintf("%d/%d", completed, m.applyState.totalResources)
	
	progressLine := fmt.Sprintf("Overall Progress: %s %s (%d%%)", 
		progressBar, stats, int(percent*100))
	
	return boxStyle.Width(m.width - 2).Render(progressLine)
}

func (m *modernPlanModel) renderApplyCurrentOperation() string {
	if m.applyState.currentOp == nil {
		return ""
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
	if op.Progress > 0 {
		opProgress = m.progress.ViewAs(op.Progress)
	} else {
		// Show animated progress
		elapsed := time.Since(op.StartTime)
		dots := int(elapsed.Seconds()) % 10
		opProgress = strings.Repeat("█", dots) + strings.Repeat("░", 10-dots)
	}
	
	content := fmt.Sprintf(
		"%s %s %s\n%s %d%%\n\nStatus: %s",
		icon,
		actionStyle.Render(strings.Title(op.Action)+"ing"),
		op.Address,
		opProgress,
		int(op.Progress*100),
		op.Status,
	)
	
	box := boxStyle.Copy().
		BorderForeground(primaryColor).
		Width(m.width - 2)
	
	return box.Render(titleStyle.Render("Current Operation") + "\n" + content)
}

func (m *modernPlanModel) renderApplyColumns() string {
	halfWidth := (m.width - 6) / 2
	
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
		// Show last 10 completed resources
		start := 0
		if len(m.applyState.completed) > 10 {
			start = len(m.applyState.completed) - 10
		}
		
		for _, res := range m.applyState.completed[start:] {
			icon := "✅"
			if !res.Success {
				icon = "❌"
			}
			
			actionStyle := dimStyle
			switch res.Action {
			case "create":
				actionStyle = createIconStyle
			case "update":
				actionStyle = updateIconStyle
			case "delete":
				actionStyle = deleteIconStyle
			}
			
			line := fmt.Sprintf("%s %s", icon, res.Address)
			content += actionStyle.Render(line) + "\n"
		}
	}
	
	return boxStyle.Width(width).Height(12).Render(content)
}

func (m *modernPlanModel) renderApplyPending(width int) string {
	content := titleStyle.Render("Pending") + "\n"
	
	if len(m.applyState.pending) == 0 {
		content += dimStyle.Render("No pending resources")
	} else {
		// Show next 10 pending resources
		end := 10
		if len(m.applyState.pending) < 10 {
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
			
			line := fmt.Sprintf("%s %s", icon, res.Address)
			content += actionStyle.Render(line) + "\n"
		}
		
		if len(m.applyState.pending) > 10 {
			content += dimStyle.Render(fmt.Sprintf("... and %d more", len(m.applyState.pending)-10))
		}
	}
	
	return boxStyle.Width(width).Height(12).Render(content)
}

func (m *modernPlanModel) renderApplyLogs() string {
	title := titleStyle.Render("Logs")
	logsBox := boxStyle.Width(m.width - 2).Render(
		title + "\n" + m.logViewport.View(),
	)
	return logsBox
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
	
	if m.applyState.showDetails {
		help += "[d] Hide Details  "
	} else {
		help += "[d] Show Details  "
	}
	
	help += "[↑↓] Scroll  "
	
	if m.applyState.applyComplete {
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
	
	// Show last N log entries
	start := 0
	maxLogs := 20
	if m.applyState.showFullLogs {
		maxLogs = 50 // Show more logs in full view
	}
	
	if len(m.applyState.logs) > maxLogs {
		start = len(m.applyState.logs) - maxLogs
	}
	
	for _, log := range m.applyState.logs[start:] {
		timestamp := log.Timestamp.Format("15:04:05")
		
		var icon string
		var style lipgloss.Style
		switch log.Level {
		case "error":
			icon = "❌"
			style = deleteIconStyle
		case "warning":
			icon = "⚠️"
			style = updateIconStyle
		case "info":
			icon = "ℹ️"
			style = dimStyle
		default:
			icon = "•"
			style = dimStyle
		}
		
		line := fmt.Sprintf("%s [%s] %s %s", timestamp, log.Level, icon, log.Message)
		content.WriteString(style.Render(line) + "\n")
	}
	
	m.logViewport.SetContent(content.String())
}

func (m *modernPlanModel) renderApplyDetailsView(header, elapsed string) string {
	if m.applyState == nil || m.applyState.currentOp == nil {
		return "No operation in progress"
	}
	
	op := m.applyState.currentOp
	
	// Find the resource in our plan
	var resource *ResourceChange
	for _, provider := range m.providers {
		for _, service := range provider.services {
			for _, res := range service.resources {
				if res.Address == op.Address {
					resource = &res
					break
				}
			}
		}
	}
	
	// Details content
	var content strings.Builder
	content.WriteString(titleStyle.Render("📋 Resource Details") + "\n\n")
	
	content.WriteString(fmt.Sprintf("Address: %s\n", op.Address))
	content.WriteString(fmt.Sprintf("Action: %s\n", op.Action))
	content.WriteString(fmt.Sprintf("Status: %s\n", op.Status))
	content.WriteString(fmt.Sprintf("Progress: %d%%\n", int(op.Progress*100)))
	content.WriteString(fmt.Sprintf("Duration: %s\n\n", time.Since(op.StartTime).Round(time.Second)))
	
	if resource != nil {
		content.WriteString(titleStyle.Render("Resource Type") + "\n")
		content.WriteString(fmt.Sprintf("%s %s\n\n", getResourceIcon(resource.Type), resource.Type))
		
		// Show changes
		if resource.Change.Before != nil || resource.Change.After != nil {
			content.WriteString(titleStyle.Render("Changes") + "\n")
			changes := m.findChangedAttributes(resource.Change.Before, resource.Change.After)
			for _, change := range changes {
				content.WriteString(m.renderAttributeChange(change))
				content.WriteString("\n")
			}
		}
	}
	
	// Recent logs for this resource
	content.WriteString("\n" + titleStyle.Render("Resource Logs") + "\n")
	for _, log := range m.applyState.logs {
		if log.Resource == op.Address {
			content.WriteString(fmt.Sprintf("%s %s\n", log.Timestamp.Format("15:04:05"), log.Message))
		}
	}
	
	detailBox := boxStyle.Width(m.width - 2).Height(m.height - 6).Render(content.String())
	
	footer := "[d] Back to overview  [q] Quit"
	
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
	
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}
	
	return nil
}