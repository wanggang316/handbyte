package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/tui"
)

func humanBytes(n int64) string { return tui.Bytes(n) }

func condensePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && scanner.Contains(home, p) {
		return "~" + p[len(home):]
	}
	return p
}

func progressBar(pct float64, width int) string {
	if width < 4 {
		width = 4
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(float64(width) * pct)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return primary.Render(bar) + fmt.Sprintf("  %3.0f%%", pct*100)
}
