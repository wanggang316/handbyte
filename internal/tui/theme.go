// Package tui holds the design tokens, the screen frame and the home screen
// of the terminal interface.
package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The palette is ink and one green. Each color has a light and a dark
// variant, so that dim text stays readable on a light terminal.
var (
	ColorInk    = lipgloss.AdaptiveColor{Light: "#111214", Dark: "#F4F4F5"}
	ColorText   = lipgloss.AdaptiveColor{Light: "#2B2D31", Dark: "#D6D8DD"}
	ColorDim    = lipgloss.AdaptiveColor{Light: "#62666D", Dark: "#7F838C"}
	ColorFaint  = lipgloss.AdaptiveColor{Light: "#C4C6CB", Dark: "#3A3C43"}
	ColorGreen  = lipgloss.AdaptiveColor{Light: "#0E9F5B", Dark: "#3DDC97"}
	ColorAmber  = lipgloss.AdaptiveColor{Light: "#A86400", Dark: "#F2B04C"}
	ColorRed    = lipgloss.AdaptiveColor{Light: "#C0293B", Dark: "#FF8A94"}
	ColorBand   = lipgloss.AdaptiveColor{Light: "#ECEDEF", Dark: "#27292E"}
	ColorKey    = lipgloss.AdaptiveColor{Light: "#E4E5E8", Dark: "#2C2E34"}
	ColorOnChip = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0F1012"}
)

// A terminal has one font size. Hierarchy comes from five levels of weight
// and tone: Strong, Title, Normal, Meta and Label.
var (
	Strong = lipgloss.NewStyle().Bold(true).Foreground(ColorInk)
	Title  = lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	Normal = lipgloss.NewStyle().Foreground(ColorText)
	Meta   = lipgloss.NewStyle().Foreground(ColorDim)
	Faint  = lipgloss.NewStyle().Foreground(ColorFaint)

	// Color has meaning: green for safe, progress and done, amber for
	// review, red only for a permanent deletion.
	Safe   = lipgloss.NewStyle().Foreground(ColorGreen)
	Review = lipgloss.NewStyle().Foreground(ColorAmber)
	Danger = lipgloss.NewStyle().Foreground(ColorRed)

	// CursorRow is the neutral band of the current row.
	CursorRow = lipgloss.NewStyle().Bold(true).Foreground(ColorInk).Background(ColorBand)
)

func hasColor() bool { return lipgloss.ColorProfile() != termenv.Ascii }

// Chip renders the key of the primary action. Without color, brackets
// carry the state.
func Chip(key string) string {
	if !hasColor() {
		return "[" + key + "]"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(ColorOnChip).Background(ColorInk).Render(" " + key + " ")
}

// KeyCap renders the key of a secondary action.
func KeyCap(key string) string {
	if !hasColor() {
		return "[" + key + "]"
	}
	return lipgloss.NewStyle().Foreground(ColorText).Background(ColorKey).Render(" " + key + " ")
}

// Mark is the two-cell logo: a block with one corner out, and the byte that
// left.
func Mark() string { return Strong.Render("▙") + Safe.Render("▝") }

// Bytes formats a size with binary units, as the other screens do.
func Bytes(n int64) string {
	const (
		k = 1024
		m = k * 1024
		g = m * 1024
		t = g * 1024
	)
	switch {
	case n >= t:
		return fmt.Sprintf("%.2f TiB", float64(n)/t)
	case n >= g:
		return fmt.Sprintf("%.2f GiB", float64(n)/g)
	case n >= m:
		return fmt.Sprintf("%.1f MiB", float64(n)/m)
	case n >= k:
		return fmt.Sprintf("%.1f KiB", float64(n)/k)
	}
	return fmt.Sprintf("%d B", n)
}
