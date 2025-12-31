package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/ci"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/workflow"
	"github.com/handleui/shimmer"
)

// LogMsg is sent when new log content arrives (ignored in TUI mode)
type LogMsg string

// JobEventMsg wraps a ci.JobEvent for Bubble Tea message passing.
type JobEventMsg struct {
	Event *ci.JobEvent
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
	shimmer    shimmer.Model
	tracker    *JobTracker
	done       bool
	err        error
	duration   time.Duration
	exitCode   int
	startTime  time.Time
	errors     *errors.GroupedErrors
	Cancelled  bool
	cancelFunc func()
	quitting   bool
	debugLogs  []string
}

// NewCheckModel creates a new TUI model for the check command
func NewCheckModel(cancelFunc func()) CheckModel {
	return NewCheckModelWithJobs(cancelFunc, nil)
}

// NewCheckModelWithJobs creates a new TUI model with pre-populated job names
func NewCheckModelWithJobs(cancelFunc func(), jobs []workflow.JobInfo) CheckModel {
	tracker := NewJobTracker(jobs)

	// Initialize shimmer with test text to verify it works
	// Base color is grey (#585858) so shimmer wave can lighten to white
	shim := shimmer.New("Initializing...", "#585858")
	shim = shim.SetLoading(true)

	model := CheckModel{
		shimmer:    shim,
		tracker:    tracker,
		startTime:  time.Now(),
		cancelFunc: cancelFunc,
		quitting:   false,
		debugLogs:  []string{},
	}

	// Debug: log registered jobs
	for _, job := range tracker.GetJobs() {
		model.debugLogs = append(model.debugLogs, fmt.Sprintf("Registered: ID=%q Name=%q", job.ID, job.Name))
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

	case JobEventMsg:
		if msg.Event != nil {
			m.debugLogs = append(m.debugLogs, fmt.Sprintf("Event: Job=%q Action=%q Success=%v", msg.Event.JobName, msg.Event.Action, msg.Event.Success))
			changed := m.tracker.ProcessEvent(msg.Event)
			if changed {
				// Update shimmer for first running job
				for _, job := range m.tracker.GetJobs() {
					if job.Status == ci.JobRunning {
						m.shimmer = m.shimmer.SetText(job.Name).SetLoading(true)
						// Continue to shimmer update below
						break
					}
				}
			}
		}
		// Fall through to shimmer update

	case DoneMsg:
		m.done = true
		m.duration = msg.Duration
		m.exitCode = msg.ExitCode
		m.errors = msg.Errors
		m.Cancelled = msg.Cancelled
		hasErrors := m.errors != nil && m.errors.Total > 0
		m.tracker.MarkAllRunningComplete(hasErrors)

		// Print completion output using tea.Println so it persists above the TUI
		// area and won't be cleared when the program exits
		completionOutput := m.renderCompletionView()
		lines := strings.Split(completionOutput, "\n")
		cmds := make([]tea.Cmd, 0, len(lines)+1)
		for _, line := range lines {
			cmds = append(cmds, tea.Println(line))
		}
		cmds = append(cmds, tea.Quit)
		return m, tea.Batch(cmds...)

	case ErrMsg:
		m.err = msg
		// Print error using tea.Println for persistence
		errOutput := ErrorStyle.Render(fmt.Sprintf("✗ Error: %s", msg.Error()))
		return m, tea.Sequence(tea.Println(errOutput), tea.Quit)

	case LogMsg:
		// Fall through to shimmer update
	}

	// Always forward messages to shimmer to keep animation running
	var cmd tea.Cmd
	m.shimmer, cmd = m.shimmer.Update(msg)
	return m, cmd
}

// GetDebugLogs returns debug logs for troubleshooting
func (m *CheckModel) GetDebugLogs() []string {
	return m.debugLogs
}

// View renders the current model state as a string
func (m *CheckModel) View() string {
	if m.err != nil {
		return ErrorStyle.Render(fmt.Sprintf("✗ Error: %s\n", m.err.Error()))
	}

	if m.done {
		// Output was already printed using tea.Println for persistence
		return ""
	}

	return m.renderStepList()
}

// renderStepList renders the step list with shimmer on current step
func (m *CheckModel) renderStepList() string {
	var b strings.Builder

	elapsed := int(time.Since(m.startTime).Seconds())
	header := fmt.Sprintf("$ act · %ds", elapsed)
	b.WriteString(SecondaryStyle.Render(header) + "\n\n")

	jobs := m.tracker.GetJobs()
	var firstRunning *TrackedJob
	for _, job := range jobs {
		if job.Status == ci.JobRunning && firstRunning == nil {
			firstRunning = job
		}
	}

	for _, job := range jobs {
		line := m.renderJob(job, firstRunning)
		b.WriteString("  " + line + "\n")
	}

	return b.String()
}

// renderJob renders a single job line
func (m *CheckModel) renderJob(job, firstRunning *TrackedJob) string {
	var icon string
	var text string

	switch job.Status {
	case ci.JobPending:
		icon = MutedStyle.Render("·")
		text = MutedStyle.Render(job.Name)

	case ci.JobRunning:
		icon = SecondaryStyle.Render("·")
		if firstRunning != nil && job.ID == firstRunning.ID {
			text = m.shimmer.View()
		} else {
			// Other running jobs show lighter grey (secondary) to distinguish from pending (muted)
			text = SecondaryStyle.Render(job.Name)
		}

	case ci.JobSuccess:
		icon = SuccessStyle.Render("✓")
		text = PrimaryStyle.Render(job.Name)

	case ci.JobFailed:
		icon = ErrorStyle.Render("✗")
		text = PrimaryStyle.Render(job.Name)
	}

	return fmt.Sprintf("%s %s", icon, text)
}

// renderCompletionView renders the final completion summary with error report
func (m *CheckModel) renderCompletionView() string {
	var b strings.Builder

	// Show final job statuses
	header := fmt.Sprintf("$ act · %s", m.duration.Round(time.Second))
	b.WriteString(SecondaryStyle.Render(header) + "\n\n")

	for _, job := range m.tracker.GetJobs() {
		line := m.renderJob(job, nil)
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n")

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
