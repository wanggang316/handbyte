package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// A path template is an absolute path in which a segment "*" stands for
// each directory at that place, for example each browser profile.

// expandTemplate returns the directories that a template names, in name
// order. A wildcard does not match a name that starts with a dot or a
// symbolic link, so a rule never reaches outside its tree.
func expandTemplate(template string) ([]string, error) {
	if !strings.Contains(template, "*") {
		return []string{template}, nil
	}
	segments := strings.Split(template, string(filepath.Separator))
	paths := []string{string(filepath.Separator)}
	for _, segment := range segments[1:] {
		var next []string
		for _, base := range paths {
			if segment != "*" {
				next = append(next, filepath.Join(base, segment))
				continue
			}
			entries, err := os.ReadDir(base)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					next = append(next, filepath.Join(base, e.Name()))
				}
			}
		}
		paths = next
	}
	return paths, nil
}

// matchTemplate reports whether path is one of the directories that a
// template names.
func matchTemplate(template, path string) bool {
	t := strings.Split(filepath.Clean(template), string(filepath.Separator))
	p := strings.Split(filepath.Clean(path), string(filepath.Separator))
	if len(t) != len(p) {
		return false
	}
	for i := range t {
		if t[i] == "*" {
			if strings.HasPrefix(p[i], ".") || p[i] == "" {
				return false
			}
			continue
		}
		if t[i] != p[i] {
			return false
		}
	}
	return true
}

// templateDepth is the number of segments, for "the more specific rule
// wins" when two rules reach one path.
func templateDepth(template string) int { return strings.Count(template, "/") }
