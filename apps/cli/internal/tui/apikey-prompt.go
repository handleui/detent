package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// APIKeyResult contains the result of the API key prompt.
type APIKeyResult struct {
	Key       string
	Cancelled bool
}

// APIKeyPromptModel is a Bubble Tea model for prompting the user
// to enter their Anthropic API key.
type APIKeyPromptModel struct {
	textInput  textinput.Model
	result     *APIKeyResult
	quitting   bool
	inputError string
}

var (
	apiKeyTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208")) // Anthropic orange
	apiKeyTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	apiKeyHintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	apiKeyErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	apiKeyDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// NewAPIKeyPromptModel creates a new API key prompt model.
func NewAPIKeyPromptModel() *APIKeyPromptModel {
	ti := textinput.New()
	ti.Placeholder = "sk-ant-api03-..."
	ti.Focus()
	ti.CharLimit = 200
	ti.Width = 60
	ti.EchoMode = textinput.EchoPassword // Hide the key as user types
	ti.EchoCharacter = '•'

	return &APIKeyPromptModel{
		textInput: ti,
	}
}

// GetResult returns the user's input after the prompt completes.
func (m *APIKeyPromptModel) GetResult() *APIKeyResult {
	return m.result
}

// Init implements tea.Model.
func (m *APIKeyPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (m *APIKeyPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyPress(keyMsg)
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// handleKeyPress processes keyboard input.
func (m *APIKeyPromptModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.result = &APIKeyResult{Cancelled: true}
		m.quitting = true
		return m, tea.Quit

	case "enter":
		key := strings.TrimSpace(m.textInput.Value())
		if key == "" {
			m.inputError = "API key cannot be empty"
			return m, nil
		}
		if !isValidAPIKey(key) {
			m.inputError = "invalid API key format (should start with sk-ant-)"
			return m, nil
		}
		m.result = &APIKeyResult{
			Key:       key,
			Cancelled: false,
		}
		m.quitting = true
		return m, tea.Quit
	}

	// Clear error on typing
	m.inputError = ""

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m *APIKeyPromptModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(apiKeyTitleStyle.Render("Anthropic API Key Required"))
	b.WriteString("\n\n")

	b.WriteString(apiKeyTextStyle.Render("Detent uses Claude to automatically fix CI errors."))
	b.WriteString("\n")
	b.WriteString(apiKeyTextStyle.Render("Get your API key from: "))
	b.WriteString(apiKeyDimStyle.Render("https://console.anthropic.com/settings/keys"))
	b.WriteString("\n\n")

	b.WriteString(apiKeyTextStyle.Render("Enter your API key:"))
	b.WriteString("\n\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n")

	if m.inputError != "" {
		b.WriteString(apiKeyErrorStyle.Render(m.inputError))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(apiKeyHintStyle.Render("[enter] save  [esc] cancel"))

	return b.String()
}

// isValidAPIKey performs basic validation on the API key format.
func isValidAPIKey(key string) bool {
	// Anthropic API keys start with "sk-ant-"
	return strings.HasPrefix(key, "sk-ant-") && len(key) > 20
}
