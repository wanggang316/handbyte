package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Size is the layout class of a terminal width.
type Size int

const (
	// Narrow is below 60 columns: one column, a short key bar.
	Narrow Size = iota
	// Medium is 60 to 99 columns: one column with a detail block.
	Medium
	// Wide is 100 columns and above: a list and a detail pane.
	Wide
)

// SizeFor returns the layout class of a terminal width.
func SizeFor(width int) Size {
	switch {
	case width < 60:
		return Narrow
	case width < 100:
		return Medium
	}
	return Wide
}

// compactHeight is the height below which the header becomes one line.
const compactHeight = 20

// Key is one entry of the key bar.
type Key struct {
	Key   string
	Label string
}

// Frame is the layout that all screens share: a header with the breadcrumb
// and a summary, the body, an optional status line and the key bar.
type Frame struct {
	Width, Height int
	// Crumb is the path below the product name, for example "Clean".
	Crumb   []string
	Summary string
	Body    []string
	Status  string
	// Primary is the main action. It is always shown, as a filled chip.
	Primary Key
	// Keys are the other actions, most important first.
	Keys []Key
}

// Margin returns the left margin for a width.
func Margin(width int) int {
	if SizeFor(width) == Narrow {
		return 1
	}
	return 2
}

// Width returns the cell width of a rendered string.
func Width(s string) int { return lipgloss.Width(s) }

// Between puts left and right on one line of the given width. When the two
// do not fit, right is dropped first and then left is cut.
func Between(left, right string, width int) string {
	gap := width - Width(left) - Width(right)
	if right == "" || gap < 1 {
		return ansi.Truncate(left, max(1, width), "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

// CutMiddle shortens a name in the middle, so that both ends stay readable.
func CutMiddle(s string, width int) string {
	if Width(s) <= width {
		return s
	}
	if width < 3 {
		return ansi.Truncate(s, max(0, width), "")
	}
	head := (width - 1) / 2
	tail := width - 1 - head
	return ansi.Truncate(s, head, "") + "…" + ansi.TruncateLeft(s, Width(s)-tail, "")
}

// Render draws the frame with exactly Height lines, each at most Width cells.
func (f Frame) Render() string {
	if f.Width <= 0 || f.Height <= 0 {
		return ""
	}
	pad := strings.Repeat(" ", Margin(f.Width))
	inner := max(1, f.Width-2*len(pad))

	left := Mark() + " " + Strong.Render("handbyte")
	for _, c := range f.Crumb {
		left += Meta.Render(" › ") + Title.Render(c)
	}
	head := []string{pad + Between(left, Meta.Render(f.Summary), inner)}
	if f.Height >= compactHeight {
		head = append(head, "")
	}

	foot := []string{pad + f.keyBar(inner)}
	if f.Status != "" {
		foot = append([]string{pad + ansi.Truncate(f.Status, inner, "…")}, foot...)
	}
	if f.Height >= compactHeight {
		foot = append([]string{""}, foot...)
	}

	room := max(0, f.Height-len(head)-len(foot))
	body := make([]string, room)
	for i := 0; i < room && i < len(f.Body); i++ {
		body[i] = ansi.Truncate(f.Body[i], f.Width, "…")
	}
	lines := append(append(head, body...), foot...)
	if len(lines) > f.Height {
		lines = append(lines[:f.Height-1], lines[len(lines)-1])
	}
	return strings.Join(lines, "\n")
}

// keyBar keeps the primary action and as many other keys as fit. A narrow
// terminal shows three keys at most, then a pointer to the help.
func (f Frame) keyBar(width int) string {
	bar := Chip(f.Primary.Key) + " " + Strong.Render(f.Primary.Label)
	keys := f.Keys
	more := ""
	if SizeFor(f.Width) == Narrow && len(keys) > 3 {
		keys = keys[:3]
		more = "  " + KeyCap("?") + Meta.Render(" more")
	}
	for _, k := range keys {
		next := "  " + KeyCap(k.Key) + Meta.Render(" "+k.Label)
		if Width(bar)+Width(next)+Width(more) > width {
			if more == "" {
				more = "  " + KeyCap("?") + Meta.Render(" more")
			}
			break
		}
		bar += next
	}
	if Width(bar)+Width(more) <= width {
		bar += more
	}
	return ansi.Truncate(bar, width, "…")
}
