package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Apply-specific types
type applyState struct {
	// Progress tracking
	startTime      time.Time
	totalResources int
	completed      []completedResource
	pending        []pendingResource
	currentOp      *currentOperation
	logs           []logEntry
	
	// Terraform process
	cmd            *exec.Cmd
	mu             sync.Mutex
	
	// State
	isApplying     bool
	applyComplete  bool
	hasErrors      bool
	errorCount     int
	warningCount   int
	
	// View state
	showFullLogs   bool
	showDetails    bool
}

type completedResource struct {
	Address   string
	Action    string
	Duration  time.Duration
	Timestamp time.Time
	Success   bool
	Error     string
}

type pendingResource struct {
	Address string
	Action  string
	Type    string
}

type currentOperation struct {
	Address   string
	Action    string
	Progress  float64
	Status    string
	StartTime time.Time
}

type logEntry struct {
	Timestamp time.Time
	Level     string // info, warning, error
	Message   string
	Resource  string // optional resource address
}

// Messages for apply state updates
type applyStartMsg struct{}
type applyCompleteMsg struct{ success bool }
type applyErrorMsg struct{ err error }

type resourceStartMsg struct {
	Address string
	Action  string
}

type resourceProgressMsg struct {
	Address  string
	Progress float64
	Status   string
}

type resourceCompleteMsg struct {
	Address string
	Success bool
	Error   string
	Duration time.Duration
}

type logMsg struct {
	Level    string
	Message  string
	Resource string
}

// TerraformJSONMessage represents the JSON output from terraform apply -json
type TerraformJSONMessage struct {
	Type       string                 `json:"@type"`
	Level      string                 `json:"@level"`
	Message    string                 `json:"@message"`
	Module     string                 `json:"@module"`
	Timestamp  string                 `json:"@timestamp"`
	Diagnostic *DiagnosticInfo        `json:"diagnostic,omitempty"`
	Hook       map[string]interface{} `json:"hook,omitempty"`
	Change     *ChangeInfo            `json:"change,omitempty"`
}

type DiagnosticInfo struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
	Address  string `json:"address,omitempty"`
}

type ChangeInfo struct {
	Resource struct {
		Addr         string `json:"addr"`
		Module       string `json:"module"`
		Resource     string `json:"resource"`
		ResourceType string `json:"resource_type"`
		ResourceName string `json:"resource_name"`
		ResourceKey  string `json:"resource_key"`
	} `json:"resource"`
	Action string `json:"action"`
}

// Initialize apply state from the plan
func (m *modernPlanModel) initApplyState() {
	m.applyState = &applyState{
		startTime:      time.Now(),
		logs:           []logEntry{},
		pending:        []pendingResource{},
		completed:      []completedResource{},
		totalResources: m.stats.totalChanges,
	}
	
	// Convert planned resources to pending
	for _, provider := range m.providers {
		for _, service := range provider.services {
			for _, resource := range service.resources {
				m.applyState.pending = append(m.applyState.pending, pendingResource{
					Address: resource.Address,
					Action:  resource.Change.Actions[0],
					Type:    resource.Type,
				})
			}
		}
	}
	
	// Add initial log
	m.applyState.logs = append(m.applyState.logs, logEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   fmt.Sprintf("Starting terraform apply with %d resources", m.applyState.totalResources),
	})
}

// Start terraform apply process
func (m *modernPlanModel) startTerraformApply() tea.Cmd {
	return func() tea.Msg {
		// Initialize apply state if needed
		if m.applyState == nil {
			m.initApplyState()
		}
		
		// Start terraform apply with JSON output
		cmd := exec.Command("terraform", "apply", "-json", "-auto-approve", "tfplan")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return applyErrorMsg{err: err}
		}
		
		m.applyState.cmd = cmd
		
		if err := cmd.Start(); err != nil {
			return applyErrorMsg{err: err}
		}
		
		// Start parsing JSON output in goroutine
		go m.parseTerraformOutput(stdout)
		
		return applyStartMsg{}
	}
}

// Parse terraform JSON output
func (m *modernPlanModel) parseTerraformOutput(stdout interface{}) {
	scanner := bufio.NewScanner(stdout.(interface{ Read([]byte) (int, error) }))
	
	for scanner.Scan() {
		line := scanner.Text()
		var msg TerraformJSONMessage
		
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Not JSON, treat as plain log
			m.sendLogMessage("info", line, "")
			continue
		}
		
		// Process based on message type
		switch msg.Type {
		case "apply_start":
			m.handleApplyStart(&msg)
		case "apply_progress":
			m.handleApplyProgress(&msg)
		case "apply_complete":
			m.handleApplyComplete(&msg)
		case "apply_errored":
			m.handleApplyError(&msg)
		case "diagnostic":
			m.handleDiagnostic(&msg)
		case "provision_start", "provision_progress", "provision_complete", "provision_errored":
			m.handleProvisionMessage(&msg)
		case "refresh_start":
			m.sendLogMessage("info", "🔄 Starting refresh...", "")
		case "refresh_complete":
			m.sendLogMessage("info", "✅ Refresh completed", "")
		default:
			if msg.Message != "" {
				m.sendLogMessage("info", msg.Message, "")
			}
		}
	}
	
	// Check if process completed
	if err := m.applyState.cmd.Wait(); err != nil {
		m.sendMsg(applyErrorMsg{err: err})
	} else {
		m.sendMsg(applyCompleteMsg{success: true})
	}
}

func (m *modernPlanModel) handleApplyStart(msg *TerraformJSONMessage) {
	if hook, ok := msg.Hook["resource"].(map[string]interface{}); ok {
		addr := hook["addr"].(string)
		m.sendMsg(resourceStartMsg{
			Address: addr,
			Action:  m.getResourceAction(addr),
		})
		
		m.sendLogMessage("info", fmt.Sprintf("🔄 Starting %s...", addr), addr)
	}
}

func (m *modernPlanModel) handleApplyProgress(msg *TerraformJSONMessage) {
	if hook, ok := msg.Hook["resource"].(map[string]interface{}); ok {
		addr := hook["addr"].(string)
		progress := 0.5 // Default progress
		status := "In progress"
		
		if p, ok := hook["progress"].(map[string]interface{}); ok {
			if pct, ok := p["percent"].(float64); ok {
				progress = pct / 100.0
			}
			if s, ok := p["status"].(string); ok {
				status = s
			}
		}
		
		m.sendMsg(resourceProgressMsg{
			Address:  addr,
			Progress: progress,
			Status:   status,
		})
	}
}

func (m *modernPlanModel) handleApplyComplete(msg *TerraformJSONMessage) {
	if hook, ok := msg.Hook["resource"].(map[string]interface{}); ok {
		addr := hook["addr"].(string)
		
		// Calculate duration
		duration := time.Since(time.Now()) // This should track from start
		
		m.sendMsg(resourceCompleteMsg{
			Address:  addr,
			Success:  true,
			Duration: duration,
		})
		
		m.sendLogMessage("info", fmt.Sprintf("✅ Completed %s", addr), addr)
	}
}

func (m *modernPlanModel) handleApplyError(msg *TerraformJSONMessage) {
	if hook, ok := msg.Hook["resource"].(map[string]interface{}); ok {
		addr := hook["addr"].(string)
		errorMsg := "Unknown error"
		
		if e, ok := hook["error"].(string); ok {
			errorMsg = e
		}
		
		m.sendMsg(resourceCompleteMsg{
			Address: addr,
			Success: false,
			Error:   errorMsg,
		})
		
		m.sendLogMessage("error", fmt.Sprintf("❌ Failed %s: %s", addr, errorMsg), addr)
	}
}

func (m *modernPlanModel) handleDiagnostic(msg *TerraformJSONMessage) {
	if msg.Diagnostic == nil {
		return
	}
	
	level := "info"
	icon := "ℹ️"
	
	switch msg.Diagnostic.Severity {
	case "error":
		level = "error"
		icon = "❌"
	case "warning":
		level = "warning"
		icon = "⚠️"
	}
	
	message := fmt.Sprintf("%s %s: %s", icon, msg.Diagnostic.Summary, msg.Diagnostic.Detail)
	m.sendLogMessage(level, message, msg.Diagnostic.Address)
}

func (m *modernPlanModel) handleProvisionMessage(msg *TerraformJSONMessage) {
	switch msg.Type {
	case "provision_start":
		m.sendLogMessage("info", "🔧 Starting provisioner...", "")
	case "provision_progress":
		if hook, ok := msg.Hook["provisioner"].(map[string]interface{}); ok {
			if output, ok := hook["output"].(string); ok {
				m.sendLogMessage("info", fmt.Sprintf("  %s", output), "")
			}
		}
	case "provision_complete":
		m.sendLogMessage("info", "✅ Provisioner completed", "")
	case "provision_errored":
		m.sendLogMessage("error", "❌ Provisioner failed", "")
	}
}

func (m *modernPlanModel) sendLogMessage(level, message, resource string) {
	m.sendMsg(logMsg{
		Level:    level,
		Message:  message,
		Resource: resource,
	})
}

func (m *modernPlanModel) getResourceAction(address string) string {
	for _, provider := range m.providers {
		for _, service := range provider.services {
			for _, resource := range service.resources {
				if resource.Address == address && len(resource.Change.Actions) > 0 {
					return resource.Change.Actions[0]
				}
			}
		}
	}
	return "update"
}

// sendMsg sends a message through the program
func (m *modernPlanModel) sendMsg(msg tea.Msg) {
	if m.program != nil {
		m.program.Send(msg)
	}
}