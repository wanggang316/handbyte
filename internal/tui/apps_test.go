package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wanggang316/handbyte/internal/apps"
	"github.com/wanggang316/handbyte/internal/cleaner"
)

var appsNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func fakeApps(t *testing.T, plans *[]cleaner.AppPlan) *Apps {
	t.Helper()
	list := []apps.App{
		{Name: "Notes", BundleID: "com.example.Notes", Path: "/Applications/Notes.app", Size: 300 << 20, Source: apps.SourceDirect, LastUsed: appsNow.Add(-400 * 24 * time.Hour), LastUsedKnown: true, Archs: []string{"arm64"}},
		{Name: "Brewed", BundleID: "com.example.brewed", Path: "/Applications/Brewed.app", Size: 100 << 20, Source: apps.SourceHomebrew, CaskToken: "brewed"},
		{Name: "Safari", BundleID: "com.apple.Safari", Path: "/Applications/Safari.app", Size: 50 << 20, System: true},
		{Name: "Busy", BundleID: "com.example.busy", Path: "/Applications/Busy.app", Size: 10 << 20, Running: true, LastUsed: appsNow, LastUsedKnown: true},
	}
	m := NewApps(AppsOptions{
		Inventory: func(context.Context) ([]apps.App, error) { return list, nil },
		Known:     func([]apps.App) []string { return nil },
		Related: func(a apps.App, _ []string, _ bool) []apps.Related {
			return []apps.Related{
				{Path: "/h/Library/Caches/" + a.BundleID, Class: "cache", Confidence: apps.High, Selected: true, Size: 20 << 20, IsDir: true},
				{Path: "/h/Library/Application Support/" + a.BundleID, Class: "data", Confidence: apps.High, Selected: false, Size: 80 << 20, IsDir: true},
			}
		},
		Leftovers: func(_, _ []string) []apps.LeftoverGroup {
			return []apps.LeftoverGroup{{BundleID: "net.gone.App", Items: []apps.Leftover{{Path: "/h/Library/Caches/net.gone.App", BundleID: "net.gone.App", Class: "cache", IsDir: true}}}}
		},
		Uninstall: func(_ context.Context, p cleaner.AppPlan) (string, []cleaner.Result, error) {
			*plans = append(*plans, p)
			return "app-test", []cleaner.Result{{Path: p.Bundle, Status: "trashed"}}, nil
		},
		Now: func() time.Time { return appsNow },
	})
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
	msg := m.Init()()
	_, cmd := m.Update(msg)
	if cmd != nil {
		for _, c := range []tea.Cmd{cmd} {
			if b, ok := c().(tea.BatchMsg); ok {
				for _, inner := range b {
					if inner != nil {
						m.Update(inner())
					}
				}
			} else {
				m.Update(c())
			}
		}
	}
	return m
}

func appsKey(m *Apps, key string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case " ":
		msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	_, cmd := m.Update(msg)
	return cmd
}

func TestAppsListFiltersAndFitsTheTerminal(t *testing.T) {
	var plans []cleaner.AppPlan
	m := fakeApps(t, &plans)
	out := m.View()
	for _, want := range []string{"4 apps", "Notes", "Brewed", "Safari", "1 All", "Bundle ID", "com.example.Notes", "unknown"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	assertFits(t, out, 110, 28)
	appsKey(m, "2")
	if out := m.View(); !strings.Contains(out, "Notes") || strings.Contains(out, "Busy") {
		t.Errorf("Unused must list only apps not opened for six months:\n%s", out)
	}
	appsKey(m, "3")
	if v := m.visible(); m.list[v[0]].Name != "Notes" {
		t.Error("Largest must sort by size")
	}
	appsKey(m, "4")
	if out := m.View(); !strings.Contains(out, "net.gone.App") {
		t.Errorf("Leftovers:\n%s", out)
	}
	for _, size := range [][2]int{{58, 20}, {80, 24}, {160, 45}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		assertFits(t, m.View(), size[0], size[1])
	}
}

func TestUninstallReviewSelectsAndMovesAsOnePlan(t *testing.T) {
	var plans []cleaner.AppPlan
	m := fakeApps(t, &plans)
	appsKey(m, "enter")
	out := m.View()
	for _, want := range []string{"Uninstall Notes", "[x]", "[ ]", "cache", "data", "one group"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is missing:\n%s", want, out)
		}
	}
	if m.review == nil || len(m.review.files) != 3 || !m.review.files[0].bundle || m.review.files[2].selected {
		t.Fatalf("review: %+v", m.review)
	}
	// The data line can be selected; the app line cannot be unselected.
	m.review.cursor = 2
	appsKey(m, " ")
	m.review.cursor = 0
	appsKey(m, " ")
	if !m.review.files[0].selected || !m.review.files[2].selected {
		t.Fatal("selection")
	}
}

func TestUninstallReviewRefusesSystemHomebrewAndRunningApps(t *testing.T) {
	var plans []cleaner.AppPlan
	m := fakeApps(t, &plans)
	for _, name := range []string{"Safari", "Brewed", "Busy"} {
		m.openReview(m.find(name), false)
		if m.review.refusal == "" {
			t.Errorf("%s: no refusal", name)
		}
		appsKey(m, "y")
		if len(plans) != 0 || !strings.Contains(m.View(), "!") {
			t.Errorf("%s: a refused app must not be uninstalled", name)
		}
		appsKey(m, "esc")
	}
	m.openReview(m.find("Brewed"), false)
	appsKey(m, "R")
	if m.review.refusal != "" || m.review.files[0].bundle {
		t.Fatal("R must offer the files of a Homebrew app without the app")
	}
}

func TestAppsMouseSelectsARow(t *testing.T) {
	var plans []cleaner.AppPlan
	m := fakeApps(t, &plans)
	m.View()
	m.Update(tea.MouseMsg{Y: m.listTop + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.cursor != 1 || m.review != nil {
		t.Fatalf("cursor %d", m.cursor)
	}
	m.Update(tea.MouseMsg{Y: m.listTop + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.review == nil {
		t.Fatal("a click on the current app must open its review")
	}
}
