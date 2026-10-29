// Package ui implements a review-first cleanup workflow.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
)

type phase int

const (
	phaseSplash phase = iota
	phaseScanning
	phaseList
	phaseConfirm
	phaseCleaning
	phaseDone
)

type row struct {
	isGroup bool
	group   string
	item    *scanner.Item
	groupOf string
}

type Model struct {
	detailsOpen             bool
	detailOffset            int
	restoring               bool
	reviewSelected          map[string]bool
	cfg                     scanner.Config
	cleanMode               cleaner.Mode
	freeBefore              int64
	width, height           int
	phase                   phase
	spin                    spinner.Model
	run                     uint64
	scanCancel, cleanCancel context.CancelFunc
	scanProg                scanner.Progress
	scanErr                 error
	scanCh                  chan scanner.Progress
	cleanCh                 chan cleaner.Progress
	items                   []scanner.Item
	collapsed               map[string]bool
	rows                    []row
	cursor                  int
	selected                map[string]bool
	cleanProg               cleaner.Progress
	cleanRes                []cleaner.Result
	helpOpen                bool
	query                   string
	editingQuery            bool
	filter                  int
	sortMode                int
	reviewOffset            int
	notice                  string
	explorePath             string
	exploreStack            []string
	reviewItems             []scanner.Item
	resultCursor            int
}

type scanStartMsg struct{}
type scanProgressMsg scanner.Progress
type scanDoneMsg struct {
	run   uint64
	items []scanner.Item
	err   error
}
type cleanProgressMsg cleaner.Progress
type cleanDoneMsg struct {
	run     uint64
	results []cleaner.Result
}
type noticeMsg string
type restoredMsg struct {
	id  string
	err error
}

func New(cfg scanner.Config, mode cleaner.Mode, freeBefore int64) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = primary
	return &Model{cfg: cfg, cleanMode: mode, freeBefore: freeBefore, spin: sp, collapsed: map[string]bool{}, selected: map[string]bool{}}
}
func (m *Model) Init() tea.Cmd {
	if m.explorePath != "" {
		return tea.Batch(m.spin.Tick, func() tea.Msg { return scanStartMsg{} })
	}
	return m.spin.Tick
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case scanStartMsg:
		return m.startScan()
	case noticeMsg:
		m.notice = string(msg)
		return m, nil
	case restoredMsg:
		m.restoring = false
		if msg.err != nil {
			m.notice = msg.err.Error()
		} else {
			m.notice = "Restored to the original path"
			for i := range m.cleanRes {
				if m.cleanRes[i].ID == msg.id {
					m.cleanRes[i].Status = "restored"
				}
			}
		}
		return m, nil
	case scanProgressMsg:
		if msg.Run != m.run || m.phase != phaseScanning {
			return m, nil
		}
		m.scanProg = scanner.Progress(msg)
		if msg.Items != nil {
			m.items = msg.Items
			m.applyExclusions()
			m.rebuildRows()
		}
		return m, m.waitScanProgress()
	case scanDoneMsg:
		if msg.run != m.run {
			return m, nil
		}
		if m.scanCancel != nil {
			m.scanCancel()
			m.scanCancel = nil
		}
		m.items, m.scanErr = msg.items, msg.err
		m.applyExclusions()
		m.phase = phaseList
		m.rebuildRows()
		if msg.err != nil {
			m.notice = "Scan incomplete: " + msg.err.Error()
		}
		return m, nil
	case cleanProgressMsg:
		if msg.Run != m.run || m.phase != phaseCleaning {
			return m, nil
		}
		m.cleanProg = cleaner.Progress(msg)
		return m, m.waitCleanProgress()
	case cleanDoneMsg:
		if msg.run != m.run {
			return m, nil
		}
		if m.cleanCancel != nil {
			m.cleanCancel()
			m.cleanCancel = nil
		}
		m.cleanRes = msg.results
		m.phase = phaseDone
		m.resultCursor = 0
		return m, nil
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.restoring {
		m.notice = "Waiting for restoration to finish"
		return m, nil
	}
	if m.detailsOpen {
		switch key {
		case "esc", "enter":
			m.detailsOpen = false
		case "j", "down":
			m.detailOffset++
		case "k", "up":
			m.detailOffset = max(0, m.detailOffset-1)
		case "pgdown":
			m.detailOffset += max(1, m.height-4)
		case "pgup":
			m.detailOffset = max(0, m.detailOffset-max(1, m.height-4))
		}
		return m, nil
	}
	if m.helpOpen {
		m.helpOpen = false
		return m, nil
	}
	if m.editingQuery {
		switch key {
		case "esc", "enter":
			m.editingQuery = false
		case "backspace", "ctrl+h":
			r := []rune(m.query)
			if len(r) > 0 {
				m.query = string(r[:len(r)-1])
			}
		case "ctrl+u":
			m.query = ""
		default:
			if msg.Type == tea.KeyRunes {
				m.query += string(msg.Runes)
			}
		}
		m.cursor = 0
		m.rebuildRows()
		return m, nil
	}
	if key == "q" || key == "ctrl+c" {
		if m.phase == phaseCleaning {
			if m.cleanCancel != nil {
				m.cleanCancel()
			}
			m.notice = "Stopping after the current action finishes"
			return m, nil
		}
		if m.scanCancel != nil {
			m.scanCancel()
		}
		return m, tea.Quit
	}
	if key == "?" {
		m.helpOpen = true
		return m, nil
	}
	switch m.phase {
	case phaseSplash:
		switch key {
		case "1":
			m.cfg.Scope = "quick"
			return m.startScan()
		case "2":
			m.cfg.Scope = "projects"
			m.cfg.SkipProjectScan = false
			return m.startScan()
		case "3":
			home, _ := os.UserHomeDir()
			return m.startExplore(home)
		case "enter", " ":
			return m.startScan()
		}
	case phaseScanning, phaseList:
		if key == "esc" && m.phase == phaseScanning {
			if m.scanCancel != nil {
				m.scanCancel()
			}
			return m, nil
		}
		return m.handleListKey(msg)
	case phaseConfirm:
		switch key {
		case "y":
			return m.startClean()
		case "n", "esc":
			m.phase = phaseList
		case "j", "down":
			m.reviewOffset = min(m.reviewOffset+1, max(0, len(m.selectedItems())-m.reviewHeight()))
		case "k", "up":
			m.reviewOffset = max(0, m.reviewOffset-1)
		case "pgdown":
			m.reviewOffset = min(m.reviewOffset+m.reviewHeight(), max(0, len(m.selectedItems())-m.reviewHeight()))
		case "pgup":
			m.reviewOffset = max(0, m.reviewOffset-m.reviewHeight())
		}
	case phaseCleaning:
		if key == "esc" && m.cleanCancel != nil {
			m.cleanCancel()
			m.notice = "Stopping after the current action finishes"
		}
	case phaseDone:
		switch key {
		case "j", "down":
			m.resultCursor = min(max(0, len(m.cleanRes)-1), m.resultCursor+1)
		case "k", "up":
			m.resultCursor = max(0, m.resultCursor-1)
		case "enter":
			return m, tea.Quit
		case "r":
			m.explorePath = ""
			return m.startScan()
		case "f":
			m.selected = map[string]bool{}
			for _, r := range m.cleanRes {
				if r.Status == "failed" || r.Status == "skipped" || r.Status == "cancelled" {
					for _, it := range m.items {
						if it.Path == r.Path && it.Selectable() {
							m.selected[it.Path] = true
						}
					}
				}
			}
			if len(m.selected) > 0 {
				m.phase = phaseConfirm
				m.reviewOffset = 0
			} else {
				m.notice = "No eligible failed targets. Rescan to refresh evidence."
			}
		case "u":
			if m.resultCursor < len(m.cleanRes) && m.cleanRes[m.resultCursor].Status == "trashed" {
				m.restoring = true
				id := m.cleanRes[m.resultCursor].ID
				return m, func() tea.Msg { return restoredMsg{id: id, err: cleaner.Restore(context.Background(), id)} }
			}
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
		m.cursor = max(0, len(m.rows)-1)
	case "pgdown", "ctrl+d":
		m.moveCursor(m.listHeight())
	case "pgup", "ctrl+u":
		m.moveCursor(-m.listHeight())
	case "/":
		m.editingQuery = true
	case "s":
		m.sortMode = (m.sortMode + 1) % 3
		m.rebuildRows()
	case "f":
		m.filter = (m.filter + 1) % 4
		m.cursor = 0
		m.rebuildRows()
	case " ":
		m.toggleAtCursor()
	case "a":
		m.toggleGroupAtCursor()
	case "A":
		m.toggleAllSafe()
	case "c":
		m.selected = map[string]bool{}
	case "enter":
		if it := m.currentItem(); it != nil && m.explorePath != "" && it.IsDir {
			m.exploreStack = append(m.exploreStack, m.explorePath)
			return m.startExplore(it.Path)
		}
		if m.currentItem() != nil {
			m.detailsOpen = true
			m.detailOffset = 0
		} else {
			m.toggleCollapseAtCursor()
		}
	case "e":
		if it := m.currentItem(); it != nil && it.IsDir {
			if m.explorePath == "" {
				m.reviewItems = append([]scanner.Item(nil), m.items...)
				m.reviewSelected = m.selected
			} else {
				m.exploreStack = append(m.exploreStack, m.explorePath)
			}
			return m.startExplore(it.Path)
		}
	case "backspace":
		if len(m.exploreStack) > 0 {
			n := len(m.exploreStack) - 1
			path := m.exploreStack[n]
			m.exploreStack = m.exploreStack[:n]
			return m.startExplore(path)
		}
		return m.leaveExplore()
	case "esc":
		return m.leaveExplore()
	case "o":
		if it := m.currentItem(); it != nil {
			path := it.Path
			return m, func() tea.Msg {
				if err := exec.Command("/usr/bin/open", "-R", path).Run(); err != nil {
					return noticeMsg(err.Error())
				}
				return noticeMsg("Revealed in Finder")
			}
		}
	case "p":
		m.protectCurrent()
	case "d":
		if m.phase == phaseList && len(m.selectedItems()) > 0 {
			m.phase = phaseConfirm
			m.reviewOffset = 0
		}
	case "r":
		if m.explorePath != "" {
			return m.startExplore(m.explorePath)
		}
		return m.startScan()
	}
	return m, nil
}

func (m *Model) leaveExplore() (tea.Model, tea.Cmd) {
	if m.explorePath == "" {
		m.query = ""
		m.filter = 0
		m.rebuildRows()
		return m, nil
	}
	if m.scanCancel != nil {
		m.scanCancel()
	}
	m.run++
	m.explorePath = ""
	m.exploreStack = nil
	m.query = ""
	m.filter = 0
	m.cursor = 0
	m.items = m.reviewItems
	for i := range m.items {
		if scanner.Excluded(m.items[i].Path, m.cfg.Exclusions) {
			m.items[i].Eligibility = scanner.Protected
			m.items[i].Reason = "Protected by an exclusion"
		}
	}
	if m.reviewSelected != nil {
		m.selected = m.reviewSelected
		m.reviewSelected = nil
	}
	m.reviewItems = nil
	if m.items == nil {
		m.phase = phaseSplash
	} else {
		m.phase = phaseList
		m.rebuildRows()
	}
	return m, nil
}

func (m *Model) currentItem() *scanner.Item {
	if m.cursor < 0 || m.cursor >= len(m.rows) || m.rows[m.cursor].isGroup {
		return nil
	}
	return m.rows[m.cursor].item
}
func (m *Model) moveCursor(delta int) { m.cursor = max(0, min(len(m.rows)-1, m.cursor+delta)) }
func (m *Model) selectItem(it *scanner.Item) {
	if !it.Selectable() {
		return
	}
	if m.selected[it.Path] {
		delete(m.selected, it.Path)
		return
	}
	for path := range m.selected {
		if scanner.Contains(path, it.Path) || scanner.Contains(it.Path, path) {
			delete(m.selected, path)
		}
	}
	m.selected[it.Path] = true
}
func (m *Model) toggleAtCursor() {
	if it := m.currentItem(); it != nil {
		m.selectItem(it)
	} else {
		m.toggleGroupAtCursor()
	}
}
func (m *Model) toggleGroupAtCursor() {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		group := r.group
		if !r.isGroup {
			group = r.groupOf
		}
		m.toggleGroup(group)
	}
}
func (m *Model) toggleGroup(group string) {
	all := true
	for _, it := range m.items {
		if itemGroup(it) == group && m.matchesFilter(it) && it.Selectable() && !m.selected[it.Path] {
			all = false
		}
	}
	for i := range m.items {
		it := &m.items[i]
		if itemGroup(*it) != group || !m.matchesFilter(*it) || !it.Selectable() {
			continue
		}
		if all {
			delete(m.selected, it.Path)
		} else if !m.selected[it.Path] {
			m.selectItem(it)
		}
	}
}
func (m *Model) matchesFilter(it scanner.Item) bool {
	if m.filter > 0 && it.Eligibility != []scanner.Eligibility{"", scanner.Recommended, scanner.Review, scanner.Protected}[m.filter] {
		return false
	}
	return m.query == "" || strings.Contains(strings.ToLower(it.Path+" "+it.Name+" "+it.Owner+" "+it.Description), strings.ToLower(m.query))
}
func (m *Model) toggleAllSafe() {
	all := true
	for _, it := range m.items {
		if it.Eligibility == scanner.Recommended && !m.selected[it.Path] {
			all = false
		}
	}
	for i := range m.items {
		it := &m.items[i]
		if it.Eligibility != scanner.Recommended {
			continue
		}
		if all {
			delete(m.selected, it.Path)
		} else if !m.selected[it.Path] {
			m.selectItem(it)
		}
	}
}
func (m *Model) toggleCollapseAtCursor() {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].isGroup {
		g := m.rows[m.cursor].group
		m.collapsed[g] = !m.collapsed[g]
		m.rebuildRows()
	}
}

func itemGroup(it scanner.Item) string {
	owner := it.Owner
	if it.Project != "" {
		owner = condensePath(it.Project)
	}
	if owner == "" {
		owner = string(it.Category)
	}
	label := "Protected / inspect"
	if it.Eligibility == scanner.Recommended {
		label = "Recommended"
	} else if it.Eligibility == scanner.Review {
		label = "Review"
	}
	return label + " · " + owner
}
func (m *Model) rebuildRows() {
	current := ""
	if it := m.currentItem(); it != nil {
		current = it.Path
	}
	groups := map[string][]*scanner.Item{}
	order := []string{}
	for i := range m.items {
		it := &m.items[i]
		if !m.matchesFilter(*it) {
			continue
		}
		g := itemGroup(*it)
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], it)
	}
	rank := func(g string) int {
		if strings.HasPrefix(g, "Recommended") {
			return 0
		}
		if strings.HasPrefix(g, "Review") {
			return 1
		}
		return 2
	}
	sort.SliceStable(order, func(i, j int) bool {
		if rank(order[i]) != rank(order[j]) {
			return rank(order[i]) < rank(order[j])
		}
		return order[i] < order[j]
	})
	m.rows = nil
	for _, g := range order {
		sort.SliceStable(groups[g], func(i, j int) bool {
			a, b := groups[g][i], groups[g][j]
			switch m.sortMode {
			case 1:
				return a.Name < b.Name
			case 2:
				return a.ModTime.Before(b.ModTime)
			}
			if a.Size == b.Size {
				return a.Path < b.Path
			}
			return a.Size > b.Size
		})
		m.rows = append(m.rows, row{isGroup: true, group: g})
		if m.collapsed[g] {
			continue
		}
		for _, it := range groups[g] {
			m.rows = append(m.rows, row{item: it, groupOf: g})
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	for i, r := range m.rows {
		if current != "" && !r.isGroup && r.item.Path == current {
			m.cursor = i
			break
		}
	}
}
func (m *Model) selectedItems() []scanner.Item {
	out := []scanner.Item{}
	seen := map[string]bool{}
	for _, it := range m.items {
		if m.selected[it.Path] && it.Selectable() && !seen[it.Path] {
			seen[it.Path] = true
			out = append(out, it)
		}
	}
	return out
}
func (m *Model) selectedTotal() int64 {
	var total int64
	for _, it := range m.selectedItems() {
		total += it.Size
	}
	return total
}

func (m *Model) protectCurrent() {
	it := m.currentItem()
	if it == nil {
		return
	}
	cfg, err := settings.Load()
	if err != nil {
		m.notice = err.Error()
		return
	}
	found := false
	paths := []string{}
	for _, path := range cfg.Exclusions {
		if path == it.Path {
			found = true
		} else {
			paths = append(paths, path)
		}
	}
	if !found {
		paths = append(paths, it.Path)
	}
	cfg.Exclusions = paths
	if err := settings.Save(cfg); err != nil {
		m.notice = err.Error()
		return
	}
	m.cfg.Exclusions = paths
	if found {
		m.notice = "Protection removed. Rescan to refresh eligibility."
	} else {
		m.notice = "Path protected for future scans"
		for i := range m.items {
			if scanner.Excluded(m.items[i].Path, paths) {
				m.items[i].Eligibility = scanner.Protected
				m.items[i].Reason = "Protected by an exclusion"
				delete(m.selected, m.items[i].Path)
			}
		}
	}
	m.rebuildRows()
}

func (m *Model) startExplore(path string) (tea.Model, tea.Cmd) {
	m.explorePath = path
	return m.startScan()
}
func (m *Model) startScan() (tea.Model, tea.Cmd) {
	if m.scanCancel != nil {
		m.scanCancel()
	}
	m.run++
	run := m.run
	m.phase = phaseScanning
	m.scanErr = nil
	m.notice = ""
	m.scanProg = scanner.Progress{}
	m.items = nil
	m.cursor = 0
	m.query = ""
	m.filter = 0
	m.selected = map[string]bool{}
	ctx, cancel := context.WithCancel(context.Background())
	m.scanCancel = cancel
	ch := make(chan scanner.Progress, 16)
	m.scanCh = ch
	cfg := m.cfg
	explore := m.explorePath
	work := func() tea.Msg {
		var items []scanner.Item
		var err error
		if explore != "" {
			items, err = scanner.Explore(ctx, explore, ch)
		} else {
			items, err = scanner.Scan(ctx, cfg, ch)
		}
		return scanDoneMsg{run: run, items: items, err: err}
	}
	return m, tea.Batch(work, m.waitScanProgress())
}
func (m *Model) startClean() (tea.Model, tea.Cmd) {
	m.run++
	run := m.run
	m.phase = phaseCleaning
	m.cleanRes = nil
	m.notice = ""
	m.cleanProg = cleaner.Progress{}
	items := m.selectedItems()
	opts := cleaner.Options{Mode: m.cleanMode, Exclusions: append([]string(nil), m.cfg.Exclusions...)}
	ctx, cancel := context.WithCancel(context.Background())
	m.cleanCancel = cancel
	ch := make(chan cleaner.Progress, 16)
	m.cleanCh = ch
	return m, tea.Batch(func() tea.Msg { return cleanDoneMsg{run: run, results: cleaner.Execute(ctx, items, opts, ch)} }, m.waitCleanProgress())
}
func (m *Model) waitScanProgress() tea.Cmd {
	ch, run := m.scanCh, m.run
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		p.Run = run
		return scanProgressMsg(p)
	}
}
func (m *Model) waitCleanProgress() tea.Cmd {
	ch, run := m.cleanCh, m.run
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		p.Run = run
		return cleanProgressMsg(p)
	}
}

func Run(cfg scanner.Config, mode cleaner.Mode, freeBefore int64) error {
	p := tea.NewProgram(New(cfg, mode, freeBefore), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) scopeLabel() string {
	if m.explorePath != "" {
		return "Explore · " + condensePath(m.explorePath)
	}
	if m.cfg.Scope == "projects" {
		return "Projects"
	}
	if m.cfg.Scope == "quick" || m.cfg.SkipProjectScan {
		return "Quick cleanup"
	}
	return "All cleanup candidates"
}
func (m *Model) actionLabel() string {
	switch m.cleanMode {
	case cleaner.ModeDryRun:
		return "Preview only"
	case cleaner.ModeRemove:
		return "Permanently delete"
	}
	return "Move to Trash"
}
func (m *Model) reviewHeight() int { return max(1, m.height-8) }
func (m *Model) listHeight() int   { return max(1, m.height-12) }

// Keep action text distinct from observed free-space changes.
func resultSummary(results []cleaner.Result) string {
	var preview, moved, removed int64
	for _, r := range results {
		preview += r.Previewed
		moved += r.Moved
		removed += r.Freed
	}
	return fmt.Sprintf("Preview %s · Trash %s · Removed estimate %s", humanBytes(preview), humanBytes(moved), humanBytes(removed))

}

func RunExplore(cfg scanner.Config, mode cleaner.Mode, freeBefore int64, path string) error {
	m := New(cfg, mode, freeBefore)
	m.explorePath = path
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *Model) applyExclusions() {
	for i := range m.items {
		if scanner.Excluded(m.items[i].Path, m.cfg.Exclusions) {
			m.items[i].Eligibility = scanner.Protected
			m.items[i].Reason = "Protected by an exclusion"
			delete(m.selected, m.items[i].Path)
		}
	}
}
