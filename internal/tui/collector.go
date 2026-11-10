package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wanggang316/handbyte/internal/cleaner"
)

// collector holds the items that the user picked in the drill-down, in the
// order of picking. They only ever go to the Trash.
type collector struct {
	items   []cleaner.Collected
	stage   collectStage
	cursor  int
	results []cleaner.Result
}

type collectStage int

const (
	collectOff collectStage = iota
	collectReview
	collectMoving
	collectDone
)

type collectDoneMsg []cleaner.Result

func (c *collector) has(path string) bool {
	for _, it := range c.items {
		if it.Path == path {
			return true
		}
	}
	return false
}

func (c *collector) total() int64 {
	var total int64
	for _, it := range c.items {
		total += it.Size
	}
	return total
}

func (c *collector) label() string {
	n := len(c.items)
	noun := "items"
	if n == 1 {
		noun = "item"
	}
	return fmt.Sprintf("%d %s · %s", n, noun, Bytes(c.total()))
}

// toggle adds a path, or removes it when it is in the list. A path that a
// cleanup rule covers is refused with a pointer to Clean.
func (m *Analyze) toggleCollected(path string, size int64) {
	for i, it := range m.collect.items {
		if it.Path == path {
			m.collect.items = append(m.collect.items[:i], m.collect.items[i+1:]...)
			m.notice = Meta.Render("Removed from the list: " + tilde(path))
			return
		}
	}
	if _, ok := m.opts.Hints.Rule(path); ok {
		m.notice = Review.Render("A cleanup rule covers this folder. Use handbyte clean for it.")
		return
	}
	item, err := cleaner.NewCollected(path, size)
	if err != nil {
		m.notice = Review.Render("Cannot collect: " + err.Error())
		return
	}
	m.collect.items = append(m.collect.items, item)
	m.notice = Meta.Render("Added to the list: " + tilde(path))
}

// collectKey handles the keys of the review and the result screens.
func (m *Analyze) collectKey(key string) (tea.Model, tea.Cmd) {
	c := &m.collect
	switch c.stage {
	case collectReview:
		switch key {
		case "esc":
			c.stage = collectOff
		case "up", "k":
			c.cursor = max(0, c.cursor-1)
		case "down", "j":
			c.cursor = min(len(c.items)-1, c.cursor+1)
		case " ", "x", "delete", "backspace":
			if c.cursor < len(c.items) {
				c.items = append(c.items[:c.cursor], c.items[c.cursor+1:]...)
				c.cursor = max(0, min(c.cursor, len(c.items)-1))
			}
			if len(c.items) == 0 {
				c.stage = collectOff
			}
		case "y":
			c.stage = collectMoving
			items, run := c.items, m.opts.Collect
			if run == nil {
				run = func(ctx context.Context, items []cleaner.Collected) []cleaner.Result {
					return cleaner.Collect(ctx, items, cleaner.CollectOptions{}, nil)
				}
			}
			return m, func() tea.Msg { return collectDoneMsg(run(context.Background(), items)) }
		}
	case collectDone:
		switch key {
		case "esc", "enter", "r":
			c.stage, c.items, c.results, c.cursor = collectOff, nil, nil, 0
			// The tree no longer matches the disk: measure again.
			return m, m.start()
		}
	}
	return m, nil
}

func (m *Analyze) viewCollect(f *Frame) {
	pad := strings.Repeat(" ", Margin(m.width))
	inner := max(1, m.width-2*len(pad))
	c := m.collect
	f.Crumb = []string{"Analyze", "Move to Trash"}
	switch c.stage {
	case collectReview, collectMoving:
		body := []string{
			pad + Strong.Render(Bytes(c.total())) + Meta.Render(" in "+c.label()[:strings.Index(c.label(), " ·")]),
			pad + Meta.Render("These items go to the Trash. You can put them back from the Trash, or with handbyte restore."),
			"",
		}
		height := max(1, m.height-len(body)-6)
		offset := max(0, min(c.cursor-height/2, len(c.items)-height))
		for i := offset; i < len(c.items) && i < offset+height; i++ {
			it := c.items[i]
			name := tilde(it.Path)
			if it.IsDir {
				name += "/"
			}
			style, marker := Normal, "  "
			if i == c.cursor {
				style, marker = Strong, Safe.Render("▸ ")
			}
			body = append(body, pad+Between(marker+style.Render(CutMiddle(name, max(6, inner-16))), style.Render(Bytes(it.Size)), inner))
		}
		f.Body = body
		if c.stage == collectMoving {
			f.Status = Meta.Render("Moving to the Trash…")
			f.Primary = Key{"…", "please wait"}
			return
		}
		f.Primary = Key{"y", "move " + c.label() + " to the Trash"}
		f.Keys = []Key{{"↑↓", "move"}, {"x", "remove from list"}, {"esc", "back"}, {"q", "quit"}}
	case collectDone:
		var moved int64
		count, failed := 0, 0
		var lines []string
		for _, r := range c.results {
			switch r.Status {
			case "trashed":
				moved += r.Moved
				count++
				lines = append(lines, pad+Between(Safe.Render("✓ ")+Normal.Render(CutMiddle(tilde(r.Path), max(6, inner-34))), Meta.Render("restore id "+r.ID), inner))
			default:
				failed++
				lines = append(lines, pad+Review.Render("! ")+Normal.Render(CutMiddle(tilde(r.Path), max(6, inner/2)))+Meta.Render("  "+r.Message))
			}
		}
		head := Safe.Render("✓ ") + Strong.Render(fmt.Sprintf("Moved %d to the Trash", count)) + Meta.Render(" · "+Bytes(moved))
		if failed > 0 {
			head += Review.Render(fmt.Sprintf(" · %d not moved", failed))
		}
		body := []string{pad + head, pad + Meta.Render("Empty the Trash in Finder to free the space. To put an item back: handbyte restore <id>"), ""}
		f.Body = append(body, lines...)
		f.Primary = Key{"enter", "scan again"}
		f.Keys = []Key{{"q", "quit"}}
	}
}

// collectedMarker marks a picked row in the lists.
func (m *Analyze) collectedMarker(dir, name string) bool {
	return m.collect.has(filepath.Join(dir, name))
}
