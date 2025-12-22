package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogMsg is sent when new log content arrives
type LogMsg string

// ProgressMsg updates the current status
type ProgressMsg struct {
	Status      string
	CurrentStep int
	TotalSteps  int
}

// DoneMsg signals completion
type DoneMsg struct {
	Duration time.Duration
	ExitCode int
}

// ErrMsg signals an error
type ErrMsg error

// CheckModel is the Bubble Tea model for the check command TUI
type CheckModel struct {
	viewport     viewport.Model
	spinner      spinner.Model
	logs         []string        // Deprecated: use allLogs instead
	allLogs      []string        // Full log history
	tailLines    []string        // Last 3 lines for compact display
	status       string
	currentStep  int
	totalSteps   int
	done         bool
	err          error
	width        int
	height       int
	duration     time.Duration
	exitCode     int
	ready        bool
	logsExpanded bool            // Track expanded/collapsed state
	startTime    time.Time       // Track when workflow started
}

// NewCheckModel creates a new TUI model for the check command
func NewCheckModel() CheckModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return CheckModel{
		spinner:      s,
		status:       "Initializing...",
		logs:         []string{},
		allLogs:      []string{},
		tailLines:    []string{},
		logsExpanded: false,
		startTime:    time.Now(),
	}
}

func (m *CheckModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForActivity,
	)
}

func waitForActivity() tea.Msg {
	return nil
}

func (m *CheckModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "o":
			// Toggle logs expanded/collapsed
			m.logsExpanded = !m.logsExpanded
			if m.logsExpanded && m.ready {
				// Populate viewport with all logs when expanding
				m.viewport.SetContent(strings.Join(m.allLogs, "\n"))
				m.viewport.GotoBottom()
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate viewport height for expanded mode
		// Leave space for: status line (1) + borders (2) + hint (1) = 4 lines
		availableHeight := msg.Height - 4

		// Clamp viewport height between 15 and 30 lines
		viewportHeight := availableHeight
		if viewportHeight < 15 {
			viewportHeight = 15
		}
		if viewportHeight > 30 {
			viewportHeight = 30
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, viewportHeight) // -4 for borders and padding
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = viewportHeight
		}

	case LogMsg:
		// Append to full log history
		m.allLogs = append(m.allLogs, string(msg))

		// Update tail lines (last 3 lines)
		if len(m.allLogs) > 3 {
			m.tailLines = m.allLogs[len(m.allLogs)-3:]
		} else {
			m.tailLines = m.allLogs
		}

		// Only update viewport if expanded
		if m.logsExpanded && m.ready {
			m.viewport.SetContent(strings.Join(m.allLogs, "\n"))
			m.viewport.GotoBottom()
		}

		return m, waitForActivity

	case ProgressMsg:
		m.status = msg.Status
		m.currentStep = msg.CurrentStep
		m.totalSteps = msg.TotalSteps

	case DoneMsg:
		m.done = true
		m.duration = msg.Duration
		m.exitCode = msg.ExitCode
		m.logsExpanded = false // Auto-collapse on completion
		return m, tea.Quit

	case ErrMsg:
		m.err = msg
		m.logsExpanded = false // Auto-collapse on error
		return m, tea.Quit

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *CheckModel) View() string {
	// If done, show completion summary (auto-collapsed)
	if m.done {
		return m.renderCompletionView()
	}

	// If not ready yet (no WindowSizeMsg), show minimal status
	if !m.ready {
		return m.status + "\n"
	}

	// Render based on expanded state
	if m.logsExpanded {
		return m.renderExpandedView()
	}

	return m.renderCompactView()
}

// renderCompactView renders the compact (collapsed) view with tail lines
func (m *CheckModel) renderCompactView() string {
	var b strings.Builder

	// Status line with spinner and elapsed time
	elapsed := time.Since(m.startTime).Round(time.Second)
	statusLine := fmt.Sprintf("%s %s (%s)", m.spinner.View(), m.status, elapsed)
	b.WriteString(statusLine + "\n")

	// Show last 3 log lines indented
	if len(m.tailLines) > 0 {
		for _, line := range m.tailLines {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}

	// Toggle hint
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	b.WriteString(hintStyle.Render("⊕ Full logs (press 'o' to expand, 'q' to quit)\n"))

	return b.String()
}

// renderExpandedView renders the expanded view with full logs in viewport
func (m *CheckModel) renderExpandedView() string {
	var b strings.Builder

	// Status line with spinner and elapsed time
	elapsed := time.Since(m.startTime).Round(time.Second)
	statusLine := fmt.Sprintf("%s %s (%s)", m.spinner.View(), m.status, elapsed)
	b.WriteString(statusLine + "\n")

	// Bordered viewport with logs
	logBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)

	b.WriteString(logBoxStyle.Render(m.viewport.View()) + "\n")

	// Toggle hint
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	b.WriteString(hintStyle.Render("⊖ Full logs (press 'o' to collapse, 'q' to quit)\n"))

	return b.String()
}

// renderCompletionView renders the final completion summary
func (m *CheckModel) renderCompletionView() string {
	statusIcon := "✓"
	statusColor := lipgloss.Color("42")
	if m.exitCode != 0 {
		statusIcon = "✗"
		statusColor = lipgloss.Color("196")
	}

	completionStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true)

	return completionStyle.Render(fmt.Sprintf("%s Completed in %s (exit code %d)\n", statusIcon, m.duration, m.exitCode))
}

