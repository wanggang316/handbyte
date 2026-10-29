package ui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
	"strings"
	"testing"
)

func TestLateProgressDoesNotChangeFinalResult(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	r := cleaner.Result{Path: "/fixture/cache"}
	m.Update(cleanDoneMsg{results: []cleaner.Result{r}})
	m.Update(cleanProgressMsg{LastResult: &r})
	if len(m.cleanRes) != 1 {
		t.Fatalf("final count changed to %d", len(m.cleanRes))
	}
}

func TestBulkSelectionNeverSelectsProtectedTargets(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeTrash, 0)
	m.items = []scanner.Item{
		{Path: "/fixture/allowed", Name: "Allowed", Category: scanner.CatDevCache, Safety: scanner.SafetyAuto, Eligibility: scanner.Recommended},
		{Path: "/fixture/protected", Name: "Protected", Category: scanner.CatDevCache, Safety: scanner.SafetyAuto, Eligibility: scanner.Protected},
	}
	m.rebuildRows()
	m.toggleAllSafe()
	if len(m.selectedItems()) != 1 || m.selected["/fixture/protected"] {
		t.Fatalf("protected target selected: %v", m.selected)
	}
}

func TestConfirmationShowsAllItemsWhenTheyFit(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	m.width = 120
	m.height = 40
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("/fixture/item-%02d", i)
		m.items = append(m.items, scanner.Item{Path: path, Name: fmt.Sprintf("item-%02d", i), Eligibility: scanner.Review})
		m.selected[path] = true
	}
	if view := m.renderConfirm(); !strings.Contains(view, "item-09") {
		t.Fatalf("selected target hidden: %s", view)
	}
}

func TestReviewFitsSmallTerminal(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	m.width = 80
	m.height = 24
	m.items = []scanner.Item{{Path: "/fixture/" + strings.Repeat("long-name", 20), Name: "Example", Description: strings.Repeat("cost ", 40), Category: scanner.CatDevCache, Eligibility: scanner.Review}}
	m.rebuildRows()
	view := m.renderList()
	if lipgloss.Height(view) > 24 {
		t.Errorf("review height %d exceeds terminal", lipgloss.Height(view))
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Errorf("row exceeds terminal: %d", lipgloss.Width(line))
			break
		}
	}
}

func TestConfirmationRequiresExplicitKey(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeRemove, 0)
	m.phase = phaseConfirm
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.phase != phaseConfirm {
		t.Fatal("Enter approved permanent deletion")
	}
}

func TestConfirmationCanScrollPastFirstPage(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	m.phase = phaseConfirm
	m.width = 80
	m.height = 24
	for i := 0; i < 40; i++ {
		path := fmt.Sprintf("/fixture/item-%02d", i)
		m.items = append(m.items, scanner.Item{Path: path, Eligibility: scanner.Review})
		m.selected[path] = true
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if !strings.Contains(m.renderConfirm(), "item-39") {
		t.Fatal("last selection cannot be reviewed")
	}
}

func TestStaleScanCannotReplaceCurrentView(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	m.run = 2
	m.phase = phaseScanning
	m.Update(scanDoneMsg{run: 1, items: []scanner.Item{{Path: "/stale"}}})
	if len(m.items) != 0 || m.phase != phaseScanning {
		t.Fatal("stale scan replaced current view")
	}
}

func TestSearchAndProtectionPersist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	m.phase = phaseList
	m.items = []scanner.Item{{Path: "/fixture/keep", Name: "Keep", Category: scanner.CatDevCache, Eligibility: scanner.Review}, {Path: "/fixture/other", Name: "Other", Category: scanner.CatDevCache, Eligibility: scanner.Review}}
	m.rebuildRows()
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("keep")})
	m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.rows) != 2 {
		t.Fatalf("search did not filter rows: %d", len(m.rows))
	}
	m.cursor = 1
	m.protectCurrent()
	cfg, err := settings.Load()
	if err != nil || len(cfg.Exclusions) != 1 || cfg.Exclusions[0] != "/fixture/keep" {
		t.Fatalf("protection not persisted: %+v %v", cfg, err)
	}
	if m.items[0].Selectable() {
		t.Fatal("protected item remains selectable")
	}
}

func TestAllViewsFitSupportedTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 28}, {120, 40}} {
		m := New(scanner.Default(), cleaner.ModeDryRun, 0)
		m.width = size[0]
		m.height = size[1]
		m.items = []scanner.Item{{Path: "/fixture/日本語\x1b[2J" + strings.Repeat("long", 50), Name: "Example", Description: strings.Repeat("cost ", 30), Eligibility: scanner.Review, ScanStatus: "partial"}}
		m.rebuildRows()
		m.cursor = 1
		for _, view := range []string{m.renderSplash(), m.renderList(), m.renderConfirm(), m.renderCleaning(), m.renderDone(), m.renderHelp()} {
			if lipgloss.Height(view) > m.height {
				t.Fatalf("height %d exceeds %d", lipgloss.Height(view), m.height)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("width %d exceeds %d", lipgloss.Width(line), m.width)
				}
			}
			if strings.Contains(view, "\x1b[2J") {
				t.Fatal("file name injected a terminal escape")
			}
		}
	}
}

func TestCollapsedGroupCanSelectEligibleItems(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	m.items = []scanner.Item{{Path: "/fixture/cache", Name: "Cache", Owner: "Example", Eligibility: scanner.Review}}
	m.rebuildRows()
	m.toggleCollapseAtCursor()
	m.toggleGroupAtCursor()
	if !m.selected["/fixture/cache"] {
		t.Fatal("collapsed group cannot select its items")
	}
}
