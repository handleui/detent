package tui

import "github.com/charmbracelet/lipgloss"

// Semantic color palette - use these consistently across all commands.
// Based on modern CLI design (Vercel, GitHub CLI, Claude Code).
const (
	ColorBrand      = "42"  // Green - Detent brand, success states
	ColorPrimary    = "255" // White - main text, emphasis
	ColorSecondary  = "245" // Light gray - supporting text
	ColorMuted      = "240" // Dark gray - hints, less important info
	ColorSuccess    = "42"  // Green - operations succeeded
	ColorError      = "203" // Red - errors, failures
	ColorWarning    = "214" // Orange - cautions, attention needed
	ColorAccent     = "45"  // Cyan - highlights, links (use sparingly)
	ColorAnthropic  = "208" // Orange - Anthropic brand color
)

// Common styles used across all commands.
var (
	// Brand styles
	BrandStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBrand))
	AnthropicStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAnthropic))

	// Text hierarchy
	PrimaryStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorPrimary))
	SecondaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSecondary))
	MutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted))
	HintStyle      = MutedStyle.Italic(true)

	// Status indicators
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSuccess))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError))
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorWarning))

	// Accent for highlights
	AccentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))

	// Bold variants
	BoldStyle        = lipgloss.NewStyle().Bold(true)
	BoldPrimaryStyle = PrimaryStyle.Bold(true)
)

// StatusIcon returns the appropriate icon for a status.
func StatusIcon(success bool) string {
	if success {
		return SuccessStyle.Render("✓")
	}
	return ErrorStyle.Render("✗")
}

// Bullet returns a muted bullet point.
func Bullet() string {
	return MutedStyle.Render("·")
}

// Arrow returns a muted arrow.
func Arrow() string {
	return MutedStyle.Render("→")
}
