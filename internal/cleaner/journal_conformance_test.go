package cleaner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The journal is shared with the app; testdata/conformance/journal holds
// records that both products write and must read the same way.
func TestJournalConformance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "conformance", "journal", "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Lines  []string
		Expect []struct {
			ID, Status, Group string
			CanRestore        bool
			Bytes             int64
		}
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	path, err := journalPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(c.Lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	history, err := History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != len(c.Expect) {
		t.Fatalf("got %d records, want %d", len(history), len(c.Expect))
	}
	for i, want := range c.Expect {
		got := history[i]
		if got.ID != want.ID || got.Status != want.Status || got.Group != want.Group || got.CanRestore() != want.CanRestore || got.Moved+got.Freed != want.Bytes {
			t.Errorf("record %d = %s %s group %q restore %v bytes %d, want %+v", i, got.ID, got.Status, got.Group, got.CanRestore(), got.Moved+got.Freed, want)
		}
	}
	// A record written now goes after the shared lines and names the CLI.
	if err := appendJournal(Result{ID: "K", Status: "removed", Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	history, err = History()
	if err != nil {
		t.Fatal(err)
	}
	if last := history[len(history)-1]; last.ID != "K" || last.Tool != "cli" {
		t.Errorf("last record = %+v", last)
	}
}
