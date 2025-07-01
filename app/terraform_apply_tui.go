package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
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
	showFullLogs    bool
	showDetails     bool
	showErrorDetails bool
	selectedSection int  // 0=completed, 1=pending, 2=logs
	selectedError   int  // Index of selected error in completed list
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
	Type       string          `json:"@type"`
	Level      string          `json:"@level"`
	Message    string          `json:"@message"`
	Module     string          `json:"@module"`
	Timestamp  string          `json:"@timestamp"`
	Diagnostic *DiagnosticInfo `json:"diagnostic,omitempty"`
	Hook       *HookInfo       `json:"hook,omitempty"`
	Change     *ChangeInfo     `json:"change,omitempty"`
}

type DiagnosticInfo struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
	Address  string `json:"address,omitempty"`
}

type HookInfo struct {
	Resource       *ResourceInfo `json:"resource,omitempty"`
	Action         string        `json:"action,omitempty"`
	IDKey          string        `json:"id_key,omitempty"`
	IDValue        string        `json:"id_value,omitempty"`
	ElapsedSeconds float64       `json:"elapsed_seconds,omitempty"`
}

type ResourceInfo struct {
	Addr            string `json:"addr"`
	Module          string `json:"module"`
	Resource        string `json:"resource"`
	ResourceType    string `json:"resource_type"`
	ResourceName    string `json:"resource_name"`
	ResourceKey     interface{} `json:"resource_key,omitempty"`
	ImpliedProvider string `json:"implied_provider,omitempty"`
}

type ChangeInfo struct {
	Resource *ResourceInfo `json:"resource"`
	Action   string        `json:"action"`
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
		
		// Debug log the message type
		if msg.Type != "" {
			m.sendLogMessage("debug", fmt.Sprintf("[DEBUG] Message type: %s", msg.Type), "")
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
		case "resource_drift":
			// Handle resource drift messages
			if msg.Hook != nil && msg.Hook.Resource != nil {
				m.sendLogMessage("info", fmt.Sprintf("Resource drift detected: %s", msg.Hook.Resource.Addr), msg.Hook.Resource.Addr)
			}
		case "diagnostic":
			m.handleDiagnostic(&msg)
		case "provision_start", "provision_progress", "provision_complete", "provision_errored":
			m.handleProvisionMessage(&msg)
		case "refresh_start":
			m.sendLogMessage("info", "🔄 Starting refresh...", "")
		case "refresh_complete":
			m.sendLogMessage("info", "✅ Refresh completed", "")
		default:
			// Log any unhandled message types for debugging
			if msg.Type != "" && msg.Type != "version" && msg.Type != "log" {
				m.sendLogMessage("debug", fmt.Sprintf("[DEBUG] Unhandled message type: %s", msg.Type), "")
			}
			
			// Check if this is an error message by content
			if msg.Message != "" {
				// Use the level from the message
				logLevel := "info"
				if msg.Level != "" {
					logLevel = msg.Level
				}
				
				// Check for error patterns in the message
				if msg.Level == "error" || 
				   (msg.Message != "" && (strings.Contains(msg.Message, ": Creation errored after") ||
				                         strings.Contains(msg.Message, ": Modification errored after") ||
				                         strings.Contains(msg.Message, ": Destruction errored after"))) {
					// Parse the resource address from error message
					if strings.Contains(msg.Message, "errored after") {
						parts := strings.SplitN(msg.Message, ":", 2)
						if len(parts) >= 1 {
							addr := strings.TrimSpace(parts[0])
							// Send a resource complete message with error
							m.sendMsg(resourceCompleteMsg{
								Address:  addr,
								Success:  false,
								Error:    msg.Message,
								Duration: 0,
							})
						}
					}
				}
				
				m.sendLogMessage(logLevel, msg.Message, "")
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
	if msg.Hook == nil || msg.Hook.Resource == nil {
		return
	}
	
	addr := msg.Hook.Resource.Addr
	action := msg.Hook.Action
	if action == "" {
		action = m.getResourceAction(addr)
	}
	
	m.sendMsg(resourceStartMsg{
		Address: addr,
		Action:  action,
	})
	m.sendLogMessage("info", fmt.Sprintf("🔄 Starting %s on %s...", action, addr), addr)
}

func (m *modernPlanModel) handleApplyProgress(msg *TerraformJSONMessage) {
	if msg.Hook == nil || msg.Hook.Resource == nil {
		return
	}
	
	addr := msg.Hook.Resource.Addr
	// Calculate progress based on elapsed time
	progress := 0.5
	if msg.Hook.ElapsedSeconds > 0 {
		// Assume most operations complete within 30 seconds
		progress = math.Min(msg.Hook.ElapsedSeconds/30.0, 0.9)
	}
	
	status := fmt.Sprintf("In progress (%.1fs)", msg.Hook.ElapsedSeconds)
	
	m.sendMsg(resourceProgressMsg{
		Address:  addr,
		Progress: progress,
		Status:   status,
	})
}

func (m *modernPlanModel) handleApplyComplete(msg *TerraformJSONMessage) {
	if msg.Hook == nil || msg.Hook.Resource == nil {
		return
	}
	
	addr := msg.Hook.Resource.Addr
	action := msg.Hook.Action
	if action == "" {
		action = m.getResourceAction(addr)
	}
	
	// Use elapsed seconds from the message
	duration := time.Duration(msg.Hook.ElapsedSeconds * float64(time.Second))
	
	m.sendMsg(resourceCompleteMsg{
		Address:  addr,
		Success:  true,
		Duration: duration,
	})
	
	// Include ID in log if available
	idInfo := ""
	if msg.Hook.IDKey != "" && msg.Hook.IDValue != "" {
		idInfo = fmt.Sprintf(" [%s=%s]", msg.Hook.IDKey, msg.Hook.IDValue)
	}
	
	m.sendLogMessage("info", fmt.Sprintf("✅ Completed %s on %s (%v)%s", action, addr, duration, idInfo), addr)
}

func (m *modernPlanModel) handleApplyError(msg *TerraformJSONMessage) {
	if msg.Hook == nil || msg.Hook.Resource == nil {
		return
	}
	
	addr := msg.Hook.Resource.Addr
	action := msg.Hook.Action
	if action == "" {
		action = m.getResourceAction(addr)
	}
	
	// Use elapsed seconds from the message
	duration := time.Duration(msg.Hook.ElapsedSeconds * float64(time.Second))
	
	// Error message is typically in the main message field
	errorMsg := msg.Message
	if errorMsg == "" {
		errorMsg = "Operation failed"
	}
	
	m.sendMsg(resourceCompleteMsg{
		Address:  addr,
		Success:  false,
		Error:    errorMsg,
		Duration: duration,
	})
	
	m.sendLogMessage("error", fmt.Sprintf("❌ Failed %s on %s after %v: %s", action, addr, duration, errorMsg), addr)
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
		// Provisioner output is typically in the message field
		if msg.Message != "" {
			m.sendLogMessage("info", fmt.Sprintf("  %s", msg.Message), "")
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