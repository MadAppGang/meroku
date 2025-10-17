package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type destroyPhase int

const (
	destroyInitializing destroyPhase = iota
	destroyPlanning
	destroyDestroying
	destroyComplete
	destroyError
)

type destroyProgressModel struct {
	phase              destroyPhase
	currentResource    string
	destroyedItems     map[string]bool // Use map to track unique destroyed resources
	destroyedCount     int             // Actual count of destroyed resources
	outputLines        []string
	outputMutex        sync.Mutex
	errorDetails       string
	errorOutput        []string
	width              int
	height             int
	startTime          time.Time
	env                string
	spinnerFrame       int
	explosionFrame     int
	terraformCmd       *exec.Cmd // Store command reference for cleanup
	interrupted        bool      // Track if user interrupted
	errorViewport      viewport.Model
	showFullError      bool // Track if showing full error view
	copiedToClip       bool // Track if error was copied
	lastProcessedLines int  // Track how many lines we've processed
}

// Blast wave rings that expand outward
func getBlastWave(frame int) string {
	waves := []string{
		"",                                    // 0: No wave yet
		"",                                    // 1: Still building
		"",                                    // 2: Starting
		"- - - - - - - -",                    // 3: First wave
		"- - - - - - - - - - - -",            // 4: Expanding
		"~ ~ ~ ~ ~ ~ ~ ~ ~ ~ ~ ~ ~ ~",        // 5: Second wave
		"· · ~ ~ ~ ~ ~ ~ ~ ~ ~ ~ ~ · ·",      // 6: Particles
		"  · · · ~ ~ ~ ~ ~ ~ ~ · · · ·",      // 7: Dissipating
		"    · · · · ~ ~ ~ · · · ·",          // 8: Fading
		"      · · · · · · · ·",              // 9: Almost gone
		"        · · · ·",                     // 10: Traces
		"",                                    // 11+: Gone
	}

	if frame < len(waves) {
		return waves[frame]
	}
	return ""
}

// Nuclear explosion animation - Ultra-detailed Braille/Unicode art
var explosionFrames = []string{
	// Frame 0: Impact point
	`
       ⠀⠀⢀⠀⠀
       ⠀⢀⣀⡀⠀
       ⠀⠈⠁⠀⠀
`,
	// Frame 1: Initial flash
	`
       ⠀⢀⣀⡀⠀
       ⢀⣿⣿⡀
       ⠈⠻⠟⠁
       ⠀⠀⠁⠀⠀
`,
	// Frame 2: Fireball burst
	`
      ⠀⢀⣤⣤⡀⠀
      ⢠⣿⣿⣿⣿⡄
      ⢸⣿⣿⣿⣿⡇
      ⠈⠻⣿⣿⠟⠁
       ⠀⠈⠁⠀⠀
`,
	// Frame 3: Expanding blast
	`
     ⢀⣠⣤⣤⣤⣄⡀
    ⣠⣿⣿⣿⣿⣿⣿⣄
    ⣿⣿⣿⣿⣿⣿⣿⣿
    ⠙⢿⣿⣿⣿⣿⡿⠋
     ⠀⠈⠛⠛⠁⠀⠀
      ⠀⠀⣿⠀⠀⠀
`,
	// Frame 4: Rising column
	`
    ⢀⣠⣴⣶⣶⣶⣦⣄⡀
   ⣴⣿⣿⣿⣿⣿⣿⣿⣿⣦
   ⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
   ⠹⣿⣿⣿⣿⣿⣿⣿⣿⠏
    ⠈⠙⠻⣿⠿⠛⠋⠁
      ⠀⢸⣿⡇⠀⠀
      ⠀⢸⣿⡇⠀⠀
      ⠀⣸⣿⣇⠀⠀
`,
	// Frame 5: Mushroom forming
	`
   ⠀⢀⣀⣀⣀⣀⣀⣀⣀⡀⠀
   ⣠⣾⣿⣿⣿⣿⣿⣿⣿⣷⣄
  ⣰⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣆
  ⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
  ⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
   ⠈⠙⠻⢿⣿⣿⡿⠟⠋⠁
     ⠀⢀⣿⣿⣿⡀⠀
     ⠀⢸⣿⣿⣿⡇⠀
     ⠀⣸⣿⣿⣿⣇⠀
     ⠀⣿⣿⣿⣿⣿⠀
`,
	// Frame 6: Mushroom cap expanding
	`
  ⠀⢀⣀⣀⣀⣀⣀⣀⣀⣀⣀⡀⠀
  ⣠⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷⣄
 ⣰⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣆
 ⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
 ⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
  ⠈⠙⠻⢿⣿⣿⣿⣿⡿⠟⠋⠁
    ⠀⢀⣿⣿⣿⣿⣿⡀⠀
    ⠀⢸⣿⣿⣿⣿⣿⡇⠀
    ⠀⣸⣿⣿⣿⣿⣿⣇⠀
    ⠀⣿⣿⣿⣿⣿⣿⣿⠀
`,
	// Frame 7: Large mushroom cloud
	`
 ⢀⣠⣤⣤⣤⣤⣤⣤⣤⣤⣤⣤⣤⣄⡀
 ⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋⠁
   ⠀⢀⣿⣿⣿⣿⣿⣿⣿⣿⡀
   ⠀⢸⣿⣿⣿⣿⣿⣿⣿⣿⡇
   ⠀⣸⣿⣿⣿⣿⣿⣿⣿⣿⣇
   ⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
`,
	// Frame 8: Massive cloud
	`
⢀⣴⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣦⡀
⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⢀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡀
  ⠀⢸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇
  ⠀⣸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣇
  ⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
`,
	// Frame 9: Peak mushroom
	`
⣴⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣶⣦
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⢀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡀
  ⠀⢸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇
  ⠀⣸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣇
  ⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
`,
	// Frame 10: Billowing edges
	`
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⢀⣀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣀⡀
  ⠀⢸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇
  ⠀⣸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣇
  ⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
`,
	// Frame 11: Smoke spreading
	`
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⢀⣀⣀⣿⣿⣿⣿⣿⣿⣿⣿⣀⣀⡀
  ⠀⠈⠛⠛⣿⣿⣿⣿⣿⣿⣿⣿⠛⠛⠁
  ⠀⠀⠀⢸⣿⣿⣿⣿⣿⣿⣿⣿⡇⠀
  ⠀⠀⠀⣸⣿⣿⣿⣿⣿⣿⣿⣿⣇⠀
  ⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀
`,
	// Frame 12: Dissipating
	`
⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⢀⣀⣀⣿⣿⣿⣿⣿⣿⣿⣿⣀⣀⡀
  ⠀⠈⠉⠉⣿⣿⣿⣿⣿⣿⣿⣿⠉⠉⠁
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀
  ⠀⠀⠀⠀⠿⠿⠿⠿⠿⠿⠿⠿⠀⠀
`,
	// Frame 13: Fading smoke
	`
⠹⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⠀⢀⣀⣿⣿⣿⣿⣿⣿⣿⣿⣀⡀⠀
  ⠀⠀⠈⠉⣿⣿⣿⣿⣿⣿⣿⣿⠉⠁⠀
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀
  ⠀⠀⠀⠀⠿⣿⣿⣿⣿⣿⣿⠿⠀⠀⠀
  ⠀⠀⠀⠀⠀⠈⠛⠛⠛⠛⠁⠀⠀⠀⠀
  ⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
`,
	// Frame 14: Final traces
	`
 ⠈⠙⠻⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀
  ⠀⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀
  ⠀⠀⠀⠀⠿⢿⣿⣿⣿⣿⡿⠿⠀⠀⠀
  ⠀⠀⠀⠀⠀⠈⠉⠉⠉⠁⠀⠀⠀⠀⠀
  ⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
`,
}

type destroyTickMsg time.Time
type destroyCompleteMsg struct{}
type destroyErrorMsg struct {
	err    error
	output []string
}

func newDestroyProgressModel(env string) *destroyProgressModel {
	return &destroyProgressModel{
		phase:              destroyInitializing,
		destroyedItems:     make(map[string]bool),
		destroyedCount:     0,
		outputLines:        []string{},
		startTime:          time.Now(),
		env:                env,
		lastProcessedLines: 0,
	}
}

func (m *destroyProgressModel) Init() tea.Cmd {
	return tea.Batch(
		m.runTerraformDestroy(),
		m.tickCmd(),
	)
}

func (m *destroyProgressModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return destroyTickMsg(t)
	})
}

func (m *destroyProgressModel) runTerraformDestroy() tea.Cmd {
	return func() tea.Msg {
		// Change to environment directory
		wd, err := os.Getwd()
		if err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}
		defer os.Chdir(wd)

		// Ensure lambda bootstrap file exists before running terraform
		// This prevents errors when the archive_file data source tries to archive a missing file
		ensureLambdaBootstrapExists()

		envPath := filepath.Join("env", m.env)
		if err := os.Chdir(envPath); err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		// Initialize terraform first
		initCmd := exec.Command("terraform", "init", "-reconfigure")
		initOut, _ := initCmd.CombinedOutput()
		m.outputMutex.Lock()
		m.outputLines = append(m.outputLines, strings.Split(string(initOut), "\n")...)
		m.outputMutex.Unlock()

		// Run terraform destroy with no color output for easier parsing
		cmd := exec.Command("terraform", "destroy", "-auto-approve", "-no-color")

		// Store command reference for interrupt handling
		m.terraformCmd = cmd

		// Create pipes for stdout and stderr
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		// Process stdout in a goroutine
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				m.outputMutex.Lock()
				m.outputLines = append(m.outputLines, line)
				// Keep only last 100 lines
				if len(m.outputLines) > 100 {
					m.outputLines = m.outputLines[len(m.outputLines)-100:]
				}
				m.outputMutex.Unlock()
			}
		}()

		// Process stderr in a goroutine
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				m.outputMutex.Lock()
				m.errorOutput = append(m.errorOutput, line)
				m.outputLines = append(m.outputLines, "ERROR: "+line)
				if len(m.outputLines) > 100 {
					m.outputLines = m.outputLines[len(m.outputLines)-100:]
				}
				m.outputMutex.Unlock()
			}
		}()

		// Wait for command to complete
		err = cmd.Wait()

		if err != nil {
			m.outputMutex.Lock()
			errorLines := make([]string, len(m.errorOutput))
			copy(errorLines, m.errorOutput)
			m.outputMutex.Unlock()

			// Check if it's actually successful (no resources to destroy)
			if len(errorLines) == 0 || strings.Contains(strings.Join(errorLines, " "), "No changes") {
				return destroyCompleteMsg{}
			}

			if len(errorLines) == 0 {
				m.outputMutex.Lock()
				errorLines = make([]string, len(m.outputLines))
				copy(errorLines, m.outputLines)
				m.outputMutex.Unlock()
			}
			return destroyErrorMsg{err: err, output: errorLines}
		}

		return destroyCompleteMsg{}
	}
}

func (m *destroyProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case destroyTickMsg:
		m.spinnerFrame++

		// Process only NEW output lines to determine phase and current resource
		m.outputMutex.Lock()
		allLines := m.outputLines
		totalLines := len(allLines)

		// Get only the new lines since last tick
		var newLines []string
		if totalLines > m.lastProcessedLines {
			newLines = allLines[m.lastProcessedLines:]
			m.lastProcessedLines = totalLines
		}
		m.outputMutex.Unlock()

		// If we have output and still initializing, move to planning
		if totalLines > 0 && m.phase == destroyInitializing {
			m.phase = destroyPlanning
		}

		// Process only new lines
		for _, line := range newLines {
			if strings.Contains(line, "Initializing") {
				if m.phase != destroyPlanning && m.phase != destroyDestroying {
					m.phase = destroyInitializing
					m.currentResource = "Initializing Terraform..."
				}
			} else if strings.Contains(line, "Destroy complete!") {
				m.phase = destroyComplete
			} else if strings.Contains(line, "Destroying...") {
				m.phase = destroyDestroying
				// Extract resource name from "Destroying... [resource_name]"
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					resourceName := parts[1]
					m.currentResource = resourceName
					// Track as in-progress (not yet destroyed)
				}
			} else if strings.Contains(line, "Destruction complete after") || strings.Contains(line, "Destroyed:") {
				// This is the actual completion message - count it
				parts := strings.Fields(line)
				if len(parts) > 0 {
					// Extract resource name (usually second field)
					var resourceName string
					if strings.Contains(line, "Destruction complete") {
						// Format: "resource_type.resource_name: Destruction complete after 1s"
						resourceName = strings.Split(parts[0], ":")[0]
					} else if strings.Contains(line, "Destroyed:") {
						// Format: "Destroyed: resource_type.resource_name"
						if len(parts) >= 2 {
							resourceName = parts[1]
						}
					}

					// Add to destroyed items map (automatically deduplicates)
					if resourceName != "" && !m.destroyedItems[resourceName] {
						m.destroyedItems[resourceName] = true
						m.destroyedCount++
					}
				}
			} else if strings.Contains(line, "Refreshing state") || strings.Contains(line, "Reading...") {
				m.phase = destroyPlanning
				m.currentResource = "Planning destruction..."
			} else if strings.Contains(line, "No changes") || strings.Contains(line, "0 destroyed") {
				m.phase = destroyComplete
				m.currentResource = "No resources to destroy"
			} else if strings.Contains(line, "Terraform will perform") || strings.Contains(line, "Plan:") {
				// Detected plan output - about to start destroying
				m.phase = destroyDestroying
			}
		}

		// Animate explosion continuously (except when complete/error)
		// Loops infinitely through all frames
		if m.phase != destroyComplete && m.phase != destroyError {
			// Cycle through explosion frames (every 2 ticks = 200ms per frame for smoother animation)
			if m.spinnerFrame%2 == 0 {
				m.explosionFrame++

				// Loop back to start when reaching the end
				if m.explosionFrame >= len(explosionFrames) {
					m.explosionFrame = 0
				}

				// Terminal bell at dramatic moments for audio feedback
				if m.explosionFrame == 2 || m.explosionFrame == 7 {
					fmt.Print("\a") // Bell sound at flash and mushroom formation
				}
			}
		}

		// Continue ticking if not complete
		if m.phase != destroyComplete && m.phase != destroyError {
			return m, m.tickCmd()
		}
		return m, nil

	case destroyCompleteMsg:
		m.phase = destroyComplete
		return m, nil

	case destroyErrorMsg:
		m.phase = destroyError
		m.errorDetails = msg.err.Error()
		if len(msg.output) > 0 {
			m.errorOutput = msg.output
		}
		// Stop the animation and wait for user to press a key
		return m, nil

	case tea.KeyMsg:
		// Handle Ctrl+C during ANY phase
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			// Kill terraform process if running
			if m.terraformCmd != nil && m.terraformCmd.Process != nil {
				m.terraformCmd.Process.Kill()
			}
			m.interrupted = true
			m.phase = destroyError
			m.errorDetails = "Operation interrupted by user"
			return m, tea.Quit
		}

		// Special handling when in error or complete state
		if m.phase == destroyComplete || m.phase == destroyError {
			// Toggle full error view with 'e' or 'f' key (only in error state)
			if m.phase == destroyError && (msg.String() == "e" || msg.String() == "f") {
				m.showFullError = !m.showFullError
				if m.showFullError {
					// Initialize viewport with full error content
					m.errorViewport = viewport.New(m.width-8, m.height-10)
					fullError := strings.Join(m.errorOutput, "\n")
					if fullError == "" {
						fullError = m.errorDetails
					}
					m.errorViewport.SetContent(fullError)
				}
				return m, nil
			}

			// Copy error to clipboard with 'c' key (only in error state)
			if m.phase == destroyError && msg.String() == "c" {
				fullError := strings.Join(m.errorOutput, "\n")
				if fullError == "" {
					fullError = m.errorDetails
				}
				err := clipboard.WriteAll(fullError)
				if err == nil {
					m.copiedToClip = true
					// Reset the copied flag after 2 seconds
					return m, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
						m.copiedToClip = false
						return nil
					})
				}
				return m, nil
			}

			// Handle viewport scrolling when showing full error
			if m.showFullError {
				switch msg.String() {
				case "up", "k":
					m.errorViewport.LineUp(1)
					return m, nil
				case "down", "j":
					m.errorViewport.LineDown(1)
					return m, nil
				case "pgup", "b":
					m.errorViewport.ViewUp()
					return m, nil
				case "pgdown", "f":
					m.errorViewport.ViewDown()
					return m, nil
				case "home", "g":
					m.errorViewport.GotoTop()
					return m, nil
				case "end", "G":
					m.errorViewport.GotoBottom()
					return m, nil
				case "esc":
					// Exit full error view
					m.showFullError = false
					return m, nil
				}
			}

			// Allow quitting with any key when not in special modes
			if !m.showFullError {
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m *destroyProgressModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Show full error view if enabled
	if m.showFullError {
		return m.renderFullErrorView()
	}

	// Spinner frames
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]

	// Get output copy
	m.outputMutex.Lock()
	outputCopy := make([]string, len(m.outputLines))
	copy(outputCopy, m.outputLines)
	m.outputMutex.Unlock()

	// Calculate content width - leave margins on sides
	contentWidth := min(m.width-8, 120)

	var content strings.Builder

	// ═══════════════════════════════════════
	// TITLE SECTION
	// ═══════════════════════════════════════
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Align(lipgloss.Center).
		Width(contentWidth)

	// Add screen shake effect during intense frames
	var titleText string
	if m.explosionFrame >= 3 && m.explosionFrame <= 7 && m.spinnerFrame%3 == 0 {
		// Subtle shake by adding random spacing
		titleText = "💥  INFRASTRUCTURE  DESTRUCTION  IN  PROGRESS"
	} else {
		titleText = "💥 INFRASTRUCTURE DESTRUCTION IN PROGRESS"
	}

	content.WriteString(titleStyle.Render(titleText))
	content.WriteString("\n\n")

	// ═══════════════════════════════════════
	// BLAST WAVE (Fixed-Height Container)
	// ═══════════════════════════════════════
	blastWaveHeight := 2 // Reserve space for blast wave

	blastWave := getBlastWave(m.explosionFrame)
	waveContainer := lipgloss.NewStyle().
		Height(blastWaveHeight).
		Width(contentWidth).
		Align(lipgloss.Center, lipgloss.Center)

	if blastWave != "" {
		waveStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Align(lipgloss.Center)

		content.WriteString(waveContainer.Render(waveStyle.Render(blastWave)))
	} else {
		content.WriteString(waveContainer.Render(""))
	}
	content.WriteString("\n")

	// ═══════════════════════════════════════
	// EXPLOSION ANIMATION (Fixed-Height Container)
	// ═══════════════════════════════════════
	// Reserve fixed space for explosion to prevent layout shifts
	explosionHeight := 11 // Height of largest frame + padding

	if m.phase != destroyComplete && m.phase != destroyError {
		// Get current explosion frame
		frameIndex := m.explosionFrame
		if frameIndex >= len(explosionFrames) {
			frameIndex = len(explosionFrames) - 1
		}

		// Enhanced color progression with detailed transitions
		var explosionColor string

		switch frameIndex {
		case 0:
			explosionColor = "231" // White flash (brightest)
		case 1:
			explosionColor = "227" // Bright yellow (flash peak)
		case 2:
			explosionColor = "226" // Yellow (burst)
		case 3:
			explosionColor = "220" // Golden yellow (expanding)
		case 4:
			explosionColor = "214" // Orange-yellow (heating)
		case 5:
			explosionColor = "208" // Orange (fireball)
		case 6:
			explosionColor = "202" // Deep orange (rising heat)
		case 7:
			explosionColor = "196" // Red (mushroom forming)
		case 8:
			explosionColor = "160" // Dark red (cloud solidifying)
		case 9:
			explosionColor = "124" // Deep red (massive cloud)
		case 10:
			explosionColor = "88"  // Burgundy (peak intensity)
		case 11:
			explosionColor = "240" // Dark gray (smoke starting)
		case 12:
			explosionColor = "244" // Medium gray (dissipating)
		case 13:
			explosionColor = "248" // Light gray (spreading smoke)
		default:
			explosionColor = "250" // Very light gray (final fade)
		}

		// Add subtle pulsing effect during peak frames by alternating intensity
		if frameIndex >= 6 && frameIndex <= 10 && m.spinnerFrame%4 < 2 {
			// Pulse effect: slightly brighter every other beat
			if frameIndex == 8 {
				explosionColor = "196" // Brighten during pulse
			}
		}

		// Create fixed-height container for explosion
		explosionContainer := lipgloss.NewStyle().
			Height(explosionHeight).
			Width(contentWidth).
			Align(lipgloss.Center, lipgloss.Center)

		// Style the explosion frame
		explosionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(explosionColor)).
			Bold(frameIndex < 10). // Bold for intense frames, normal for smoke
			Align(lipgloss.Center)

		// Render frame inside container (vertically centered)
		styledFrame := explosionStyle.Render(explosionFrames[frameIndex])
		content.WriteString(explosionContainer.Render(styledFrame))
		content.WriteString("\n")
	} else {
		// Show empty space of same height when complete/error
		emptyContainer := lipgloss.NewStyle().
			Height(explosionHeight).
			Width(contentWidth)
		content.WriteString(emptyContainer.Render(""))
		content.WriteString("\n")
	}

	// ═══════════════════════════════════════
	// STATUS SECTION (Fixed-Height Container)
	// ═══════════════════════════════════════
	statusHeight := 3 // Reserve fixed space for status to prevent layout shifts

	var statusText string
	var statusColor string

	switch m.phase {
	case destroyInitializing:
		statusText = fmt.Sprintf("%s Initializing Terraform", spinner)
		statusColor = "220"
	case destroyPlanning:
		statusText = fmt.Sprintf("%s Planning destruction", spinner)
		statusColor = "214"
	case destroyDestroying:
		resourceName := m.currentResource
		if resourceName == "" {
			resourceName = m.env
		}
		// Truncate long resource names to prevent layout shifts
		maxResourceNameLen := contentWidth - 15 // Reserve space for spinner and "Destroying: "
		if len(resourceName) > maxResourceNameLen {
			resourceName = resourceName[:maxResourceNameLen-3] + "..."
		}
		statusText = fmt.Sprintf("%s Destroying: %s", spinner, resourceName)
		statusColor = "196"
	case destroyComplete:
		statusText = "✅ Destruction complete!"
		statusColor = "82"
	case destroyError:
		if m.interrupted {
			statusText = "⚠️  Operation cancelled by user"
			statusColor = "214"
		} else {
			statusText = "❌ Destruction failed!"
			statusColor = "196"
		}
	}

	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(statusColor)).
		Align(lipgloss.Center).
		Width(contentWidth)

	// Use fixed-height container for status
	statusContainer := lipgloss.NewStyle().
		Height(statusHeight).
		Width(contentWidth).
		Align(lipgloss.Center, lipgloss.Center)

	content.WriteString(statusContainer.Render(statusStyle.Render(statusText)))
	content.WriteString("\n")

	// ═══════════════════════════════════════
	// OUTPUT BOX (ALWAYS SHOWN)
	// ═══════════════════════════════════════
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Width(contentWidth)

	content.WriteString(labelStyle.Render("Terraform Output:"))
	content.WriteString("\n")

	// Calculate dynamic height for output box based on available space
	// Account for all fixed-height elements:
	// - Title section: 3 lines (title + 2 newlines)
	// - Blast wave: 2 lines + 1 newline = 3 lines
	// - Explosion: 11 lines + 1 newline = 12 lines
	// - Status: 3 lines + 1 newline = 4 lines
	// - Output label: 1 line + 1 newline = 2 lines
	// - Footer: 1 line
	// Total fixed: 25 lines
	fixedContentHeight := 25
	outputHeight := max(5, m.height-fixedContentHeight-4) // Minimum 5 lines for output, -4 for padding/borders

	// Create bordered output box
	outputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(contentWidth).
		Height(outputHeight)

	var outputContent strings.Builder

	if len(outputCopy) == 0 {
		// Show waiting message when no output yet
		waitingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true).
			Align(lipgloss.Center).
			Width(contentWidth - 6)

		outputContent.WriteString(waitingStyle.Render("Waiting for Terraform output..."))
	} else {
		// Show actual output
		linesToShow := outputHeight - 4 // Account for padding and borders
		start := 0
		if len(outputCopy) > linesToShow {
			start = len(outputCopy) - linesToShow
		}

		maxLineWidth := contentWidth - 6 // Account for padding
		for i := start; i < len(outputCopy); i++ {
			line := outputCopy[i]

			// Truncate long lines
			if len(line) > maxLineWidth {
				line = line[:maxLineWidth-3] + "..."
			}

			// Color-code important lines
			if strings.Contains(line, "Destroying...") || strings.Contains(line, "Destruction complete") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(line)
			} else if strings.Contains(line, "ERROR") || strings.Contains(line, "Error") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(line)
			} else if strings.Contains(line, "Success") || strings.Contains(line, "complete!") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(line)
			}

			outputContent.WriteString(line + "\n")
		}
	}

	content.WriteString(outputBoxStyle.Render(outputContent.String()))
	content.WriteString("\n")

	// ═══════════════════════════════════════
	// FOOTER SECTION (Stats & Instructions)
	// ═══════════════════════════════════════
	var footerParts []string

	// Progress stats
	if m.destroyedCount > 0 {
		footerParts = append(footerParts,
			fmt.Sprintf("Destroyed: %d resources", m.destroyedCount))
	}

	// Elapsed time
	elapsed := time.Since(m.startTime).Seconds()
	footerParts = append(footerParts, fmt.Sprintf("Elapsed: %.1fs", elapsed))

	// Instructions
	if m.phase == destroyComplete {
		footerParts = append(footerParts, "Press any key to continue")
	} else if m.phase == destroyError {
		// Show error-specific options
		if m.copiedToClip {
			footerParts = append(footerParts, "✓ Copied!")
		} else {
			footerParts = append(footerParts, "[E] Full Error • [C] Copy • Press any key to exit")
		}
	} else {
		// Show cancel instruction during active phases
		footerParts = append(footerParts, "Press Ctrl+C or Q to cancel")
	}

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Align(lipgloss.Center).
		Width(contentWidth)

	content.WriteString(footerStyle.Render(strings.Join(footerParts, " • ")))

	// ═══════════════════════════════════════
	// CENTER EVERYTHING HORIZONTALLY
	// ═══════════════════════════════════════
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Top,
		content.String(),
		lipgloss.WithWhitespaceChars(" "),
	)
}

func (m *destroyProgressModel) renderFullErrorView() string {
	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Align(lipgloss.Center).
		Width(m.width)

	content.WriteString(titleStyle.Render("❌ FULL ERROR OUTPUT"))
	content.WriteString("\n\n")

	// Viewport with error content
	viewportStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(m.width - 4).
		Height(m.height - 8)

	content.WriteString(viewportStyle.Render(m.errorViewport.View()))
	content.WriteString("\n")

	// Footer with instructions
	footerParts := []string{}

	// Show scroll position
	scrollInfo := fmt.Sprintf("%3.f%%", m.errorViewport.ScrollPercent()*100)
	footerParts = append(footerParts, scrollInfo)

	if m.copiedToClip {
		footerParts = append(footerParts, "✓ Copied to clipboard!")
	} else {
		footerParts = append(footerParts, "[↑↓] Scroll • [PgUp/PgDn] Page • [C] Copy • [ESC] Back")
	}

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Align(lipgloss.Center).
		Width(m.width)

	content.WriteString(footerStyle.Render(strings.Join(footerParts, " • ")))

	return content.String()
}

func runTerraformDestroyWithProgress(env string) error {
	p := tea.NewProgram(
		newDestroyProgressModel(env),
		tea.WithAltScreen(),
	)

	model, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running destroy progress TUI: %w", err)
	}

	// Check if there was an error
	if finalModel, ok := model.(*destroyProgressModel); ok {
		if finalModel.phase == destroyError {
			errorMsg := finalModel.errorDetails
			if len(finalModel.errorOutput) > 0 {
				errorMsg = strings.Join(finalModel.errorOutput, "\n")
			}
			return fmt.Errorf("terraform destroy failed: %s", errorMsg)
		}
	}

	return nil
}
