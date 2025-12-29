package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// CommandResult contains the user's decision about a command.
type CommandResult struct {
	Allowed   bool
	Always    bool
	Cancelled bool
}

// CommandPromptModel prompts user to approve an unknown command.
type CommandPromptModel struct {
	command       string
	selectedIndex int
	result        *CommandResult
	quitting      bool
}

// NewCommandPromptModel creates a command approval prompt.
func NewCommandPromptModel(command string) *CommandPromptModel {
	return &CommandPromptModel{command: command}
}

// GetResult returns the user's choice.
func (m *CommandPromptModel) GetResult() *CommandResult {
	return m.result
}

// Init implements tea.Model.
func (m *CommandPromptModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *CommandPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c", "esc":
			m.result = &CommandResult{Cancelled: true}
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
			m.result = &CommandResult{
				Allowed: m.selectedIndex < 2,
				Always:  m.selectedIndex == 0,
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m *CommandPromptModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(WarningStyle.Bold(true).Render("Command Approval"))
	b.WriteString("\n\n")
	b.WriteString(SecondaryStyle.Render("Run: "))
	b.WriteString(AccentStyle.Render(m.command))
	b.WriteString("\n\n")

	options := []struct{ label, hint string }{
		{"Always allow", "saves to config"},
		{"Allow once", "this session"},
		{"Deny", "block"},
	}

	for i, opt := range options {
		cursor, style := "  ", PrimaryStyle
		if i == m.selectedIndex {
			cursor, style = "> ", SuccessStyle
		}
		b.WriteString(style.Render(cursor + opt.label))
		b.WriteString(" ")
		b.WriteString(HintStyle.Render("(" + opt.hint + ")"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(HintStyle.Render("[↑/↓ enter esc]"))
	return b.String()
}
