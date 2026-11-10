package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/usage"
)

func analyzeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, size := range map[string]int{
		"media/movie.mov":     900_000,
		"media/song.mp3":      90_000,
		"docs/report.pdf":     40_000,
		"docs/deep/notes.txt": 4_000,
		"readme.txt":          1_000,
		"a-long-directory-name-that-does-not-fit-in-a-narrow-terminal/data.bin": 200_000,
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func scannedAnalyze(t *testing.T, width, height int) (*Analyze, *[]string) {
	t.Helper()
	root := analyzeFixture(t)
	result, err := usage.Scan(context.Background(), root, usage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m := NewAnalyze(root, scanner.NewHints(root, nil))
	var shown []string
	m.reveal = func(path string) error { shown = append(shown, path); return nil }
	m.scanning = true
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m.Update(analyzeDoneMsg{result: result})
	return m, &shown
}

func analyzeKey(m *Analyze, key string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	_, cmd := m.Update(msg)
	return cmd
}

func assertFits(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Errorf("%dx%d: %d lines", width, height, len(lines))
	}
	for i, line := range lines {
		if w := Width(line); w > width {
			t.Errorf("%dx%d: line %d has %d cells: %q", width, height, i, w, line)
		}
	}
}

func TestAnalyzeFitsEveryCheckedTerminalSize(t *testing.T) {
	for _, size := range [][2]int{{58, 20}, {80, 24}, {110, 28}, {160, 45}, {40, 10}} {
		m, _ := scannedAnalyze(t, size[0], size[1])
		assertFits(t, m.View(), size[0], size[1])
		analyzeKey(m, "tab")
		assertFits(t, m.View(), size[0], size[1])
	}
}

func TestAnalyzeListsTheLargestFirstAndNeverCutsTheSize(t *testing.T) {
	m, _ := scannedAnalyze(t, 58, 20)
	out := m.View()
	media, long := strings.Index(out, "media/"), strings.Index(out, "a-long")
	if media < 0 || long < 0 || media > long {
		t.Fatalf("media/ must come first:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "a-long") && (!strings.Contains(line, "KiB") || !strings.Contains(line, "…")) {
			t.Errorf("a long name must be cut in the middle and keep its size: %q", line)
		}
	}
}

func TestAnalyzeWideTerminalShowsTheDetailPane(t *testing.T) {
	m, _ := scannedAnalyze(t, 110, 28)
	out := m.View()
	for _, want := range []string{"On disk", "Content", "Files", "Share", "open media/"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	if narrow, _ := scannedAnalyze(t, 80, 24); strings.Contains(narrow.View(), "On disk") {
		t.Error("the detail pane is for wide terminals only")
	}
}

func TestAnalyzeNavigation(t *testing.T) {
	m, shown := scannedAnalyze(t, 110, 28)
	analyzeKey(m, "enter")
	if len(m.levels) != 2 || !strings.Contains(m.View(), "movie.mov") {
		t.Fatalf("enter must open media/:\n%s", m.View())
	}
	analyzeKey(m, "enter")
	if len(*shown) != 1 || !strings.HasSuffix((*shown)[0], "media/movie.mov") {
		t.Fatalf("enter on a file must show it in Finder: %v", *shown)
	}
	analyzeKey(m, "left")
	if len(m.levels) != 1 {
		t.Fatal("left must go up")
	}
	if m.levels[0].cursor != 0 {
		t.Fatal("the cursor of the parent must be kept")
	}
	analyzeKey(m, "tab")
	out := m.View()
	if !strings.Contains(out, "Largest files") || strings.Index(out, "movie.mov") > strings.Index(out, "data.bin") {
		t.Fatalf("the largest files must be listed by size:\n%s", out)
	}
	analyzeKey(m, "down")
	analyzeKey(m, "enter")
	if len(*shown) != 2 || !strings.HasSuffix((*shown)[1], "data.bin") {
		t.Fatalf("enter in the file list: %v", *shown)
	}
	analyzeKey(m, "esc")
	if m.files {
		t.Fatal("esc must leave the file list")
	}
	if cmd := analyzeKey(m, "esc"); cmd == nil {
		t.Fatal("esc at the root must close the screen")
	}
}

func TestAnalyzeScanningScreenShowsRunningTotals(t *testing.T) {
	m := NewAnalyze("/tmp/example", scanner.Hints{})
	m.scanning = true
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(analyzeProgressMsg(usage.Progress{Files: 12345, Allocated: 3 << 30, Current: "/tmp/example/b/deep", Top: []usage.TopProgress{
		{Name: "a", Dir: true, Allocated: 1 << 30, Done: true},
		{Name: "b", Dir: true, Allocated: 2 << 30},
		{Name: "c", Dir: true},
	}}))
	out := m.View()
	assertFits(t, out, 80, 24)
	for _, want := range []string{"3.00 GiB found so far", "12,345 files", "✓ a/", "2.00 GiB+", "· c/", "stop and use what was found"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	if strings.Index(out, "b/") > strings.Index(out, "a/") {
		t.Errorf("the largest entry must be first:\n%s", out)
	}
}

func TestAnalyzeHelpListsTheKeysThatANarrowBarHides(t *testing.T) {
	m, _ := scannedAnalyze(t, 58, 20)
	if strings.Contains(m.View(), "scan again") {
		t.Fatal("the narrow key bar is expected to hide some keys")
	}
	analyzeKey(m, "?")
	out := m.View()
	assertFits(t, out, 58, 20)
	for _, want := range []string{"scan again", "largest files", "Finder", "back"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	analyzeKey(m, "x")
	if strings.Contains(m.View(), "KEYS") {
		t.Fatal("a key must close the help")
	}
}

func TestAnalyzeMarksFoldersThatACleanupRuleCovers(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"Library/Caches/go-build/00/a", "Documents/a"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, 8192), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := usage.Scan(context.Background(), root, usage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m := NewAnalyze(root, scanner.NewHints(result.Path, nil))
	m.scanning = true
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(analyzeDoneMsg{result: result})
	for _, name := range []string{"Library", "Caches"} {
		for i, r := range m.rows(m.levels[len(m.levels)-1].node) {
			if r.node.Name == name {
				m.levels[len(m.levels)-1].cursor = i
			}
		}
		analyzeKey(m, "enter")
	}
	out := m.View()
	if !strings.Contains(out, "go-build/") || !strings.Contains(out, "●") || !strings.Contains(out, "Cleanable") || !strings.Contains(out, "handbyte clean") {
		t.Fatalf("go-build must be marked as cleanable:\n%s", out)
	}
	analyzeKey(m, "left")
	analyzeKey(m, "left")
	if strings.Contains(m.View(), "Cleanable") {
		t.Fatalf("the home folder itself is not covered:\n%s", m.View())
	}
}
