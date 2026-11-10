package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wanggang316/handbyte/internal/usage"
)

func testHome(width, height int) *Home {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	m := NewHome(HomeInfo{Volume: "Macintosh HD", Version: "v0.3.0", Total: 494 << 30, Free: 182 << 30, Actions: 2, LastAction: now.Add(-72 * time.Hour), Now: now})
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func press(m *Home, key string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	}
	_, cmd := m.Update(msg)
	return cmd
}

func TestHomeFitsEveryCheckedTerminalSize(t *testing.T) {
	for _, size := range [][2]int{{58, 20}, {80, 24}, {110, 28}, {160, 45}, {40, 10}} {
		out := testHome(size[0], size[1]).View()
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := Width(line); w > size[0] {
				t.Errorf("%dx%d: line %d has %d cells: %q", size[0], size[1], i, w, line)
			}
		}
		for _, want := range []string{"Clean", "Apps", "Analyze", "History", "enter"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d: %q is missing", size[0], size[1], want)
			}
		}
	}
}

func TestHomeShowsLiveNumbersAndTheHintOfTheCurrentEntry(t *testing.T) {
	m := testHome(110, 28)
	out := m.View()
	for _, want := range []string{"182.00 GiB free", "of 494.00 GiB", "last cleaned 3 days ago", "312.00 GiB used", "2 actions", "Nothing is selected for you", "open Clean"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing in:\n%s", want, out)
		}
	}
	press(m, "3")
	if out := m.View(); !strings.Contains(out, "This view is read-only") || !strings.Contains(out, "open Analyze") {
		t.Errorf("the hint did not follow the cursor:\n%s", out)
	}
}

func TestNarrowHomeMovesTheHintToTheStatusLine(t *testing.T) {
	out := testHome(58, 20).View()
	if strings.Contains(out, "╭") || !strings.Contains(out, "Scans known cache") {
		t.Errorf("narrow home must show the hint as one line:\n%s", out)
	}
}

func TestHomeKeys(t *testing.T) {
	m := testHome(110, 28)
	press(m, "down")
	if cmd := press(m, "enter"); cmd != nil || m.notice == "" {
		t.Fatalf("a planned entry must stay on the home screen with a notice")
	}
	press(m, "down")
	if cmd := press(m, "enter"); cmd == nil || m.Choice != ActionAnalyze {
		t.Fatalf("enter on Analyze: choice %d", m.Choice)
	}
	m = testHome(110, 28)
	if cmd := press(m, "enter"); cmd == nil || m.Choice != ActionClean {
		t.Fatalf("enter on Clean: choice %d", m.Choice)
	}
	m = testHome(110, 28)
	press(m, "4")
	if cmd := press(m, "q"); cmd == nil || m.Choice != ActionQuit {
		t.Fatalf("q must quit")
	}
	m = testHome(110, 28)
	press(m, "?")
	if out := m.View(); !strings.Contains(out, "handbyte restore <id>") {
		t.Errorf("help is missing:\n%s", out)
	}
}

func TestHomeShowsTheCategoriesOfTheLastAnalyze(t *testing.T) {
	m := testHome(110, 28)
	if out := m.View(); !strings.Contains(out, "Open Analyze once") {
		t.Fatalf("without a result the home screen must say how to get one:\n%s", out)
	}
	m.info.Home = &usage.Summary{Allocated: 150 << 30, ScannedAt: m.info.Now.Add(-2 * time.Hour), Categories: []usage.CategoryTotal{
		{ID: "developer", Allocated: 96 << 30}, {ID: "media", Allocated: 40 << 30}, {ID: "other", Allocated: 14 << 30},
	}}
	out := m.View()
	for _, want := range []string{"Developer 96.00 GiB", "Media 40.00 GiB", "System and other 176.00 GiB", "home 150.00 GiB · 2 h ago"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	assertFits(t, out, 110, 28)
}
