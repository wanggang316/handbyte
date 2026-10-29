package scanner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
)

// Explore is inspection only. Browsing a path does not make it a cleanup target.
func Explore(ctx context.Context, path string, progress chan<- Progress) ([]Item, error) {
	if progress != nil {
		defer close(progress)
	}
	root, err := Physical(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, Item{Name: entry.Name(), Path: filepath.Join(root, entry.Name()), IsDir: entry.IsDir(), Category: CatOther, Safety: SafetyCaution, Eligibility: Protected, Source: "explore", ScanStatus: "pending", Reason: "Inspection only. Reveal in Finder to manage personal files.", Description: "Browsing does not authorize deletion."})
	}
	emit := func(current string) {
		if progress == nil {
			return
		}
		snapshot := append([]Item(nil), items...)
		select {
		case progress <- Progress{CurrentPath: current, ItemsFound: len(items), Items: snapshot}:
		default:
		}
	}
	emit(root)
	for i := range items {
		if err := ctx.Err(); err != nil {
			return items, err
		}
		it := &items[i]
		size, count, mt, err := du(ctx, it.Path)
		it.Size, it.FileCount, it.ModTime = size, count, mt
		it.ScanStatus = "complete"
		if err != nil {
			it.ScanStatus = "partial"
			it.ScanError = err.Error()
		}
		emit(it.Path)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Size > items[j].Size })
	return items, ctx.Err()
}
