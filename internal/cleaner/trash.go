package cleaner

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Finder uses native Trash semantics and returns the actual destination.
// Paths are argv values, never interpolated into script source.
const trashScript = `on run argv
 tell application "Finder"
  set targetItem to POSIX file (item 1 of argv) as alias
  set trashedItem to delete targetItem
  return POSIX path of (trashedItem as alias)
 end tell
end run`

const restoreScript = `ObjC.import('Foundation');
function run(argv) {
 if (!$.NSFileManager.defaultManager.moveItemAtPathToPathError(argv[0], argv[1], null))
  throw Error('Restore refused. Check the source, destination and permissions.');
 return JSON.stringify(argv[1]);
}`

func runNativeScript(language, script string, args ...string) ([]byte, error) {
	// Wait for the current action's outcome before responding to cancellation.
	// A timeout is uncertain and must never trigger permanent deletion fallback.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	argv := append([]string{"-l", language, "-e", script, "--"}, args...)
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", argv...).Output()
	if err != nil {
		detail := ""
		if exit, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		return nil, fmt.Errorf("native file operation: %w: %s", err, detail)
	}
	return out, nil
}

func nativeFileOperation(script string, args ...string) (string, error) {
	out, err := runNativeScript("JavaScript", script, args...)
	if err != nil {
		return "", err
	}
	var destination string
	if err := json.Unmarshal(out, &destination); err != nil {
		return "", fmt.Errorf("operation completed but destination could not be read: %w", err)
	}
	return destination, nil
}

func moveToTrash(path string) (string, error) {
	out, err := runNativeScript("AppleScript", trashScript, path)
	if err != nil {
		return "", err
	}
	destination := strings.TrimSuffix(string(out), "\n")
	if !filepath.IsAbs(destination) {
		return "", fmt.Errorf("operation completed but Trash destination was not returned")
	}
	return filepath.Clean(destination), nil
}
