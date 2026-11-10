package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/wanggang316/handbyte/catalog"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
)

// SmartNext is what the user chose at the end of Smart Clean.
type SmartNext int

const (
	SmartDone SmartNext = iota
	// SmartReview opens the full review list ("Choose items").
	SmartReview
	// SmartDeep opens the review list with the project scan.
	SmartDeep
)

// SmartOptions connects Smart Clean to the scanner and the executor. Tests
// replace both functions.
type SmartOptions struct {
	Scan    func(ctx context.Context, progress chan<- scanner.Progress) ([]scanner.Item, error)
	Execute func(ctx context.Context, items []scanner.Item, progress chan<- cleaner.Progress) []cleaner.Result
	// Mode is the removal mode, for the words of the confirmation.
	Mode cleaner.Mode
	// Yes cleans without the confirmation.
	Yes bool
	// FreeBytes reads the free space, before and after the clean.
	FreeBytes func() int64
}

type smartPhase int

const (
	smartScanning smartPhase = iota
	smartReady
	smartCleaning
	smartFinished
)

type smartProgressMsg scanner.Progress
type smartScanDoneMsg struct {
	items []scanner.Item
	err   error
}
type smartCleanProgressMsg cleaner.Progress
type smartCleanDoneMsg []cleaner.Result
type smartTickMsg time.Time

// Smart is Smart Clean in the terminal. It is not a full-screen program: it
// draws the area lines in place and the final state stays in the scroll
// history.
type Smart struct {
	opts    SmartOptions
	width   int
	phase   smartPhase
	started time.Time
	elapsed time.Duration
	frame   int
	cancel  context.CancelFunc

	scanUpdates  chan scanner.Progress
	cleanUpdates chan cleaner.Progress
	progress     scanner.Progress
	items        []scanner.Item
	scanErr      error
	stopped      bool
	// cancelled is true when the user left without a clean.
	cancelled bool

	safe    []scanner.Item
	results map[string]cleaner.Result
	cleaned int
	before  int64
	after   int64

	// Next is set when the program ends.
	Next SmartNext
}

// NewSmart returns Smart Clean with the given connections.
func NewSmart(opts SmartOptions) *Smart {
	return &Smart{opts: opts, width: 100, results: map[string]cleaner.Result{}}
}

func (m *Smart) Init() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.started = time.Now()
	if m.opts.FreeBytes != nil {
		m.before = m.opts.FreeBytes()
	}
	updates := make(chan scanner.Progress, 8)
	m.scanUpdates = updates
	scan := m.opts.Scan
	return tea.Batch(func() tea.Msg {
		items, err := scan(ctx, updates)
		return smartScanDoneMsg{items, err}
	}, waitSmartScan(updates), smartTick())
}

func waitSmartScan(updates chan scanner.Progress) tea.Cmd {
	return func() tea.Msg {
		if p, ok := <-updates; ok {
			return smartProgressMsg(p)
		}
		return nil
	}
}

func waitSmartClean(updates chan cleaner.Progress) tea.Cmd {
	return func() tea.Msg {
		if p, ok := <-updates; ok {
			return smartCleanProgressMsg(p)
		}
		return nil
	}
}

func smartTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return smartTickMsg(t) })
}

func (m *Smart) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case smartTickMsg:
		if m.phase == smartScanning || m.phase == smartCleaning {
			m.frame++
			if m.phase == smartScanning {
				m.elapsed = time.Since(m.started)
			}
			return m, smartTick()
		}
	case smartProgressMsg:
		m.progress = scanner.Progress(msg)
		return m, waitSmartScan(m.scanUpdates)
	case smartScanDoneMsg:
		m.elapsed = time.Since(m.started)
		m.items, m.scanErr = msg.items, msg.err
		m.safe = SmartSelection(m.items)
		m.phase = smartReady
		if len(m.safe) == 0 {
			m.phase = smartFinished
			return m, tea.Quit
		}
		if m.opts.Yes {
			return m, m.clean()
		}
	case smartCleanProgressMsg:
		p := cleaner.Progress(msg)
		if p.LastResult != nil {
			m.results[p.LastResult.Path] = *p.LastResult
		}
		return m, waitSmartClean(m.cleanUpdates)
	case smartCleanDoneMsg:
		for _, r := range msg {
			m.results[r.Path] = r
		}
		if m.opts.FreeBytes != nil {
			m.after = m.opts.FreeBytes()
		}
		m.phase = smartFinished
		return m, tea.Quit
	case tea.KeyMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m *Smart) key(key string) (tea.Model, tea.Cmd) {
	switch m.phase {
	case smartScanning:
		switch key {
		case "esc":
			m.stopped = true
			m.cancel()
		case "ctrl+c", "q":
			m.cancel()
			m.phase, m.cancelled = smartFinished, true
			return m, tea.Quit
		}
	case smartReady:
		switch key {
		case "enter":
			return m, m.clean()
		case "r":
			m.Next = SmartReview
			return m, tea.Quit
		case "d":
			m.Next = SmartDeep
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.phase, m.cancelled = smartFinished, true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Smart) clean() tea.Cmd {
	m.phase = smartCleaning
	updates := make(chan cleaner.Progress, 8)
	m.cleanUpdates = updates
	items, execute := m.safe, m.opts.Execute
	return tea.Batch(func() tea.Msg {
		return smartCleanDoneMsg(execute(context.Background(), items, updates))
	}, waitSmartClean(updates), smartTick())
}

// smartSafe is an item that Smart Clean may take: the policy recommends it
// and its rule is of the safe tier.
func smartSafe(it scanner.Item) bool {
	return it.Eligibility == scanner.Recommended && it.Safety == scanner.SafetyAuto && it.ScanStatus == "complete"
}

// SmartSelection is what Smart Clean takes: the safe items, without one
// inside another. It never takes an item that needs review.
func SmartSelection(items []scanner.Item) []scanner.Item {
	var out []scanner.Item
	for _, it := range items {
		if !smartSafe(it) {
			continue
		}
		inside := false
		for _, other := range items {
			if other.Path != it.Path && smartSafe(other) && scanner.Contains(other.Path, it.Path) {
				inside = true
			}
		}
		if !inside {
			out = append(out, it)
		}
	}
	return out
}

// areaLine is the state of one area for the screen.
type areaLine struct {
	id, title string
	safe      int64
	review    int64
	skipped   int64
	running   int64
	count     int
	tasks     int
	done      int
	cleaned   int
	failed    int
	current   string
}

func (m *Smart) areas() []areaLine {
	titles := catalog.Load()
	lines := map[string]*areaLine{}
	order := []string{}
	for _, a := range titles.Areas {
		lines[a.ID] = &areaLine{id: a.ID, title: a.Title}
		order = append(order, a.ID)
	}
	items := m.items
	if m.phase == smartScanning {
		items = m.progress.Items
	}
	safe := map[string]bool{}
	for _, it := range SmartSelection(items) {
		safe[it.Path] = true
	}
	for _, it := range items {
		l := lines[it.Area]
		if l == nil || it.ScanStatus == "unavailable" {
			continue
		}
		switch {
		case safe[it.Path]:
			l.safe += it.Size
			l.count++
			if r, ok := m.results[it.Path]; ok {
				if r.Status == "removed" || r.Status == "trashed" || r.Status == "previewed" {
					l.cleaned++
				} else {
					l.failed++
				}
			}
		case it.Eligibility == scanner.Review || it.Eligibility == scanner.Recommended:
			l.review += it.Size
		default:
			l.skipped += it.Size
			if it.Activity == scanner.Active {
				l.running += it.Size
			}
		}
	}
	for id, n := range m.progress.AreaTasks {
		if l := lines[id]; l != nil {
			l.tasks = n
			l.done = m.progress.AreaDone[id]
		}
	}
	var out []areaLine
	for _, id := range order {
		l := lines[id]
		// An area with no location on this Mac is left out once the scan
		// knows its locations.
		if m.phase != smartScanning && l.safe == 0 && l.review == 0 && l.skipped == 0 {
			continue
		}
		if m.phase == smartScanning && m.progress.AreaTasks != nil && l.tasks == 0 {
			continue
		}
		out = append(out, *l)
	}
	return out
}

func (m *Smart) totals() (safe, review, skipped int64) {
	for _, l := range m.areas() {
		safe += l.safe
		review += l.review
		skipped += l.skipped
	}
	return
}

func (m *Smart) View() string {
	width := max(40, m.width)
	inner := width - 4
	pad := "  "
	var b strings.Builder
	line := func(s string) { b.WriteString(ansi.Truncate(pad+s, width, "…") + "\n") }

	title := Mark() + " " + Strong.Render("handbyte") + Meta.Render(" · ") + Title.Render("Smart Clean")
	status := ""
	switch m.phase {
	case smartScanning:
		status = Meta.Render(fmt.Sprintf("scanning · %s files · %.1f s", count(m.progress.FilesSeen), m.elapsed.Seconds()))
	case smartReady:
		status = Meta.Render(fmt.Sprintf("scan complete · %.1f s", m.elapsed.Seconds()))
	case smartCleaning:
		status = Meta.Render("cleaning")
	case smartFinished:
		status = Safe.Render("done")
		if m.cancelled {
			status = Meta.Render("cancelled")
		}
	}
	line(Between(title, status, inner))

	areas := m.areas()
	// One progress line: by areas while scanning, by items while cleaning.
	frac := 1.0
	switch m.phase {
	case smartScanning:
		total, done := 0, 0
		for _, l := range areas {
			total += l.tasks
			done += l.done
		}
		frac = 0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
	case smartCleaning:
		frac = float64(len(m.results)) / float64(max(1, len(m.safe)))
	}
	filled := int(frac * float64(inner))
	line(Safe.Render(strings.Repeat("━", filled)) + Faint.Render(strings.Repeat("─", max(0, inner-filled))))
	b.WriteString("\n")

	spin := spinner[m.frame%len(spinner)]
	narrow := SizeFor(width) == Narrow
	nameWidth := 26
	if narrow {
		nameWidth = min(26, inner-16)
	}
	done := "cleaned"
	if m.opts.Mode == cleaner.ModeDryRun {
		done = "would be cleaned"
	}
	for _, l := range areas {
		name := fmt.Sprintf("%-*s", nameWidth, CutMiddle(l.title, nameWidth))
		size := fmt.Sprintf("%11s", Bytes(l.safe))
		note := m.areaNote(l)
		if narrow {
			note = ""
		}
		switch {
		case m.phase == smartScanning && l.tasks > 0 && l.done >= l.tasks:
			line(Safe.Render("✓ ") + Normal.Render(name) + Normal.Render(size) + "   " + Meta.Render(note))
		case m.phase == smartScanning && l.done > 0:
			line(Safe.Render(spin+" ") + Title.Render(name) + Title.Render(size+"+"))
		case m.phase == smartScanning:
			line(Meta.Render("· " + name))
		case l.count == 0:
			line(Meta.Render("  "+name+fmt.Sprintf("%11s", "—")) + "   " + Meta.Render(note))
		case m.phase == smartCleaning && l.cleaned+l.failed < l.count:
			line(Safe.Render(spin+" ") + Title.Render(name) + Title.Render(size) + "   " + Meta.Render("checking again, then cleaning"))
		case m.phase == smartFinished && l.cleaned > 0:
			result := done
			if l.failed > 0 {
				result = fmt.Sprintf("%s · %d skipped", done, l.failed)
			}
			line(Safe.Render("✓ ") + Normal.Render(name) + Normal.Render(size) + "   " + Safe.Render(result))
		default:
			line(Safe.Render("✓ ") + Normal.Render(name) + Normal.Render(size) + "   " + Meta.Render(note))
		}
	}
	b.WriteString("\n")

	safe, review, skipped := m.totals()
	switch m.phase {
	case smartScanning:
		line(Strong.Render(Bytes(safe)) + Meta.Render(" safe to clean so far"))
		b.WriteString("\n")
		line(KeyCap("esc") + Meta.Render(" stop and use what was found"))
	case smartReady:
		words := func(s string) string {
			if narrow {
				return ""
			}
			return "   " + s
		}
		line(Safe.Render("● Safe to clean   ") + Strong.Render(fmt.Sprintf("%11s", Bytes(safe))) + Meta.Render(words(m.modeWords())))
		if review > 0 {
			line(Review.Render("▲ Needs review    ") + Normal.Render(fmt.Sprintf("%11s", Bytes(review))) + Meta.Render(words("not included")))
		}
		if skipped > 0 {
			line(Meta.Render("■ Skipped         " + fmt.Sprintf("%11s", Bytes(skipped)) + words("its app runs, or it is in use or protected")))
		}
		b.WriteString("\n")
		if narrow {
			line(Chip("enter") + Strong.Render(" clean "+Bytes(safe)) + "  " + KeyCap("r") + Meta.Render(" choose") + "  " + KeyCap("esc"))
		} else {
			line(Chip("enter") + Strong.Render(" clean "+Bytes(safe)) + "   " + KeyCap("r") + Meta.Render(" choose items") + "   " + KeyCap("d") + Meta.Render(" deep scan") + "   " + KeyCap("esc") + Meta.Render(" cancel"))
		}
	case smartCleaning:
		line(Meta.Render("Each item is checked again before it is cleaned."))
	case smartFinished:
		m.viewResult(line, safe, review)
	}
	return b.String()
}

func (m *Smart) areaNote(l areaLine) string {
	var parts []string
	if l.count > 0 {
		noun := "items"
		if l.count == 1 {
			noun = "item"
		}
		parts = append(parts, fmt.Sprintf("%d %s", l.count, noun))
	}
	if l.review > 0 {
		parts = append(parts, Bytes(l.review)+" needs review")
	}
	switch {
	case l.running > 0 && l.running == l.skipped:
		parts = append(parts, Bytes(l.skipped)+" skipped: the app runs")
	case l.skipped > 0:
		parts = append(parts, Bytes(l.skipped)+" skipped")
	}
	return strings.Join(parts, " · ")
}

func (m *Smart) modeWords() string {
	switch m.opts.Mode {
	case cleaner.ModeDryRun:
		return "preview only, nothing changes"
	case cleaner.ModeTrash:
		return "moved to the Trash"
	case cleaner.ModeRemove:
		return "removed, not through the Trash"
	}
	return "caches and logs that rebuild by themselves"
}

func (m *Smart) viewResult(line func(string), safe, review int64) {
	if m.cancelled {
		line(Meta.Render("Cancelled. Nothing was changed."))
		return
	}
	if len(m.safe) == 0 {
		if m.scanErr != nil && !m.stopped {
			line(Review.Render("! The scan did not complete: " + m.scanErr.Error()))
		} else {
			line(Meta.Render("Nothing safe to clean was found."))
		}
		if review > 0 {
			line(Meta.Render(Bytes(review)+" needs review: ") + Normal.Render("handbyte clean --review"))
		}
		return
	}
	var removed, moved, previewed int64
	var skipped []cleaner.Result
	for _, r := range m.results {
		removed += r.Freed
		moved += r.Moved
		previewed += r.Previewed
		if r.Status != "removed" && r.Status != "trashed" && r.Status != "previewed" {
			skipped = append(skipped, r)
		}
	}
	switch {
	case previewed > 0 && removed+moved == 0:
		line(Safe.Render("✓ Preview: ") + Strong.Render(Bytes(previewed)) + Meta.Render(" would be cleaned. Nothing was changed."))
	default:
		text := Safe.Render("✓ ") + Strong.Render("Removed "+Bytes(removed))
		if moved > 0 {
			text += Meta.Render(" · ") + Normal.Render("moved "+Bytes(moved)+" to the Trash")
		}
		if m.before > 0 && m.after > 0 {
			text += Meta.Render("   free space "+Bytes(m.before)+" → ") + Strong.Render(Bytes(m.after))
		}
		line(text)
	}
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].Path < skipped[j].Path })
	for i, r := range skipped {
		if i == 3 {
			line(Meta.Render(fmt.Sprintf("  and %d more skipped", len(skipped)-3)))
			break
		}
		line(Review.Render("! ") + Normal.Render(tilde(r.Path)) + Meta.Render(" · "+r.Message))
	}
	if review > 0 {
		line(Meta.Render(Bytes(review)+" more needs review: ") + Normal.Render("handbyte clean --review") + Meta.Render("   ·   details: ") + Normal.Render("handbyte history"))
	} else {
		line(Meta.Render("Details: ") + Normal.Render("handbyte history"))
	}
}

// RunSmart runs Smart Clean in place in the terminal.
func RunSmart(opts SmartOptions) (SmartNext, error) {
	m := NewSmart(opts)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return SmartDone, err
	}
	return m.Next, nil
}
