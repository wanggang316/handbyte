package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/wanggang316/handbyte/catalog"
	"github.com/wanggang316/handbyte/internal/usage"
)

// Action is what the user chose on the home screen.
type Action int

const (
	ActionQuit Action = iota
	ActionClean
	ActionAnalyze
	ActionHistory
)

// HomeInfo is the live data of the home screen.
type HomeInfo struct {
	Volume  string
	Version string
	// Total and Free are the bytes of the startup volume.
	Total, Free int64
	// Actions is the number of recorded cleanup actions; LastAction is the
	// time of the newest one.
	Actions    int
	LastAction time.Time
	Now        time.Time
	// Home is the last Analyze result of the home folder, if any.
	Home *usage.Summary
}

type homeEntry struct {
	name    string
	desc    string
	hint    []string
	action  Action
	primary string
	// planned entries are shown but cannot be opened yet.
	planned bool
}

var homeEntries = []homeEntry{
	{
		name:    "Clean",
		desc:    "Remove caches and build output that rebuild by themselves",
		hint:    []string{"Scans known cache and build folders. You see every item and its cost before you choose.", "Nothing is selected for you. Items go to the Trash, so you can restore them later."},
		action:  ActionClean,
		primary: "open Clean",
	},
	{
		name:    "Apps",
		desc:    "See what each app uses, uninstall it with its leftovers",
		hint:    []string{"Shows the size and the last use of each app, and removes an app with the files it left.", "Not in this version. It comes in a later release."},
		primary: "not available yet",
		planned: true,
	},
	{
		name:    "Analyze",
		desc:    "Find what fills the disk, folder by folder",
		hint:    []string{"Shows the folders of your home directory by size. Open a folder to go deeper.", "This view is read-only. It does not remove files."},
		action:  ActionAnalyze,
		primary: "open Analyze",
	},
	{
		name:    "History",
		desc:    "See past cleanups and restore items from the Trash",
		hint:    []string{"Lists the cleanups that handbyte recorded on this Mac.", "To put an item back, use: handbyte restore <id>"},
		action:  ActionHistory,
		primary: "open History",
	},
}

// Home is the Bubble Tea model of the home screen.
type Home struct {
	info          HomeInfo
	cursor        int
	width, height int
	help          bool
	notice        string
	// Choice is set when the program ends.
	Choice Action
}

// NewHome returns the home screen for the given data.
func NewHome(info HomeInfo) *Home { return &Home{info: info} }

func (m *Home) Init() tea.Cmd { return nil }

func (m *Home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		m.notice = ""
		if m.help {
			m.help = false
			return m, nil
		}
		switch key := msg.String(); key {
		case "q", "esc", "ctrl+c":
			m.Choice = ActionQuit
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(homeEntries)-1, m.cursor+1)
		case "1", "2", "3", "4":
			m.cursor = int(key[0] - '1')
		case "?":
			m.help = true
		case "enter":
			e := homeEntries[m.cursor]
			if e.planned {
				m.notice = e.name + " is not in this version."
				return m, nil
			}
			m.Choice = e.action
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Home) stat(e homeEntry) string {
	switch e.name {
	case "Clean":
		if m.info.LastAction.IsZero() {
			return Meta.Render("not used yet")
		}
		return Meta.Render("last cleaned " + ago(m.info.Now.Sub(m.info.LastAction)))
	case "Apps":
		return Meta.Render("later version")
	case "Analyze":
		if h := m.info.Home; h != nil {
			return Meta.Render("home " + Bytes(h.Allocated) + " · " + since(m.info.Now.Sub(h.ScannedAt)) + " ago")
		}
		if m.info.Total > 0 {
			return Meta.Render(Bytes(m.info.Total-m.info.Free) + " used")
		}
	case "History":
		if m.info.Actions == 1 {
			return Meta.Render("1 action")
		}
		return Meta.Render(fmt.Sprintf("%d actions", m.info.Actions))
	}
	return ""
}

func ago(d time.Duration) string {
	switch {
	case d < time.Hour:
		return "less than 1 hour ago"
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

func (m *Home) View() string {
	if m.width == 0 {
		return ""
	}
	size := SizeFor(m.width)
	pad := strings.Repeat(" ", Margin(m.width))
	inner := max(1, m.width-2*len(pad))
	roomy := m.height >= compactHeight

	summary := m.info.Volume
	if m.info.Version != "" {
		summary += " · " + m.info.Version
	}
	f := Frame{Width: m.width, Height: m.height, Summary: summary, Status: m.notice}

	if m.help {
		f.Crumb = []string{"Help"}
		f.Body = m.helpLines(pad)
		f.Primary = Key{"any key", "back"}
		return f.Render()
	}

	var body []string
	if m.info.Total > 0 {
		body = append(body, pad+Strong.Render(Bytes(m.info.Free)+" free")+Meta.Render(" of "+Bytes(m.info.Total)))
		if roomy {
			parts := m.volumeParts()
			body = append(body, pad+CategoryBar(parts, m.info.Total, inner))
			if m.info.Home != nil {
				body = append(body, pad+CategoryLegend(parts, inner))
			} else {
				body = append(body, pad+Meta.Render("Open Analyze once to see what fills the disk."))
			}
		}
		body = append(body, "")
	}
	if roomy {
		body = append(body, pad+Meta.Render("WHAT DO YOU WANT TO DO?"), "")
	}
	for i, e := range homeEntries {
		current := i == m.cursor
		marker, number, name := "  ", Meta, Normal
		if current {
			marker, number, name = Safe.Render("▸ "), Strong, Strong
		}
		left := marker + number.Render(fmt.Sprintf("%d  ", i+1)) + name.Render(fmt.Sprintf("%-10s", e.name))
		stat := m.stat(e)
		if size != Narrow {
			room := inner - Width(left) - Width(stat) - 2
			desc := ansi.Truncate(e.desc, max(0, room), "…")
			if current {
				left += Normal.Render(desc)
			} else {
				left += Meta.Render(desc)
			}
		}
		body = append(body, pad+Between(left, stat, inner))
	}

	e := homeEntries[m.cursor]
	if roomy && size != Narrow {
		body = append(body, "")
		body = append(body, hintBox(e.name, e.hint, inner, pad)...)
	} else if f.Status == "" {
		f.Status = Meta.Render(e.hint[0])
	}

	f.Body = body
	f.Primary = Key{"enter", e.primary}
	f.Keys = []Key{{"↑↓", "move"}, {"1-4", "jump"}, {"?", "help"}, {"q", "quit"}}
	return f.Render()
}

// volumeParts splits the used space: the categories of the home folder from
// the last Analyze, and the rest of the disk. Without a result, all used
// space is one part.
func (m *Home) volumeParts() []CategoryShare {
	used := max(0, m.info.Total-m.info.Free)
	h := m.info.Home
	if h == nil {
		return []CategoryShare{{"other", "Used", used}}
	}
	titles := catalog.LoadStorage()
	var parts []CategoryShare
	var home int64
	for _, c := range h.Categories {
		if c.ID == catalog.OtherCategory {
			continue
		}
		parts = append(parts, CategoryShare{c.ID, titles.Title(c.ID), c.Allocated})
		home += c.Allocated
	}
	// Other in the home folder and all outside it share one grey part, so
	// that the bar has few colors.
	if rest := used - home; rest > 0 {
		parts = append(parts, CategoryShare{"other", "System and other", rest})
	}
	return parts
}

// hintBox explains the highlighted entry in a rounded box.
func hintBox(title string, text []string, width int, pad string) []string {
	rule := max(0, width-Width(title)-5)
	lines := []string{pad + Meta.Render("╭─ ") + Strong.Render(title) + Meta.Render(" "+strings.Repeat("─", rule)+"╮")}
	for _, paragraph := range text {
		for _, line := range strings.Split(ansi.Wordwrap(paragraph, max(1, width-4), ""), "\n") {
			fill := strings.Repeat(" ", max(0, width-4-Width(line)))
			lines = append(lines, pad+Meta.Render("│ ")+Normal.Render(line)+fill+Meta.Render(" │"))
		}
	}
	return append(lines, pad+Meta.Render("╰"+strings.Repeat("─", max(0, width-2))+"╯"))
}

func (m *Home) helpLines(pad string) []string {
	rows := [][2]string{
		{"handbyte", "This screen"},
		{"handbyte clean", "Review and clean caches and build output"},
		{"handbyte analyze [path]", "Browse a folder by size, read-only"},
		{"handbyte history", "Past cleanup actions"},
		{"handbyte restore <id>", "Put a Trash action back"},
		{"handbyte config", "Project roots and protected paths"},
		{"handbyte help", "All commands and options"},
	}
	lines := []string{pad + Meta.Render("COMMANDS"), ""}
	for _, r := range rows {
		lines = append(lines, pad+Title.Render(fmt.Sprintf("%-26s", r[0]))+Meta.Render(r[1]))
	}
	return lines
}

// RunHome shows the home screen and returns the chosen action.
func RunHome(info HomeInfo) (Action, error) {
	m := NewHome(info)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		return ActionQuit, err
	}
	return m.Choice, nil
}
