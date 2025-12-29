package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/persistence"
)

// Available models to cycle through.
var availableModels = []string{
	"claude-sonnet-4-5",
	"claude-opus-4-5",
	"claude-haiku-4-5",
}

// EditMode indicates the current editing state.
type EditMode int

// Edit modes.
const (
	ModeView EditMode = iota
	ModeEditText
)

// Model is the Bubble Tea model for interactive config editing.
type Model struct {
	// Data
	config *persistence.ConfigWithSources
	values map[string]FieldValue

	// Navigation
	cursor int // Currently highlighted field index

	// Editing
	mode      EditMode
	textInput textinput.Model
	editField string // Which field is being edited

	// State
	inRepo         bool   // true if in a git repository
	globalPath     string // Path to global config
	localPath      string // Path to local config
	version        string // App version for header
	repoIdentifier string // owner/repo for header
	quitting       bool
	dirty          bool // Unsaved changes exist
	saved          bool // Changes were saved during this session
}

// Options for creating a new Model.
type Options struct {
	InRepo         bool
	GlobalPath     string
	LocalPath      string
	Version        string
	RepoIdentifier string
}

// NewModel creates a new config model.
func NewModel(cfg *persistence.ConfigWithSources, opts Options) *Model {
	ti := textinput.New()
	ti.Placeholder = "enter value"
	ti.Width = 30

	return &Model{
		config:         cfg,
		values:         GetFieldValues(cfg),
		cursor:         0,
		mode:           ModeView,
		textInput:      ti,
		inRepo:         opts.InRepo,
		globalPath:     opts.GlobalPath,
		localPath:      opts.LocalPath,
		version:        opts.Version,
		repoIdentifier: opts.RepoIdentifier,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle text input mode
	if m.mode == ModeEditText {
		return m.updateTextInput(msg)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKeyPress(keyMsg)
	}

	// Handle editor closed message
	if _, ok := msg.(editorClosedMsg); ok {
		// Reload config and clear screen to avoid duplication
		m.config, _ = persistence.LoadWithSources(m.config.RepoRoot)
		m.values = GetFieldValues(m.config)
		m.dirty = false
		return m, tea.ClearScreen
	}

	return m, nil
}

// updateTextInput handles input during text editing mode.
func (m *Model) updateTextInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.mode = ModeView
			m.textInput.Blur()
			return m, nil
		case "enter":
			m.saveTextInput()
			m.mode = ModeView
			m.textInput.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// editorClosedMsg is sent when the editor process exits.
type editorClosedMsg struct {
	err error
}

// handleKeyPress processes keyboard input in view mode.
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		if m.dirty {
			// Auto-save before quitting
			_ = m.saveConfig()
		}
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(EditableFields)-1 {
			m.cursor++
		}

	case "left", "h":
		m.handleLeft()

	case "right", "l":
		m.handleRight()

	case "enter", " ":
		m.handleEnter()

	case "e":
		cmd := m.openInEditor()
		return m, cmd
	}

	return m, nil
}

// handleLeft handles left arrow / h key for cycling/decrementing.
func (m *Model) handleLeft() {
	if m.cursor < 0 || m.cursor >= len(EditableFields) {
		return
	}

	field := EditableFields[m.cursor]
	switch field.Key {
	case "model":
		m.cycleModel(-1)
	case "timeout":
		m.adjustTimeout(-1)
	}
}

// handleRight handles right arrow / l key for cycling/incrementing.
func (m *Model) handleRight() {
	if m.cursor < 0 || m.cursor >= len(EditableFields) {
		return
	}

	field := EditableFields[m.cursor]
	switch field.Key {
	case "model":
		m.cycleModel(1)
	case "timeout":
		m.adjustTimeout(1)
	}
}

// handleEnter handles enter/space for toggling or opening text input.
func (m *Model) handleEnter() {
	if m.cursor < 0 || m.cursor >= len(EditableFields) {
		return
	}

	field := EditableFields[m.cursor]
	switch field.Key {
	case "verbose":
		m.toggleVerbose()
	case "budget":
		m.startTextEdit("budget", formatBudgetRaw(m.config.BudgetUSD.Value))
	case "api_key":
		m.startTextEdit("api_key", "")
		m.textInput.EchoMode = textinput.EchoPassword
		m.textInput.EchoCharacter = '•'
	}
}

// cycleModel cycles through available models.
func (m *Model) cycleModel(direction int) {
	current := m.config.Model.Value
	currentIndex := 0
	for i, model := range availableModels {
		if model == current {
			currentIndex = i
			break
		}
	}

	newIndex := (currentIndex + direction + len(availableModels)) % len(availableModels)
	m.config.Model.Value = availableModels[newIndex]
	m.config.Model.Source = persistence.SourceLocal
	m.values = GetFieldValues(m.config)
	m.dirty = true
}

// adjustTimeout increments or decrements timeout.
func (m *Model) adjustTimeout(delta int) {
	newValue := m.config.TimeoutMins.Value + delta
	if newValue < 1 {
		newValue = 1
	}
	if newValue > 60 {
		newValue = 60
	}
	m.config.TimeoutMins.Value = newValue
	m.config.TimeoutMins.Source = persistence.SourceLocal
	m.values = GetFieldValues(m.config)
	m.dirty = true
}

// toggleVerbose toggles the verbose flag.
func (m *Model) toggleVerbose() {
	m.config.Verbose.Value = !m.config.Verbose.Value
	m.config.Verbose.Source = persistence.SourceLocal
	m.values = GetFieldValues(m.config)
	m.dirty = true
}

// startTextEdit begins text input mode for a field.
func (m *Model) startTextEdit(fieldKey, initialValue string) {
	m.editField = fieldKey
	m.textInput.SetValue(initialValue)
	m.textInput.Focus()
	m.textInput.EchoMode = textinput.EchoNormal
	m.mode = ModeEditText
}

// saveTextInput saves the current text input value.
func (m *Model) saveTextInput() {
	value := strings.TrimSpace(m.textInput.Value())

	switch m.editField {
	case "budget":
		// Parse USD value
		value = strings.TrimPrefix(value, "$")
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			if f < 0 {
				f = 0
			}
			if f > 100 {
				f = 100
			}
			m.config.BudgetUSD.Value = f
			m.config.BudgetUSD.Source = persistence.SourceLocal
			m.dirty = true
		}
	case "api_key":
		if value != "" {
			m.config.APIKey.Value = value
			m.config.APIKey.Source = persistence.SourceGlobal // API key always global
			m.dirty = true
		}
	}

	m.values = GetFieldValues(m.config)
}

// saveConfig persists changes to disk.
func (m *Model) saveConfig() error {
	if !m.dirty {
		return nil
	}

	// Update the underlying config structs
	if m.config.Local == nil {
		m.config.Local = &persistence.LocalConfig{}
	}

	// Apply changes to local config (except API key)
	m.config.Local.Model = m.config.Model.Value
	timeout := m.config.TimeoutMins.Value
	m.config.Local.TimeoutMins = &timeout
	budget := m.config.BudgetUSD.Value
	m.config.Local.BudgetUSD = &budget
	verbose := m.config.Verbose.Value
	m.config.Local.Verbose = &verbose

	// Save local config
	if m.config.RepoRoot != "" {
		if err := persistence.SaveLocalWithSources(m.config); err != nil {
			return err
		}
	}

	// API key goes to global - load config and save
	if m.config.APIKey.Value != "" {
		cfg, err := persistence.Load("")
		if err == nil {
			_ = cfg.SetAPIKey(m.config.APIKey.Value)
		}
	}

	m.dirty = false
	m.saved = true
	return nil
}

// WasSaved returns true if changes were saved during this session.
func (m *Model) WasSaved() bool {
	return m.saved
}

// openInEditor opens the config file in the user's editor.
func (m *Model) openInEditor() tea.Cmd {
	// Save any pending changes first
	if m.dirty {
		_ = m.saveConfig()
	}

	// Open local config if in repo, otherwise global
	path := m.localPath
	if !m.inRepo || m.localPath == "" {
		path = m.globalPath
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "code"
	}

	c := exec.Command(editor, path) //nolint:gosec // User-controlled editor is intentional
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorClosedMsg{err}
	})
}

// GetGlobalPath returns the global config file path.
func GetGlobalPath() string {
	path, _ := persistence.GetConfigPath()
	return path
}

// GetLocalPath returns the local config file path for a given repo root.
func GetLocalPath(repoRoot string) string {
	return filepath.Join(repoRoot, "detent.jsonc")
}

// formatBudgetRaw returns the raw budget value for editing.
func formatBudgetRaw(usd float64) string {
	if usd == 0 {
		return "0"
	}
	return strconv.FormatFloat(usd, 'f', 2, 64)
}
