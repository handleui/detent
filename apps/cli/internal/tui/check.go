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
	logs         []string
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
}

// NewCheckModel creates a new TUI model for the check command
func NewCheckModel() CheckModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return CheckModel{
		spinner: s,
		status:  "Initializing...",
		logs:    []string{},
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
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 4 // Space for status and progress
		footerHeight := 1 // Space for quit instruction when done
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMargin)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargin
		}

	case LogMsg:
		m.logs = append(m.logs, string(msg))
		m.viewport.SetContent(strings.Join(m.logs, "\n"))
		m.viewport.GotoBottom()
		return m, waitForActivity

	case ProgressMsg:
		m.status = msg.Status
		m.currentStep = msg.CurrentStep
		m.totalSteps = msg.TotalSteps

	case DoneMsg:
		m.done = true
		m.duration = msg.Duration
		m.exitCode = msg.ExitCode
		return m, tea.Quit

	case ErrMsg:
		m.err = msg
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
	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Header section (no box)
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	if m.done {
		// Show completion status
		statusIcon := "✓"
		statusColor := lipgloss.Color("42")
		if m.exitCode != 0 {
			statusIcon = "✗"
			statusColor = lipgloss.Color("196")
		}

		completionStyle := lipgloss.NewStyle().
			Foreground(statusColor).
			Bold(true)

		b.WriteString(completionStyle.Render(fmt.Sprintf("%s Completed in %s (exit code %d)\n\n", statusIcon, m.duration, m.exitCode)))
	} else {
		// Show running status with spinner
		if m.totalSteps > 0 {
			progress := float64(m.currentStep) / float64(m.totalSteps)
			progressBar := renderProgressBar(progress, 40)
			b.WriteString(headerStyle.Render(fmt.Sprintf("Running: %s (step %d/%d)\n", m.status, m.currentStep, m.totalSteps)))
			b.WriteString(progressBar + "\n\n")
		} else {
			b.WriteString(fmt.Sprintf("%s %s\n\n", m.spinner.View(), statusStyle.Render(m.status)))
		}
	}

	// Logs viewport with border
	logBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)

	b.WriteString(logBoxStyle.Render(m.viewport.View()))

	// Footer
	if m.done {
		b.WriteString("\n")
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		b.WriteString(helpStyle.Render("(press q to quit)"))
	}

	return b.String()
}

// renderProgressBar creates a text-based progress bar
func renderProgressBar(progress float64, width int) string {
	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("━", filled) + strings.Repeat("─", width-filled)
	percentage := int(progress * 100)

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	styledBar := barStyle.Render(bar[:filled]) + emptyStyle.Render(bar[filled:])

	return fmt.Sprintf("%s %3d%%", styledBar, percentage)
}
