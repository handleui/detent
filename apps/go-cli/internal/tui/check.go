package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/go-cli/internal/output"
	"github.com/detentsh/core/ci"
	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/workflow"
	"github.com/handleui/shimmer"
)

// LogMsg is sent when new log content arrives (ignored in TUI mode)
type LogMsg string

// JobEventMsg wraps a ci.JobEvent for Bubble Tea message passing.
type JobEventMsg struct {
	Event *ci.JobEvent
}

// StepEventMsg wraps a ci.StepEvent for Bubble Tea message passing.
type StepEventMsg struct {
	Event *ci.StepEvent
}

// ManifestMsg wraps a ci.ManifestEvent for Bubble Tea message passing.
// This initializes the TUI with all job and step information.
type ManifestMsg struct {
	Manifest *ci.ManifestInfo
}

// DoneMsg signals completion
type DoneMsg struct {
	Duration  time.Duration
	ExitCode  int
	Errors    *errors.ComprehensiveErrorGroup
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
	errors     *errors.ComprehensiveErrorGroup
	Cancelled  bool
	cancelFunc func()
	quitting   bool
	debugLogs  []string
	waiting    bool // True before manifest is received
}

// NewCheckModel creates a new TUI model for the check command.
// TUI starts in waiting state until manifest is received.
func NewCheckModel(cancelFunc func()) CheckModel {
	// Initialize shimmer with waiting message
	// Base color is grey (#585858) so shimmer wave can lighten to white
	shim := shimmer.New("Waiting for workflow", "#585858")
	shim = shim.SetLoading(true)

	return CheckModel{
		shimmer:    shim,
		tracker:    nil, // Will be initialized from manifest
		startTime:  time.Now(),
		cancelFunc: cancelFunc,
		quitting:   false,
		debugLogs:  []string{},
		waiting:    true,
	}
}

// NewCheckModelWithJobs creates a new TUI model with pre-populated job names.
// This is the legacy constructor for backward compatibility.
func NewCheckModelWithJobs(cancelFunc func(), jobs []workflow.JobInfo) CheckModel {
	tracker := NewJobTracker(jobs)

	// Initialize shimmer
	shim := shimmer.New("Initializing...", "#585858")
	shim = shim.SetLoading(true)

	model := CheckModel{
		shimmer:    shim,
		tracker:    tracker,
		startTime:  time.Now(),
		cancelFunc: cancelFunc,
		quitting:   false,
		debugLogs:  []string{},
		waiting:    len(jobs) == 0, // Wait if no jobs provided
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

	case ManifestMsg:
		if msg.Manifest != nil {
			m.debugLogs = append(m.debugLogs, fmt.Sprintf("Manifest received: %d jobs", len(msg.Manifest.Jobs)))
			m.tracker = NewJobTrackerFromManifest(msg.Manifest)
			m.waiting = false

			// Update shimmer with first job name
			if len(m.tracker.GetJobs()) > 0 {
				firstJob := m.tracker.GetJobs()[0]
				m.shimmer = m.shimmer.SetText(firstJob.Name).SetLoading(true)
			}
		}
		// Fall through to shimmer update

	case JobEventMsg:
		if msg.Event != nil {
			m.debugLogs = append(m.debugLogs, fmt.Sprintf("Job Event: ID=%q Action=%q Success=%v", msg.Event.JobID, msg.Event.Action, msg.Event.Success))
			if m.tracker != nil {
				changed := m.tracker.ProcessEvent(msg.Event)
				if changed {
					m.updateShimmerForCurrentStep()
				}
			}
		}
		// Fall through to shimmer update

	case StepEventMsg:
		if msg.Event != nil {
			m.debugLogs = append(m.debugLogs, fmt.Sprintf("Step Event: Job=%q Step=%d Name=%q", msg.Event.JobID, msg.Event.StepIdx, msg.Event.StepName))
			if m.tracker != nil {
				changed := m.tracker.ProcessStepEvent(msg.Event)
				if changed {
					m.updateShimmerForCurrentStep()
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
		if m.tracker != nil {
			m.tracker.MarkAllRunningComplete(hasErrors)
		}
		return m, tea.Quit

	case ErrMsg:
		m.err = msg
		return m, tea.Quit

	case LogMsg:
		// Fall through to shimmer update
	}

	// Always forward messages to shimmer to keep animation running
	var cmd tea.Cmd
	m.shimmer, cmd = m.shimmer.Update(msg)
	return m, cmd
}

// updateShimmerForCurrentStep updates shimmer text to show current running step
func (m *CheckModel) updateShimmerForCurrentStep() {
	if m.tracker == nil {
		return
	}

	for _, job := range m.tracker.GetJobs() {
		if job.Status == ci.JobRunning {
			// Find current step name
			if job.CurrentStep >= 0 && job.CurrentStep < len(job.Steps) {
				stepName := job.Steps[job.CurrentStep].Name
				m.shimmer = m.shimmer.SetText(stepName).SetLoading(true)
			} else {
				m.shimmer = m.shimmer.SetText(job.Name).SetLoading(true)
			}
			return
		}
	}
}

// GetDebugLogs returns debug logs for troubleshooting
func (m *CheckModel) GetDebugLogs() []string {
	return m.debugLogs
}

// GetCompletionOutput returns the completion output to be printed after TUI exits
func (m *CheckModel) GetCompletionOutput() string {
	if m.err != nil {
		return ErrorStyle.Render(fmt.Sprintf("✗ Error: %s\n", m.err.Error()))
	}
	return m.renderCompletionView()
}

// View renders the current model state as a string
func (m *CheckModel) View() string {
	if m.err != nil || m.done {
		// Return empty to clear the TUI area - completion output is printed
		// after the TUI exits by the calling code
		return ""
	}

	if m.waiting || m.tracker == nil {
		return m.renderWaitingView()
	}

	return m.renderStepList()
}

// renderWaitingView renders the waiting state before manifest is received
func (m *CheckModel) renderWaitingView() string {
	var b strings.Builder

	elapsed := int(time.Since(m.startTime).Seconds())
	header := fmt.Sprintf("$ act · %ds", elapsed)
	b.WriteString(SecondaryStyle.Render(header) + "\n\n")
	b.WriteString("  " + m.shimmer.View() + "\n")

	// Show helpful message after a few seconds
	if elapsed > 5 {
		b.WriteString(MutedStyle.Render("  This may take a moment on first run.") + "\n")
	}

	return b.String()
}

// renderStepList renders the job list during execution (compact - no step expansion)
func (m *CheckModel) renderStepList() string {
	var b strings.Builder

	elapsed := int(time.Since(m.startTime).Seconds())
	header := fmt.Sprintf("$ act · %ds", elapsed)
	b.WriteString(SecondaryStyle.Render(header) + "\n\n")

	jobs := m.tracker.GetJobs()

	for _, job := range jobs {
		// Render job line (with current step shown inline for running jobs)
		jobLine := m.renderJobCompact(job)
		b.WriteString("  " + jobLine + "\n")
	}

	return b.String()
}

// renderJobCompact renders a job line with current step inline (for running view)
func (m *CheckModel) renderJobCompact(job *TrackedJob) string {
	if job.IsReusable {
		return m.renderReusableJob(job)
	}

	var icon string
	var text string

	switch job.Status {
	case ci.JobPending:
		icon = MutedStyle.Render("·")
		text = MutedStyle.Render(job.Name)

	case ci.JobRunning:
		icon = SecondaryStyle.Render("·")
		// Show current step name inline with shimmer
		if job.CurrentStep >= 0 && job.CurrentStep < len(job.Steps) {
			stepName := job.Steps[job.CurrentStep].Name
			text = m.shimmer.View() + MutedStyle.Render(" · "+stepName)
		} else {
			text = m.shimmer.View()
		}

	case ci.JobSuccess:
		icon = SuccessStyle.Render("✓")
		text = PrimaryStyle.Render(job.Name)

	case ci.JobFailed:
		icon = ErrorStyle.Render("✗")
		text = PrimaryStyle.Render(job.Name)

	case ci.JobSkipped:
		icon = SecondaryStyle.Render("⏭")
		text = SecondaryStyle.Render(job.Name)

	case ci.JobSkippedSecurity:
		icon = SecondaryStyle.Render("🔒")
		text = SecondaryStyle.Render(job.Name)
	}

	return fmt.Sprintf("%s %s", icon, text)
}

// renderJob renders a single job line
func (m *CheckModel) renderJob(job *TrackedJob) string {
	var icon string
	var text string

	// Special handling for reusable workflows
	if job.IsReusable {
		return m.renderReusableJob(job)
	}

	switch job.Status {
	case ci.JobPending:
		icon = MutedStyle.Render("·")
		text = MutedStyle.Render(job.Name)

	case ci.JobRunning:
		icon = SecondaryStyle.Render("·")
		// Show job name (steps shown below)
		text = SecondaryStyle.Render(job.Name)

	case ci.JobSuccess:
		icon = SuccessStyle.Render("✓")
		text = PrimaryStyle.Render(job.Name)

	case ci.JobFailed:
		icon = ErrorStyle.Render("✗")
		text = PrimaryStyle.Render(job.Name)

	case ci.JobSkipped:
		icon = SecondaryStyle.Render("⏭")
		text = SecondaryStyle.Render(job.Name)

	case ci.JobSkippedSecurity:
		icon = SecondaryStyle.Render("🔒")
		text = SecondaryStyle.Render(job.Name)
	}

	return fmt.Sprintf("%s %s", icon, text)
}

// renderReusableJob renders a job that uses a reusable workflow
func (m *CheckModel) renderReusableJob(job *TrackedJob) string {
	var icon string
	var text string

	switch job.Status {
	case ci.JobPending:
		icon = MutedStyle.Render("⟲")
		text = MutedStyle.Render(job.Name + " (reusable)")

	case ci.JobRunning:
		icon = SecondaryStyle.Render("⟲")
		text = SecondaryStyle.Render(job.Name + " (reusable)")

	case ci.JobSuccess:
		icon = SuccessStyle.Render("⟲")
		text = PrimaryStyle.Render(job.Name + " (reusable)")

	case ci.JobFailed:
		icon = ErrorStyle.Render("⟲")
		text = PrimaryStyle.Render(job.Name + " (reusable)")

	case ci.JobSkipped:
		icon = SecondaryStyle.Render("⟲")
		text = SecondaryStyle.Render(job.Name + " (reusable)")

	case ci.JobSkippedSecurity:
		icon = SecondaryStyle.Render("🔒")
		text = SecondaryStyle.Render(job.Name + " (reusable)")
	}

	return fmt.Sprintf("%s %s", icon, text)
}

// renderStep renders a single step line
func (m *CheckModel) renderStep(job *TrackedJob, step *TrackedStep) string {
	var icon string
	var text string

	isCurrentStep := job.Status == ci.JobRunning && job.CurrentStep == step.Index

	switch step.Status {
	case ci.StepPending:
		icon = MutedStyle.Render("·")
		text = MutedStyle.Render(step.Name)

	case ci.StepRunning:
		icon = SecondaryStyle.Render("·")
		if isCurrentStep {
			text = m.shimmer.View()
		} else {
			text = SecondaryStyle.Render(step.Name)
		}

	case ci.StepSuccess:
		icon = SuccessStyle.Render("✓")
		text = PrimaryStyle.Render(step.Name)

	case ci.StepFailed:
		icon = ErrorStyle.Render("✗")
		text = PrimaryStyle.Render(step.Name)

	case ci.StepSkipped:
		icon = SecondaryStyle.Render("⏭")
		text = SecondaryStyle.Render(step.Name)

	case ci.StepCancelled:
		icon = MutedStyle.Render("·")
		text = MutedStyle.Render(step.Name)
	}

	return fmt.Sprintf("%s %s", icon, text)
}

// renderCompletionView renders the final completion summary with error report
func (m *CheckModel) renderCompletionView() string {
	var b strings.Builder

	// Show final job statuses
	header := fmt.Sprintf("$ act · %s", m.duration.Round(time.Second))
	b.WriteString(SecondaryStyle.Render(header) + "\n\n")

	if m.tracker != nil {
		for _, job := range m.tracker.GetJobs() {
			line := m.renderJob(job)
			b.WriteString("  " + line + "\n")

			// Only expand steps for failed jobs (keeps output compact)
			if job.Status == ci.JobFailed && len(job.Steps) > 0 && !job.IsReusable {
				for _, step := range job.Steps {
					stepLine := m.renderStep(job, step)
					b.WriteString("    " + stepLine + "\n")
				}
			}
		}
	}
	b.WriteString("\n")

	// Check for security-skipped jobs
	hasSecuritySkipped := false
	if m.tracker != nil {
		for _, job := range m.tracker.GetJobs() {
			if job.Status == ci.JobSkippedSecurity {
				hasSecuritySkipped = true
				break
			}
		}
	}

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

	// Show hint for security-skipped jobs
	if hasSecuritySkipped {
		b.WriteString("\n")
		b.WriteString(HintStyle.Render("Locked jobs skipped for safety. Manage with: detent workflows") + "\n")
	}

	return b.String()
}
