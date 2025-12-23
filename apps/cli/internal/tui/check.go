package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/output"
)

const (
	// Viewport display dimensions
	viewportMinHeight     = 15
	viewportMaxHeight     = 30
	viewportReservedLines = 4  // status + borders + hint
	viewportBorderPadding = 4  // borders and padding

	// Log tail display settings
	tailLinesToShow = 3

	// Color codes for spinner and status
	spinnerColor        = "205"
	checkStatusGreen    = "42"
	checkStatusRed      = "196"
	hintTextGray        = "241"
	logBoxBorderColor   = "63"
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
	Duration  time.Duration
	ExitCode  int
	Errors    *errors.GroupedErrors
	Cancelled bool // True if workflow was cancelled via Ctrl+C
}

// ErrMsg signals an error
type ErrMsg error

// CheckModel is the Bubble Tea model for the check command TUI
type CheckModel struct {
	viewport     viewport.Model
	spinner      spinner.Model
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
	logsExpanded bool                  // Track expanded/collapsed state
	startTime    time.Time             // Track when workflow started
	logsDirty    bool                  // Track if logs need viewport update
	errors       *errors.GroupedErrors // Extracted errors from workflow run
	Cancelled    bool                  // Track if workflow was cancelled via Ctrl+C
	cancelFunc   func()                // Context cancel function for 'q' key handling
	quitting     bool                  // Track if we're in the process of quitting
}

// NewCheckModel creates a new TUI model for the check command
// cancelFunc is the context cancellation function to call when 'q' is pressed
func NewCheckModel(cancelFunc func()) CheckModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(spinnerColor))

	return CheckModel{
		spinner:      s,
		status:       "Initializing...",
		allLogs:      []string{},
		tailLines:    []string{},
		logsExpanded: false,
		startTime:    time.Now(),
		cancelFunc:   cancelFunc,
		quitting:     false,
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
			// Don't handle quit if already done or quitting
			if m.done || m.quitting {
				return m, tea.Quit
			}
			// Cancel the context and wait for DoneMsg
			m.quitting = true
			m.status = "Stopping workflow gracefully..."
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, nil
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
		availableHeight := msg.Height - viewportReservedLines

		// Clamp viewport height between min and max
		viewportHeight := availableHeight
		if viewportHeight < viewportMinHeight {
			viewportHeight = viewportMinHeight
		}
		if viewportHeight > viewportMaxHeight {
			viewportHeight = viewportMaxHeight
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width-viewportBorderPadding, viewportHeight)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - viewportBorderPadding
			m.viewport.Height = viewportHeight
		}

	case LogMsg:
		// Append to full log history
		m.allLogs = append(m.allLogs, string(msg))

		// Update tail lines (last N lines)
		if len(m.allLogs) > tailLinesToShow {
			m.tailLines = m.allLogs[len(m.allLogs)-tailLinesToShow:]
		} else {
			m.tailLines = m.allLogs
		}

		// Mark logs as dirty instead of immediate update
		m.logsDirty = true

		return m, waitForActivity

	case ProgressMsg:
		m.status = msg.Status
		m.currentStep = msg.CurrentStep
		m.totalSteps = msg.TotalSteps

	case DoneMsg:
		m.done = true
		m.duration = msg.Duration
		m.exitCode = msg.ExitCode
		m.errors = msg.Errors
		m.Cancelled = msg.Cancelled
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
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(hintTextGray))
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

	// Update viewport only if logs changed
	if m.logsDirty {
		m.viewport.SetContent(strings.Join(m.allLogs, "\n"))
		m.viewport.GotoBottom()
		m.logsDirty = false
	}

	// Bordered viewport with logs
	logBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(logBoxBorderColor)).
		Padding(0, 1)

	b.WriteString(logBoxStyle.Render(m.viewport.View()) + "\n")

	// Toggle hint
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(hintTextGray))
	b.WriteString(hintStyle.Render("⊖ Full logs (press 'o' to collapse, 'q' to quit)\n"))

	return b.String()
}

// renderCompletionView renders the final completion summary with error report
func (m *CheckModel) renderCompletionView() string {
	var b strings.Builder

	// Determine status message
	statusIcon := "✓"
	statusColor := lipgloss.Color(checkStatusGreen)
	statusText := "Check passed"

	if m.exitCode != 0 {
		statusIcon = "✗"
		statusColor = lipgloss.Color(checkStatusRed)
		statusText = "Check failed"
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true)

	// Print status header
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s %s in %s\n", statusIcon, statusText, m.duration)))

	// If we have errors, format and display them
	if m.errors != nil {
		var errBuf bytes.Buffer
		output.FormatText(&errBuf, m.errors)
		b.WriteString(errBuf.String())
	}

	return b.String()
}

