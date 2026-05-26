// Package ui implements the hand-clean Bubble Tea TUI.
package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wanggang316/hand-clean/internal/cleaner"
	"github.com/wanggang316/hand-clean/internal/scanner"
)

// phase tracks which screen is currently rendered.
type phase int

const (
	phaseSplash phase = iota
	phaseScanning
	phaseList
	phaseConfirm
	phaseCleaning
	phaseDone
)

// row is a flattened list entry: either a group header or an item.
type row struct {
	isGroup bool
	group   scanner.Category // valid if isGroup
	item    *scanner.Item    // valid if !isGroup
	groupOf scanner.Category // group the item belongs to (for hiding when collapsed)
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg        scanner.Config
	cleanMode  cleaner.Mode
	freeBefore int64

	width  int
	height int

	phase phase

	spin spinner.Model

	// scanning state
	scanCtx    context.Context
	scanCancel context.CancelFunc
	scanProg   scanner.Progress
	scanErr    error
	scanCh     chan scanner.Progress
	cleanCh    chan cleaner.Progress

	// list state
	items     []scanner.Item
	collapsed map[scanner.Category]bool
	rows      []row
	cursor    int
	selected  map[string]bool // path -> selected

	// cleaning state
	cleanProg cleaner.Progress
	cleanRes  []cleaner.Result

	helpOpen bool
}

// New constructs a model with sensible defaults.
func New(cfg scanner.Config, mode cleaner.Mode, freeBefore int64) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = primary
	return &Model{
		cfg:        cfg,
		cleanMode:  mode,
		freeBefore: freeBefore,
		spin:       sp,
		phase:      phaseSplash,
		collapsed:  make(map[scanner.Category]bool),
		selected:   make(map[string]bool),
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.spin.Tick
}

// ---- messages ----

type scanStartMsg struct{}
type scanProgressMsg scanner.Progress
type scanDoneMsg struct {
	items []scanner.Item
	err   error
}
type cleanProgressMsg cleaner.Progress
type cleanDoneMsg struct {
	results []cleaner.Result
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case scanStartMsg:
		return m.startScan()

	case scanProgressMsg:
		m.scanProg = scanner.Progress(msg)
		return m, m.waitScanProgress()

	case scanDoneMsg:
		m.items = msg.items
		m.scanErr = msg.err
		m.rebuildRows()
		m.phase = phaseList
		return m, nil

	case cleanProgressMsg:
		m.cleanProg = cleaner.Progress(msg)
		if msg.LastResult != nil {
			m.cleanRes = append(m.cleanRes, *msg.LastResult)
		}
		return m, m.waitCleanProgress()

	case cleanDoneMsg:
		m.cleanRes = msg.results
		m.phase = phaseDone
		return m, nil
	}

	return m, nil
}

// ---- key handling ----

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpOpen {
		m.helpOpen = false
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		if m.scanCancel != nil {
			m.scanCancel()
		}
		return m, tea.Quit
	case "?":
		m.helpOpen = true
		return m, nil
	}

	switch m.phase {
	case phaseSplash:
		switch msg.String() {
		case "enter", " ":
			return m, func() tea.Msg { return scanStartMsg{} }
		}
	case phaseScanning:
		switch msg.String() {
		case "esc":
			if m.scanCancel != nil {
				m.scanCancel()
			}
		}
	case phaseList:
		return m.handleListKey(msg)
	case phaseConfirm:
		switch msg.String() {
		case "y", "Y", "enter":
			return m.startClean()
		case "n", "N", "esc":
			m.phase = phaseList
		}
	case phaseDone:
		switch msg.String() {
		case "enter", " ":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.rows) - 1
	case "pgdown", "ctrl+d":
		m.moveCursor(10)
	case "pgup", "ctrl+u":
		m.moveCursor(-10)
	case " ":
		m.toggleAtCursor()
	case "a":
		m.toggleGroupAtCursor()
	case "A":
		m.toggleAllSafe()
	case "c":
		for k := range m.selected {
			delete(m.selected, k)
		}
	case "enter":
		m.toggleCollapseAtCursor()
	case "d":
		if len(m.selectedItems()) > 0 {
			m.phase = phaseConfirm
		}
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m *Model) toggleAtCursor() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.isGroup {
		m.toggleGroup(r.group)
		return
	}
	if m.selected[r.item.Path] {
		delete(m.selected, r.item.Path)
	} else {
		m.selected[r.item.Path] = true
	}
}

func (m *Model) toggleGroupAtCursor() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	cat := r.group
	if !r.isGroup {
		cat = r.groupOf
	}
	m.toggleGroup(cat)
}

func (m *Model) toggleGroup(cat scanner.Category) {
	// If any item in the group is unselected, select all; otherwise unselect all.
	allSel := true
	for _, it := range m.items {
		if it.Category != cat {
			continue
		}
		if !m.selected[it.Path] {
			allSel = false
			break
		}
	}
	for _, it := range m.items {
		if it.Category != cat {
			continue
		}
		if allSel {
			delete(m.selected, it.Path)
		} else {
			m.selected[it.Path] = true
		}
	}
}

func (m *Model) toggleAllSafe() {
	anyUnsel := false
	for _, it := range m.items {
		if it.Safety == scanner.SafetyAuto && !m.selected[it.Path] {
			anyUnsel = true
			break
		}
	}
	for _, it := range m.items {
		if it.Safety != scanner.SafetyAuto {
			continue
		}
		if anyUnsel {
			m.selected[it.Path] = true
		} else {
			delete(m.selected, it.Path)
		}
	}
}

func (m *Model) toggleCollapseAtCursor() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if !r.isGroup {
		return
	}
	m.collapsed[r.group] = !m.collapsed[r.group]
	m.rebuildRows()
}

// ---- list shaping ----

func (m *Model) rebuildRows() {
	// Group items by category, keeping size-sorted order within each.
	byCat := make(map[scanner.Category][]*scanner.Item)
	for i := range m.items {
		it := &m.items[i]
		byCat[it.Category] = append(byCat[it.Category], it)
	}
	// Order categories by total size desc.
	type catTotal struct {
		cat   scanner.Category
		bytes int64
	}
	var cats []catTotal
	for c, list := range byCat {
		var sum int64
		for _, it := range list {
			sum += it.Size
		}
		cats = append(cats, catTotal{c, sum})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].bytes > cats[j].bytes })

	m.rows = m.rows[:0]
	for _, c := range cats {
		m.rows = append(m.rows, row{isGroup: true, group: c.cat})
		if m.collapsed[c.cat] {
			continue
		}
		for _, it := range byCat[c.cat] {
			m.rows = append(m.rows, row{item: it, groupOf: c.cat})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) selectedItems() []scanner.Item {
	var out []scanner.Item
	for _, it := range m.items {
		if m.selected[it.Path] {
			out = append(out, it)
		}
	}
	return out
}

func (m *Model) selectedTotal() int64 {
	var sum int64
	for _, it := range m.items {
		if m.selected[it.Path] {
			sum += it.Size
		}
	}
	return sum
}

// ---- scan & clean commands ----

func (m *Model) startScan() (tea.Model, tea.Cmd) {
	m.phase = phaseScanning
	ctx, cancel := context.WithCancel(context.Background())
	m.scanCtx, m.scanCancel = ctx, cancel
	m.scanCh = make(chan scanner.Progress, 64)
	ch := m.scanCh
	prog := teaProgram
	go func() {
		items, err := scanner.Scan(ctx, m.cfg, ch)
		prog.Send(scanDoneMsg{items: items, err: err})
	}()
	return m, waitProgress(ch, func(p scanner.Progress) tea.Msg { return scanProgressMsg(p) })
}

func (m *Model) startClean() (tea.Model, tea.Cmd) {
	m.phase = phaseCleaning
	paths := []string{}
	sizes := map[string]int64{}
	for _, it := range m.selectedItems() {
		paths = append(paths, it.Path)
		sizes[it.Path] = it.Size
	}
	ctx := context.Background()
	m.cleanCh = make(chan cleaner.Progress, 32)
	ch := m.cleanCh
	prog := teaProgram
	go func() {
		results := cleaner.Clean(ctx, paths, sizes, m.cleanMode, ch)
		prog.Send(cleanDoneMsg{results: results})
	}()
	return m, waitProgress(ch, func(p cleaner.Progress) tea.Msg { return cleanProgressMsg(p) })
}

// waitProgress returns a Cmd that reads one value from ch and wraps it as a
// tea.Msg via convert. When the channel is closed it returns nil (no-op).
func waitProgress[T any](ch <-chan T, convert func(T) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return convert(p)
	}
}

func (m *Model) waitScanProgress() tea.Cmd {
	return waitProgress(m.scanCh, func(p scanner.Progress) tea.Msg { return scanProgressMsg(p) })
}

func (m *Model) waitCleanProgress() tea.Cmd {
	return waitProgress(m.cleanCh, func(p cleaner.Progress) tea.Msg { return cleanProgressMsg(p) })
}

// ---- View ----

func (m *Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if m.helpOpen {
		return m.renderHelp()
	}
	switch m.phase {
	case phaseSplash:
		return m.renderSplash()
	case phaseScanning:
		return m.renderScanning()
	case phaseList:
		return m.renderList()
	case phaseConfirm:
		return m.renderConfirm()
	case phaseCleaning:
		return m.renderCleaning()
	case phaseDone:
		return m.renderDone()
	}
	return ""
}

func (m *Model) renderSplash() string {
	title := titleStyle.Render("hand-clean")
	sub := mutedStyle.Render("macOS disk cleanup, with eyes open.")
	modeLabel := "move to Trash"
	switch m.cleanMode {
	case cleaner.ModeRemove:
		modeLabel = dangerStyle.Render("rm -rf") + " (irrevocable)"
	case cleaner.ModeDryRun:
		modeLabel = accentStyle.Render("dry-run") + " (no files touched)"
	}
	body := strings.Join([]string{
		title,
		sub,
		"",
		"  free space   " + humanBytes(m.freeBefore),
		"  delete mode  " + modeLabel,
		"",
		subtleStyle.Render("  scans Xcode, dev caches, project artifacts, VMs, …"),
		"",
		"  " + primary.Render("[enter]") + " start scan      " + mutedStyle.Render("[q] quit   [?] help"),
	}, "\n")
	return lipgloss.NewStyle().Padding(2, 2).Render(body)
}

func (m *Model) renderScanning() string {
	header := titleStyle.Render("hand-clean") + "  " + mutedStyle.Render("scanning…")
	stats := fmt.Sprintf(
		"  %s   %s in %s files, %d items so far",
		m.spin.View(),
		accentStyle.Render(humanBytes(m.scanProg.BytesSeen)),
		formatInt(m.scanProg.FilesSeen),
		m.scanProg.ItemsFound,
	)
	current := mutedStyle.Render("  " + truncate(m.scanProg.CurrentPath, m.width-4))
	footer := mutedStyle.Render("  [esc] cancel   [q] quit")
	return strings.Join([]string{header, "", stats, current, "", footer}, "\n")
}

func (m *Model) renderList() string {
	header := m.renderHeader()
	body := m.renderRows()
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *Model) renderHeader() string {
	selCount := len(m.selectedItems())
	totalCount := len(m.items)
	var total int64
	for _, it := range m.items {
		total += it.Size
	}
	left := titleStyle.Render("hand-clean") + "  " +
		mutedStyle.Render(fmt.Sprintf("%d items, %s found", totalCount, humanBytes(total)))
	right := ""
	if selCount > 0 {
		right = accentStyle.Render(fmt.Sprintf("%d selected · %s", selCount, humanBytes(m.selectedTotal())))
	} else {
		right = mutedStyle.Render("nothing selected")
	}
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return headerStyle.Width(m.width).Render(left + strings.Repeat(" ", pad) + right)
}

func (m *Model) renderRows() string {
	contentH := m.height - 4 // header + footer
	if contentH < 5 {
		contentH = 5
	}
	// scroll window around cursor
	start := 0
	if m.cursor >= contentH {
		start = m.cursor - contentH + 1
	}
	end := start + contentH
	if end > len(m.rows) {
		end = len(m.rows)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		r := m.rows[i]
		line := m.renderRow(r, i == m.cursor)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	// Pad remaining lines so the footer stays anchored.
	for i := end - start; i < contentH; i++ {
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *Model) renderRow(r row, cursor bool) string {
	if r.isGroup {
		marker := "▾"
		if m.collapsed[r.group] {
			marker = "▸"
		}
		// Count + total for the group.
		var bytes int64
		var count int
		for _, it := range m.items {
			if it.Category == r.group {
				bytes += it.Size
				count++
			}
		}
		text := fmt.Sprintf(" %s  %s  %s  %s",
			marker,
			groupStyle.Render(string(r.group)),
			mutedStyle.Render(fmt.Sprintf("(%d)", count)),
			accentStyle.Render(humanBytes(bytes)),
		)
		if cursor {
			return rowCursorStyle.Width(m.width).Render(text)
		}
		return text
	}

	it := r.item
	check := "[ ]"
	if m.selected[it.Path] {
		check = primary.Render("[✓]")
	}
	badge := safetyBadge(it.Safety)
	size := accentStyle.Render(rightPad(humanBytes(it.Size), 8))
	path := condensePath(it.Path)
	desc := subtleStyle.Render("· " + it.Description)
	// Width-bound the path so the row doesn't wrap.
	avail := m.width - 4 - 3 - 8 - 8 - 2
	if avail < 20 {
		avail = 20
	}
	pathPart := truncate(path, avail)
	text := fmt.Sprintf("    %s %s %s %s", check, badge, size, pathPart)
	if cursor {
		text = text + " " + desc
		return rowCursorStyle.Width(m.width).Render(text)
	}
	return text + " " + desc
}

func (m *Model) renderFooter() string {
	keys := []string{
		primary.Render("[space]") + " toggle",
		primary.Render("[a]") + " group",
		primary.Render("[A]") + " all safe",
		primary.Render("[c]") + " clear",
		primary.Render("[enter]") + " collapse",
		primary.Render("[d]") + " delete",
		mutedStyle.Render("[q] quit  [?] help"),
	}
	return footerStyle.Width(m.width).Render("  " + strings.Join(keys, "  "))
}

func (m *Model) renderConfirm() string {
	sel := m.selectedItems()
	total := m.selectedTotal()
	verb := "Move to Trash"
	switch m.cleanMode {
	case cleaner.ModeRemove:
		verb = dangerStyle.Render("Permanently delete")
	case cleaner.ModeDryRun:
		verb = accentStyle.Render("Dry-run (no changes)")
	}
	lines := []string{
		titleStyle.Render("Confirm"),
		"",
		fmt.Sprintf("  %s %d items totalling %s ?", verb, len(sel), accentStyle.Render(humanBytes(total))),
		"",
	}
	// Preview the top 8 items.
	preview := sel
	if len(preview) > 8 {
		preview = preview[:8]
	}
	for _, it := range preview {
		lines = append(lines, fmt.Sprintf("    %s  %s", rightPad(humanBytes(it.Size), 8), condensePath(it.Path)))
	}
	if len(sel) > len(preview) {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("    … and %d more", len(sel)-len(preview))))
	}
	lines = append(lines, "", "  "+primary.Render("[y]")+" confirm    "+mutedStyle.Render("[n] cancel"))

	box := dialogStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) renderCleaning() string {
	header := titleStyle.Render("hand-clean") + "  " + mutedStyle.Render("cleaning…")
	pct := 0.0
	if m.cleanProg.Total > 0 {
		pct = float64(m.cleanProg.Completed) / float64(m.cleanProg.Total)
	}
	bar := progressBar(pct, m.width-12)
	stats := fmt.Sprintf("  %s  %d / %d   %s freed",
		m.spin.View(),
		m.cleanProg.Completed, m.cleanProg.Total,
		accentStyle.Render(humanBytes(m.cleanProg.BytesFreed)),
	)
	current := mutedStyle.Render("  " + truncate(m.cleanProg.Current, m.width-4))
	return strings.Join([]string{header, "", "  " + bar, stats, current}, "\n")
}

func (m *Model) renderDone() string {
	var freed int64
	failed := 0
	for _, r := range m.cleanRes {
		if r.Err == nil {
			freed += r.Freed
		} else {
			failed++
		}
	}
	title := accentStyle.Render("✓ done")
	lines := []string{
		titleStyle.Render("hand-clean") + "  " + title,
		"",
		fmt.Sprintf("  cleaned %d items", len(m.cleanRes)-failed),
		fmt.Sprintf("  freed   %s", accentStyle.Render(humanBytes(freed))),
	}
	if failed > 0 {
		lines = append(lines, dangerStyle.Render(fmt.Sprintf("  failed  %d  (permission / busy)", failed)))
	}
	lines = append(lines, "", mutedStyle.Render("  [enter] exit"))
	return strings.Join(lines, "\n")
}

func (m *Model) renderHelp() string {
	lines := []string{
		titleStyle.Render("hand-clean help"),
		"",
		"  j/↓  k/↑       navigate",
		"  g G            jump to top / bottom",
		"  space          toggle selection",
		"  a              toggle whole group",
		"  A              toggle all auto-safe items",
		"  c              clear selection",
		"  enter          collapse / expand group",
		"  d              delete selected",
		"  q / ctrl+c     quit",
		"  ?              this help (any key dismisses)",
		"",
		mutedStyle.Render("  press any key to return"),
	}
	box := dialogStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ---- helpers ----

func safetyBadge(s scanner.Safety) string {
	switch s {
	case scanner.SafetyAuto:
		return badgeSafe
	case scanner.SafetyReview:
		return badgeReview
	default:
		return badgeCaution
	}
}

func humanBytes(n int64) string {
	const (
		k = 1024
		M = k * 1024
		G = M * 1024
		T = G * 1024
	)
	switch {
	case n >= T:
		return fmt.Sprintf("%.2f TiB", float64(n)/T)
	case n >= G:
		return fmt.Sprintf("%.2f GiB", float64(n)/G)
	case n >= M:
		return fmt.Sprintf("%.1f MiB", float64(n)/M)
	case n >= k:
		return fmt.Sprintf("%.1f KiB", float64(n)/k)
	}
	return fmt.Sprintf("%d B", n)
}

func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	r := []rune(s)
	// rough: trim from the left so the tail stays visible (paths).
	for len(r) > 0 && lipgloss.Width(string(r))+1 > max {
		r = r[1:]
	}
	return "…" + string(r)
}

func rightPad(s string, n int) string {
	if lipgloss.Width(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-lipgloss.Width(s))
}

func condensePath(p string) string {
	if home, err := homeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func progressBar(pct float64, width int) string {
	if width < 4 {
		width = 4
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(float64(width) * pct)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return primary.Render(bar) + fmt.Sprintf("  %3.0f%%", pct*100)
}

// ---- program plumbing ----
//
// Scanner & cleaner emit streams of Progress values via channels. Bubble Tea
// commands are pure, so we bridge by holding the active program reference
// and reading from per-model channels via tea.Cmd.

var teaProgram *tea.Program

// Run starts the TUI. It blocks until the user exits.
func Run(cfg scanner.Config, mode cleaner.Mode, freeBefore int64) error {
	m := New(cfg, mode, freeBefore)
	teaProgram = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := teaProgram.Run()
	return err
}

func homeDir() (string, error) {
	return os.UserHomeDir()
}
