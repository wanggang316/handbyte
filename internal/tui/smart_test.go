package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
)

func smartItems() []scanner.Item {
	return []scanner.Item{
		{Path: "/h/Library/Developer/Xcode/DerivedData/App", Area: "xcode", Size: 6 << 30, Eligibility: scanner.Recommended, ScanStatus: "complete", Rebuildable: true},
		{Path: "/h/Library/Caches/go-build", Area: "packages", Size: 2 << 30, Eligibility: scanner.Recommended, ScanStatus: "complete", Rebuildable: true},
		// A review-tier rule stays out of Smart Clean, also when the policy recommends it.
		{Path: "/h/.pub-cache", Area: "packages", Size: 1 << 30, Eligibility: scanner.Recommended, Safety: scanner.SafetyReview, ScanStatus: "complete", Rebuildable: true},
		{Path: "/h/Library/Caches/Google/Chrome", Area: "browsers", Size: 1 << 30, Eligibility: scanner.Protected, Activity: scanner.Active, ScanStatus: "complete", Reason: "Owner is active"},
	}
}

// runSmart drives the model without a terminal: it runs each command and
// feeds the messages back, up to a limit.
func runSmart(t *testing.T, m *Smart, keys ...string) {
	t.Helper()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	queue := []tea.Cmd{m.Init()}
	pending := append([]string(nil), keys...)
	for steps := 0; steps < 200 && len(queue) > 0; steps++ {
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		switch msg := msg.(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
			continue
		case smartTickMsg, nil:
			continue
		case tea.QuitMsg:
			return
		}
		_, next := m.Update(msg)
		queue = append(queue, next)
		if m.phase == smartReady && len(pending) > 0 {
			key := pending[0]
			pending = pending[1:]
			k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
			if key == "enter" {
				k = tea.KeyMsg{Type: tea.KeyEnter}
			}
			_, next := m.Update(k)
			queue = append(queue, next)
		}
	}
}

func fakeSmart(items []scanner.Item, executed *[]scanner.Item) SmartOptions {
	return SmartOptions{
		Mode: cleaner.ModeByData,
		Scan: func(_ context.Context, progress chan<- scanner.Progress) ([]scanner.Item, error) {
			progress <- scanner.Progress{Items: items[:1], AreaTasks: map[string]int{"xcode": 1, "packages": 2, "browsers": 1}, AreaDone: map[string]int{"xcode": 1}}
			close(progress)
			return items, nil
		},
		Execute: func(_ context.Context, items []scanner.Item, progress chan<- cleaner.Progress) []cleaner.Result {
			*executed = append(*executed, items...)
			var out []cleaner.Result
			for _, it := range items {
				out = append(out, cleaner.Result{Path: it.Path, Status: "removed", Freed: it.Size})
			}
			close(progress)
			return out
		},
		FreeBytes: func() int64 { return 100 << 30 },
	}
}

func TestSmartCleanScansConfirmsAndCleansOnlySafeItems(t *testing.T) {
	var executed []scanner.Item
	m := NewSmart(fakeSmart(smartItems(), &executed))
	runSmart(t, m)
	out := m.View()
	for _, want := range []string{"Smart Clean", "Xcode build data", "Package manager caches", "Safe to clean", "8.00 GiB", "Needs review", "1.00 GiB", "Skipped", "skipped: the app runs", "enter", "choose items", "deep scan"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	if len(executed) != 0 {
		t.Fatal("nothing may be cleaned before the confirmation")
	}
	m = NewSmart(fakeSmart(smartItems(), &executed))
	runSmart(t, m, "enter")
	if len(executed) != 2 || executed[0].Eligibility != scanner.Recommended || executed[1].Eligibility != scanner.Recommended {
		t.Fatalf("only the two safe items may be cleaned: %+v", executed)
	}
	out = m.View()
	for _, want := range []string{"Removed 8.00 GiB", "cleaned", "1.00 GiB more needs review", "handbyte clean --review"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if Width(line) > 100 {
			t.Errorf("line wider than the terminal: %q", line)
		}
	}
}

func TestSmartCleanWithYesNeedsNoKey(t *testing.T) {
	var executed []scanner.Item
	opts := fakeSmart(smartItems(), &executed)
	opts.Yes = true
	runSmart(t, NewSmart(opts))
	if len(executed) != 2 {
		t.Fatalf("--yes must clean the safe items: %d", len(executed))
	}
}

func TestSmartCleanKeysLeadToTheReviewOrTheDeepScan(t *testing.T) {
	for key, want := range map[string]SmartNext{"r": SmartReview, "d": SmartDeep} {
		var executed []scanner.Item
		m := NewSmart(fakeSmart(smartItems(), &executed))
		runSmart(t, m, key)
		if m.Next != want || len(executed) != 0 {
			t.Errorf("%s: next %d, executed %d", key, m.Next, len(executed))
		}
	}
	var executed []scanner.Item
	m := NewSmart(fakeSmart(smartItems(), &executed))
	runSmart(t, m, "esc")
	if !strings.Contains(m.View(), "Nothing was changed") || len(executed) != 0 {
		t.Fatalf("esc must cancel:\n%s", m.View())
	}
}

func TestSmartCleanWithNothingSafeSaysSo(t *testing.T) {
	var executed []scanner.Item
	items := smartItems()[2:]
	m := NewSmart(fakeSmart(items, &executed))
	runSmart(t, m)
	if out := m.View(); !strings.Contains(out, "Nothing safe to clean") || !strings.Contains(out, "needs review") {
		t.Fatalf("result:\n%s", out)
	}
}

func TestSmartSelectionTakesAnOuterItemOnce(t *testing.T) {
	items := []scanner.Item{
		{Path: "/h/a", Eligibility: scanner.Recommended, ScanStatus: "complete"},
		{Path: "/h/a/b", Eligibility: scanner.Recommended, ScanStatus: "complete"},
		{Path: "/h/c", Eligibility: scanner.Recommended, ScanStatus: "partial"},
		{Path: "/h/d", Eligibility: scanner.Review, ScanStatus: "complete"},
		{Path: "/h/e", Eligibility: scanner.Recommended, Safety: scanner.SafetyReview, ScanStatus: "complete"},
	}
	got := SmartSelection(items)
	if len(got) != 1 || got[0].Path != "/h/a" {
		t.Fatalf("selection: %+v", got)
	}
}
