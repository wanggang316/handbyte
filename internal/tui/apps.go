package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/wanggang316/handbyte/internal/apps"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
)

// AppsOptions connects the apps screen to the inventory and the executor.
// Tests replace the functions.
type AppsOptions struct {
	Home      string
	Inventory func(ctx context.Context) ([]apps.App, error)
	Sizes     func(ctx context.Context, list []apps.App, done func(int, int64))
	Known     func(list []apps.App) []string
	// Related returns the files of an app in the home folder; all is the
	// inventory. System returns its files outside the home folder, which
	// are listed only.
	Related   func(app apps.App, all []apps.App, withData bool) []apps.Related
	System    func(app apps.App, all []apps.App) []apps.Related
	Leftovers func(known, names []string) []apps.LeftoverGroup
	Uninstall func(ctx context.Context, plan cleaner.AppPlan) (string, []cleaner.Result, error)
	Now       func() time.Time
	// Open is the app to review at start, by name or bundle ID.
	Open string
	// WithData selects the app's data at start.
	WithData bool
	// Mouse turns on clicks and the scroll wheel.
	Mouse bool
}

type appsTab int

const (
	tabAll appsTab = iota
	tabUnused
	tabLargest
	tabLeftovers
)

var appsTabNames = []string{"All", "Unused", "Largest", "Leftovers"}

type appsLoadedMsg struct {
	list []apps.App
	err  error
}
type appSizeMsg struct {
	index int
	size  int64
}
type leftoversMsg []apps.LeftoverGroup
type uninstallDoneMsg struct {
	group   string
	results []cleaner.Result
	err     error
}

// reviewFile is one line of an uninstall review.
type reviewFile struct {
	path     string
	label    string
	class    string
	conf     string
	size     int64
	selected bool
	isDir    bool
	bundle   bool
	// locked files belong to root: listed, never selected.
	locked bool
}

// Apps is the app list: facts, filters, uninstall and leftovers.
type Apps struct {
	opts          AppsOptions
	width, height int
	loading       bool
	err           error
	list          []apps.App
	sizesDone     int
	sizes         chan appSizeMsg
	leftovers     []apps.LeftoverGroup
	tab           appsTab
	cursor        int
	query         string
	searching     bool
	notice        string

	// review is the uninstall review; nil when the list shows.
	review      *appsReview
	result      *uninstallDoneMsg
	resultTitle string
	opened      bool
	// listTop and listOffset place the visible list on the screen.
	listTop, listOffset int
}

type appsReview struct {
	title   string
	app     *apps.App
	reset   bool
	files   []reviewFile
	cursor  int
	refusal string
	working bool
	locked  int
}

// NewApps returns the apps screen.
func NewApps(opts AppsOptions) *Apps {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Apps{opts: opts, loading: true}
}

func (m *Apps) Init() tea.Cmd {
	inventory := m.opts.Inventory
	return func() tea.Msg {
		list, err := inventory(context.Background())
		return appsLoadedMsg{list, err}
	}
}

func (m *Apps) waitSize() tea.Cmd {
	ch := m.sizes
	return func() tea.Msg {
		if s, ok := <-ch; ok {
			return s
		}
		return nil
	}
}

func (m *Apps) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case appsLoadedMsg:
		m.loading = false
		m.list, m.err = msg.list, msg.err
		var cmds []tea.Cmd
		if m.opts.Sizes != nil && len(m.list) > 0 {
			ch := make(chan appSizeMsg, len(m.list))
			m.sizes = ch
			list := append([]apps.App(nil), m.list...)
			sizes := m.opts.Sizes
			go func() {
				sizes(context.Background(), list, func(i int, n int64) { ch <- appSizeMsg{i, n} })
				close(ch)
			}()
			cmds = append(cmds, m.waitSize())
		}
		if m.opts.Leftovers != nil {
			list, known, leftovers := m.list, m.opts.Known, m.opts.Leftovers
			cmds = append(cmds, func() tea.Msg {
				var names []string
				for _, a := range list {
					names = append(names, a.Name)
				}
				return leftoversMsg(leftovers(known(list), names))
			})
		}
		if m.opts.Open != "" && !m.opened {
			m.opened = true
			if a := m.find(m.opts.Open); a != nil {
				m.openReview(a, false)
			} else {
				m.notice = Review.Render("No app named " + m.opts.Open + ".")
			}
		}
		return m, tea.Batch(cmds...)
	case appSizeMsg:
		if msg.index < len(m.list) {
			m.list[msg.index].Size = msg.size
			m.sizesDone++
		}
		return m, m.waitSize()
	case leftoversMsg:
		m.leftovers = msg
	case uninstallDoneMsg:
		m.result = &msg
		m.review = nil
	case tea.KeyMsg:
		return m.key(msg)
	case tea.MouseMsg:
		if m.review != nil || m.result != nil || m.loading {
			return m, nil
		}
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			m.cursor = max(0, m.cursor-1)
		case msg.Button == tea.MouseButtonWheelDown:
			m.cursor = min(m.rowCount()-1, m.cursor+1)
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			if row, ok := MouseRow(msg.Y, m.listTop, m.listOffset, m.rowCount()); ok {
				if row == m.cursor {
					return m.key(tea.KeyMsg{Type: tea.KeyEnter})
				}
				m.cursor = row
			}
		}
	}
	return m, nil
}

func (m *Apps) find(name string) *apps.App {
	for i := range m.list {
		a := &m.list[i]
		if strings.EqualFold(a.Name, name) || strings.EqualFold(a.BundleID, name) {
			return a
		}
	}
	return nil
}

// visible is the list of the current tab and search, as indexes.
func (m *Apps) visible() []int {
	var out []int
	q := strings.ToLower(m.query)
	now := m.opts.Now()
	for i, a := range m.list {
		if q != "" && !strings.Contains(strings.ToLower(a.Name), q) && !strings.Contains(strings.ToLower(a.BundleID), q) {
			continue
		}
		if m.tab == tabUnused && !a.Unused(now) {
			continue
		}
		out = append(out, i)
	}
	switch m.tab {
	case tabLargest:
		sort.SliceStable(out, func(i, j int) bool { return m.list[out[i]].Size > m.list[out[j]].Size })
	case tabUnused:
		sort.SliceStable(out, func(i, j int) bool { return m.list[out[i]].LastUsed.Before(m.list[out[j]].LastUsed) })
	}
	return out
}

func (m *Apps) openReview(a *apps.App, reset bool) {
	r := &appsReview{app: a, reset: reset, title: a.Name}
	if !reset {
		r.files = append(r.files, reviewFile{path: a.Path, label: tilde(a.Path), class: "app", conf: "app", size: a.Size, selected: true, isDir: true, bundle: true})
	}
	related := m.opts.Related(*a, m.list, m.opts.WithData)
	for _, f := range related {
		r.files = append(r.files, reviewFile{path: f.Path, label: tilde(f.Path), class: f.Class, conf: f.Confidence, size: f.Size, selected: f.Selected, isDir: f.IsDir})
	}
	if m.opts.System != nil {
		for _, f := range m.opts.System(*a, m.list) {
			r.files = append(r.files, reviewFile{path: f.Path, label: f.Path, class: f.Class, conf: "root", size: f.Size, isDir: f.IsDir, locked: true})
			r.locked++
		}
	}
	switch {
	case a.System:
		r.refusal = a.Name + " is part of macOS and cannot be uninstalled."
	case a.Source == apps.SourceHomebrew && !reset:
		r.refusal = "Installed with Homebrew. Run: handbyte uninstall --brew " + a.CaskToken + "   Press R to remove only its files."
	case a.SystemExtension && !reset:
		r.refusal = a.Name + " installs a system extension. Use its own uninstaller, or turn the extension off in System Settings first."
	case a.Running && !reset:
		r.refusal = a.Name + " is running. Quit it, then press r to check again."
	}
	m.review = r
}

func (m *Apps) openLeftovers(g apps.LeftoverGroup) {
	r := &appsReview{title: g.BundleID, reset: true}
	for _, l := range g.Items {
		r.files = append(r.files, reviewFile{path: l.Path, label: tilde(l.Path), class: l.Class, conf: "leftover", size: l.Size, isDir: l.IsDir})
	}
	m.review = r
}

func (m *Apps) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	m.notice = ""
	if m.result != nil {
		switch key {
		case "q":
			return m, tea.Quit
		case "esc", "enter":
			m.result = nil
			m.loading = true
			return m, m.Init()
		}
		return m, nil
	}
	if m.review != nil {
		return m.reviewKey(key)
	}
	if m.searching {
		switch msg.Type {
		case tea.KeyEsc:
			m.searching, m.query = false, ""
		case tea.KeyEnter:
			m.searching = false
		case tea.KeyBackspace:
			if m.query != "" {
				m.query = m.query[:len(m.query)-1]
			}
		case tea.KeyRunes, tea.KeySpace:
			m.query += string(msg.Runes)
		}
		m.cursor = 0
		return m, nil
	}
	rows := m.rowCount()
	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(rows-1, m.cursor+1)
	case "1", "2", "3", "4":
		m.tab, m.cursor = appsTab(key[0]-'1'), 0
	case "tab":
		m.tab, m.cursor = (m.tab+1)%4, 0
	case "/":
		m.searching = true
	case "enter", "u":
		if m.tab == tabLeftovers {
			if m.cursor < len(m.leftovers) {
				m.openLeftovers(m.leftovers[m.cursor])
			}
		} else if v := m.visible(); m.cursor < len(v) {
			m.openReview(&m.list[v[m.cursor]], false)
		}
	case "R":
		if m.tab != tabLeftovers {
			if v := m.visible(); m.cursor < len(v) {
				m.openReview(&m.list[v[m.cursor]], true)
			}
		}
	}
	return m, nil
}

func (m *Apps) rowCount() int {
	if m.tab == tabLeftovers {
		return len(m.leftovers)
	}
	return len(m.visible())
}

func (m *Apps) reviewKey(key string) (tea.Model, tea.Cmd) {
	r := m.review
	if r.working {
		return m, nil
	}
	switch key {
	case "esc", "q":
		m.review = nil
	case "up", "k":
		r.cursor = max(0, r.cursor-1)
	case "down", "j":
		r.cursor = min(len(r.files)-1, r.cursor+1)
	case " ":
		if r.cursor < len(r.files) && !r.files[r.cursor].bundle && !r.files[r.cursor].locked {
			r.files[r.cursor].selected = !r.files[r.cursor].selected
		}
	case "r":
		if r.app != nil {
			m.openReview(r.app, r.reset)
		}
	case "R":
		if r.app != nil && !r.reset {
			m.openReview(r.app, true)
		}
	case "y":
		if r.refusal != "" {
			m.notice = Review.Render(r.refusal)
			return m, nil
		}
		plan := cleaner.AppPlan{Name: r.title}
		if r.app != nil {
			plan.BundleID, plan.System, plan.Cask = r.app.BundleID, r.app.System, r.app.CaskToken
		}
		for _, f := range r.files {
			if !f.selected {
				continue
			}
			if f.bundle {
				id, err := scanner.EntryIdentity(f.path)
				if err != nil {
					m.notice = Review.Render("The app cannot be read: " + err.Error())
					return m, nil
				}
				plan.Bundle, plan.BundleIdentity = f.path, id
				continue
			}
			c, err := cleaner.NewAppFile(f.path, f.size)
			if err != nil {
				m.notice = Review.Render(tilde(f.path) + ": " + err.Error())
				return m, nil
			}
			plan.Files = append(plan.Files, c)
		}
		if plan.Bundle == "" && len(plan.Files) == 0 {
			m.notice = Meta.Render("Nothing is selected.")
			return m, nil
		}
		r.working = true
		m.resultTitle = r.title
		run := m.opts.Uninstall
		return m, func() tea.Msg {
			group, results, err := run(context.Background(), plan)
			return uninstallDoneMsg{group, results, err}
		}
	}
	return m, nil
}

func (m *Apps) View() string {
	if m.width == 0 {
		return ""
	}
	f := Frame{Width: m.width, Height: m.height, Crumb: []string{"Apps"}, Status: m.notice}
	pad := strings.Repeat(" ", Margin(m.width))
	inner := max(1, m.width-2*len(pad))
	switch {
	case m.loading:
		f.Body = []string{pad + Safe.Render(spinner[0]+" ") + Meta.Render("Reading the apps in /Applications and ~/Applications…")}
		f.Primary = Key{"q", "quit"}
	case m.err != nil:
		f.Body = []string{pad + Danger.Render("The apps cannot be read: "+m.err.Error())}
		f.Primary = Key{"q", "quit"}
	case m.result != nil:
		m.viewResult(&f, pad, inner)
	case m.review != nil:
		m.viewReview(&f, pad, inner)
	default:
		m.viewList(&f, pad, inner)
	}
	return f.Render()
}

func (m *Apps) viewList(f *Frame, pad string, inner int) {
	var total int64
	for _, a := range m.list {
		total += a.Size
	}
	f.Summary = fmt.Sprintf("%d apps · %s", len(m.list), Bytes(total))
	if m.sizesDone < len(m.list) && m.opts.Sizes != nil {
		f.Summary += fmt.Sprintf(" · measuring %d/%d", m.sizesDone, len(m.list))
	}
	tabs := ""
	for i, name := range appsTabNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if appsTab(i) == m.tab {
			tabs += CursorRow.Render(label)
		} else {
			tabs += Meta.Render(label)
		}
		tabs += " "
	}
	if m.searching || m.query != "" {
		tabs += Meta.Render("  / ") + Normal.Render(m.query)
		if m.searching {
			tabs += Safe.Render("▌")
		}
	}
	body := []string{pad + tabs, ""}
	height := max(1, m.height-len(body)-6)
	wide := SizeFor(m.width) == Wide
	listWidth := inner
	if wide {
		listWidth = inner * 6 / 10
	}
	var lines []string
	var detail []string
	now := m.opts.Now()
	if m.tab == tabLeftovers {
		offset := max(0, min(m.cursor-height/2, len(m.leftovers)-height))
		m.listTop, m.listOffset = HeaderLines(m.height)+len(body), offset
		for i := offset; i < len(m.leftovers) && i < offset+height; i++ {
			g := m.leftovers[i]
			marker, style := "  ", Normal
			if i == m.cursor {
				marker, style = Safe.Render("▸ "), Strong
			}
			right := fmt.Sprintf("%d items", len(g.Items))
			lines = append(lines, Between(marker+style.Render(CutMiddle(g.BundleID, max(6, listWidth-16))), Meta.Render(right), listWidth))
		}
		if len(m.leftovers) == 0 {
			lines = append(lines, Meta.Render("No data of removed apps was found."))
		}
		if wide && m.cursor < len(m.leftovers) {
			g := m.leftovers[m.cursor]
			detail = append(detail, Title.Render(CutMiddle(g.BundleID, inner-listWidth-3)), "", Meta.Render("No installed app has this bundle ID."), Meta.Render("Review it before you remove it."), "")
			for _, l := range g.Items {
				detail = append(detail, Meta.Render(CutMiddle(tilde(l.Path), inner-listWidth-3)))
			}
		}
	} else {
		v := m.visible()
		offset := max(0, min(m.cursor-height/2, len(v)-height))
		m.listTop, m.listOffset = HeaderLines(m.height)+len(body), offset
		for i := offset; i < len(v) && i < offset+height; i++ {
			a := m.list[v[i]]
			marker, style := "  ", Normal
			if i == m.cursor {
				marker, style = Safe.Render("▸ "), Strong
			}
			name := a.Name
			flags := ""
			if a.Running {
				flags += Safe.Render(" ●")
			}
			if a.System {
				flags += Meta.Render(" ◆")
			}
			size := "…"
			if a.Size > 0 {
				size = Bytes(a.Size)
			}
			right := Meta.Render(fmt.Sprintf("%-14s", lastUsed(a, now))) + Normal.Render(fmt.Sprintf("%11s", size))
			if SizeFor(m.width) == Narrow {
				right = Normal.Render(fmt.Sprintf("%11s", size))
			}
			left := marker + style.Render(CutMiddle(name, max(6, listWidth-Width(right)-6))) + flags
			lines = append(lines, Between(left, right, listWidth))
		}
		if len(v) == 0 {
			lines = append(lines, Meta.Render("No app matches."))
		}
		if wide && m.cursor < len(v) {
			detail = m.appDetail(m.list[v[m.cursor]], inner-listWidth-3, now)
		}
	}
	for i := 0; i < max(len(lines), len(detail)); i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		if wide {
			line += strings.Repeat(" ", max(0, listWidth-Width(line))) + Faint.Render(" │ ")
			if i < len(detail) {
				line += detail[i]
			}
		}
		body = append(body, pad+line)
	}
	f.Body = body
	f.Primary = Key{"enter", "uninstall…"}
	if m.tab == tabLeftovers {
		f.Primary = Key{"enter", "review leftovers"}
	}
	f.Keys = []Key{{"↑↓", "move"}, {"1-4", "filter"}, {"/", "search"}, {"R", "reset"}, {"q", "quit"}}
}

func lastUsed(a apps.App, now time.Time) string {
	if !a.LastUsedKnown {
		return "unknown"
	}
	return since(now.Sub(a.LastUsed)) + " ago"
}

func (m *Apps) appDetail(a apps.App, width int, now time.Time) []string {
	width = max(10, width)
	row := func(k, v string) string {
		return Meta.Render(fmt.Sprintf("%-12s", k)) + Normal.Render(ansi.Truncate(v, max(1, width-12), "…"))
	}
	lines := []string{Title.Render(CutMiddle(a.Name, width)), ""}
	lines = append(lines,
		row("Bundle ID", a.BundleID),
		row("Version", a.Version),
		row("Source", a.Source),
		row("Last used", lastUsed(a, now)),
		row("Size", Bytes(a.Size)),
		row("Runs on", strings.Join(a.Archs, ", ")),
	)
	if a.CaskToken != "" {
		lines = append(lines, row("Cask", a.CaskToken))
	}
	if a.Team != "" {
		lines = append(lines, row("Team", a.Team))
	}
	switch {
	case a.System:
		lines = append(lines, "", Meta.Render("◆ Part of macOS. Not offered."))
	case a.Running:
		lines = append(lines, "", Safe.Render("● Running"))
	}
	if !a.LastUsedKnown {
		lines = append(lines, "", Meta.Render("Spotlight has no date of last use."), Meta.Render("Unknown is not the same as never."))
	}
	return lines
}

func (m *Apps) viewReview(f *Frame, pad string, inner int) {
	r := m.review
	verb := "Uninstall"
	if r.reset {
		verb = "Remove files of"
	}
	if r.app == nil {
		verb = "Leftovers of"
	}
	f.Crumb = []string{"Apps", r.title}
	var selected int64
	count := 0
	for _, x := range r.files {
		if x.selected {
			selected += x.size
			count++
		}
	}
	body := []string{
		pad + Strong.Render(verb+" "+r.title) + Meta.Render(fmt.Sprintf("   %d of %d selected · %s", count, len(r.files), Bytes(selected))),
		pad + Meta.Render("Everything selected goes to the Trash as one group; handbyte restore puts it all back."),
		"",
	}
	if r.refusal != "" {
		body = append(body, pad+Review.Render("! "+r.refusal), "")
	}
	if r.locked > 0 {
		body = append(body, pad+Meta.Render(fmt.Sprintf("%d items marked [-] are outside your home folder and belong to root. Remove them in Finder.", r.locked)), "")
	}
	height := max(1, m.height-len(body)-6)
	offset := max(0, min(r.cursor-height/2, len(r.files)-height))
	for i := offset; i < len(r.files) && i < offset+height; i++ {
		x := r.files[i]
		box := "[ ]"
		switch {
		case x.locked:
			box = Meta.Render("[-]")
		case x.selected:
			box = Safe.Render("[x]")
		}
		marker, style := "  ", Normal
		if i == r.cursor {
			marker, style = Safe.Render("▸ "), Strong
		}
		tag := fmt.Sprintf("%-9s %-8s", x.class, x.conf)
		right := Meta.Render(tag) + Normal.Render(fmt.Sprintf("%11s", Bytes(x.size)))
		if SizeFor(m.width) == Narrow {
			right = Normal.Render(fmt.Sprintf("%11s", Bytes(x.size)))
		}
		left := marker + box + " " + style.Render(CutMiddle(x.label, max(6, inner-Width(right)-8)))
		body = append(body, pad+Between(left, right, inner))
	}
	f.Body = body
	if r.working {
		f.Primary = Key{"…", "moving to the Trash"}
		return
	}
	f.Primary = Key{"y", "move " + Bytes(selected) + " to the Trash"}
	f.Keys = []Key{{"space", "select"}, {"↑↓", "move"}, {"esc", "back"}}
	if r.app != nil && !r.reset {
		f.Keys = append(f.Keys, Key{"R", "keep the app, remove its files"})
	}
}

func (m *Apps) viewResult(f *Frame, pad string, inner int) {
	res := m.result
	f.Crumb = []string{"Apps", m.resultTitle}
	var moved int64
	n := 0
	var lines []string
	for _, r := range res.results {
		if r.Status == "trashed" || r.Status == "previewed" {
			moved += r.Moved + r.Previewed
			n++
			continue
		}
		lines = append(lines, pad+Review.Render("! ")+Normal.Render(CutMiddle(tilde(r.Path), inner/2))+Meta.Render("  "+r.Message))
	}
	head := Safe.Render("✓ ") + Strong.Render(fmt.Sprintf("Moved %d items to the Trash", n)) + Meta.Render(" · "+Bytes(moved))
	if res.err != nil {
		head = Review.Render("! ") + Strong.Render(res.err.Error())
	}
	body := []string{pad + head}
	if res.group != "" && n > 0 {
		body = append(body, pad+Meta.Render("Put everything back: ")+Normal.Render("handbyte restore "+res.group))
	}
	f.Body = append(append(body, ""), lines...)
	f.Primary = Key{"enter", "back to the list"}
	f.Keys = []Key{{"q", "quit"}}
}

// RunApps shows the app list until the user leaves it.
func RunApps(opts AppsOptions) error {
	options := []tea.ProgramOption{tea.WithAltScreen()}
	if opts.Mouse {
		options = append(options, tea.WithMouseCellMotion())
	}
	_, err := tea.NewProgram(NewApps(opts), options...).Run()
	return err
}
