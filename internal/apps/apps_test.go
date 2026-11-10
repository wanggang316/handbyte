package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type appCase struct {
	Name string `json:"name"`
	App  *struct {
		BundleID string   `json:"bundleID"`
		Name     string   `json:"name"`
		Zap      []string `json:"zap"`
	} `json:"app"`
	Others    []string          `json:"others"`
	WithData  bool              `json:"withData"`
	Installed []string          `json:"installed"`
	Names     []string          `json:"installedNames"`
	Files     []string          `json:"files"`
	Expect    []json.RawMessage `json:"expect"`
}

func makeHome(t *testing.T, files []string) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		full := filepath.Join(home, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// The cases are shared with the macOS app. See testdata/conformance/README.md.
func TestAppConformance(t *testing.T) {
	paths, _ := filepath.Glob("../../testdata/conformance/apps/*.json")
	if len(paths) == 0 {
		t.Fatal("no conformance cases")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var c appCase
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			home := makeHome(t, c.Files)
			var got []string
			if c.App != nil {
				for _, r := range FindRelated(home, Target{BundleID: c.App.BundleID, Name: c.App.Name, Zap: c.App.Zap}, c.Others, c.WithData) {
					rel, _ := filepath.Rel(home, r.Path)
					got = append(got, mustJSON(map[string]any{"path": rel, "class": r.Class, "confidence": r.Confidence, "selected": r.Selected}))
				}
			} else {
				for _, l := range FindLeftovers(home, c.Installed, c.Names) {
					rel, _ := filepath.Rel(home, l.Path)
					got = append(got, mustJSON(map[string]any{"path": rel, "bundleID": l.BundleID, "class": l.Class}))
				}
			}
			var want []string
			for _, e := range c.Expect {
				var m map[string]any
				if err := json.Unmarshal(e, &m); err != nil {
					t.Fatal(err)
				}
				want = append(want, mustJSON(m))
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("%s\ngot:\n%s\nwant:\n%s", c.Name, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestInventoryReadsBundlesSourcesAndRunningState(t *testing.T) {
	root := t.TempDir()
	caskroom := t.TempDir()
	mk := func(app, id string, extra ...string) {
		dir := filepath.Join(root, app, "Contents")
		if err := os.MkdirAll(filepath.Join(dir, "MacOS"), 0o700); err != nil {
			t.Fatal(err)
		}
		plist := `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + id + `</string><key>CFBundleShortVersionString</key><string>1.0</string><key>CFBundleExecutable</key><string>run</string></dict></plist>`
		if err := os.WriteFile(filepath.Join(dir, "Info.plist"), []byte(plist), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, e := range extra {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, e)), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, e), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("Store.app", "com.example.store", "_MASReceipt/receipt")
	mk("Brewed.app", "com.example.brewed")
	mk("Plain.app", "com.example.plain")
	mk("Safari.app", "com.apple.Safari")
	if err := os.Symlink(filepath.Join(root, "Plain.app"), filepath.Join(root, "Link.app")); err != nil {
		t.Fatal(err)
	}
	receipt := `{"uninstall_artifacts":[{"app":["Brewed.app"]},{"zap":[{"trash":["~/Library/Caches/brewed"]}]}]}`
	if err := os.MkdirAll(filepath.Join(caskroom, "brewed", ".metadata"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caskroom, "brewed", ".metadata", "INSTALL_RECEIPT.json"), []byte(receipt), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Roots:     []string{root},
		Caskrooms: []string{caskroom},
		Processes: func() []string {
			return []string{filepath.Join(root, "Plain.app/Contents/MacOS/run"), "/usr/bin/other"}
		},
	}
	apps, err := Inventory(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]App{}
	for _, a := range apps {
		byName[a.Name] = a
	}
	if len(apps) != 4 {
		t.Fatalf("apps: %+v", apps)
	}
	if byName["Store"].Source != SourceAppStore || byName["Brewed"].Source != SourceHomebrew || byName["Brewed"].CaskToken != "brewed" || byName["Plain"].Source != SourceDirect {
		t.Errorf("sources: %+v", byName)
	}
	if len(byName["Brewed"].Zap) != 1 || !byName["Plain"].Running || byName["Store"].Running || !byName["Safari"].System {
		t.Errorf("facts: %+v", byName)
	}
	if byName["Plain"].Size == 0 || byName["Plain"].LastUsedKnown {
		t.Errorf("size and last use: %+v", byName["Plain"])
	}
}
