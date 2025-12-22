package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PreflightCheck represents a single pre-flight check
type PreflightCheck struct {
	Name   string
	Status string // "pending", "running", "success", "error"
	Error  error
}

var (
	checkPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	checkRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	checkSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	checkErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// RenderPreflightCheck renders a single check line
func RenderPreflightCheck(check PreflightCheck) string {
	var icon string
	var style lipgloss.Style
	var suffix string

	switch check.Status {
	case "pending":
		icon = "○"
		style = checkPendingStyle
	case "running":
		icon = "◐"
		style = checkRunningStyle
	case "success":
		icon = "✓"
		style = checkSuccessStyle
	case "error":
		icon = "✗"
		style = checkErrorStyle
		if check.Error != nil {
			suffix = fmt.Sprintf(" (%s)", check.Error.Error())
		}
	default:
		icon = "○"
		style = checkPendingStyle
	}

	return style.Render(fmt.Sprintf("%s %s%s", icon, check.Name, suffix))
}

// PreflightDisplay manages the display of pre-flight checks
type PreflightDisplay struct {
	checks []PreflightCheck
}

// NewPreflightDisplay creates a new pre-flight display
func NewPreflightDisplay(checkNames []string) *PreflightDisplay {
	checks := make([]PreflightCheck, len(checkNames))
	for i, name := range checkNames {
		checks[i] = PreflightCheck{
			Name:   name,
			Status: "pending",
		}
	}
	return &PreflightDisplay{checks: checks}
}

// UpdateCheck updates a specific check's status
func (p *PreflightDisplay) UpdateCheck(name, status string, err error) {
	for i := range p.checks {
		if p.checks[i].Name == name {
			p.checks[i].Status = status
			p.checks[i].Error = err
			break
		}
	}
}

// Render renders all checks to stderr
func (p *PreflightDisplay) Render() {
	// Clear previous output
	fmt.Fprint(os.Stderr, "\033[2J\033[H")

	var lines []string
	for _, check := range p.checks {
		lines = append(lines, RenderPreflightCheck(check))
	}

	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}

// RenderFinal renders the final state and waits for user
func (p *PreflightDisplay) RenderFinal() {
	// Don't clear - just show final state
	var lines []string
	for _, check := range p.checks {
		lines = append(lines, RenderPreflightCheck(check))
	}

	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}

// AllSuccess returns true if all checks passed
func (p *PreflightDisplay) AllSuccess() bool {
	for _, check := range p.checks {
		if check.Status != "success" {
			return false
		}
	}
	return true
}
