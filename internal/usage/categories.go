package usage

import (
	"path/filepath"
	"sort"

	"github.com/wanggang316/handbyte/catalog"
)

// CategoryTotal is the content of one storage category.
type CategoryTotal struct {
	ID        string
	Allocated int64
	Logical   int64
}

// Categorize sums a scan result by the path rules of the storage catalog.
// A folder belongs to the category of the longest rule that is the folder
// or one of its parents, and to "other" when no rule matches. When a rule
// lies below a folder, the folder is split by its children.
func Categorize(r *Result, rules []catalog.CategoryPath) []CategoryTotal {
	totals := map[string]*CategoryTotal{}
	add := func(id string, allocated, logical int64) {
		if allocated == 0 && logical == 0 {
			return
		}
		t := totals[id]
		if t == nil {
			t = &CategoryTotal{ID: id}
			totals[id] = t
		}
		t.Allocated += allocated
		t.Logical += logical
	}
	categoryOf := func(path string) string {
		best, id := -1, catalog.OtherCategory
		for _, rule := range rules {
			if (path == rule.Path || isBelow(path, rule.Path)) && len(rule.Path) > best {
				best, id = len(rule.Path), rule.Category
			}
		}
		return id
	}
	hasRuleBelow := func(path string) bool {
		for _, rule := range rules {
			if isBelow(rule.Path, path) {
				return true
			}
		}
		return false
	}
	var walk func(n *Node, path string)
	walk = func(n *Node, path string) {
		id := categoryOf(path)
		if !n.Dir || !hasRuleBelow(path) {
			add(id, n.Allocated, n.Logical)
			return
		}
		var allocated, logical int64
		for _, c := range n.Children {
			if c.Dir {
				walk(c, filepath.Join(path, c.Name))
				continue
			}
			allocated += c.Allocated
			logical += c.Logical
		}
		add(id, allocated+n.OtherAllocated, logical+n.otherLogical())
	}
	walk(r.Root, r.Path)

	out := make([]CategoryTotal, 0, len(totals))
	for _, t := range totals {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Allocated != out[j].Allocated {
			return out[i].Allocated > out[j].Allocated
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// otherLogical is the content of the files that are not in Children.
func (n *Node) otherLogical() int64 {
	logical := n.Logical
	for _, c := range n.Children {
		logical -= c.Logical
	}
	return logical
}

// isBelow reports whether path is strictly inside dir.
func isBelow(path, dir string) bool {
	if dir == "/" {
		return path != "/" && filepath.IsAbs(path)
	}
	return len(path) > len(dir) && path[len(dir)] == '/' && path[:len(dir)] == dir
}
