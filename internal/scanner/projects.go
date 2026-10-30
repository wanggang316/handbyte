package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

type projectTarget struct{ path, project, scope, owner string }

// Each artifact needs a matching ecosystem marker within its scan root.
func artifactMarkers(name string) map[string][]string {
	switch name {
	case "node_modules", ".next", ".nuxt", ".turbo", ".parcel-cache":
		return map[string][]string{"Node.js": {"package.json"}}
	case "target":
		return map[string][]string{"Rust": {"Cargo.toml"}, "Maven": {"pom.xml"}}
	case ".build":
		return map[string][]string{"Swift": {"Package.swift"}}
	case "Pods":
		return map[string][]string{"CocoaPods": {"Podfile"}}
	case ".gradle":
		return map[string][]string{"Gradle": {"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}}
	case ".dart_tool":
		return map[string][]string{"Dart": {"pubspec.yaml"}}
	case "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", "venv", ".venv":
		return map[string][]string{"Python": {"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}}
	case "DerivedData":
		return map[string][]string{"Xcode": {"Package.swift", "project.yml"}}
	case "build", "dist":
		return map[string][]string{"Node.js": {"package.json"}, "Python": {"pyproject.toml", "setup.py"}, "CMake": {"CMakeLists.txt"}, "Gradle": {"build.gradle", "build.gradle.kts"}, "Make": {"Makefile"}}
	}
	return nil
}

func regularMarker(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func projectEvidence(path, scope string) (projectTarget, bool) {
	found := projectTarget{path: path, scope: scope}
	markers := artifactMarkers(filepath.Base(path))
	if len(markers) == 0 {
		return found, false
	}
	dir := filepath.Dir(path)
	for Contains(scope, dir) {
		// Require one unambiguous owner at the nearest matching project boundary.
		var owners []string
		for owner, names := range markers {
			for _, name := range names {
				if regularMarker(filepath.Join(dir, name)) {
					owners = append(owners, owner)
					break
				}
			}
		}
		if len(owners) == 1 {
			found.project = dir
			found.owner = owners[0]
			return found, true
		}
		if len(owners) > 1 {
			return found, false
		}
		if filepath.Base(path) == "DerivedData" {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return found, false
			}
			for _, e := range entries {
				if e.IsDir() && strings.HasSuffix(e.Name(), ".xcodeproj") {
					found.project = dir
					found.owner = "Xcode"
					return found, true
				}
			}
		}
		if git, err := lstatGit(dir); err != nil || git {
			return found, false
		}
		if dir == scope {
			break
		}
		dir = filepath.Dir(dir)
	}
	return found, false
}

func artifactConsequence(owner, name string) string {
	switch name {
	case "node_modules":
		return "Dependencies must be downloaded again with the project's package manager. Offline builds may fail."
	case "Pods":
		return "Run pod install to download dependencies again. Offline builds may fail."
	case "venv", ".venv":
		return "Recreate the environment and reinstall dependencies. Locally installed packages may not be reproducible."
	}
	return owner + " generated output. The next build or run must recreate it."
}

func ownerLabel(source string) string {
	switch source {
	case "xcode-derived", "xcode-devicesupport", "xcode-archives", "coresim-cache":
		return "Xcode"
	case "swiftpm-cache":
		return "Swift Package Manager"
	case "go-build", "go-mod-cache":
		return "Go"
	case "npm-cache":
		return "npm"
	case "yarn-cache":
		return "Yarn"
	case "cargo-registry", "cargo-git", "rustup":
		return "Rust"
	case "gradle-cache":
		return "Gradle"
	case "brew-cache":
		return "Homebrew"
	case "jetbrains":
		return "JetBrains"
	case "playwright":
		return "Playwright"
	case "cypress":
		return "Cypress"
	case "codex":
		return "Codex"
	}
	return "Unknown owner"
}
