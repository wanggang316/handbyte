package tui

import (
	"strings"
	"testing"
)

func TestSizeForBreakpoints(t *testing.T) {
	cases := map[int]Size{20: Narrow, 59: Narrow, 60: Medium, 99: Medium, 100: Wide, 200: Wide}
	for width, want := range cases {
		if got := SizeFor(width); got != want {
			t.Errorf("width %d: got %d, want %d", width, got, want)
		}
	}
}

func frameAt(width, height int) Frame {
	return Frame{
		Width: width, Height: height,
		Crumb:   []string{"Clean"},
		Summary: "Macintosh HD · 182 GiB free",
		Body:    []string{"one", "two", strings.Repeat("long ", 80)},
		Status:  "Scan complete",
		Primary: Key{"enter", "review 3 items"},
		Keys:    []Key{{"↑↓", "move"}, {"space", "select"}, {"/", "filter"}, {"p", "protect"}, {"?", "help"}, {"q", "quit"}},
	}
}

func TestFrameFillsTheTerminalAndNeverOverflows(t *testing.T) {
	for _, size := range [][2]int{{58, 20}, {80, 24}, {110, 28}, {160, 45}, {30, 8}, {12, 3}} {
		out := frameAt(size[0], size[1]).Render()
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := Width(line); w > size[0] {
				t.Errorf("%dx%d: line %d has %d cells: %q", size[0], size[1], i, w, line)
			}
		}
	}
}

func TestKeyBarKeepsThePrimaryActionAtEveryWidth(t *testing.T) {
	for _, width := range []int{40, 58, 80, 110} {
		lines := strings.Split(frameAt(width, 24).Render(), "\n")
		bar := lines[len(lines)-1]
		if !strings.Contains(bar, "enter") {
			t.Errorf("width %d: primary key lost: %q", width, bar)
		}
	}
	narrow := strings.Split(frameAt(58, 24).Render(), "\n")
	if bar := narrow[len(narrow)-1]; !strings.Contains(bar, "more") || strings.Contains(bar, "protect") {
		t.Errorf("narrow key bar must end with the help pointer: %q", bar)
	}
	wide := strings.Split(frameAt(160, 24).Render(), "\n")
	if bar := wide[len(wide)-1]; !strings.Contains(bar, "quit") {
		t.Errorf("wide key bar must show all keys: %q", bar)
	}
}

func TestCompactHeaderBelowTwentyRows(t *testing.T) {
	tall := strings.Split(frameAt(80, 24).Render(), "\n")
	short := strings.Split(frameAt(80, 12).Render(), "\n")
	if tall[1] != "" || !strings.Contains(tall[2], "one") {
		t.Errorf("tall frame needs an empty line below the header: %q", tall[:3])
	}
	if !strings.Contains(short[1], "one") {
		t.Errorf("short frame must start the body on line 2: %q", short[:2])
	}
}

func TestBetweenAndCutMiddle(t *testing.T) {
	if got := Between("left", "right", 12); got != "left   right" {
		t.Errorf("Between: %q", got)
	}
	if got := Between("left side", "right", 10); Width(got) > 10 || strings.Contains(got, "right") {
		t.Errorf("Between must drop the right part: %q", got)
	}
	if got := CutMiddle("Library/Developer/Xcode/DerivedData", 15); Width(got) != 15 || !strings.HasPrefix(got, "Library") || !strings.HasSuffix(got, "edData") {
		t.Errorf("CutMiddle: %q", got)
	}
	if got := CutMiddle("短名字", 10); got != "短名字" {
		t.Errorf("CutMiddle changed a short name: %q", got)
	}
	if got := CutMiddle("中文目录名称很长很长", 9); Width(got) > 9 {
		t.Errorf("CutMiddle overflows with wide characters: %q (%d)", got, Width(got))
	}
}
