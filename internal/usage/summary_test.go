package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSummariesAreKeptPerFolderNewestFirst(t *testing.T) {
	file := filepath.Join(t.TempDir(), "usage.json")
	if _, ok := LoadSummary(file, "/a"); ok {
		t.Fatal("no file means no summary")
	}
	base := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	for i := 0; i < summaryLimit+2; i++ {
		if err := SaveSummary(file, Summary{Path: "/f" + string(rune('a'+i)), ScannedAt: base.Add(time.Duration(i) * time.Minute), Allocated: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := LoadSummary(file, "/fa"); ok {
		t.Fatal("the oldest summary must be dropped")
	}
	if s, ok := LoadSummary(file, "/fj"); !ok || s.Allocated != 9 {
		t.Fatalf("newest summary: %+v %v", s, ok)
	}
	if err := SaveSummary(file, Summary{Path: "/fj", ScannedAt: base.Add(time.Hour), Allocated: 99}); err != nil {
		t.Fatal(err)
	}
	if s, _ := LoadSummary(file, "/fj"); s.Allocated != 99 {
		t.Fatalf("a new scan must replace the old summary: %+v", s)
	}
}

func TestADamagedSummaryFileIsIgnoredAndReplaced(t *testing.T) {
	file := filepath.Join(t.TempDir(), "usage.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadSummary(file, "/a"); ok {
		t.Fatal("a damaged file has no summary")
	}
	if err := SaveSummary(file, Summary{Path: "/a"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadSummary(file, "/a"); !ok {
		t.Fatal("the file must be replaced")
	}
}
