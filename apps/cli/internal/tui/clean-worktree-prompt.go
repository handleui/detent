package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	maxDisplayedFiles = 10
)

// CleanWorktreeAction represents the user's choice for how to handle uncommitted changes
type CleanWorktreeAction int

const (
	// ActionCommit indicates the user wants to commit their changes
	ActionCommit CleanWorktreeAction = iota
	// ActionStash indicates the user wants to stash their changes (with warning)
	ActionStash
	// ActionCancel indicates the user wants to cancel the operation
	ActionCancel
)

// CleanWorktreeResult contains the user's decision about how to proceed
type CleanWorktreeResult struct {
	Action        CleanWorktreeAction
	CommitMessage string // Only set if Action == ActionCommit
	Cancelled     bool
}

// CleanWorktreePromptModel is a Bubble Tea model for prompting the user
// to clean their worktree (commit, stash, or cancel)
type CleanWorktreePromptModel struct {
	files         []string // Dirty files to display
	step          int      // State machine step: 0=choose action, 1=input message
	selectedIndex int      // Menu cursor position (for step 0)
	action        CleanWorktreeAction
	textInput     textinput.Model
	result        *CleanWorktreeResult
	quitting      bool
}

var (
	promptTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	promptTextStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	promptFileStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	promptSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess))
	promptNormalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	promptHintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
)

// NewCleanWorktreePromptModel creates a new prompt model with the given dirty files
func NewCleanWorktreePromptModel(files []string) *CleanWorktreePromptModel {
	ti := textinput.New()
	ti.Placeholder = "Enter commit message..."
	ti.Focus()
	ti.CharLimit = 200
	ti.Width = 50

	return &CleanWorktreePromptModel{
		files:     files,
		step:      0,
		textInput: ti,
	}
}

// GetResult returns the user's choice after the prompt completes
func (m *CleanWorktreePromptModel) GetResult() *CleanWorktreeResult {
	return m.result
}

// Init implements tea.Model
func (m *CleanWorktreePromptModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m *CleanWorktreePromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyPress(keyMsg)
	}

	// Update textinput for step 1 (commit message input)
	if m.step == 1 {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleKeyPress processes keyboard input based on current step
func (m *CleanWorktreePromptModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case 0: // Choose action
		return m.handleActionSelection(msg)
	case 1: // Input commit message
		return m.handleCommitMessageInput(msg)
	default:
		return m, nil
	}
}

// handleActionSelection handles key presses for selecting an action (step 0)
func (m *CleanWorktreePromptModel) handleActionSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.result = &CleanWorktreeResult{Cancelled: true}
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
		m.action = CleanWorktreeAction(m.selectedIndex)
		switch m.action {
		case ActionCommit:
			// Move to commit message input
			m.step = 1
			m.textInput.Focus()
		case ActionStash:
			// Execute stash immediately
			m.result = &CleanWorktreeResult{
				Action:    ActionStash,
				Cancelled: false,
			}
			m.quitting = true
			return m, tea.Quit
		case ActionCancel:
			m.result = &CleanWorktreeResult{
				Action:    ActionCancel,
				Cancelled: true,
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// handleCommitMessageInput handles key presses for commit message input (step 1)
func (m *CleanWorktreePromptModel) handleCommitMessageInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.result = &CleanWorktreeResult{Cancelled: true}
		m.quitting = true
		return m, tea.Quit

	case "esc":
		// Go back to action selection
		m.step = 0
		return m, nil

	case "enter":
		message := strings.TrimSpace(m.textInput.Value())
		if message == "" {
			return m, nil
		}
		m.result = &CleanWorktreeResult{
			Action:        ActionCommit,
			CommitMessage: message,
			Cancelled:     false,
		}
		m.quitting = true
		return m, tea.Quit
	}

	// Update text input
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View implements tea.Model
func (m *CleanWorktreePromptModel) View() string {
	if m.quitting {
		return ""
	}

	var content strings.Builder
	content.WriteString("\n")

	switch m.step {
	case 0:
		content.WriteString(m.renderActionSelection())
	case 1:
		content.WriteString(m.renderCommitMessageInput())
	}

	return content.String()
}

// renderActionSelection renders the initial action selection screen (step 0)
func (m *CleanWorktreePromptModel) renderActionSelection() string {
	var b strings.Builder

	b.WriteString(promptTitleStyle.Render("Uncommitted changes detected"))
	b.WriteString("\n\n")

	b.WriteString(promptTextStyle.Render(
		"Detent requires a clean worktree to ensure the tested state",
	))
	b.WriteString("\n")
	b.WriteString(promptTextStyle.Render(
		"matches what will run in CI/CD.",
	))
	b.WriteString("\n\n")

	b.WriteString(promptTextStyle.Render("Uncommitted files:"))
	b.WriteString("\n")
	b.WriteString(m.renderFileList())
	b.WriteString("\n")

	b.WriteString(promptTextStyle.Render("How would you like to proceed?"))
	b.WriteString("\n\n")

	// Menu options
	options := []string{
		"Commit changes (recommended)",
		"Stash changes (run will test code WITHOUT stashed changes)",
		"Cancel",
	}

	for i, option := range options {
		cursor := "  "
		style := promptNormalStyle
		if i == m.selectedIndex {
			cursor = "> "
			style = promptSelectedStyle
		}
		b.WriteString(style.Render(cursor + option))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(promptHintStyle.Render("[up/down to select, enter to confirm, esc to cancel]"))

	return b.String()
}

// renderCommitMessageInput renders the commit message input screen (step 1)
func (m *CleanWorktreePromptModel) renderCommitMessageInput() string {
	var b strings.Builder

	b.WriteString(promptTitleStyle.Render("Commit changes"))
	b.WriteString("\n\n")
	b.WriteString(promptTextStyle.Render("Enter commit message:"))
	b.WriteString("\n\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(promptHintStyle.Render("[enter to commit, esc to go back]"))

	return b.String()
}

// renderFileList renders the list of dirty files
func (m *CleanWorktreePromptModel) renderFileList() string {
	var b strings.Builder

	displayCount := len(m.files)
	hasMore := false
	if displayCount > maxDisplayedFiles {
		displayCount = maxDisplayedFiles
		hasMore = true
	}

	for i := 0; i < displayCount; i++ {
		b.WriteString(promptFileStyle.Render(fmt.Sprintf("  %s", m.files[i])))
		b.WriteString("\n")
	}

	if hasMore {
		remaining := len(m.files) - maxDisplayedFiles
		b.WriteString(promptFileStyle.Render(fmt.Sprintf("  ... and %d more files", remaining)))
		b.WriteString("\n")
	}

	return b.String()
}
