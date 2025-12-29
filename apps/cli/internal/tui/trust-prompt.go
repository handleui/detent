package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// TrustPromptInfo contains information to display in the trust prompt.
type TrustPromptInfo struct {
	RemoteURL      string // e.g., "github.com/user/repo" or empty for local repos
	FirstCommitSHA string // Short SHA for display (e.g., "abc123def456")
}

// TrustPromptResult contains the user's decision.
type TrustPromptResult struct {
	Trusted   bool
	Cancelled bool
}

// TrustPromptModel is a Bubble Tea model for prompting the user
// to trust a repository before executing commands from it.
type TrustPromptModel struct {
	info          TrustPromptInfo
	selectedIndex int // 0 = Yes, 1 = No
	result        *TrustPromptResult
	quitting      bool
}

var (
	trustTitleStyle   = BoldPrimaryStyle
	trustTextStyle    = SecondaryStyle
	trustInfoStyle    = MutedStyle
	trustSelectedStyle = SuccessStyle
	trustNormalStyle  = PrimaryStyle
	trustHintStyle    = HintStyle
)

// NewTrustPromptModel creates a new trust prompt model.
func NewTrustPromptModel(info TrustPromptInfo) *TrustPromptModel {
	return &TrustPromptModel{
		info:          info,
		selectedIndex: 0, // Default to "Yes"
	}
}

// GetResult returns the user's choice after the prompt completes.
func (m *TrustPromptModel) GetResult() *TrustPromptResult {
	return m.result
}

// Init implements tea.Model.
func (m *TrustPromptModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *TrustPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyPress(keyMsg)
	}
	return m, nil
}

// handleKeyPress processes keyboard input.
func (m *TrustPromptModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.result = &TrustPromptResult{Cancelled: true}
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}

	case "down", "j":
		if m.selectedIndex < 1 {
			m.selectedIndex++
		}

	case "enter":
		// User made an explicit choice - this is NOT a cancellation.
		// Cancelled=true is only for ctrl+c/esc (abort without deciding).
		m.result = &TrustPromptResult{
			Trusted:   m.selectedIndex == 0,
			Cancelled: false,
		}
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model.
func (m *TrustPromptModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(trustTitleStyle.Render("Repository Trust Required"))
	b.WriteString("\n\n")

	b.WriteString(trustTextStyle.Render("Detent will execute commands from this repository's configuration"))
	b.WriteString("\n")
	b.WriteString(trustTextStyle.Render("(Makefiles, package.json scripts, etc.)."))
	b.WriteString("\n\n")

	// Show repository info
	if m.info.RemoteURL != "" {
		b.WriteString(trustTextStyle.Render("Repository: "))
		b.WriteString(trustInfoStyle.Render(m.info.RemoteURL))
		b.WriteString("\n")
	}
	b.WriteString(trustTextStyle.Render("First commit: "))
	b.WriteString(trustInfoStyle.Render(m.info.FirstCommitSHA))
	b.WriteString("\n\n")

	// Menu options
	options := []string{"Yes, trust this repository", "No, cancel"}

	for i, option := range options {
		cursor := "  "
		style := trustNormalStyle
		if i == m.selectedIndex {
			cursor = "> "
			style = trustSelectedStyle
		}
		b.WriteString(style.Render(cursor + option))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(trustHintStyle.Render("[up/down to select, enter to confirm, esc to cancel]"))

	return b.String()
}
