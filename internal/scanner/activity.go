package scanner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Owners are conservative process families. An unrelated build may block a cache;
// a missing process snapshot must never authorize its removal.
func ownerProcesses(it Item) []string {
	if strings.HasPrefix(it.Source, "artifact:") {
		return []string{"node", "npm", "pnpm", "yarn", "bun", "cargo", "rustc", "go", "python", "python3", "java", "gradle", "swift", "swiftc", "xcodebuild", "Xcode", "make", "cmake", "ninja", "clang", "clang++", "gcc"}
	}
	switch it.Source {
	case "go-build", "go-mod-cache":
		return []string{"go", "gopls"}
	case "npm-cache", "yarn-cache", "playwright", "cypress":
		return []string{"node", "npm", "pnpm", "yarn", "bun", "chrome", "Chromium", "Cypress"}
	case "cargo-registry", "cargo-git":
		return []string{"cargo", "rustc", "rust-analyzer"}
	case "xcode-derived", "xcode-devicesupport", "coresim-cache", "swiftpm-cache":
		return []string{"Xcode", "xcodebuild", "swift", "swiftc", "SourceKitService", "Simulator", "CoreSimulatorService"}
	case "gradle-cache":
		return []string{"java", "gradle"}
	case "brew-cache":
		return []string{"brew", "ruby"}
	case "jetbrains":
		return []string{"idea", "pycharm", "goland", "clion", "rider", "webstorm", "java"}
	case "codex":
		return []string{"codex", "Codex"}
	}
	return nil
}

func activityFromProcesses(it Item, lines string) (Activity, error) {
	names := ownerProcesses(it)
	if len(names) == 0 {
		return Unknown, errors.New("no known owner")
	}
	for _, line := range strings.Split(lines, "\n") {
		executable := filepath.Base(strings.TrimSpace(line))
		for _, name := range names {
			if strings.EqualFold(executable, name) {
				return Active, nil
			}
		}
	}
	return Idle, nil
}

func processProbe(ctx context.Context) ActivityProbe {
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(bounded, "/bin/ps", "-axo", "comm=").Output()
	return func(_ context.Context, it Item) (Activity, error) {
		if err != nil {
			return Unknown, err
		}
		return activityFromProcesses(it, string(out))
	}
}

func ProbeActivity(ctx context.Context, it Item) (Activity, error) { return processProbe(ctx)(ctx, it) }

func checkTracked(ctx context.Context, path string) error {
	// Find the enclosing repository without invoking Git outside a repository.
	root := filepath.Dir(path)
	for {
		if info, err := lstatGit(root); err != nil {
			return err
		} else if info {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil
		}
		root = parent
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(bounded, "git", "-c", "core.fsmonitor=false", "-C", root, "ls-files", "-z", "--", ":(top,literal,icase)"+rel)
	cmd.Env = gitEnvironment()
	out, err := cmd.Output()
	if err != nil {
		return errors.New("Git ownership could not be checked")
	}
	if len(out) > 0 {
		return errors.New("directory contains Git-tracked files")
	}
	return nil
}
