// Package tui holds the design tokens, the screen frame and the home screen
// of the terminal interface.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The palette is ink and one green. Each color has a light and a dark
// variant, so that dim text stays readable on a light terminal. Text colors
// keep 4.5:1 on white, Solarized light, black and grey (theme_test.go);
// Faint is for tracks and rules only.
var (
	ColorInk    = lipgloss.AdaptiveColor{Light: "#111214", Dark: "#F4F4F5"}
	ColorText   = lipgloss.AdaptiveColor{Light: "#2B2D31", Dark: "#D6D8DD"}
	ColorDim    = lipgloss.AdaptiveColor{Light: "#62666D", Dark: "#888C95"}
	ColorFaint  = lipgloss.AdaptiveColor{Light: "#C4C6CB", Dark: "#3A3C43"}
	ColorGreen  = lipgloss.AdaptiveColor{Light: "#0B7A50", Dark: "#3DDC97"}
	ColorAmber  = lipgloss.AdaptiveColor{Light: "#955800", Dark: "#F2B04C"}
	ColorRed    = lipgloss.AdaptiveColor{Light: "#C0293B", Dark: "#FF8A94"}
	ColorBand   = lipgloss.AdaptiveColor{Light: "#ECEDEF", Dark: "#27292E"}
	ColorKey    = lipgloss.AdaptiveColor{Light: "#E4E5E8", Dark: "#2C2E34"}
	ColorOnChip = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0F1012"}
)

// categoryColors are the hues of the storage categories, the same as in
// the app. Caches and other stay grey, so that the eye goes to the large
// categories first.
var categoryColors = map[string]lipgloss.AdaptiveColor{
	"apps":      {Light: "#6A57E8", Dark: "#8F7DFF"},
	"developer": {Light: "#2F7FE0", Dark: "#4AA3FF"},
	"documents": {Light: "#129C8B", Dark: "#35C6B4"},
	"media":     {Light: "#D07A1C", Dark: "#F0A04B"},
	"caches":    {Light: "#8B8F98", Dark: "#6B6F79"},
	"other":     {Light: "#B5B8BF", Dark: "#454850"},
}

// categoryGlyphs carry the categories without color.
var categoryGlyphs = map[string]string{
	"apps": "▓", "developer": "█", "documents": "▒", "media": "▚", "caches": "░", "other": "·",
}

// CategoryStyle returns the style of a storage category.
func CategoryStyle(id string) lipgloss.Style {
	if c, ok := categoryColors[id]; ok {
		return lipgloss.NewStyle().Foreground(c)
	}
	return Meta
}

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

// CategoryShare is one part of a category bar.
type CategoryShare struct {
	ID, Title string
	Bytes     int64
}

// CategoryBar draws the parts as one bar of the given width, then the
// remaining space as a faint track. total is the width of the whole bar in
// bytes. With color the bar is solid blocks; without color each category
// has its own glyph.
func CategoryBar(parts []CategoryShare, total int64, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, p := range parts {
		cells := int(float64(p.Bytes)/float64(total)*float64(width) + 0.5)
		cells = min(cells, width-used)
		if cells <= 0 {
			continue
		}
		glyph := "█"
		if !hasColor() {
			glyph = categoryGlyphs[p.ID]
			if glyph == "" {
				glyph = "#"
			}
		}
		b.WriteString(CategoryStyle(p.ID).Render(strings.Repeat(glyph, cells)))
		used += cells
	}
	b.WriteString(Faint.Render(strings.Repeat("░", width-used)))
	return b.String()
}

// CategoryLegend names the parts with their sizes, as many as fit in width.
func CategoryLegend(parts []CategoryShare, width int) string {
	line := ""
	for _, p := range parts {
		next := CategoryStyle(p.ID).Render("■") + " " + Normal.Render(p.Title) + " " + Meta.Render(Bytes(p.Bytes))
		if line != "" {
			next = "   " + next
		}
		if Width(line)+Width(next) > width {
			break
		}
		line += next
	}
	return line
}
