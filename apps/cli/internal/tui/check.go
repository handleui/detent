package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/workflow"
	"github.com/handleui/shimmer"
)

// Step status constants
const (
	StepPending = "pending"
	StepRunning = "running"
	StepSuccess = "success"
	StepError   = "error"
)

// Step represents a workflow step with its status
type Step struct {
	Name       string
	Status     string
	ErrorCount int
}

// LogMsg is sent when new log content arrives (ignored in TUI mode)
type LogMsg string

// ProgressMsg updates the current status
type ProgressMsg struct {
	Status string // Step info (for display, not used for matching)
	JobID  string // Job ID for matching against known jobs
}

// DoneMsg signals completion
type DoneMsg struct {
	Duration  time.Duration
	ExitCode  int
	Errors    *errors.GroupedErrors
	Cancelled bool
}

// ErrMsg signals an error
type ErrMsg error

// CheckModel is the Bubble Tea model for the check command TUI
type CheckModel struct {
	shimmer      shimmer.Model
	steps        []Step
	jobMap       map[string]int // Maps job ID to step index
	jobNameMap   map[string]int // Maps job Name to step index (act uses names)
	currentStep  int
	done         bool
	err          error
	duration     time.Duration
	exitCode     int
	startTime    time.Time
	errors       *errors.GroupedErrors
	Cancelled    bool
	cancelFunc   func()
	quitting     bool
}

// NewCheckModel creates a new TUI model for the check command
func NewCheckModel(cancelFunc func()) CheckModel {
	return NewCheckModelWithJobs(cancelFunc, nil)
}

// NewCheckModelWithJobs creates a new TUI model with pre-populated job names
func NewCheckModelWithJobs(cancelFunc func(), jobs []workflow.JobInfo) CheckModel {
	model := CheckModel{
		shimmer:    shimmer.New("", "#FFFFFF"),
		steps:      []Step{},
		jobMap:     make(map[string]int),
		jobNameMap: make(map[string]int),
		startTime:  time.Now(),
		cancelFunc: cancelFunc,
		quitting:   false,
	}

	if len(jobs) > 0 {
		model.steps = make([]Step, len(jobs))
		for i, job := range jobs {
			model.steps[i] = Step{
				Name:   job.Name,
				Status: StepPending,
			}
			// Map by both ID and Name for flexible matching
			model.jobMap[job.ID] = i
			model.jobNameMap[job.Name] = i
		}
	}

	return model
}

// Init initializes the model
func (m *CheckModel) Init() tea.Cmd {
	return m.shimmer.Init()
}

// Update handles messages and updates the model state
func (m *CheckModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.done || m.quitting {
				return m, tea.Quit
			}
			m.quitting = true
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, tea.Quit
		}

	case ProgressMsg:
		m.updateSteps(msg)
		return m, nil

	case DoneMsg:
		m.done = true
		m.duration = msg.Duration
		m.exitCode = msg.ExitCode
		m.errors = msg.Errors
		m.Cancelled = msg.Cancelled
		m.markCurrentStepComplete()
		return m, tea.Quit

	case ErrMsg:
		m.err = msg
		return m, tea.Quit

	case shimmer.TickMsg:
		var cmd tea.Cmd
		m.shimmer, cmd = m.shimmer.Update(msg)
		return m, cmd

	case LogMsg:
		// Logs are ignored - verbose mode shows them
		return m, nil
	}

	return m, nil
}

// updateSteps updates the step list based on progress message
// Observer pattern - only update pre-populated steps, never add new ones
func (m *CheckModel) updateSteps(msg ProgressMsg) {
	if msg.JobID == "" {
		return
	}

	// Try to find step by job ID first, then by job Name
	idx := -1
	if i, ok := m.jobMap[msg.JobID]; ok {
		idx = i
	} else if i, ok := m.jobNameMap[msg.JobID]; ok {
		idx = i
	}

	// No match found - ignore (don't add new steps)
	if idx < 0 {
		return
	}

	// Mark previous running steps as complete
	for i := range m.steps {
		if m.steps[i].Status == StepRunning && i != idx {
			m.steps[i].Status = StepSuccess
		}
	}

	// Update matched step to running
	m.steps[idx].Status = StepRunning
	m.currentStep = idx + 1

	// Set shimmer to animate the step NAME
	m.shimmer = m.shimmer.SetText(m.steps[idx].Name)
}

// markCurrentStepComplete marks the current step as complete based on errors
func (m *CheckModel) markCurrentStepComplete() {
	if m.currentStep > 0 && m.currentStep <= len(m.steps) {
		idx := m.currentStep - 1
		if m.errors != nil && m.errors.Total > 0 {
			m.steps[idx].Status = StepError
			m.steps[idx].ErrorCount = m.errors.Total
		} else {
			m.steps[idx].Status = StepSuccess
		}
	}
}

// View renders the current model state as a string
func (m *CheckModel) View() string {
	if m.done {
		return m.renderCompletionView()
	}

	return m.renderStepList()
}

// renderStepList renders the step list with shimmer on current step
func (m *CheckModel) renderStepList() string {
	var b strings.Builder

	// Command header with elapsed time
	elapsed := int(time.Since(m.startTime).Seconds())
	header := fmt.Sprintf("$ act · %ds", elapsed)
	b.WriteString(SecondaryStyle.Render(header) + "\n\n")

	// Step list
	for i, step := range m.steps {
		line := m.renderStep(step, i+1 == m.currentStep)
		b.WriteString("  " + line + "\n")
	}

	return b.String()
}

// renderStep renders a single step line
func (m *CheckModel) renderStep(step Step, isCurrent bool) string {
	var icon string
	var text string

	switch step.Status {
	case StepPending:
		icon = MutedStyle.Render("·")
		text = MutedStyle.Render(step.Name)

	case StepRunning:
		icon = PrimaryStyle.Render("·")
		if isCurrent {
			text = m.shimmer.View() // Shimmer animates step NAME
		} else {
			text = PrimaryStyle.Render(step.Name)
		}

	case StepSuccess:
		icon = SuccessStyle.Render("✓")
		text = PrimaryStyle.Render(step.Name)

	case StepError:
		icon = ErrorStyle.Render("✗")
		text = PrimaryStyle.Render(step.Name)
		if step.ErrorCount > 0 {
			errText := "error"
			if step.ErrorCount > 1 {
				errText = "errors"
			}
			text += " " + ErrorStyle.Render(fmt.Sprintf("· %d %s", step.ErrorCount, errText))
		}
	}

	return fmt.Sprintf("%s %s", icon, text)
}

// renderCompletionView renders the final completion summary with error report
func (m *CheckModel) renderCompletionView() string {
	var b strings.Builder

	hasIssues := m.errors != nil && m.errors.Total > 0
	workflowFailed := m.exitCode != 0

	switch {
	case hasIssues:
		var errBuf bytes.Buffer
		output.FormatText(&errBuf, m.errors)
		b.WriteString(errBuf.String())
	case workflowFailed:
		headerStyle := ErrorStyle.Bold(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("✗ Check failed in %s\n", m.duration)))
	default:
		headerStyle := SuccessStyle.Bold(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("✓ Check passed in %s\n", m.duration)))
	}

	return b.String()
}
