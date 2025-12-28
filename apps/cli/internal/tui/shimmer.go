package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Default configuration values for the shimmer effect.
const (
	DefaultInterval   = 50 * time.Millisecond // Animation frame rate
	DefaultPeakLight  = 90                    // Maximum lightness percentage (0-100)
	DefaultWaveWidth  = 8                     // Number of characters in the wave
	DefaultWavePause  = 8                     // Pause between wave loops (in characters)
)

// Direction controls which way the shimmer wave moves.
type Direction int

// Shimmer wave direction constants.
const (
	DirectionRight Direction = iota // Wave moves left to right (default)
	DirectionLeft                   // Wave moves right to left
)

// ShimmerModel creates an animated shimmer effect on text.
// A wave of brightened color sweeps across the text, creating a loading indicator.
type ShimmerModel struct {
	text       string
	baseColor  string
	isLoading  bool
	position   int
	waveColors []string

	// Configurable options
	interval  time.Duration
	peakLight int
	waveWidth int
	wavePause int
	direction Direction
}

// ShimmerTickMsg triggers a shimmer animation frame.
type ShimmerTickMsg time.Time

// Option is a function that configures a ShimmerModel.
type Option func(*ShimmerModel)

// WithInterval sets the animation speed (time between frames).
func WithInterval(d time.Duration) Option {
	return func(m *ShimmerModel) {
		m.interval = d
	}
}

// WithPeakLight sets the maximum lightness (0-100). Higher = brighter shimmer.
func WithPeakLight(percent int) Option {
	return func(m *ShimmerModel) {
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		m.peakLight = percent
	}
}

// WithWaveWidth sets the width of the shimmer wave in characters.
func WithWaveWidth(width int) Option {
	return func(m *ShimmerModel) {
		if width < 2 {
			width = 2
		}
		m.waveWidth = width
	}
}

// WithWavePause sets the pause between wave loops (in character positions).
func WithWavePause(pause int) Option {
	return func(m *ShimmerModel) {
		if pause < 0 {
			pause = 0
		}
		m.wavePause = pause
	}
}

// WithDirection sets the wave direction (left or right).
func WithDirection(dir Direction) Option {
	return func(m *ShimmerModel) {
		m.direction = dir
	}
}

// NewShimmerModel creates a new shimmer model with the given text and base color.
// Color should be a hex color like "#00D787".
func NewShimmerModel(text, baseColor string, opts ...Option) ShimmerModel {
	m := ShimmerModel{
		text:      text,
		baseColor: baseColor,
		isLoading: true,
		position:  0,
		interval:  DefaultInterval,
		peakLight: DefaultPeakLight,
		waveWidth: DefaultWaveWidth,
		wavePause: DefaultWavePause,
		direction: DirectionRight,
	}

	for _, opt := range opts {
		opt(&m)
	}

	m.waveColors = m.generateWaveColors()
	return m
}

// SetText updates the text being displayed.
func (m ShimmerModel) SetText(text string) ShimmerModel {
	m.text = text
	return m
}

// SetLoading enables or disables the shimmer animation.
func (m ShimmerModel) SetLoading(loading bool) ShimmerModel {
	m.isLoading = loading
	return m
}

// Init starts the shimmer animation tick.
func (m ShimmerModel) Init() tea.Cmd {
	if m.isLoading {
		return m.tick()
	}
	return nil
}

// Update handles shimmer animation ticks.
func (m ShimmerModel) Update(msg tea.Msg) (ShimmerModel, tea.Cmd) {
	if _, ok := msg.(ShimmerTickMsg); ok && m.isLoading {
		totalLength := len([]rune(m.text)) + len(m.waveColors) + m.wavePause
		m.position = (m.position + 1) % totalLength
		return m, m.tick()
	}
	return m, nil
}

// View renders the shimmer text with per-character coloring.
func (m ShimmerModel) View() string {
	baseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.baseColor))

	if !m.isLoading {
		return baseStyle.Render(m.text)
	}

	runes := []rune(m.text)
	var sb strings.Builder

	for i, char := range runes {
		color := m.getCharacterColor(i, len(runes))
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
		sb.WriteString(style.Render(string(char)))
	}

	return sb.String()
}

func (m ShimmerModel) tick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return ShimmerTickMsg(t)
	})
}

func (m ShimmerModel) getCharacterColor(index, textLen int) string {
	var distance int

	if m.direction == DirectionLeft {
		// Reverse: wave moves right to left
		distance = m.position - (textLen - 1 - index)
	} else {
		// Default: wave moves left to right
		distance = m.position - index
	}

	if distance >= 0 && distance < len(m.waveColors) {
		return m.waveColors[distance]
	}

	return m.baseColor
}

// generateWaveColors creates a high-contrast gradient for the shimmer wave.
// The wave ramps up to peakLight then back down for a smooth pulse.
func (m ShimmerModel) generateWaveColors() []string {
	r, g, b := parseHexColor(m.baseColor)

	// Build symmetric wave: 0 -> peak -> 0
	steps := m.waveWidth
	if steps < 2 {
		steps = 2
	}

	colors := make([]string, steps)
	mid := steps / 2

	for i := range steps {
		// Calculate distance from edges (0 at edges, 1 at center)
		var ratio float64
		if i <= mid {
			ratio = float64(i) / float64(mid)
		} else {
			ratio = float64(steps-1-i) / float64(steps-1-mid)
		}

		pct := int(ratio * float64(m.peakLight))
		colors[i] = fmt.Sprintf("#%02X%02X%02X",
			lighten(r, pct),
			lighten(g, pct),
			lighten(b, pct),
		)
	}

	return colors
}

// parseHexColor parses a hex color string like "#FFC000" or "FFC000".
func parseHexColor(hex string) (r, g, b int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 215, 135 // Default to green (#00D787) if invalid
	}
	rr, _ := strconv.ParseInt(hex[0:2], 16, 64)
	gg, _ := strconv.ParseInt(hex[2:4], 16, 64)
	bb, _ := strconv.ParseInt(hex[4:6], 16, 64)
	return int(rr), int(gg), int(bb)
}

// lighten blends a color channel toward white (255) by a percentage.
func lighten(value, percent int) int {
	result := value + (255-value)*percent/100
	if result > 255 {
		return 255
	}
	return result
}
