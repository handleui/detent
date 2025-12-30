package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/handleui/shimmer"
)

// PreflightModel is a single-line Bubble Tea model for preflight checks
type PreflightModel struct {
	shimmer  shimmer.Model
	text     string
	done     bool
	err      error
	quitting bool
}

// PreflightUpdateMsg updates the preflight status text
type PreflightUpdateMsg string

// PreflightDoneMsg signals preflight completion
type PreflightDoneMsg struct {
	Err error
}

// NewPreflightModel creates a new single-line preflight display
func NewPreflightModel() PreflightModel {
	return PreflightModel{
		shimmer: shimmer.New("Preparing", "#FFFFFF"),
	}
}

// Init initializes the preflight model
func (m PreflightModel) Init() tea.Cmd {
	return m.shimmer.Init()
}

// Update handles messages
func (m PreflightModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}

	case PreflightUpdateMsg:
		m.text = string(msg)
		m.shimmer = m.shimmer.SetText(m.text)
		return m, nil

	case PreflightDoneMsg:
		m.done = true
		m.err = msg.Err
		return m, tea.Quit

	case shimmer.TickMsg:
		var cmd tea.Cmd
		m.shimmer, cmd = m.shimmer.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the preflight line
func (m PreflightModel) View() string {
	if m.quitting {
		return ""
	}

	if m.done {
		if m.err != nil {
			return ErrorStyle.Render(fmt.Sprintf("✗ %s", m.err.Error())) + "\n"
		}
		return "" // Clear line on success, main TUI will take over
	}

	return PrimaryStyle.Render("· ") + m.shimmer.View() + "\n"
}

// WasCancelled returns true if the user quit
func (m PreflightModel) WasCancelled() bool {
	return m.quitting
}

// PreflightDisplay manages the display of pre-flight checks (legacy multi-line)
type PreflightDisplay struct {
	checks   []PreflightCheck
	rendered bool
	numLines int
}

// PreflightCheck represents a single pre-flight check
type PreflightCheck struct {
	Name   string
	Status string
	Error  error
}

// NewPreflightDisplay creates a new pre-flight display
func NewPreflightDisplay(checkNames []string) *PreflightDisplay {
	checks := make([]PreflightCheck, len(checkNames))
	for i, name := range checkNames {
		checks[i] = PreflightCheck{
			Name:   name,
			Status: "pending",
		}
	}
	return &PreflightDisplay{checks: checks}
}

// UpdateCheck updates a specific check's status
func (p *PreflightDisplay) UpdateCheck(name, status string, err error) {
	for i := range p.checks {
		if p.checks[i].Name == name {
			p.checks[i].Status = status
			p.checks[i].Error = err
			break
		}
	}
}

// Render renders all checks to stderr
func (p *PreflightDisplay) Render() {
	// Single line: show current running check
	var current string
	for _, check := range p.checks {
		if check.Status == "running" {
			current = check.Name
			break
		}
	}

	if p.rendered {
		fmt.Fprint(os.Stderr, "\033[1A\033[J")
	}

	if current != "" {
		fmt.Fprintln(os.Stderr, PrimaryStyle.Render("· "+current))
	}

	p.rendered = true
	p.numLines = 1
}

// RenderFinal renders the final state
func (p *PreflightDisplay) RenderFinal() {
	if p.rendered {
		fmt.Fprint(os.Stderr, "\033[1A\033[J")
	}

	// Find error if any
	for _, check := range p.checks {
		if check.Status != "error" || check.Error == nil {
			continue
		}
		fmt.Fprintln(os.Stderr, ErrorStyle.Render(fmt.Sprintf("✗ %s", check.Error.Error())))
		fmt.Fprintln(os.Stderr)
		p.rendered = true
		p.numLines = 2
		return
	}

	p.rendered = true
	p.numLines = 0
}

// Clear removes the display from screen
func (p *PreflightDisplay) Clear() {
	if p.rendered && p.numLines > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA\033[J", p.numLines)
	}
	p.rendered = false
	p.numLines = 0
}
