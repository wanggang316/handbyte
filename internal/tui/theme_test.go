package tui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Reference backgrounds: a plain and a Solarized light terminal, a black
// and a grey dark terminal.
var (
	lightBackgrounds = []string{"#FFFFFF", "#FDF6E3"}
	darkBackgrounds  = []string{"#000000", "#1E1E1E"}
)

func luminance(hex string) float64 {
	var c [3]float64
	for i := range c {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		x := float64(v) / 255
		if x <= 0.03928 {
			c[i] = x / 12.92
		} else {
			c[i] = math.Pow((x+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func checkContrast(t *testing.T, name string, c lipgloss.AdaptiveColor, minimum float64) {
	t.Helper()
	for _, bg := range lightBackgrounds {
		if r := contrast(c.Light, bg); r < minimum {
			t.Errorf("%s: light %s on %s is %.2f:1, want %.1f:1", name, c.Light, bg, r, minimum)
		}
	}
	for _, bg := range darkBackgrounds {
		if r := contrast(c.Dark, bg); r < minimum {
			t.Errorf("%s: dark %s on %s is %.2f:1, want %.1f:1", name, c.Dark, bg, r, minimum)
		}
	}
}

// Text colors need 4.5:1 (WCAG AA) on light and dark terminals. Faint is
// only for tracks and rules, so it has no minimum.
func TestTextColorsReadOnLightAndDarkTerminals(t *testing.T) {
	for name, c := range map[string]lipgloss.AdaptiveColor{
		"Ink": ColorInk, "Text": ColorText, "Dim": ColorDim,
		"Green": ColorGreen, "Amber": ColorAmber, "Red": ColorRed,
	} {
		checkContrast(t, name, c, 4.5)
	}
}

// Category hues fill bars, so they need 3:1 (WCAG non-text) on the plain
// backgrounds. Other is grey on purpose.
func TestCategoryColorsShowOnLightAndDarkTerminals(t *testing.T) {
	for id, c := range categoryColors {
		if id == "other" {
			continue
		}
		for _, pair := range [][2]string{{c.Light, lightBackgrounds[0]}, {c.Dark, darkBackgrounds[1]}} {
			if r := contrast(pair[0], pair[1]); r < 3 {
				t.Errorf("%s: %s on %s is %.2f:1, want 3:1", id, pair[0], pair[1], r)
			}
		}
	}
}

// withTerminal renders with the given profile and background for one test.
func withTerminal(t *testing.T, profile termenv.Profile, dark bool) {
	t.Helper()
	oldProfile, oldDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(profile)
	lipgloss.SetHasDarkBackground(dark)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(oldProfile)
		lipgloss.SetHasDarkBackground(oldDark)
	})
}

// sgr is the true-color foreground sequence of a hex color, as termenv
// writes it.
func sgr(hex string) string {
	return termenv.TrueColor.Color(hex).Sequence(false)
}

func keyScreens(t *testing.T, width, height int) map[string]string {
	t.Helper()
	analyze, _ := scannedAnalyze(t, width, height)
	return map[string]string{
		"home":    testHome(width, height).View(),
		"analyze": analyze.View(),
		"apps":    fakeApps(t, nil).View(),
	}
}

func TestKeyScreensUseTheVariantOfTheTerminalBackground(t *testing.T) {
	for _, dark := range []bool{false, true} {
		withTerminal(t, termenv.TrueColor, dark)
		want, other := ColorText.Light, ColorText.Dark
		if dark {
			want, other = other, want
		}
		for name, out := range keyScreens(t, 110, 28) {
			if !strings.Contains(out, sgr(want)) || strings.Contains(out, sgr(other)) {
				t.Errorf("%s, dark %v: text must use %s, not %s", name, dark, want, other)
			}
		}
		home := testHome(80, 24).View()
		assertFits(t, home, 80, 24)
	}
}

// NO_COLOR gives the Ascii profile: no color sequence, and brackets and
// glyphs keep the structure.
func TestNoColorKeepsTheStructure(t *testing.T) {
	withTerminal(t, termenv.Ascii, true)
	for name, out := range keyScreens(t, 110, 28) {
		if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
			t.Errorf("%s: color sequence without color:\n%q", name, out)
		}
		assertFits(t, out, 110, 28)
	}
	if home := testHome(110, 28).View(); !strings.Contains(home, "[enter]") {
		t.Errorf("the primary key must show in brackets:\n%s", home)
	}
}
