// Package system measures and cleans the files of the system rules: old
// crash reports and rotated logs that belong to root. Measuring needs no
// rights; cleaning runs one fixed find command for each rule as root.
package system

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wanggang316/handbyte/catalog"
)

// Measure is the result of one rule.
type Measure struct {
	Rule  catalog.SystemRule
	Files int
	Bytes int64
	// Partial is true when a folder could not be read.
	Partial bool
}

// matches reports whether a file name matches a rule pattern.
func matches(name string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(name, strings.TrimPrefix(p, "*")) {
			return true
		}
	}
	return false
}

// MeasureRule counts the regular files the rule would remove. root replaces
// the rule root (or the measure folders of a command rule), for tests;
// empty uses the rule.
func MeasureRule(rule catalog.SystemRule, root string, now time.Time) Measure {
	if len(rule.Command) > 0 {
		folders := rule.Measure
		if root != "" {
			folders = []string{root}
		}
		m := Measure{Rule: rule}
		for _, f := range folders {
			part := walk(f, func(fs.DirEntry, os.FileInfo) bool { return true })
			m.Files, m.Bytes, m.Partial = m.Files+part.Files, m.Bytes+part.Bytes, m.Partial || part.Partial
		}
		return m
	}
	if root == "" {
		root = rule.Root
	}
	// find -mtime +N takes files older than N whole days, so at least
	// N+1 days.
	limit := now.Add(-time.Duration(rule.MinAgeDays+1) * 24 * time.Hour)
	m := walk(root, func(d fs.DirEntry, info os.FileInfo) bool {
		return matches(d.Name(), rule.Names) && info.ModTime().Before(limit)
	})
	m.Rule = rule
	return m
}

// walk counts the regular files below root that take passes, on the volume
// of root.
func walk(root string, take func(fs.DirEntry, os.FileInfo) bool) Measure {
	var m Measure
	var device uint64
	if info, err := os.Lstat(root); err == nil {
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			device = uint64(st.Dev)
		}
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			m.Partial = true
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			m.Partial = true
			return nil
		}
		st, _ := info.Sys().(*syscall.Stat_t)
		if d.IsDir() {
			// As find -xdev: stay on the volume of the root.
			if st != nil && uint64(st.Dev) != device {
				return fs.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() || !take(d, info) {
			return nil
		}
		m.Files++
		if st != nil {
			m.Bytes += st.Blocks * 512
		} else {
			m.Bytes += info.Size()
		}
		return nil
	})
	return m
}

// Command is the fixed command of a rule. For a find rule it removes
// regular files that match a pattern and are older than the minimum age, on
// the volume of the root; find does not follow symbolic links.
func Command(rule catalog.SystemRule) []string {
	if len(rule.Command) > 0 {
		return rule.Command
	}
	args := []string{"/usr/bin/find", rule.Root, "-xdev", "-type", "f", "("}
	for i, n := range rule.Names {
		if i > 0 {
			args = append(args, "-o")
		}
		args = append(args, "-name", n)
	}
	return append(args, ")", "-mtime", "+"+strconv.Itoa(rule.MinAgeDays), "-print", "-delete")
}

// ShellCommand is Command as one line for a shell, with every argument
// quoted; the app passes it to the administrator dialog.
func ShellCommand(rule catalog.SystemRule) string {
	var parts []string
	for _, a := range Command(rule) {
		parts = append(parts, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return strings.Join(parts, " ")
}

// Clean runs the command of each rule as root, through sudo when this
// process is not root, and returns the number of files removed. sudo asks
// for the password once for the run. noPrompt makes sudo fail instead of
// asking, for a run without a terminal.
func Clean(ctx context.Context, rules []catalog.SystemRule, noPrompt bool) (int, error) {
	removed := 0
	for _, r := range rules {
		args := Command(r)
		var cmd *exec.Cmd
		if os.Geteuid() == 0 {
			cmd = exec.CommandContext(ctx, args[0], args[1:]...)
		} else {
			sudo := []string{"--"}
			if noPrompt {
				sudo = []string{"-n", "--"}
			}
			cmd = exec.CommandContext(ctx, "/usr/bin/sudo", append(sudo, args...)...)
			cmd.Stdin = os.Stdin
		}
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if len(r.Command) == 0 {
			removed += strings.Count(string(out), "\n")
		}
		if err != nil {
			return removed, errors.New(r.Title + ": " + err.Error())
		}
	}
	return removed, nil
}
