package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.AdaptiveColor{Light: "#5A45FF", Dark: "#B19BFF"}
	colorAccent    = lipgloss.AdaptiveColor{Light: "#0DB48E", Dark: "#43E1B5"}
	colorWarn      = lipgloss.AdaptiveColor{Light: "#C26900", Dark: "#FFB46B"}
	colorDanger    = lipgloss.AdaptiveColor{Light: "#B0233A", Dark: "#FF7A8B"}
	colorMuted     = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#9A9A9A"}
	colorSubtle    = lipgloss.AdaptiveColor{Light: "#8C8C8C", Dark: "#C4C4C4"}
	colorSelected  = lipgloss.AdaptiveColor{Light: "#FFF7D6", Dark: "#3A3500"}
	colorCursor    = lipgloss.AdaptiveColor{Light: "#1E1E1E", Dark: "#F5F5F5"}
	colorPanelBg   = lipgloss.AdaptiveColor{Light: "#F4F4F8", Dark: "#1B1B1F"}
	colorBorderDim = lipgloss.AdaptiveColor{Light: "#D9D9DE", Dark: "#3A3A3F"}
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorderDim).
			BorderBottom(true).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorderDim).
			BorderTop(true).
			Padding(0, 1)

	mutedStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	subtleStyle = lipgloss.NewStyle().Foreground(colorSubtle)
	accentStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	dangerStyle = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)
	primary     = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	groupStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	rowCursorStyle = lipgloss.NewStyle().
			Background(colorSelected).
			Foreground(colorCursor)

	dialogStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Background(colorPanelBg)

	badgeSafe = lipgloss.NewStyle().
			Background(colorAccent).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render(" safe ")

	badgeReview = lipgloss.NewStyle().
			Background(colorWarn).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render("review")

	badgeCaution = lipgloss.NewStyle().
			Background(colorDanger).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render(" hold ")
)
