package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MakeTargetResult contains the user's decision about a make target.
type MakeTargetResult struct {
	Allowed    bool // Target is allowed (either always or session)
	Always     bool // If Allowed, persist to config (vs session-only)
	Cancelled  bool // User aborted with ctrl+c/esc
}

// MakeTargetPromptModel is a Bubble Tea model for prompting the user
// to approve an unknown make target.
type MakeTargetPromptModel struct {
	target        string
	selectedIndex int // 0 = Always, 1 = Session, 2 = Deny
	result        *MakeTargetResult
	quitting      bool
}

var (
	makeTargetTitleStyle    = WarningStyle.Bold(true)
	makeTargetTextStyle     = SecondaryStyle
	makeTargetCommandStyle  = AccentStyle
	makeTargetSelectedStyle = SuccessStyle
	makeTargetNormalStyle   = PrimaryStyle
	makeTargetHintStyle     = HintStyle
)

// NewMakeTargetPromptModel creates a new make target prompt model.
func NewMakeTargetPromptModel(target string) *MakeTargetPromptModel {
	return &MakeTargetPromptModel{
		target:        target,
		selectedIndex: 0, // Default to "Allow"
	}
}

// GetResult returns the user's choice after the prompt completes.
func (m *MakeTargetPromptModel) GetResult() *MakeTargetResult {
	return m.result
}

// Init implements tea.Model.
func (m *MakeTargetPromptModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *MakeTargetPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyPress(keyMsg)
	}
	return m, nil
}

// handleKeyPress processes keyboard input.
func (m *MakeTargetPromptModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.result = &MakeTargetResult{Cancelled: true}
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}

	case "down", "j":
		if m.selectedIndex < 2 {
			m.selectedIndex++
		}

	case "enter":
		m.result = &MakeTargetResult{
			Allowed:   m.selectedIndex < 2, // 0 or 1 = allowed
			Always:    m.selectedIndex == 0,
			Cancelled: false,
		}
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model.
func (m *MakeTargetPromptModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(makeTargetTitleStyle.Render("Unknown Make Target"))
	b.WriteString("\n\n")

	b.WriteString(makeTargetTextStyle.Render("The AI wants to run: "))
	b.WriteString(makeTargetCommandStyle.Render(fmt.Sprintf("make %s", m.target)))
	b.WriteString("\n\n")

	b.WriteString(makeTargetTextStyle.Render("This target is not in the standard allowlist."))
	b.WriteString("\n\n")

	// Menu options - visually distinct to prevent user mistakes
	type menuOption struct {
		label string
		hint  string
	}
	options := []menuOption{
		{"Always allow", "saves to config"},
		{"Allow once", "this session only"},
		{"Deny", "block this target"},
	}

	for i, opt := range options {
		cursor := "  "
		style := makeTargetNormalStyle
		if i == m.selectedIndex {
			cursor = "> "
			style = makeTargetSelectedStyle
		}
		b.WriteString(style.Render(cursor + opt.label))
		b.WriteString(" ")
		b.WriteString(makeTargetHintStyle.Render("(" + opt.hint + ")"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(makeTargetHintStyle.Render("[up/down to select, enter to confirm, esc to cancel]"))

	return b.String()
}
