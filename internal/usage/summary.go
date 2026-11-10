package usage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Summary is the short form of a scan that is kept between runs, so that a
// screen can show the last result at once and replace it when a new scan
// is ready.
type Summary struct {
	Path       string          `json:"path"`
	ScannedAt  time.Time       `json:"scanned_at"`
	Allocated  int64           `json:"allocated_bytes"`
	Logical    int64           `json:"logical_bytes"`
	Files      int64           `json:"file_count"`
	Incomplete bool            `json:"incomplete"`
	Categories []CategoryTotal `json:"categories"`
	// Top has one entry for each direct child of the scanned folder.
	Top []SummaryEntry `json:"top"`
}

// SummaryEntry is one direct child of the scanned folder.
type SummaryEntry struct {
	Name      string `json:"name"`
	Dir       bool   `json:"is_dir"`
	Allocated int64  `json:"allocated_bytes"`
}

// summaryLimit is the number of folders whose summaries are kept.
const summaryLimit = 8

// Summarize makes the summary of a complete scan.
func Summarize(r *Result, categories []CategoryTotal, at time.Time) Summary {
	s := Summary{Path: r.Path, ScannedAt: at, Allocated: r.Root.Allocated, Logical: r.Root.Logical, Files: r.Root.Files, Incomplete: r.Root.Incomplete, Categories: categories}
	for _, c := range r.Root.Children {
		s.Top = append(s.Top, SummaryEntry{c.Name, c.Dir, c.Allocated})
	}
	return s
}

type summaryFile struct {
	Version   int       `json:"version"`
	Summaries []Summary `json:"summaries"`
}

// SaveSummary stores s in the file at path. The file keeps the newest
// summaries of a few folders. A cancelled scan must not be stored: its
// numbers are too low.
func SaveSummary(path string, s Summary) error {
	file, _ := readSummaries(path)
	kept := []Summary{s}
	for _, other := range file.Summaries {
		if other.Path != s.Path {
			kept = append(kept, other)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].ScannedAt.After(kept[j].ScannedAt) })
	if len(kept) > summaryLimit {
		kept = kept[:summaryLimit]
	}
	data, err := json.MarshalIndent(summaryFile{Version: 1, Summaries: kept}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Write a new file and rename it, so that a reader never sees half a file.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".usage-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadSummary returns the stored summary of a folder.
func LoadSummary(path, folder string) (Summary, bool) {
	file, err := readSummaries(path)
	if err != nil {
		return Summary{}, false
	}
	for _, s := range file.Summaries {
		if s.Path == folder {
			return s, true
		}
	}
	return Summary{}, false
}

func readSummaries(path string) (summaryFile, error) {
	var file summaryFile
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return file, errors.New("summary file must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return file, err
	}
	if err := json.Unmarshal(data, &file); err != nil || file.Version != 1 {
		return summaryFile{}, errors.New("summary file is not readable")
	}
	return file, nil
}
