package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type usageCase struct {
	Name  string `json:"name"`
	Files []struct {
		Path  string `json:"path"`
		Bytes int    `json:"bytes"`
	} `json:"files"`
	HardLinks []struct{ Path, Target string } `json:"hardLinks"`
	Clones    []struct{ Path, Target string } `json:"clones"`
	Symlinks  []struct{ Path, Target string } `json:"symlinks"`
	Expect    struct {
		Files        int64 `json:"files"`
		LogicalBytes int64 `json:"logicalBytes"`
		Children     []struct {
			Name         string `json:"name"`
			Dir          bool   `json:"dir"`
			Files        int64  `json:"files"`
			LogicalBytes int64  `json:"logicalBytes"`
		} `json:"children"`
		LargestFiles []string `json:"largestFiles"`
	} `json:"expect"`
}

func write(t *testing.T, root, rel string, size int) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
}

// clone makes an APFS clone. It skips the test when the volume cannot.
func clone(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("cp", "-c", src, dst).CombinedOutput()
	if err != nil && strings.Contains(string(out), "not supported") {
		t.Skipf("cannot clone on this volume: %s", out)
	}
	if err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
}

// The cases are shared with the macOS app. See testdata/conformance/README.md.
func TestUsageConformance(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/conformance/usage/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no conformance cases: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var c usageCase
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			root := t.TempDir()
			for _, f := range c.Files {
				write(t, root, f.Path, f.Bytes)
			}
			for _, l := range c.HardLinks {
				if err := os.Link(filepath.Join(root, l.Target), filepath.Join(root, l.Path)); err != nil {
					t.Fatal(err)
				}
			}
			for _, l := range c.Clones {
				clone(t, filepath.Join(root, l.Target), filepath.Join(root, l.Path))
			}
			for _, l := range c.Symlinks {
				if err := os.Symlink(l.Target, filepath.Join(root, l.Path)); err != nil {
					t.Fatal(err)
				}
			}
			r, err := Scan(context.Background(), root, Options{Workers: 3})
			if err != nil {
				t.Fatal(err)
			}
			if r.Root.Files != c.Expect.Files || r.Root.Logical != c.Expect.LogicalBytes || r.Root.Incomplete || r.Cancelled {
				t.Fatalf("%s: root %+v", c.Name, *r.Root)
			}
			var got, want []string
			for _, n := range r.Root.Children {
				got = append(got, fmt.Sprintf("%s dir=%t files=%d logical=%d", n.Name, n.Dir, n.Files, n.Logical))
			}
			for _, n := range c.Expect.Children {
				want = append(want, fmt.Sprintf("%s dir=%t files=%d logical=%d", n.Name, n.Dir, n.Files, n.LogicalBytes))
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("children\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			var largest []string
			for i := 0; i < len(c.Expect.LargestFiles) && i < len(r.Largest); i++ {
				rel, _ := filepath.Rel(r.Path, r.Largest[i].Path)
				largest = append(largest, rel)
			}
			if strings.Join(largest, ",") != strings.Join(c.Expect.LargestFiles, ",") {
				t.Fatalf("largest files: got %v, want %v", largest, c.Expect.LargestFiles)
			}
		})
	}
}

func TestUnreadableDirectoryMarksTheParentChain(t *testing.T) {
	root := t.TempDir()
	write(t, root, "ok/a", 100)
	write(t, root, "outer/locked/secret", 100)
	locked := filepath.Join(root, "outer/locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o700)
	r, err := Scan(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	outer := find(t, r.Root, "outer")
	if !r.Root.Incomplete || !outer.Incomplete || !find(t, outer, "locked").Incomplete || find(t, outer, "locked").Err == "" {
		t.Fatalf("the unreadable directory must mark its parents: root %+v outer %+v", *r.Root, *outer)
	}
	if find(t, r.Root, "ok").Incomplete || r.Root.Files != 1 {
		t.Fatalf("a readable sibling must stay complete: %+v", *r.Root)
	}
}

func TestSmallFilesAreSummedAndTheListOfLargestIsBounded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 30; i++ {
		write(t, root, fmt.Sprintf("many/f%02d", i), 1000+i*5000)
	}
	r, err := Scan(context.Background(), root, Options{FilesPerDir: 4, LargestFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	many := r.Root.Children[0]
	if len(many.Children) != 4 || many.OtherFiles != 26 || many.Files != 30 {
		t.Fatalf("kept %d, other %d, files %d", len(many.Children), many.OtherFiles, many.Files)
	}
	var kept int64
	for _, c := range many.Children {
		kept += c.Allocated
	}
	if kept+many.OtherAllocated != many.Allocated {
		t.Fatalf("kept %d + other %d != %d", kept, many.OtherAllocated, many.Allocated)
	}
	if len(r.Largest) != 3 || filepath.Base(r.Largest[0].Path) != "f29" || filepath.Base(r.Largest[2].Path) != "f27" {
		t.Fatalf("largest: %+v", r.Largest)
	}
}

func TestCancelledScanKeepsThePartialResult(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		write(t, root, fmt.Sprintf("d%02d/sub/file", i), 100)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Scan(ctx, root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Cancelled || r.Root == nil || !r.Root.Incomplete {
		t.Fatalf("a cancelled scan must return an incomplete tree: %+v", r)
	}
}

func TestProgressReportsEachTopLevelEntry(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/x", 5000)
	write(t, root, "b/y", 9000)
	write(t, root, "c.bin", 100)
	var last Progress
	r, err := Scan(context.Background(), root, Options{Progress: func(p Progress) { last = p }})
	if err != nil {
		t.Fatal(err)
	}
	if last.Files != 3 || last.Allocated != r.Root.Allocated || len(last.Top) != 3 {
		t.Fatalf("final progress: %+v", last)
	}
	var sum int64
	for _, top := range last.Top {
		if !top.Done {
			t.Errorf("%s is not done", top.Name)
		}
		sum += top.Allocated
	}
	if sum != r.Root.Allocated {
		t.Fatalf("top totals %d != root %d", sum, r.Root.Allocated)
	}
}

func TestScanRejectsAFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "file", 10)
	if _, err := Scan(context.Background(), filepath.Join(root, "file"), Options{}); err == nil {
		t.Fatal("a file is not a tree")
	}
}

func find(t *testing.T, n *Node, name string) *Node {
	t.Helper()
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s is missing", name)
	return nil
}

func TestFullClonesCountOnce(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/original", 100_000)
	clone(t, filepath.Join(root, "a/original"), filepath.Join(root, "a/copy"))
	clone(t, filepath.Join(root, "a/original"), filepath.Join(root, "b/copy"))
	single, err := Scan(context.Background(), filepath.Join(root, "a"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Scan(context.Background(), root, Options{Workers: 3})
	if err != nil {
		t.Fatal(err)
	}
	size := find(t, single.Root, "original").Allocated + find(t, single.Root, "copy").Allocated
	if size == 0 || r.Root.Allocated != size || r.Root.Files != 3 || r.Root.Logical != 300_000 || r.Root.MayShare {
		t.Fatalf("three full clones must count once: allocated %d, want %d; %+v", r.Root.Allocated, size, *r.Root)
	}
	// In one directory, the name that sorts first carries the size.
	if find(t, single.Root, "copy").Allocated != size || find(t, single.Root, "original").Allocated != 0 {
		t.Fatalf("copy must carry the size: %+v", single.Root.Children)
	}
}

func TestPartialClonesCountInFullAndMayShare(t *testing.T) {
	root := t.TempDir()
	write(t, root, "original", 100_000)
	clone(t, filepath.Join(root, "original"), filepath.Join(root, "dir/changed"))
	f, err := os.OpenFile(filepath.Join(root, "dir/changed"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("changed"), 0); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Scan(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	dir, original := find(t, r.Root, "dir"), find(t, r.Root, "original")
	changed := find(t, dir, "changed")
	if original.Allocated == 0 || changed.Allocated == 0 || r.Root.Allocated != original.Allocated+changed.Allocated {
		t.Fatalf("partial clones must count in full: %+v", r.Root.Children)
	}
	if !changed.MayShare || !original.MayShare || !dir.MayShare || !r.Root.MayShare {
		t.Fatalf("partial clones and their parents must have the mark: %+v", r.Root.Children)
	}
	for _, l := range r.Largest {
		if !l.MayShare {
			t.Fatalf("largest file without the mark: %+v", l)
		}
	}
}

func TestPlainFilesHaveNoMark(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/x", 5000)
	r, err := Scan(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Root.MayShare || find(t, r.Root, "a").MayShare {
		t.Fatalf("a plain file must not have the mark: %+v", *r.Root)
	}
}

func TestCloneStateWithoutReferenceCount(t *testing.T) {
	data := make([]byte, 16)
	data[8] = efMayShareBlocks | efSharesAllBlocks
	if c := parseClone(attrCmnExtCloneID|attrCmnExtExtFlags, data); !c.full || c.partial {
		t.Fatalf("a file that shares all blocks is a full clone: %+v", c)
	}
	data[8] = efMayShareBlocks
	if c := parseClone(attrCmnExtCloneID|attrCmnExtExtFlags, data); c.full || !c.partial {
		t.Fatalf("a file that may share blocks is a partial clone: %+v", c)
	}
	if c := parseClone(0, data); c.full || c.partial {
		t.Fatalf("no attributes, no clone: %+v", c)
	}
}
