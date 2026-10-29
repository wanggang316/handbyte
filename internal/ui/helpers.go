package ui

import (
	"fmt"
	"github.com/wanggang316/handbyte/internal/scanner"
	"os"
	"strings"
)

func humanBytes(n int64) string {
	const (
		k = 1024
		M = k * 1024
		G = M * 1024
		T = G * 1024
	)
	switch {
	case n >= T:
		return fmt.Sprintf("%.2f TiB", float64(n)/T)
	case n >= G:
		return fmt.Sprintf("%.2f GiB", float64(n)/G)
	case n >= M:
		return fmt.Sprintf("%.1f MiB", float64(n)/M)
	case n >= k:
		return fmt.Sprintf("%.1f KiB", float64(n)/k)
	}
	return fmt.Sprintf("%d B", n)
}

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
