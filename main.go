// handbyte is a macOS disk cleanup CLI.
//
// Run "handbyte help" for the commands. Options without a command are the
// form of earlier releases; they still work and name the new command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
	"github.com/wanggang316/handbyte/internal/tui"
	"github.com/wanggang316/handbyte/internal/ui"
	"github.com/wanggang316/handbyte/internal/usage"
)

// version is set by the release build.
var version = "dev"

const usageText = `handbyte cleans caches and build output on macOS.

Usage:
  handbyte                    Home screen
  handbyte clean              Review and clean caches and build output
  handbyte analyze [path]     Folders by size and the largest files, read-only
  handbyte history            Past cleanup actions
  handbyte restore <id>       Put a Trash action back
  handbyte config             Show the project roots and the protected paths
  handbyte config roots <paths>      Save the project roots (comma-separated)
  handbyte config protect <path>     Protect a path from cleanup
  handbyte config unprotect <path>   Remove that protection
  handbyte version            Print the version

Options of clean:
  --dry-run        Preview only; change nothing
  --rm             Delete directly, not through the Trash (permanent)
  --scope=<s>      all, quick or projects (default all)
  --min=<size>     Drop items smaller than this, for example 200M (default 50M)
  --roots=<paths>  Project roots for this scan only
  --no-projects    Skip the project scan
  --workers=<n>    Concurrent scan workers (default 6)
  --json           Print the scan as JSON and do not clean

Options of analyze and history:
  --json           Print the data as JSON
`

// usageError is an error of the command line, not of the operation.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	err := dispatch(args)
	var usage usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &usage):
		fmt.Fprintf(os.Stderr, "handbyte: %s\nRun \"handbyte help\" for the commands.\n", usage.msg)
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func dispatch(args []string) error {
	if len(args) == 0 {
		if interactive() {
			return runHome()
		}
		fmt.Print(usageText)
		return nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Print(usageText)
		return nil
	case "version", "--version":
		fmt.Println(version)
		return nil
	case "clean":
		return cmdClean(args[1:])
	case "analyze":
		return cmdAnalyze(args[1:])
	case "history":
		return cmdHistory(args[1:])
	case "restore":
		return cmdRestore(args[1:])
	case "config":
		return cmdConfig(args[1:])
	}
	if strings.HasPrefix(args[0], "-") {
		return runLegacy(args)
	}
	return usageError{"unknown command " + strconv.Quote(args[0])}
}

// interactive reports whether both ends are a terminal.
func interactive() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

// parse accepts options before, between and after the positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usageError{err.Error()}
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// scanFlags are the options that shape a cleanup scan.
type scanFlags struct {
	scope, min, roots *string
	noProjects        *bool
	workers           *int
}

func addScanFlags(fs *flag.FlagSet) scanFlags {
	return scanFlags{
		scope:      fs.String("scope", "all", ""),
		min:        fs.String("min", "50M", ""),
		roots:      fs.String("roots", "", ""),
		noProjects: fs.Bool("no-projects", false, ""),
		workers:    fs.Int("workers", 6, ""),
	}
}

func (f scanFlags) config(saved settings.Config) (scanner.Config, error) {
	if *f.scope != "all" && *f.scope != "quick" && *f.scope != "projects" {
		return scanner.Config{}, usageError{"invalid --scope: " + *f.scope}
	}
	minSize, err := parseSize(*f.min)
	if err != nil {
		return scanner.Config{}, usageError{"invalid --min: " + err.Error()}
	}
	cfg := scanner.Config{
		Scope:           *f.scope,
		ProjectRoots:    saved.Roots,
		Exclusions:      saved.Exclusions,
		MinSize:         minSize,
		Workers:         *f.workers,
		SkipProjectScan: *f.noProjects,
	}
	if roots := splitRoots(*f.roots); len(roots) > 0 {
		cfg.ProjectRoots = roots
	}
	return cfg, nil
}

func splitRoots(list string) []string {
	var roots []string
	for _, r := range strings.Split(list, ",") {
		if r = strings.TrimSpace(r); r != "" {
			roots = append(roots, r)
		}
	}
	return roots
}

func cleanMode(dryRun, remove bool) cleaner.Mode {
	switch {
	case dryRun:
		return cleaner.ModeDryRun
	case remove:
		return cleaner.ModeRemove
	}
	return cleaner.ModeTrash
}

func cmdClean(args []string) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	scan := addScanFlags(fs)
	dryRun := fs.Bool("dry-run", false, "")
	remove := fs.Bool("rm", false, "")
	asJSON := fs.Bool("json", false, "")
	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usageError{"clean takes no path; use: handbyte analyze " + positional[0]}
	}
	saved, err := settings.Load()
	if err != nil {
		return err
	}
	cfg, err := scan.config(saved)
	if err != nil {
		return err
	}
	if *asJSON {
		return printScan(cfg, "")
	}
	free, _ := freeBytes("/")
	return ui.Run(cfg, cleanMode(*dryRun, *remove), free)
}

func cmdAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "")
	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return usageError{"analyze takes one path"}
	}
	path, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if len(positional) == 1 {
		path = positional[0]
		if path == "~" || strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, strings.TrimPrefix(positional[0], "~"))
		}
	}
	if *asJSON {
		return printUsage(path)
	}
	return tui.RunAnalyze(path)
}

// printUsage scans a folder with the storage engine and prints the first
// level and the largest files as JSON.
func printUsage(path string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := usage.Scan(ctx, path, usage.Options{})
	if err != nil {
		return err
	}
	type child struct {
		Name      string `json:"name"`
		Dir       bool   `json:"is_dir"`
		Allocated int64  `json:"allocated_bytes"`
		Logical   int64  `json:"logical_bytes"`
		Files     int64  `json:"file_count"`
		Status    string `json:"status"`
		Note      string `json:"note,omitempty"`
	}
	type file struct {
		Path      string `json:"path"`
		Allocated int64  `json:"allocated_bytes"`
		Logical   int64  `json:"logical_bytes"`
	}
	status := func(n *usage.Node) string {
		switch {
		case n.Skipped != "":
			return "skipped"
		case n.Incomplete:
			return "partial"
		}
		return "complete"
	}
	root := result.Root
	report := struct {
		Schema     int     `json:"schema_version"`
		Path       string  `json:"path"`
		Status     string  `json:"status"`
		Allocated  int64   `json:"allocated_bytes"`
		Logical    int64   `json:"logical_bytes"`
		Files      int64   `json:"file_count"`
		OtherFiles int64   `json:"other_file_count"`
		OtherBytes int64   `json:"other_allocated_bytes"`
		Children   []child `json:"children"`
		Largest    []file  `json:"largest_files"`
	}{1, result.Path, status(root), root.Allocated, root.Logical, root.Files, root.OtherFiles, root.OtherAllocated, []child{}, []file{}}
	if result.Cancelled {
		report.Status = "cancelled"
	}
	for _, n := range root.Children {
		note := n.Skipped
		if note == "" {
			note = n.Err
		}
		report.Children = append(report.Children, child{n.Name, n.Dir, n.Allocated, n.Logical, n.Files, status(n), note})
	}
	for _, f := range result.Largest {
		report.Largest = append(report.Largest, file{f.Path, f.Allocated, f.Logical})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func cmdHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "")
	if positional, err := parse(fs, args); err != nil {
		return err
	} else if len(positional) > 0 {
		return usageError{"history takes no argument"}
	}
	results, err := cleaner.History()
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(results)
	}
	printHistory(os.Stdout, results)
	return nil
}

func printHistory(w io.Writer, results []cleaner.Result) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No actions recorded.")
		return
	}
	home, _ := os.UserHomeDir()
	for i := len(results) - 1; i >= 0; i-- {
		r := results[i]
		path := r.Path
		if home != "" && scanner.Contains(home, path) {
			path = "~" + path[len(home):]
		}
		when := r.EndedAt
		if when.IsZero() {
			when = r.StartedAt
		}
		fmt.Fprintf(w, "%s  %-10s %10s  %s\n    id %s\n", when.Local().Format("2006-01-02 15:04"), r.Status, tui.Bytes(r.Moved+r.Freed+r.Previewed), path, r.ID)
	}
	fmt.Fprintln(w, "\nPut an item back from the Trash: handbyte restore <id>")
}

func cmdRestore(args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return usageError{"restore needs one action ID; see: handbyte history"}
	}
	if err := cleaner.Restore(context.Background(), args[0]); err != nil {
		return err
	}
	fmt.Println("Restored to the original path")
	return nil
}

func cmdConfig(args []string) error {
	saved, err := settings.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		printConfig(os.Stdout, saved)
		return nil
	}
	switch {
	case args[0] == "roots" && len(args) >= 2:
		roots := splitRoots(strings.Join(args[1:], ","))
		if len(roots) == 0 {
			return usageError{"config roots needs one path or more"}
		}
		saved.Roots = roots
		if err := settings.Save(saved); err != nil {
			return err
		}
		fmt.Println("Project roots saved")
		return nil
	case (args[0] == "protect" || args[0] == "unprotect") && len(args) == 2:
		return setProtection(saved, args[1], args[0] == "protect")
	}
	return usageError{"usage: handbyte config [roots <paths> | protect <path> | unprotect <path>]"}
}

func printConfig(w io.Writer, saved settings.Config) {
	fmt.Fprintln(w, "Project roots")
	roots, note := saved.Roots, ""
	if len(roots) == 0 {
		roots, note = scanner.DefaultProjectRoots, "  (default)"
	}
	for _, r := range roots {
		fmt.Fprintf(w, "  %s%s\n", r, note)
	}
	fmt.Fprintln(w, "\nProtected paths")
	if len(saved.Exclusions) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, p := range saved.Exclusions {
		fmt.Fprintf(w, "  %s\n", p)
	}
}

// setProtection adds or removes one exact protected path.
func setProtection(saved settings.Config, path string, protect bool) error {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
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
	if protect {
		paths = append(paths, path)
	}
	saved.Exclusions = paths
	if err := settings.Save(saved); err != nil {
		return err
	}
	fmt.Println("Protected paths updated")
	return nil
}

// printScan runs a cleanup scan, or a folder scan when explore is set, and
// prints the result as JSON. A scan that is not complete is an error.
func printScan(cfg scanner.Config, explore string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var items []scanner.Item
	var err error
	if explore != "" {
		items, err = scanner.Explore(ctx, explore, nil)
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
	free, _ := freeBytes("/")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if encodeErr := enc.Encode(map[string]any{
		"schema_version": 2, "scan_status": status, "warnings": warnings,
		"free_bytes": free,
		"items":      items,
	}); encodeErr != nil {
		return encodeErr
	}
	if err != nil {
		return errors.New("scan is not complete")
	}
	return nil
}

// runHome shows the home screen and opens what the user chooses, until the
// user quits.
func runHome() error {
	for {
		info := tui.HomeInfo{Volume: volumeName(), Version: version, Now: time.Now()}
		info.Total, info.Free = volumeBytes("/")
		if results, err := cleaner.History(); err == nil {
			info.Actions = len(results)
			for _, r := range results {
				if r.EndedAt.After(info.LastAction) && r.Status != "previewed" {
					info.LastAction = r.EndedAt
				}
			}
		}
		action, err := tui.RunHome(info)
		if err != nil {
			return err
		}
		switch action {
		case tui.ActionClean:
			err = cmdClean(nil)
		case tui.ActionAnalyze:
			err = cmdAnalyze(nil)
		case tui.ActionHistory:
			return cmdHistory(nil)
		default:
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// volumeName returns the name of the startup volume. macOS keeps a symbolic
// link to "/" with that name in /Volumes.
func volumeName() string {
	entries, err := os.ReadDir("/Volumes")
	if err == nil {
		for _, e := range entries {
			if target, err := os.Readlink(filepath.Join("/Volumes", e.Name())); err == nil && target == "/" {
				return e.Name()
			}
		}
	}
	return "Startup disk"
}

// runLegacy accepts the options of earlier releases, which had no command,
// and names the command that replaces them.
func runLegacy(args []string) error {
	fs := flag.NewFlagSet("handbyte", flag.ContinueOnError)
	scan := addScanFlags(fs)
	explore := fs.String("explore", "", "")
	saveRoots := fs.Bool("save-roots", false, "")
	protect := fs.String("protect", "", "")
	unprotect := fs.String("unprotect", "", "")
	history := fs.Bool("history", false, "")
	restore := fs.String("restore", "", "")
	remove := fs.Bool("rm", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	scanOnly := fs.Bool("scan-only", false, "")
	if positional, err := parse(fs, args); err != nil {
		return err
	} else if len(positional) > 0 {
		return usageError{"unexpected argument " + strconv.Quote(positional[0])}
	}
	replacement := func(command string) {
		fmt.Fprintf(os.Stderr, "handbyte: options without a command stop in a later release. Use: %s\n", command)
	}
	saved, err := settings.Load()
	switch {
	case *history:
		replacement("handbyte history --json")
		return cmdHistory([]string{"--json"})
	case *restore != "":
		replacement("handbyte restore " + *restore)
		return cmdRestore([]string{*restore})
	case err != nil:
		return err
	case *protect != "":
		replacement("handbyte config protect " + *protect)
		return setProtection(saved, *protect, true)
	case *unprotect != "":
		replacement("handbyte config unprotect " + *unprotect)
		return setProtection(saved, *unprotect, false)
	case *saveRoots:
		if *scan.roots == "" {
			return usageError{"--save-roots requires --roots"}
		}
		replacement("handbyte config roots " + *scan.roots)
		return cmdConfig([]string{"roots", *scan.roots})
	}
	cfg, err := scan.config(saved)
	if err != nil {
		return err
	}
	free, _ := freeBytes("/")
	mode := cleanMode(*dryRun, *remove)
	switch {
	case *explore != "" && *scanOnly:
		replacement("handbyte analyze " + *explore + " --json")
		return printScan(cfg, *explore)
	case *explore != "":
		replacement("handbyte analyze " + *explore)
		return ui.RunExplore(cfg, mode, free, *explore)
	case *scanOnly:
		replacement("handbyte clean --json")
		return printScan(cfg, "")
	}
	replacement("handbyte clean")
	return ui.Run(cfg, mode, free)
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

// volumeBytes returns the size and the available bytes of the filesystem
// hosting path, or zeros when they cannot be read.
func volumeBytes(path string) (total, free int64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	return int64(st.Blocks) * int64(st.Bsize), int64(st.Bavail) * int64(st.Bsize)
}
