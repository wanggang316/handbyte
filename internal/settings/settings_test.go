package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTripAndInvalidFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := Config{Version: 1, Roots: []string{"~/work"}, Exclusions: []string{"~/work/keep"}}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Exclusions) != 1 || got.Exclusions[0] != want.Exclusions[0] {
		t.Fatalf("exclusion not saved: %+v", got)
	}
	dir, _ := os.UserConfigDir()
	if err := os.WriteFile(filepath.Join(dir, "handbyte", "config.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("invalid config silently ignored")
	}
}
