package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/usage"
)

func collectFixture(t *testing.T) (*Analyze, string, *[]cleaner.Collected) {
	t.Helper()
	home, err := scanner.Physical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	for path, size := range map[string]int{
		"Downloads/old.dmg":            300_000,
		"Downloads/notes.txt":          1_000,
		"Library/Caches/go-build/a":    50_000,
		"Documents/archive/photo.heic": 90_000,
	} {
		full := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := usage.Scan(context.Background(), home, usage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var moved []cleaner.Collected
	m := NewAnalyze(home, AnalyzeOptions{
		Hints: scanner.NewHints(home, nil),
		Collect: func(_ context.Context, items []cleaner.Collected) []cleaner.Result {
			moved = append(moved, items...)
			var out []cleaner.Result
			for _, it := range items {
				out = append(out, cleaner.Result{ID: "id-" + filepath.Base(it.Path), Path: it.Path, Status: "trashed", Moved: it.Size})
			}
			return out
		},
	})
	m.reveal = func(string) error { return nil }
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
	m.Update(analyzeDoneMsg{result: result})
	return m, home, &moved
}

// openRow puts the cursor on a row of the opened folder and opens it.
func pick(t *testing.T, m *Analyze, name string) {
	t.Helper()
	for i, r := range m.rows(m.levels[len(m.levels)-1].node) {
		if r.node != nil && r.node.Name == name {
			m.levels[len(m.levels)-1].cursor = i
			return
		}
	}
	t.Fatalf("%s is not in the list", name)
}

func space(m *Analyze) { m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}) }

func TestCollectorPicksReviewsAndMovesToTheTrash(t *testing.T) {
	m, home, moved := collectFixture(t)
	pick(t, m, "Downloads")
	analyzeKey(m, "enter")
	pick(t, m, "old.dmg")
	space(m)
	if out := m.View(); !strings.Contains(out, "◆") || !strings.Contains(out, "review 1 item") {
		t.Fatalf("a picked row must be marked and the review must be the main action:\n%s", out)
	}
	analyzeKey(m, "tab")
	m.fileCursor = 0
	for i, f := range m.result.Largest {
		if strings.HasSuffix(f.Path, "photo.heic") {
			m.fileCursor = i
		}
	}
	space(m)
	analyzeKey(m, "c")
	out := m.View()
	assertFits(t, out, 110, 28)
	for _, want := range []string{"Move to Trash", "~/Downloads/old.dmg", "~/Documents/archive/photo.heic", "move 2 items"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m.Update(cmd())
	if len(*moved) != 2 || (*moved)[0].Path != filepath.Join(home, "Downloads/old.dmg") {
		t.Fatalf("moved: %+v", *moved)
	}
	out = m.View()
	if !strings.Contains(out, "Moved 2 to the Trash") || !strings.Contains(out, "restore id id-old.dmg") {
		t.Fatalf("result:\n%s", out)
	}
	if cmd := analyzeKey(m, "enter"); cmd == nil || !m.scanning || len(m.collect.items) != 0 {
		t.Fatal("after the move the folder must be measured again with an empty list")
	}
}

func TestCollectorRefusesRuleCoveredAndStructuralFolders(t *testing.T) {
	m, _, _ := collectFixture(t)
	pick(t, m, "Library")
	analyzeKey(m, "enter")
	pick(t, m, "Caches")
	space(m)
	if !strings.Contains(m.notice, "standard folder") || len(m.collect.items) != 0 {
		t.Fatalf("~/Library/Caches must be refused: %q", m.notice)
	}
	analyzeKey(m, "enter")
	pick(t, m, "go-build")
	space(m)
	if !strings.Contains(m.notice, "handbyte clean") || len(m.collect.items) != 0 {
		t.Fatalf("a folder that a rule covers must point to Clean: %q", m.notice)
	}
}

func TestCollectorReviewCanRemoveItemsAndGoBack(t *testing.T) {
	m, _, moved := collectFixture(t)
	pick(t, m, "Downloads")
	analyzeKey(m, "enter")
	pick(t, m, "old.dmg")
	space(m)
	space(m)
	if len(m.collect.items) != 0 {
		t.Fatal("space on a picked row must remove it")
	}
	space(m)
	analyzeKey(m, "c")
	analyzeKey(m, "x")
	if m.collect.stage != collectOff || len(*moved) != 0 {
		t.Fatal("removing the last item must close the review without a move")
	}
}
