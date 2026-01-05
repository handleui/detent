package workflows

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/go-cli/internal/persistence"
	"github.com/detent/go-cli/internal/tui"
)

// JobState constants match persistence.JobState* constants.
const (
	StateAuto = ""     // Default behavior (sensitive jobs skip, others run)
	StateRun  = "run"  // Force job to run (bypass security skip)
	StateSkip = "skip" // Force job to skip
)

// JobItem represents a job for display in the TUI.
type JobItem struct {
	ID        string // Job ID from workflow
	Name      string // Display name
	Sensitive bool   // Whether job is sensitive (publish/deploy/release)
	State     string // Current override state ("", "run", "skip")
}

// Model is the Bubble Tea model for workflow job configuration.
type Model struct {
	jobs     []JobItem
	cursor   int
	quitting bool
	saved    bool
}

// Options for creating a new Model.
type Options struct {
	Jobs      []JobItem
	Overrides map[string]string
}

// NewModel creates a new workflows model.
func NewModel(opts Options) *Model {
	jobs := make([]JobItem, len(opts.Jobs))
	copy(jobs, opts.Jobs)

	// Apply existing overrides
	if opts.Overrides != nil {
		for i := range jobs {
			if state, ok := opts.Overrides[jobs[i].ID]; ok {
				jobs[i].State = state
			}
		}
	}

	return &Model{
		jobs:   jobs,
		cursor: 0,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyPress(keyMsg)
	}
	return m, nil
}

// handleKeyPress processes keyboard input.
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "s":
		// Save and quit
		m.saved = true
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.jobs)-1 {
			m.cursor++
		}

	case "left", "h":
		m.cycleState(-1)

	case "right", "l", "enter", " ":
		m.cycleState(1)
	}

	return m, nil
}

// cycleState cycles through auto -> run -> skip states.
func (m *Model) cycleState(direction int) {
	if len(m.jobs) == 0 || m.cursor < 0 || m.cursor >= len(m.jobs) {
		return
	}

	states := []string{StateAuto, StateRun, StateSkip}
	current := m.jobs[m.cursor].State

	// Find current index
	currentIndex := 0
	for i, s := range states {
		if s == current {
			currentIndex = i
			break
		}
	}

	// Cycle
	newIndex := (currentIndex + direction + len(states)) % len(states)
	m.jobs[m.cursor].State = states[newIndex]
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Job list
	for i, job := range m.jobs {
		b.WriteString(m.renderJob(i, job))
		b.WriteString("\n")
	}

	// Help footer
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderJob renders a single job row.
func (m *Model) renderJob(index int, job JobItem) string {
	isSelected := index == m.cursor

	// Cursor
	cursor := "  "
	if isSelected {
		cursor = tui.BrandStyle.Render("> ")
	}

	// State indicator with color
	var stateDisplay string
	switch job.State {
	case StateRun:
		if isSelected {
			stateDisplay = tui.BrandStyle.Render("[run] ")
		} else {
			stateDisplay = tui.SuccessStyle.Render("[run] ")
		}
	case StateSkip:
		if isSelected {
			stateDisplay = tui.BrandStyle.Render("[skip]")
		} else {
			stateDisplay = tui.WarningStyle.Render("[skip]")
		}
	default:
		if isSelected {
			stateDisplay = tui.BrandStyle.Render("[auto]")
		} else {
			stateDisplay = tui.MutedStyle.Render("[auto]")
		}
	}

	// Sensitive indicator
	sensitiveIcon := "  "
	if job.Sensitive {
		sensitiveIcon = tui.WarningStyle.Render("! ")
	}

	// Job name styling
	var nameDisplay string
	if isSelected {
		nameDisplay = tui.BrandStyle.Bold(true).Render(job.Name)
	} else {
		nameDisplay = tui.PrimaryStyle.Render(job.Name)
	}

	// Job ID if different from name
	var idDisplay string
	if job.ID != job.Name {
		idDisplay = " " + tui.MutedStyle.Render("("+job.ID+")")
	}

	return cursor + stateDisplay + " " + sensitiveIcon + nameDisplay + idDisplay
}

// renderHelp renders the keyboard shortcuts.
func (m *Model) renderHelp() string {
	return tui.HintStyle.Render("[j/k] navigate  [enter/h/l] cycle state  [s] save  [q] cancel")
}

// WasSaved returns true if changes were saved.
func (m *Model) WasSaved() bool {
	return m.saved
}

// GetOverrides returns the current job overrides as a map.
// Only non-auto states are included.
func (m *Model) GetOverrides() map[string]string {
	overrides := make(map[string]string)
	for _, job := range m.jobs {
		if job.State != StateAuto {
			overrides[job.ID] = job.State
		}
	}
	// Return nil if empty to clean up config
	if len(overrides) == 0 {
		return nil
	}
	return overrides
}

// StateLabel returns a human-readable label for a state.
func StateLabel(state string) string {
	switch state {
	case persistence.JobStateRun:
		return "run"
	case persistence.JobStateSkip:
		return "skip"
	default:
		return "auto"
	}
}
