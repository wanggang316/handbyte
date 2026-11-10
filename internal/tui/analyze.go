package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/wanggang316/handbyte/catalog"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/usage"
)

type analyzeProgressMsg usage.Progress
type analyzeDoneMsg struct {
	result *usage.Result
	err    error
}
type analyzeTickMsg time.Time

// level is one opened directory and the cursor in it.
type level struct {
	node   *usage.Node
	cursor int
}

// Analyze is the storage drill-down: a directory tree by size, and the
// list of the largest files. It is read-only.
type Analyze struct {
	path          string
	width, height int

	scanning bool
	started  time.Time
	elapsed  time.Duration
	progress usage.Progress
	updates  chan usage.Progress
	cancel   context.CancelFunc
	frame    int

	result     *usage.Result
	err        error
	levels     []level
	files      bool
	fileCursor int
	notice     string
	help       bool

	// reveal shows a path in Finder. Tests replace it.
	reveal func(path string) error
	opts   AnalyzeOptions
	// last is the summary of the previous scan of this folder.
	last    usage.Summary
	hasLast bool
	// categories of the finished scan, largest first.
	categories []CategoryShare
	collect    collector
}

// AnalyzeOptions connects the drill-down to the catalog and the cache.
type AnalyzeOptions struct {
	// Hints marks the folders that a cleanup rule covers.
	Hints scanner.Hints
	// Categories are the path rules of the storage categories.
	Categories []catalog.CategoryPath
	// SummaryFile keeps the last result of each folder. Empty: no cache.
	SummaryFile string
	// Collect moves picked items to the Trash. Tests replace it; nil uses
	// cleaner.Collect with the Finder.
	Collect func(context.Context, []cleaner.Collected) []cleaner.Result
}

// NewAnalyze returns the drill-down for a path.
func NewAnalyze(path string, opts AnalyzeOptions) *Analyze {
	return &Analyze{path: path, opts: opts, reveal: func(p string) error { return exec.Command("open", "-R", p).Run() }}
}

// rule returns the cleanup rule that covers a row of the opened folder.
func (m *Analyze) rule(r analyzeRow) (scanner.Preset, bool) {
	if r.node == nil || !r.node.Dir {
		return scanner.Preset{}, false
	}
	return m.opts.Hints.Rule(filepath.Join(m.currentPath(), r.node.Name))
}

func (m *Analyze) Init() tea.Cmd { return m.start() }

func (m *Analyze) start() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan usage.Progress, 1)
	m.scanning, m.started, m.cancel, m.updates = true, time.Now(), cancel, updates
	m.result, m.err, m.levels, m.files, m.fileCursor, m.progress, m.categories = nil, nil, nil, false, 0, usage.Progress{}, nil
	path := m.path
	if m.opts.SummaryFile != "" {
		if physical, err := filepath.EvalSymlinks(path); err == nil {
			if abs, err := filepath.Abs(physical); err == nil {
				m.last, m.hasLast = usage.LoadSummary(m.opts.SummaryFile, abs)
			}
		}
	}
	scan := func() tea.Msg {
		result, err := usage.Scan(ctx, path, usage.Options{Progress: func(p usage.Progress) {
			// Keep the newest snapshot only; the screen needs no history.
			select {
			case updates <- p:
			default:
			}
		}})
		close(updates)
		return analyzeDoneMsg{result, err}
	}
	return tea.Batch(scan, waitAnalyze(updates), analyzeTick())
}

func waitAnalyze(updates chan usage.Progress) tea.Cmd {
	return func() tea.Msg {
		if p, ok := <-updates; ok {
			return analyzeProgressMsg(p)
		}
		return nil
	}
}

func analyzeTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return analyzeTickMsg(t) })
}

func (m *Analyze) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case analyzeProgressMsg:
		m.progress = usage.Progress(msg)
		return m, waitAnalyze(m.updates)
	case analyzeTickMsg:
		if m.scanning {
			m.frame++
			m.elapsed = time.Since(m.started)
			return m, analyzeTick()
		}
	case analyzeDoneMsg:
		m.scanning = false
		m.result, m.err = msg.result, msg.err
		if msg.result != nil {
			m.elapsed = msg.result.Elapsed
			m.levels = []level{{node: msg.result.Root}}
			m.finish(msg.result)
		}
	case collectDoneMsg:
		m.collect.results = msg
		m.collect.stage = collectDone
	case tea.KeyMsg:
		return m.key(msg.String())
	}
	return m, nil
}

// finish sums the categories and keeps the summary of a complete scan.
func (m *Analyze) finish(r *usage.Result) {
	totals := usage.Categorize(r, m.opts.Categories)
	titles := catalog.LoadStorage()
	for _, t := range totals {
		m.categories = append(m.categories, CategoryShare{t.ID, titles.Title(t.ID), t.Allocated})
	}
	if m.opts.SummaryFile == "" || r.Cancelled {
		return
	}
	if err := usage.SaveSummary(m.opts.SummaryFile, usage.Summarize(r, totals, time.Now())); err != nil {
		m.notice = Review.Render("The result could not be kept for the next run: " + err.Error())
	}
}

func (m *Analyze) key(key string) (tea.Model, tea.Cmd) {
	m.notice = ""
	if key == "ctrl+c" || key == "q" {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if m.scanning {
		if key == "esc" {
			m.cancel()
		}
		return m, nil
	}
	if m.result == nil {
		if key == "esc" || key == "enter" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.help {
		m.help = false
		return m, nil
	}
	if m.collect.stage != collectOff {
		return m.collectKey(key)
	}
	if key == "?" {
		m.help = true
		return m, nil
	}
	current := &m.levels[len(m.levels)-1]
	rows := m.rows(current.node)
	switch key {
	case "up", "k":
		m.move(-1, len(rows))
	case "down", "j":
		m.move(1, len(rows))
	case "pgup":
		m.move(-m.listHeight(), len(rows))
	case "pgdown":
		m.move(m.listHeight(), len(rows))
	case "tab":
		m.files = !m.files
	case " ":
		if m.files {
			if m.fileCursor < len(m.result.Largest) {
				f := m.result.Largest[m.fileCursor]
				m.toggleCollected(f.Path, f.Allocated)
			}
		} else if current.cursor < len(rows) && rows[current.cursor].node != nil && rows[current.cursor].node.Skipped == "" {
			n := rows[current.cursor].node
			m.toggleCollected(filepath.Join(m.currentPath(), n.Name), n.Allocated)
		}
	case "c":
		if len(m.collect.items) > 0 {
			m.collect.stage, m.collect.cursor = collectReview, 0
		}
	case "r":
		return m, m.start()
	case "esc", "left", "h", "backspace":
		switch {
		case m.files:
			m.files = false
		case len(m.levels) > 1:
			m.levels = m.levels[:len(m.levels)-1]
		case key == "esc":
			return m, tea.Quit
		}
	case "enter", "right", "l":
		if m.files {
			if m.fileCursor < len(m.result.Largest) {
				m.show(m.result.Largest[m.fileCursor].Path)
			}
			break
		}
		if current.cursor >= len(rows) || rows[current.cursor].node == nil {
			break
		}
		node := rows[current.cursor].node
		switch {
		case node.Dir && node.Skipped == "":
			m.levels = append(m.levels, level{node: node})
		case key == "enter":
			m.show(filepath.Join(m.currentPath(), node.Name))
		}
	case "o":
		if m.files {
			if m.fileCursor < len(m.result.Largest) {
				m.show(m.result.Largest[m.fileCursor].Path)
			}
		} else if current.cursor < len(rows) && rows[current.cursor].node != nil {
			m.show(filepath.Join(m.currentPath(), rows[current.cursor].node.Name))
		} else {
			m.show(m.currentPath())
		}
	}
	return m, nil
}

func (m *Analyze) move(delta, rows int) {
	if m.files {
		m.fileCursor = max(0, min(len(m.result.Largest)-1, m.fileCursor+delta))
		return
	}
	current := &m.levels[len(m.levels)-1]
	current.cursor = max(0, min(rows-1, current.cursor+delta))
}

func (m *Analyze) show(path string) {
	if err := m.reveal(path); err != nil {
		m.notice = Review.Render("Finder could not show the item: " + err.Error())
		return
	}
	m.notice = Meta.Render("Shown in Finder: " + tilde(path))
}

// currentPath is the full path of the opened directory.
func (m *Analyze) currentPath() string {
	path := m.result.Path
	for _, l := range m.levels[1:] {
		path = filepath.Join(path, l.node.Name)
	}
	return path
}

func tilde(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			return "~" + path[len(home):]
		}
	}
	return path
}

// analyzeRow is one line of the drill-down. node is nil for the line that
// sums the small files.
type analyzeRow struct {
	node      *usage.Node
	name      string
	allocated int64
}

func (m *Analyze) rows(dir *usage.Node) []analyzeRow {
	rows := make([]analyzeRow, 0, len(dir.Children)+1)
	for _, c := range dir.Children {
		name := c.Name
		if c.Dir {
			name += "/"
		}
		rows = append(rows, analyzeRow{c, name, c.Allocated})
	}
	if dir.OtherFiles > 0 {
		rows = append(rows, analyzeRow{nil, fmt.Sprintf("%d smaller files", dir.OtherFiles), dir.OtherAllocated})
	}
	return rows
}

func (m *Analyze) listHeight() int {
	reserved := 6
	if m.height < compactHeight {
		reserved = 3
	}
	return max(1, m.height-reserved)
}

func (m *Analyze) View() string {
	if m.width == 0 {
		return ""
	}
	f := Frame{Width: m.width, Height: m.height, Crumb: []string{"Analyze"}, Status: m.notice}
	switch {
	case m.scanning:
		m.viewScanning(&f)
	case m.result == nil:
		f.Body = []string{strings.Repeat(" ", Margin(m.width)) + Danger.Render("The folder cannot be read: "+fmt.Sprint(m.err))}
		f.Primary = Key{"enter", "close"}
	case m.collect.stage != collectOff:
		m.viewCollect(&f)
	case m.files:
		m.viewFiles(&f)
	default:
		m.viewTree(&f)
	}
	if m.help {
		// The key bar of a narrow terminal is short; this lists all keys.
		pad := strings.Repeat(" ", Margin(m.width))
		body := []string{pad + Meta.Render("KEYS"), ""}
		for _, k := range append([]Key{f.Primary}, f.Keys...) {
			body = append(body, pad+Title.Render(fmt.Sprintf("%-8s", k.Key))+Meta.Render(k.Label))
		}
		f.Crumb, f.Body, f.Status = append(f.Crumb, "Help"), body, ""
		f.Primary, f.Keys = Key{"any key", "back"}, nil
	}
	return f.Render()
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *Analyze) viewScanning(f *Frame) {
	pad := strings.Repeat(" ", Margin(m.width))
	inner := max(1, m.width-2*len(pad))
	p := m.progress
	seconds := m.elapsed.Seconds()
	rate := 0.0
	if seconds > 0 {
		rate = float64(p.Files) / seconds
	}
	f.Summary = tilde(m.path)
	lastSize := map[string]int64{}
	found := Strong.Render(Bytes(p.Allocated)) + Meta.Render(" found so far")
	if m.hasLast {
		found += Meta.Render(fmt.Sprintf("   last time %s, %s ago", Bytes(m.last.Allocated), since(time.Since(m.last.ScannedAt))))
		for _, e := range m.last.Top {
			lastSize[e.Name] = e.Allocated
		}
	}
	body := []string{
		pad + found,
		pad + Meta.Render(fmt.Sprintf("%s files · %s files per second · %.1f s", count(p.Files), count(int64(rate)), seconds)),
		"",
	}
	top := append([]usage.TopProgress(nil), p.Top...)
	sort.SliceStable(top, func(i, j int) bool { return top[i].Allocated > top[j].Allocated })
	room := max(0, m.height-len(body)-6)
	for i, t := range top {
		if i >= room {
			break
		}
		name := t.Name
		if t.Dir {
			name += "/"
		}
		name = CutMiddle(name, max(8, inner-16))
		switch {
		case t.Done:
			body = append(body, pad+Between(Safe.Render("✓ ")+Normal.Render(name), Normal.Render(Bytes(t.Allocated)), inner))
		case t.Allocated > 0:
			body = append(body, pad+Between(Safe.Render(spinner[m.frame%len(spinner)]+" ")+Title.Render(name), Title.Render(Bytes(t.Allocated)+"+"), inner))
		default:
			// Until the folder is measured, its size of the last run stands
			// in, in a faint tone, so that the list is not empty.
			was := ""
			if size, ok := lastSize[t.Name]; ok {
				was = Faint.Render(Bytes(size))
			}
			body = append(body, pad+Between(Meta.Render("· "+name), was, inner))
		}
	}
	f.Body = body
	f.Status = Meta.Render(ansi.TruncateLeft(tilde(p.Current), max(0, Width(tilde(p.Current))-inner), "…"))
	f.Primary = Key{"esc", "stop and use what was found"}
	f.Keys = []Key{{"q", "quit"}}
}

func count(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func (m *Analyze) summary() string {
	s := Bytes(m.result.Root.Allocated)
	switch {
	case m.result.Cancelled:
		s += " · stopped, not complete"
	case m.result.Root.Incomplete:
		s += " · some folders not readable"
	}
	return s
}

func (m *Analyze) viewTree(f *Frame) {
	pad := strings.Repeat(" ", Margin(m.width))
	inner := max(1, m.width-2*len(pad))
	current := m.levels[len(m.levels)-1]
	rows := m.rows(current.node)
	f.Summary = m.summary()

	listWidth := inner
	wide := SizeFor(m.width) == Wide
	if wide {
		listWidth = inner * 6 / 10
	}
	head := Title.Render(CutMiddle(tilde(m.currentPath()), max(8, listWidth-14))) + Meta.Render("  "+Bytes(current.node.Allocated))
	body := []string{pad + head}
	if len(m.levels) == 1 && len(m.categories) > 0 && m.height >= compactHeight {
		body = append(body, pad+CategoryBar(m.categories, m.result.Root.Allocated, inner), pad+CategoryLegend(m.categories, inner))
	}
	body = append(body, "")

	height := max(1, m.listHeight()-len(body))
	offset := max(0, min(current.cursor-height/2, len(rows)-height))
	var largest int64
	for _, r := range rows {
		largest = max(largest, r.allocated)
	}
	var list []string
	for i := offset; i < len(rows) && i < offset+height; i++ {
		list = append(list, m.treeRow(rows[i], current.node.Allocated, largest, listWidth, i == current.cursor))
	}
	if len(rows) == 0 {
		list = append(list, Meta.Render("This folder is empty."))
	}
	var detail []string
	if wide && current.cursor < len(rows) {
		detail = m.detail(rows[current.cursor], current.node, inner-listWidth-3)
	}
	for i := 0; i < max(len(list), len(detail)); i++ {
		line := ""
		if i < len(list) {
			line = list[i]
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

	f.Primary = Key{"enter", "show in Finder"}
	if current.cursor < len(rows) {
		if n := rows[current.cursor].node; n != nil && n.Dir && n.Skipped == "" {
			f.Primary = Key{"enter", "open " + CutMiddle(rows[current.cursor].name, 24)}
		}
	}
	if f.Status == "" && !wide && current.cursor < len(rows) && rows[current.cursor].node != nil {
		f.Status = Meta.Render(tilde(filepath.Join(m.currentPath(), rows[current.cursor].node.Name)))
	}
	back := Key{"←", "up"}
	if len(m.levels) == 1 {
		back = Key{"esc", "close"}
	}
	f.Keys = []Key{{"↑↓", "move"}, back, {"space", "pick for Trash"}, {"tab", "largest files"}, {"o", "Finder"}, {"r", "scan again"}, {"q", "quit"}}
	m.collectKeys(f)
}

// collectKeys puts the review of the picked items first in the key bar.
func (m *Analyze) collectKeys(f *Frame) {
	if len(m.collect.items) == 0 {
		return
	}
	f.Keys = append([]Key{f.Primary}, f.Keys...)
	f.Primary = Key{"c", "review " + m.collect.label()}
}

// treeRow draws one line: marker, name, size, bar and percent. The size
// column is never cut; the name is cut in the middle. The percent is the
// share of the folder. The bar compares the row with the largest row, so
// that a folder of many small items still shows a clear order.
func (m *Analyze) treeRow(r analyzeRow, parent, largest int64, width int, current bool) string {
	share := 0.0
	if parent > 0 {
		share = float64(r.allocated) / float64(parent)
	}
	size := Bytes(r.allocated)
	note := ""
	if n := r.node; n != nil {
		switch {
		case n.Skipped != "":
			size, note = "—", n.Skipped
		case n.Incomplete:
			size = "≥ " + size
		}
	}
	right := fmt.Sprintf("%12s", size)
	barWidth := 0
	if SizeFor(m.width) != Narrow {
		barWidth = 10
	}
	nameWidth := max(6, width-2-12-1-barWidth-6)
	name := CutMiddle(r.name, nameWidth)
	if note != "" {
		name = CutMiddle(r.name+"  ("+note+")", nameWidth)
	}
	name += strings.Repeat(" ", max(0, nameWidth-Width(name)))
	percent := fmt.Sprintf("%4.0f%%", share*100)

	style, marker := Normal, "  "
	if r.node == nil || r.node.Skipped != "" {
		style = Meta
	}
	picked := r.node != nil && m.collectedMarker(m.currentPath(), r.node.Name)
	if picked {
		marker = Review.Render("◆ ")
	}
	if current {
		style, marker = Strong, Safe.Render("▸ ")
		if picked {
			marker = Review.Render("◆ ")
		}
	}
	line := marker + style.Render(name) + " " + style.Render(right)
	if barWidth > 0 {
		filled := 0
		if largest > 0 {
			filled = int(float64(r.allocated)/float64(largest)*float64(barWidth) + 0.5)
		}
		line += " " + Normal.Render(strings.Repeat("█", filled)) + Faint.Render(strings.Repeat("░", barWidth-filled))
	}
	line += Meta.Render(" " + percent)
	if r.node != nil && r.node.Incomplete && r.node.Skipped == "" {
		line += Review.Render(" !")
	}
	// The mark has its own column, so that a marked row is as wide as the
	// others and the detail pane stays aligned.
	if _, ok := m.rule(r); ok {
		line += Safe.Render(" ●")
	} else {
		line += "  "
	}
	return line
}

// detail describes the highlighted row in the right pane of a wide terminal.
func (m *Analyze) detail(r analyzeRow, parent *usage.Node, width int) []string {
	width = max(10, width)
	if r.node == nil {
		return []string{Title.Render(r.name), "", Meta.Render("On disk     ") + Normal.Render(Bytes(r.allocated)), "", Meta.Render("Files too small for the list.")}
	}
	n := r.node
	lines := []string{Title.Render(CutMiddle(r.name, width)), ""}
	for _, part := range strings.Split(ansi.Wrap(tilde(filepath.Join(m.currentPath(), n.Name)), width, "/"), "\n") {
		lines = append(lines, Meta.Render(part))
	}
	lines = append(lines, "",
		Meta.Render("On disk     ")+Normal.Render(Bytes(n.Allocated)),
		Meta.Render("Content     ")+Normal.Render(Bytes(n.Logical)),
	)
	if n.Dir {
		lines = append(lines, Meta.Render("Files       ")+Normal.Render(count(n.Files)))
	}
	if parent.Allocated > 0 {
		lines = append(lines, Meta.Render("Share       ")+Normal.Render(fmt.Sprintf("%.1f%% of this folder", float64(n.Allocated)/float64(parent.Allocated)*100)))
	}
	if rule, ok := m.rule(r); ok {
		lines = append(lines, "", Safe.Render("● Cleanable"))
		for _, part := range strings.Split(ansi.Wordwrap(rule.Description, width, ""), "\n") {
			lines = append(lines, Meta.Render(part))
		}
		lines = append(lines, Meta.Render("Review it: ")+Normal.Render("handbyte clean"))
	}
	switch {
	case n.Skipped != "":
		lines = append(lines, "", Meta.Render("Not measured: "+n.Skipped+"."))
	case n.Err != "":
		lines = append(lines, "", Review.Render("! Not readable. The size is a lower bound."))
	case n.Incomplete:
		lines = append(lines, "", Review.Render("! A folder below is not readable."), Review.Render("  The size is a lower bound."))
	}
	return lines
}

func (m *Analyze) viewFiles(f *Frame) {
	pad := strings.Repeat(" ", Margin(m.width))
	inner := max(1, m.width-2*len(pad))
	f.Crumb = []string{"Analyze", "Largest files"}
	f.Summary = m.summary()
	files := m.result.Largest
	body := []string{pad + Title.Render(CutMiddle(tilde(m.result.Path), max(8, inner-30))) + Meta.Render(fmt.Sprintf("  the %d largest files", len(files))), ""}
	height := max(1, m.listHeight()-2)
	offset := max(0, min(m.fileCursor-height/2, len(files)-height))
	for i := offset; i < len(files) && i < offset+height; i++ {
		rel, err := filepath.Rel(m.result.Path, files[i].Path)
		if err != nil {
			rel = files[i].Path
		}
		style, marker := Normal, "  "
		if m.collect.has(files[i].Path) {
			marker = Review.Render("◆ ")
		}
		if i == m.fileCursor {
			style = Strong
			if !m.collect.has(files[i].Path) {
				marker = Safe.Render("▸ ")
			}
		}
		name := CutMiddle(rel, max(6, inner-2-13))
		body = append(body, pad+Between(marker+style.Render(name), style.Render(Bytes(files[i].Allocated)), inner))
	}
	if len(files) == 0 {
		body = append(body, pad+Meta.Render("No files."))
	}
	f.Body = body
	f.Primary = Key{"enter", "show in Finder"}
	f.Keys = []Key{{"↑↓", "move"}, {"space", "pick for Trash"}, {"tab", "folders"}, {"r", "scan again"}, {"q", "quit"}}
	m.collectKeys(f)
}

// since says how long ago, in the largest whole unit.
func since(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

// RunAnalyze shows the drill-down for a path until the user leaves it.
func RunAnalyze(path string, opts AnalyzeOptions) error {
	_, err := tea.NewProgram(NewAnalyze(path, opts), tea.WithAltScreen()).Run()
	return err
}
