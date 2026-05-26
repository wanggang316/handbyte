// hand-clean is an interactive macOS disk cleanup CLI.
//
// Usage:
//
//	hand-clean                run interactive TUI (move-to-Trash on confirm)
//	hand-clean --rm           use rm -rf instead of Trash (unrecoverable)
//	hand-clean --dry-run      preview deletions only
//	hand-clean --min=200M     drop items smaller than this from the list
//	hand-clean --roots=p1,p2  override project-scan roots (default ~/dev …)
//	hand-clean --no-projects  skip the project artifact scan entirely
//	hand-clean --workers=8    cap concurrent du workers
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/wanggang316/hand-clean/internal/cleaner"
	"github.com/wanggang316/hand-clean/internal/scanner"
	"github.com/wanggang316/hand-clean/internal/ui"
)

func main() {
	var (
		flagRM         = flag.Bool("rm", false, "delete files directly instead of moving to Trash")
		flagDryRun     = flag.Bool("dry-run", false, "report what would be deleted without changing anything")
		flagMin        = flag.String("min", "50M", "drop items smaller than this size (e.g. 100M, 1G)")
		flagRoots      = flag.String("roots", "", "comma-separated project roots (default: ~/dev,~/Projects,~/Code,~/workspace)")
		flagNoProjects = flag.Bool("no-projects", false, "skip the project artifact scan")
		flagWorkers    = flag.Int("workers", 6, "number of concurrent scan workers")
		flagScanOnly   = flag.Bool("scan-only", false, "scan and print JSON results, skip TUI")
	)
	flag.Parse()

	minSize, err := parseSize(*flagMin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --min: %v\n", err)
		os.Exit(2)
	}
	cfg := scanner.Config{
		MinSize:         minSize,
		Workers:         *flagWorkers,
		SkipProjectScan: *flagNoProjects,
	}
	if *flagRoots != "" {
		for _, r := range strings.Split(*flagRoots, ",") {
			if r = strings.TrimSpace(r); r != "" {
				cfg.ProjectRoots = append(cfg.ProjectRoots, r)
			}
		}
	}

	mode := cleaner.ModeTrash
	switch {
	case *flagDryRun:
		mode = cleaner.ModeDryRun
	case *flagRM:
		mode = cleaner.ModeRemove
	}

	free, _ := freeBytes("/")

	if *flagScanOnly {
		items, err := scanner.Scan(context.Background(), cfg, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"free_bytes": free,
			"items":      items,
		})
		return
	}

	if err := ui.Run(cfg, mode, free); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseSize accepts an integer with optional K / M / G / T suffix (case-insensitive).
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	unit := int64(1)
	switch s[len(s)-1] {
	case 'K', 'k':
		unit = 1 << 10
		s = s[:len(s)-1]
	case 'M', 'm':
		unit = 1 << 20
		s = s[:len(s)-1]
	case 'G', 'g':
		unit = 1 << 30
		s = s[:len(s)-1]
	case 'T', 't':
		unit = 1 << 40
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse size: %w", err)
	}
	return n * unit, nil
}

// freeBytes returns available bytes on the filesystem hosting path.
func freeBytes(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
