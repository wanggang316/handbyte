package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectArtifactsRequireEcosystemEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "dev")
	for _, path := range []string{"loose/target", "web/node_modules", "web/packages/widget/.next", "swift/.build", "rust/target"} {
		target := filepath.Join(root, path)
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "data"), []byte("output"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"web/package.json", "web/packages/widget/package.json", "swift/Package.swift", "rust/Cargo.toml"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	items, err := Scan(context.Background(), Config{ProjectRoots: []string{root}, Workers: 2, ActivityProbe: idleProbe}, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, it := range items {
		if it.Category != CatProjectBuild {
			continue
		}
		if filepath.Base(filepath.Dir(it.Path)) == "loose" {
			t.Error("unproven target classified as Rust")
		}
		if it.Project == "" || it.Owner == "" || !it.Selectable() {
			t.Errorf("missing project evidence: %+v", it)
		}
		count++
	}
	if count != 4 {
		t.Fatalf("wanted four artifacts, got %d", count)
	}
}

func TestXcodeTargetsAreSplitAndArchivesStayProtected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths := []string{"Library/Developer/Xcode/DerivedData/App-one", "Library/Developer/Xcode/DerivedData/App-two", "Library/Developer/Xcode/iOS DeviceSupport/18.0", "Library/Developer/Xcode/Archives/2026-10-01/App.xcarchive"}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(home, p), 0700); err != nil {
			t.Fatal(err)
		}
	}
	items, err := Scan(context.Background(), Config{Workers: 2, SkipProjectScan: true, ActivityProbe: idleProbe}, nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := 0
	device := 0
	for _, it := range items {
		switch it.Source {
		case "xcode-derived":
			derived++
			if !it.Selectable() {
				t.Error(it.Reason)
			}
		case "xcode-devicesupport":
			device++
			if filepath.Base(it.Path) != "18.0" {
				t.Error("whole DeviceSupport root offered")
			}
		case "xcode-archives":
			if it.Selectable() {
				t.Error("Archives became selectable")
			}
		}
	}
	if derived != 2 || device != 1 {
		t.Fatalf("expected 2 projects and 1 device version, got %d and %d", derived, device)
	}
}

func TestArtifactNamesDoNotHideProjectRoots(t *testing.T) {
	for _, name := range []string{"build", "target", "dist"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			project := filepath.Join(home, name)
			target := filepath.Join(project, ".next")
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(project, "package.json"), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, root := range []string{project, home} {
				items, err := Scan(context.Background(), Config{Scope: "projects", ProjectRoots: []string{root}, Workers: 1, ActivityProbe: idleProbe}, nil)
				if err != nil || len(items) != 1 || filepath.Base(items[0].Path) != ".next" {
					t.Errorf("root %s hides project: %+v, %v", root, items, err)
				}
			}
		})
	}
}
