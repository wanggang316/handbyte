package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type ActivityProbe func(context.Context, Item) (Activity, error)

// Contains uses path boundaries, not string prefixes.
func Contains(parent, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func Physical(path string) (string, error) {
	expanded, ok := expandHome(path)
	if !ok {
		return "", errors.New("home directory unavailable")
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func identity(path string) (Identity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Identity{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Identity{}, errors.New("target is not a real directory")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Identity{}, errors.New("file identity unavailable")
	}
	return Identity{Device: uint64(st.Dev), Inode: uint64(st.Ino)}, nil
}

// Excluded also protects an ancestor whose deletion would include a protected child.
func Excluded(path string, exclusions []string) bool {
	for _, pattern := range exclusions {
		expanded, ok := expandHome(pattern)
		if !ok {
			return true
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return true
		}
		if Contains(abs, path) || Contains(path, abs) {
			return true
		}
		if physical, err := Physical(abs); err == nil && (Contains(physical, path) || Contains(path, physical)) {
			return true
		}
	}
	return false
}

func ruleAllows(it Item) bool {
	if strings.HasPrefix(it.Source, "artifact:") {
		return it.Project != "" && it.Scope != "" && Contains(it.Scope, it.Path) && it.Path != it.Scope && Contains(it.Project, it.Path) && it.Path != it.Project
	}
	for _, p := range Presets {
		if p.Source != it.Source || p.Safety == SafetyCaution {
			continue
		}
		switch p.Source {
		case "library-caches", "library-logs", "xdg-cache", "trash", "rustup", "pnpm-store", "maven-cache":
			continue
		}
		root, err := Physical(p.Path)
		if err != nil {
			continue
		}
		if (!p.ExpandChildren && it.Path == root) || (p.ExpandChildren && filepath.Dir(it.Path) == root) {
			return true
		}
	}
	return false
}

func checkBoundary(ctx context.Context, it Item, exclusions []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	home, err = Physical(home)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(it.Path) || it.Path == "/" || it.Path == home {
		return errors.New("structural directory is protected")
	}
	physical, err := Physical(it.Path)
	if err != nil {
		return err
	}
	if physical != it.Path {
		return errors.New("target path now crosses a symbolic link")
	}
	if it.ScanStatus != "complete" {
		return errors.New("scan is incomplete")
	}
	if Excluded(it.Path, exclusions) {
		return errors.New("protected by an exclusion")
	}
	if !ruleAllows(it) {
		return errors.New("inspection only: no supported cleanup rule")
	}
	id, err := identity(it.Path)
	if err != nil {
		return err
	}
	if it.Identity.Inode == 0 || id != it.Identity {
		return errors.New("target identity changed; rescan required")
	}
	return nil
}

// Validate rechecks ownership, contents and activity at the action boundary.
// Path checks reduce races but do not make recursive filesystem operations atomic.
func Validate(ctx context.Context, it Item, exclusions []string, probe ActivityProbe) error {
	if err := checkBoundary(ctx, it, exclusions); err != nil {
		return err
	}
	if !it.Selectable() {
		return fmt.Errorf("target is protected: %s", it.Reason)
	}
	if err := protectContents(ctx, it.Path); err != nil {
		return err
	}
	if err := checkTracked(ctx, it.Path); err != nil {
		return err
	}
	_, _, latest, err := du(ctx, it.Path)
	if err != nil {
		return fmt.Errorf("cannot recheck contents: %w", err)
	}
	if latest.After(it.ModTime) {
		return errors.New("contents changed since review; rescan required")
	}
	if probe == nil {
		probe = ProbeActivity
	}
	state, err := probe(ctx, it)
	if err != nil || state != Idle {
		return fmt.Errorf("owner is active or cannot be checked (%s)", state)
	}
	return checkBoundary(ctx, it, exclusions)
}

func protectContents(ctx context.Context, path string) error {
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		name := d.Name()
		if name == ".git" || name == ".svn" || name == ".hg" || strings.HasSuffix(name, ".xcarchive") || strings.HasSuffix(name, ".dSYM") || name == "keypair.json" || name == "credentials.json" || name == ".env" || strings.HasSuffix(name, ".pem") {
			return fmt.Errorf("protected content: %s", name)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link requires inspection: %s", name)
		}
		return nil
	})
}

func assess(ctx context.Context, it *Item, exclusions []string, probe ActivityProbe) {
	it.Name = filepath.Base(it.Path)
	it.ScannedAt = time.Now()
	it.Eligibility = Protected
	if err := checkBoundary(ctx, *it, exclusions); err != nil {
		it.Reason = err.Error()
		return
	}
	if err := protectContents(ctx, it.Path); err != nil {
		it.Reason = err.Error()
		return
	}
	if err := checkTracked(ctx, it.Path); err != nil {
		it.Reason = err.Error()
		return
	}
	if probe == nil {
		probe = ProbeActivity
	}
	state, err := probe(ctx, *it)
	it.Activity = state
	if err != nil || state != Idle {
		it.Reason = "Owner is active or cannot be checked"
		return
	}
	it.Eligibility = Review
	it.Reason = "Review the rebuild or download cost"
	if !it.ModTime.IsZero() && time.Since(it.ModTime) > 7*24*time.Hour {
		it.Eligibility = Recommended
		it.Reason = "No owner process detected; file activity is older than 7 days"
	}
}
