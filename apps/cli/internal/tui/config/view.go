package config

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/tui"
)

// Styles for the config viewer.
var (
	fieldStyle    = tui.SecondaryStyle
	selectedStyle = tui.BrandStyle.Bold(true) // GREEN for active selection
	valueStyle    = tui.PrimaryStyle
	mutedValue    = tui.MutedStyle
	hintStyle     = tui.HintStyle
)

// View implements tea.Model.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header with context indicator
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Field list
	for i, field := range EditableFields {
		b.WriteString(m.renderField(i, field))
		b.WriteString("\n")
	}

	// Help footer
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderHeader renders the branded header.
func (m *Model) renderHeader() string {
	return tui.Header(m.version, "config")
}

// renderField renders a single configuration field.
func (m *Model) renderField(index int, field Field) string {
	isSelected := index == m.cursor
	isEditing := isSelected && m.mode == ModeEditText && m.editField == field.Key
	value, ok := m.values[field.Key]
	if !ok {
		return ""
	}

	// Cursor
	cursor := "  "
	if isSelected {
		cursor = tui.BrandStyle.Render("> ")
	}

	// Field name styling
	nameStyle := fieldStyle
	if isSelected {
		nameStyle = selectedStyle
	}

	// Format field name with padding
	name := nameStyle.Render(padRight(field.Key, 20))

	// Value - show text input if editing this field
	var displayValue string
	if isEditing {
		displayValue = m.textInput.View()
	} else {
		// Value styling - muted for empty/off values, green for selected
		switch value.DisplayValue {
		case "":
			displayValue = mutedValue.Render("not set")
		case "off", "unlimited":
			if isSelected {
				displayValue = selectedStyle.Render(value.DisplayValue)
			} else {
				displayValue = mutedValue.Render(value.DisplayValue)
			}
		default:
			if isSelected {
				displayValue = selectedStyle.Render(value.DisplayValue)
			} else {
				displayValue = valueStyle.Render(value.DisplayValue)
			}
		}
		// Pad value for alignment
		displayValue = padRight(displayValue, 20)
	}

	// Source badge
	badge := tui.SourceBadge(value.Source.String())

	return cursor + name + displayValue + "  " + badge
}

// renderHelp renders the keyboard shortcuts.
func (m *Model) renderHelp() string {
	// Different hints for edit mode
	if m.mode == ModeEditText {
		return hintStyle.Render("[enter] save  [esc] cancel")
	}

	var hints []string
	hints = append(hints, "[j/k] navigate")

	// Show context-specific edit hints
	if m.cursor >= 0 && m.cursor < len(EditableFields) {
		field := EditableFields[m.cursor]
		switch field.Key {
		case "model":
			hints = append(hints, "[←/→] cycle")
		case "timeout":
			hints = append(hints, "[←/→] adjust")
		case "budget_per_run", "budget_monthly":
			hints = append(hints, "[enter] edit")
		case "api_key":
			hints = append(hints, "[enter] edit")
		}
	}

	hints = append(hints, "[e] open file", "[q] save & quit")

	return hintStyle.Render(strings.Join(hints, "  "))
}

// padRight pads a string to the given width.
func padRight(s string, width int) string {
	// Account for ANSI escape sequences
	visibleLen := lipgloss.Width(s)
	if visibleLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visibleLen)
}
