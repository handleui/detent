package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/handleui/shimmer"
)

// PreflightModel is a single-line Bubble Tea model for preflight checks
type PreflightModel struct {
	shimmer  shimmer.Model
	done     bool
	err      error
	quitting bool
}

// PreflightUpdateMsg updates the preflight status text (ignored - fixed message)
type PreflightUpdateMsg string

// PreflightDoneMsg signals preflight completion
type PreflightDoneMsg struct {
	Err error
}

// NewPreflightModel creates a new single-line preflight display
func NewPreflightModel() PreflightModel {
	return PreflightModel{
		shimmer: shimmer.New("Running preflight checks", "#8a8a8a"),
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
		// Ignored - using fixed message
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
	if m.quitting || m.done {
		if m.err != nil {
			return ErrorStyle.Render(fmt.Sprintf("✗ %s", m.err.Error())) + "\n\n"
		}
		return "" // Clear line on success, main TUI takes over
	}

	return MutedStyle.Render("· ") + m.shimmer.View() + "\n"
}

// WasCancelled returns true if the user quit
func (m PreflightModel) WasCancelled() bool {
	return m.quitting
}
