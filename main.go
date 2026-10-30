// handbyte is an interactive macOS disk cleanup CLI.
//
// Usage:
//
//	handbyte                run interactive TUI (move-to-Trash on confirm)
//	handbyte --rm           use rm -rf instead of Trash (unrecoverable)
//	handbyte --dry-run      preview deletions only
//	handbyte --min=200M     drop items smaller than this from the list
//	handbyte --roots=p1,p2  override project-scan roots (default ~/dev …)
//	handbyte --no-projects  skip the project artifact scan entirely
//	handbyte --workers=8    cap concurrent du workers
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
	"github.com/wanggang316/handbyte/internal/ui"
)

func main() {
	var (
		flagScope      = flag.String("scope", "all", "scan scope: all, quick, or projects")
		flagExplore    = flag.String("explore", "", "browse a directory without cleanup actions")
		flagSaveRoots  = flag.Bool("save-roots", false, "save --roots for future scans and exit")
		flagProtect    = flag.String("protect", "", "protect a path from cleanup and exit")
		flagUnprotect  = flag.String("unprotect", "", "remove an exact path protection and exit")
		flagHistory    = flag.Bool("history", false, "print recorded actions as JSON")
		flagRestore    = flag.String("restore", "", "restore a recorded Trash action by ID")
		flagRM         = flag.Bool("rm", false, "delete files directly instead of moving to Trash")
		flagDryRun     = flag.Bool("dry-run", false, "report what would be deleted without changing anything")
		flagMin        = flag.String("min", "50M", "drop items smaller than this size (e.g. 100M, 1G)")
		flagRoots      = flag.String("roots", "", "comma-separated project roots (default: ~/dev,~/Projects,~/Code,~/workspace)")
		flagNoProjects = flag.Bool("no-projects", false, "skip the project artifact scan")
		flagWorkers    = flag.Int("workers", 6, "number of concurrent scan workers")
		flagScanOnly   = flag.Bool("scan-only", false, "scan and print JSON results, skip TUI")
	)
	flag.Parse()
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	if *flagHistory {
		results, err := cleaner.History()
		if err != nil {
			fail(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
			fail(err)
		}
		return
	}
	if *flagRestore != "" {
		if err := cleaner.Restore(context.Background(), *flagRestore); err != nil {
			fail(err)
		}
		fmt.Println("Restored to the original path")
		return
	}
	saved, err := settings.Load()
	if err != nil {
		fail(err)
	}
	if *flagProtect != "" || *flagUnprotect != "" {
		path := *flagProtect
		if path == "" {
			path = *flagUnprotect
		}
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}
		path, err = filepath.Abs(path)
		if err != nil {
			fail(err)
		}
		if physical, e := scanner.Physical(path); e == nil {
			path = physical
		}
		paths := []string{}
		for _, existing := range saved.Exclusions {
			if existing != path {
				paths = append(paths, existing)
			}
		}
		if *flagProtect != "" {
			paths = append(paths, path)
		}
		saved.Exclusions = paths
		if err := settings.Save(saved); err != nil {
			fail(err)
		}
		fmt.Println("Protected paths updated")
		return
	}
	if *flagScope != "all" && *flagScope != "quick" && *flagScope != "projects" {
		fail(fmt.Errorf("invalid --scope: %s", *flagScope))
	}

	minSize, err := parseSize(*flagMin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --min: %v\n", err)
		os.Exit(2)
	}
	cfg := scanner.Config{
		Scope:           *flagScope,
		ProjectRoots:    saved.Roots,
		Exclusions:      saved.Exclusions,
		MinSize:         minSize,
		Workers:         *flagWorkers,
		SkipProjectScan: *flagNoProjects,
	}
	if *flagRoots != "" {
		cfg.ProjectRoots = nil
		for _, r := range strings.Split(*flagRoots, ",") {
			if r = strings.TrimSpace(r); r != "" {
				cfg.ProjectRoots = append(cfg.ProjectRoots, r)
			}
		}
	}

	if *flagSaveRoots {
		if *flagRoots == "" {
			fail(fmt.Errorf("--save-roots requires --roots"))
		}
		saved.Roots = cfg.ProjectRoots
		if err := settings.Save(saved); err != nil {
			fail(err)
		}
		fmt.Println("Project roots saved")
		return
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
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		var items []scanner.Item
		if *flagExplore != "" {
			items, err = scanner.Explore(ctx, *flagExplore, nil)
		} else {
			items, err = scanner.Scan(ctx, cfg, nil)
		}
		status := "complete"
		warnings := []string{}
		for _, it := range items {
			if it.ScanStatus != "complete" {
				status = "partial"
				if it.ScanError != "" {
					warnings = append(warnings, it.ScanError)
				}
			}
		}
		if err != nil {
			status = "partial"
			warnings = append(warnings, err.Error())
			if ctx.Err() != nil {
				status = "cancelled"
			}
		}
		if items == nil {
			items = []scanner.Item{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		encodeErr := enc.Encode(map[string]any{
			"schema_version": 2, "scan_status": status, "warnings": warnings,
			"free_bytes": free,
			"items":      items,
		})
		if encodeErr != nil {
			fail(encodeErr)
		}
		if err != nil {
			os.Exit(1)
		}
		return
	}

	if *flagExplore != "" {
		if err := ui.RunExplore(cfg, mode, free, *flagExplore); err != nil {
			fail(err)
		}
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
	if n < 0 || n > math.MaxInt64/unit {
		return 0, fmt.Errorf("size must be non-negative and fit in int64")
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
