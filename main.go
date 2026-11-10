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
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wanggang316/handbyte/catalog"
	"github.com/wanggang316/handbyte/internal/apps"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
	"github.com/wanggang316/handbyte/internal/system"
	"github.com/wanggang316/handbyte/internal/tui"
	"github.com/wanggang316/handbyte/internal/ui"
	"github.com/wanggang316/handbyte/internal/usage"
)

// version is set by the release build.
var version = "dev"

const usageText = `handbyte cleans caches and build output on macOS.

Usage:
  handbyte                    Home screen
  handbyte clean              Smart Clean: scan, confirm once, clean the safe items
  handbyte analyze [path]     Folders by size and the largest files, read-only
  handbyte apps               Apps by size and last use; uninstall them; leftovers
  handbyte uninstall <app>    Review an app and its files, then move them to the Trash
  handbyte history            Past cleanup actions
  handbyte restore <id>       Put a Trash action back
  handbyte config             Show the project roots and the protected paths
  handbyte config roots <paths>      Save the project roots (comma-separated)
  handbyte config protect <path>     Protect a path from cleanup
  handbyte config mouse on|off       Mouse support in the full-screen views
  handbyte config unprotect <path>   Remove that protection
  handbyte version            Print the version

Options of clean:
  --review         Choose the items yourself, in the full list
  --deep           Also scan the project roots, then choose the items
  --yes            Clean the safe items without the confirmation
  --dry-run        Preview only; change nothing
  --trash          Move everything to the Trash, also caches
  --rm             Delete everything directly, not through the Trash
  --system         Old system logs and crash reports; asks for your password
  --scope=<s>      With --review: all, quick or projects (default all)
  --min=<size>     Drop items smaller than this, for example 200M (default 50M)
  --roots=<paths>  Project roots for this scan only
  --no-projects    Skip the project scan
  --workers=<n>    Concurrent scan workers (default 6)
  --json           Print the scan as JSON and do not clean

Options of apps:
  --unused         Only apps not opened for six months
  --leftovers      Data of apps that are no longer installed
  --json           Print the data as JSON

Options of uninstall:
  --with-data      Also select the app's own data (documents, databases)
  --reset          Keep the app; remove only its files
  --dry-run        Show what would move; change nothing
  --yes            Move without the review (without a terminal: required)

Options of analyze and history:
  --json           Print the data as JSON

By default, data that rebuilds by itself (caches, logs, build output) is
removed directly, so the freed space is real. Everything else goes to the
Trash.
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
	case "apps":
		return cmdApps(args[1:], "")
	case "uninstall":
		return cmdUninstall(args[1:])
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
	trash := fs.Bool("trash", false, "")
	asJSON := fs.Bool("json", false, "")
	review := fs.Bool("review", false, "")
	deep := fs.Bool("deep", false, "")
	yes := fs.Bool("yes", false, "")
	systemFiles := fs.Bool("system", false, "")
	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usageError{"clean takes no path; use: handbyte analyze " + positional[0]}
	}
	if *remove && *trash {
		return usageError{"--rm and --trash exclude each other"}
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
	mode := cleanMode(*dryRun, *remove)
	if mode == cleaner.ModeTrash && !*trash {
		mode = cleaner.ModeByData
	}
	free, _ := freeBytes("/")
	if *systemFiles {
		return cleanSystem(*dryRun, *yes)
	}
	if *review || *deep {
		if *deep {
			cfg.Scope, cfg.SkipProjectScan = "all", false
		}
		return ui.RunReview(cfg, mode, free)
	}

	// Smart Clean: the known locations only, every size, safe items only.
	smart := cfg
	smart.Scope, smart.SkipProjectScan, smart.MinSize = "quick", true, 1<<20
	opts := tui.SmartOptions{
		Mode: mode,
		Yes:  *yes,
		Scan: func(ctx context.Context, progress chan<- scanner.Progress) ([]scanner.Item, error) {
			return scanner.Scan(ctx, smart, progress)
		},
		Execute: func(ctx context.Context, items []scanner.Item, progress chan<- cleaner.Progress) []cleaner.Result {
			return cleaner.Execute(ctx, items, cleaner.Options{Mode: mode, Exclusions: smart.Exclusions}, progress)
		},
		FreeBytes: func() int64 { n, _ := freeBytes("/"); return n },
		SystemBytes: func() int64 {
			var total int64
			for _, r := range catalog.LoadSystem().Rules {
				total += system.MeasureRule(r, "", time.Now()).Bytes
			}
			return total
		},
	}
	if !interactive() {
		return smartPlain(os.Stdout, opts)
	}
	if !saved.IntroShown {
		for _, line := range []string{
			"Smart Clean takes only safe items: caches, logs and build output that apps make again.",
			"Those are removed, so the space is free at once. Everything else goes to the Trash.",
			"handbyte history lists each action; handbyte restore <id> puts Trash items back.",
		} {
			fmt.Println("  " + tui.Meta.Render(line))
		}
		fmt.Println()
		saved.IntroShown = true
		_ = settings.Save(saved)
	}
	next, err := tui.RunSmart(opts)
	if err != nil {
		return err
	}
	switch next {
	case tui.SmartReview:
		return ui.RunReview(cfg, mode, free)
	case tui.SmartDeep:
		cfg.Scope, cfg.SkipProjectScan = "all", false
		return ui.RunReview(cfg, mode, free)
	}
	return nil
}

// cleanSystem removes the old files of the system rules. They belong to
// root, so the command runs through sudo, which asks for the password once.
func cleanSystem(dryRun, yes bool) error {
	var rules []catalog.SystemRule
	var total int64
	for _, r := range catalog.LoadSystem().Rules {
		m := system.MeasureRule(r, "", time.Now())
		note := ""
		if m.Partial {
			note = "  (some folders could not be read; more may be removed)"
		}
		fmt.Printf("%-32s %6d files %10s%s\n", r.Title, m.Files, tui.Bytes(m.Bytes), note)
		if m.Files > 0 || m.Partial {
			rules = append(rules, r)
		}
		total += m.Bytes
	}
	if len(rules) == 0 {
		fmt.Println("Nothing to clean.")
		return nil
	}
	if dryRun {
		fmt.Println("Preview only. Nothing was changed.")
		return nil
	}
	if !yes {
		if !interactive() {
			fmt.Println("Nothing was changed. Run with --yes to clean; sudo must not need a password then.")
			return nil
		}
		fmt.Printf("Remove %s of old system logs? macOS asks for your password. [y/N] ", tui.Bytes(total))
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Nothing was changed.")
			return nil
		}
	}
	removed, err := system.Clean(context.Background(), rules, !interactive())
	fmt.Printf("Removed %d files.\n", removed)
	return err
}

// smartPlain is Smart Clean without a terminal: it prints the summary and
// cleans only with --yes.
func smartPlain(w io.Writer, opts tui.SmartOptions) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	items, err := opts.Scan(ctx, nil)
	if err != nil && ctx.Err() == nil {
		return err
	}
	safe := tui.SmartSelection(items)
	var safeBytes, reviewBytes int64
	for _, it := range safe {
		safeBytes += it.Size
	}
	taken := map[string]bool{}
	for _, it := range safe {
		taken[it.Path] = true
	}
	for _, it := range items {
		if !taken[it.Path] && it.Selectable() && it.ScanStatus == "complete" {
			reviewBytes += it.Size
		}
	}
	noun := "items"
	if len(safe) == 1 {
		noun = "item"
	}
	fmt.Fprintf(w, "Safe to clean: %s in %d %s\n", tui.Bytes(safeBytes), len(safe), noun)
	if reviewBytes > 0 {
		fmt.Fprintf(w, "Needs review:  %s (not included; see handbyte clean --review)\n", tui.Bytes(reviewBytes))
	}
	if !opts.Yes || len(safe) == 0 {
		if len(safe) > 0 {
			fmt.Fprintln(w, "Nothing was changed. Run handbyte clean --yes to clean the safe items.")
		}
		return nil
	}
	results := opts.Execute(ctx, safe, nil)
	var removed, moved, previewed int64
	failed := 0
	for _, r := range results {
		removed += r.Freed
		moved += r.Moved
		previewed += r.Previewed
		switch r.Status {
		case "removed", "trashed", "previewed":
		default:
			failed++
			fmt.Fprintf(w, "skipped %s: %s\n", r.Path, r.Message)
		}
	}
	if previewed > 0 {
		fmt.Fprintf(w, "Preview: %s would be cleaned. Nothing was changed.\n", tui.Bytes(previewed))
		return nil
	}
	fmt.Fprintf(w, "Removed %s, moved %s to the Trash, %d skipped. Details: handbyte history\n", tui.Bytes(removed), tui.Bytes(moved), failed)
	return nil
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
	return tui.RunAnalyze(path, analyzeOptions())
}

// analyzeOptions connects the drill-down to the catalog, the settings and
// the result cache.
func analyzeOptions() tui.AnalyzeOptions {
	saved, _ := settings.Load()
	home, _ := os.UserHomeDir()
	if physical, err := filepath.EvalSymlinks(home); err == nil {
		home = physical
	}
	roots := saved.Roots
	if len(roots) == 0 {
		roots = scanner.DefaultProjectRoots
	}
	opts := tui.AnalyzeOptions{
		Hints:      scanner.NewHints(home, saved.Roots),
		Categories: catalog.LoadStorage().Paths(home, roots),
		Mouse:      !saved.NoMouse,
	}
	if dir, err := settings.Dir(); err == nil {
		opts.SummaryFile = filepath.Join(dir, "usage.json")
	}
	return opts
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

func appsOptions(home string) apps.Options {
	o := apps.DefaultOptions(home)
	return o
}

func physicalHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(home); err == nil {
		home = p
	}
	return home, nil
}

func cmdApps(args []string, open string) error {
	fs := flag.NewFlagSet("apps", flag.ContinueOnError)
	unused := fs.Bool("unused", false, "")
	leftovers := fs.Bool("leftovers", false, "")
	asJSON := fs.Bool("json", false, "")
	withData := fs.Bool("with-data", false, "")
	if positional, err := parse(fs, args); err != nil {
		return err
	} else if len(positional) > 0 {
		return usageError{"apps takes no argument; to remove one app: handbyte uninstall <app>"}
	}
	home, err := physicalHome()
	if err != nil {
		return err
	}
	opts := appsOptions(home)
	if *asJSON || !interactive() {
		return printApps(os.Stdout, opts, *unused, *leftovers)
	}
	fast := opts
	fast.SkipSizes = true
	saved, _ := settings.Load()
	return tui.RunApps(tui.AppsOptions{
		Mouse:     !saved.NoMouse,
		Home:      home,
		Open:      open,
		WithData:  *withData,
		Inventory: func(ctx context.Context) ([]apps.App, error) { return apps.Inventory(ctx, fast) },
		Sizes:     apps.MeasureSizes,
		Known:     apps.KnownBundleIDs,
		Related: func(a apps.App, others []string, withData bool) []apps.Related {
			files := apps.FindRelated(home, apps.Target{BundleID: a.BundleID, Name: a.Name, Zap: a.Zap}, others, withData)
			apps.MeasureRelated(context.Background(), files)
			return files
		},
		Leftovers: func(known, names []string) []apps.LeftoverGroup {
			items := apps.FindLeftovers(home, known, names)
			for i := range items {
				r := []apps.Related{{Path: items[i].Path, IsDir: items[i].IsDir}}
				apps.MeasureRelated(context.Background(), r)
				items[i].Size = r[0].Size
			}
			return apps.GroupLeftovers(items)
		},
		Uninstall: func(ctx context.Context, plan cleaner.AppPlan) (string, []cleaner.Result, error) {
			return cleaner.Uninstall(ctx, plan, cleaner.AppOptions{Roots: opts.Roots, Running: bundleRunning})
		},
	})
}

// bundleRunning reports whether a process of the app bundle runs now.
func bundleRunning(bundle string) bool {
	out, err := exec.Command("/bin/ps", "-axo", "comm=").Output()
	if err != nil {
		// A missing process list must never allow the move.
		return true
	}
	return strings.Contains(string(out), bundle+"/Contents/")
}

func printApps(w io.Writer, opts apps.Options, unused, leftovers bool) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	list, err := apps.Inventory(ctx, opts)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if leftovers {
		var names []string
		for _, a := range list {
			names = append(names, a.Name)
		}
		groups := apps.GroupLeftovers(apps.FindLeftovers(opts.Home, apps.KnownBundleIDs(list), names))
		if groups == nil {
			groups = []apps.LeftoverGroup{}
		}
		return enc.Encode(map[string]any{"schema_version": 1, "leftovers": groups})
	}
	out := []apps.App{}
	now := time.Now()
	for _, a := range list {
		if !unused || a.Unused(now) {
			out = append(out, a)
		}
	}
	return enc.Encode(map[string]any{"schema_version": 1, "apps": out})
}

func cmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	withData := fs.Bool("with-data", false, "")
	reset := fs.Bool("reset", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	yes := fs.Bool("yes", false, "")
	positional, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return usageError{"uninstall needs one app name or bundle ID; see: handbyte apps"}
	}
	name := positional[0]
	if interactive() && !*yes && !*dryRun && !*reset {
		flagArgs := []string{}
		if *withData {
			flagArgs = append(flagArgs, "--with-data")
		}
		return cmdApps(flagArgs, name)
	}
	home, err := physicalHome()
	if err != nil {
		return err
	}
	opts := appsOptions(home)
	opts.SkipSizes = true
	list, err := apps.Inventory(context.Background(), opts)
	if err != nil {
		return err
	}
	var app *apps.App
	var others []string
	for i := range list {
		if strings.EqualFold(list[i].Name, name) || strings.EqualFold(list[i].BundleID, name) {
			app = &list[i]
		} else {
			others = append(others, list[i].BundleID)
		}
	}
	if app == nil {
		return fmt.Errorf("no app named %q in %s", name, strings.Join(opts.Roots, " or "))
	}
	files := apps.FindRelated(home, apps.Target{BundleID: app.BundleID, Name: app.Name, Zap: app.Zap}, others, *withData)
	apps.MeasureRelated(context.Background(), files)
	plan := cleaner.AppPlan{Name: app.Name, BundleID: app.BundleID, System: app.System, Cask: app.CaskToken}
	if !*reset {
		id, err := scanner.EntryIdentity(app.Path)
		if err != nil {
			return err
		}
		plan.Bundle, plan.BundleIdentity = app.Path, id
		fmt.Printf("app       %s\n", app.Path)
	}
	for _, f := range files {
		mark := "keep"
		if f.Selected {
			mark = "move"
			c, err := cleaner.NewAppFile(f.Path, f.Size)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Path, err)
			}
			plan.Files = append(plan.Files, c)
		}
		fmt.Printf("%-9s %-8s %-6s %10s  %s\n", f.Class, f.Confidence, mark, tui.Bytes(f.Size), f.Path)
	}
	if !*yes && !*dryRun {
		fmt.Println("Nothing was changed. Run with --yes to move the items marked move to the Trash.")
		return nil
	}
	group, results, err := cleaner.Uninstall(context.Background(), plan, cleaner.AppOptions{Roots: opts.Roots, Running: bundleRunning, DryRun: *dryRun})
	moved := 0
	for _, r := range results {
		if r.Status == "trashed" || r.Status == "previewed" {
			moved++
		} else {
			fmt.Printf("skipped %s: %s\n", r.Path, r.Message)
		}
	}
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Printf("Preview: %d items would move to the Trash.\n", moved)
		return nil
	}
	fmt.Printf("Moved %d items to the Trash. Put them back: handbyte restore %s\n", moved, group)
	return nil
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
		fmt.Fprintf(w, "%s  %-10s %10s  %s\n    id %s", when.Local().Format("2006-01-02 15:04"), r.Status, tui.Bytes(r.Moved+r.Freed+r.Previewed), path, r.ID)
		if r.Group != "" {
			fmt.Fprintf(w, " · group %s", r.Group)
		}
		if r.Tool == "app" {
			fmt.Fprint(w, " · by the app")
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\nPut an item back from the Trash: handbyte restore <id>, or <group> for all items of one uninstall")
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
	case args[0] == "mouse" && len(args) == 2 && (args[1] == "on" || args[1] == "off"):
		saved.NoMouse = args[1] == "off"
		if err := settings.Save(saved); err != nil {
			return err
		}
		fmt.Println("Mouse support " + args[1])
		return nil
	}
	return usageError{"usage: handbyte config [roots <paths> | protect <path> | unprotect <path> | mouse on|off]"}
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
	mouse := "on"
	if saved.NoMouse {
		mouse = "off"
	}
	fmt.Fprintln(w, "\nMouse support\n  "+mouse)
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
		if dir, err := settings.Dir(); err == nil {
			if home, err := os.UserHomeDir(); err == nil {
				if physical, err := filepath.EvalSymlinks(home); err == nil {
					home = physical
				}
				if s, ok := usage.LoadSummary(filepath.Join(dir, "usage.json"), home); ok {
					info.Home = &s
				}
			}
		}
		saved, _ := settings.Load()
		info.FirstRun = !saved.IntroShown
		action, err := tui.RunHome(info)
		if info.FirstRun {
			saved.IntroShown = true
			_ = settings.Save(saved)
		}
		if err != nil {
			return err
		}
		switch action {
		case tui.ActionClean:
			err = cmdClean(nil)
		case tui.ActionApps:
			err = cmdApps(nil, "")
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
