// Package usage measures what a directory tree holds. It is the engine of
// the storage views: it reads sizes, and it never changes a file.
package usage

import (
	"container/heap"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Node is a directory or a file of the scanned tree. The sizes and the file
// count of a directory include all that is below it.
type Node struct {
	Name string
	Dir  bool
	// Allocated is the space on disk. Logical is the length of the content.
	Allocated int64
	Logical   int64
	Files     int64
	// Children holds the subdirectories and the largest files of a
	// directory, largest first. Smaller files are in OtherFiles.
	Children []*Node
	// OtherFiles and OtherAllocated describe the files of this directory
	// that are not in Children.
	OtherFiles     int64
	OtherAllocated int64
	// Incomplete is true when this directory, or one below it, could not be
	// read in full. The sizes are then a lower bound.
	Incomplete bool
	// Skipped says why a directory was not entered: another volume, or
	// content that is not on this Mac.
	Skipped string
	Err     string
}

// File is one entry of the list of largest files.
type File struct {
	Path      string
	Allocated int64
	Logical   int64
}

// Progress is a snapshot of a running scan.
type Progress struct {
	Files     int64
	Allocated int64
	Current   string
	// Top has one entry for each direct child of the root, in name order.
	Top []TopProgress
}

// TopProgress is the running total of one direct child of the root.
type TopProgress struct {
	Name      string
	Dir       bool
	Allocated int64
	Done      bool
}

// Options controls a scan. The zero value is valid.
type Options struct {
	// Workers is the number of directories read at one time. Default: the
	// CPU count, at most 12.
	Workers int
	// LargestFiles is the length of the list of largest files. Default 100.
	LargestFiles int
	// FilesPerDir is the number of files kept as nodes in each directory.
	// Default 50.
	FilesPerDir int
	// Progress receives a snapshot about ten times each second.
	Progress func(Progress)
}

// Result is the outcome of a scan. After a cancellation it holds what was
// measured until then, and Cancelled is true.
type Result struct {
	Path      string
	Root      *Node
	Largest   []File
	Cancelled bool
	Elapsed   time.Duration
}

const sfDataless = 0x40000000

type identity struct{ dev, ino uint64 }

type scan struct {
	ctx      context.Context
	opts     Options
	devices  map[uint64]bool
	skipPath string
	slots    chan struct{}

	files     atomic.Int64
	allocated atomic.Int64
	current   atomic.Value

	mu      sync.Mutex
	links   map[identity]bool
	largest fileHeap
}

// Scan measures the tree below path. An unreadable directory does not stop
// the scan; it marks the directory and its parents as incomplete.
func Scan(ctx context.Context, path string, opts Options) (*Result, error) {
	started := time.Now()
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("not a directory: " + root)
	}
	if opts.Workers <= 0 {
		opts.Workers = min(runtime.NumCPU(), 12)
	}
	if opts.LargestFiles <= 0 {
		opts.LargestFiles = 100
	}
	if opts.FilesPerDir <= 0 {
		opts.FilesPerDir = 50
	}
	s := &scan{ctx: ctx, opts: opts, devices: map[uint64]bool{}, slots: make(chan struct{}, opts.Workers), links: map[identity]bool{}}
	s.current.Store(root)
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		s.devices[uint64(st.Dev)] = true
	}
	if root == "/" {
		// The startup disk is two volumes that firmlinks join. Count the data
		// volume through the firmlinks, and do not enter it a second time.
		const data = "/System/Volumes/Data"
		if di, err := os.Lstat(data); err == nil {
			if st, ok := di.Sys().(*syscall.Stat_t); ok {
				s.devices[uint64(st.Dev)] = true
			}
		}
		s.skipPath = "/System/Volumes"
	}

	node := &Node{Name: root, Dir: true}
	tops := s.readTop(root)

	stop := make(chan struct{})
	var reporter sync.WaitGroup
	if opts.Progress != nil {
		reporter.Add(1)
		go func() {
			defer reporter.Done()
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					opts.Progress(s.snapshot(tops))
				}
			}
		}()
	}

	s.slots <- struct{}{}
	s.walkTop(root, node, tops)
	<-s.slots
	close(stop)
	reporter.Wait()
	if opts.Progress != nil {
		opts.Progress(s.snapshot(tops))
	}

	largest := make([]File, len(s.largest))
	copy(largest, s.largest)
	sort.Slice(largest, func(i, j int) bool {
		if largest[i].Allocated != largest[j].Allocated {
			return largest[i].Allocated > largest[j].Allocated
		}
		if largest[i].Logical != largest[j].Logical {
			return largest[i].Logical > largest[j].Logical
		}
		return largest[i].Path < largest[j].Path
	})
	return &Result{Path: root, Root: node, Largest: largest, Cancelled: ctx.Err() != nil, Elapsed: time.Since(started)}, nil
}

// top is the running state of one direct child of the root.
type top struct {
	name      string
	dir       bool
	allocated atomic.Int64
	done      atomic.Bool
}

type entry struct {
	name string
	info os.FileInfo
	st   *syscall.Stat_t
}

// read lists a directory in name order. It returns the entries it could
// read and the first error.
func read(dir string) ([]entry, error) {
	list, err := os.ReadDir(dir)
	entries := make([]entry, 0, len(list))
	for _, d := range list {
		info, ierr := d.Info()
		if ierr != nil {
			if err == nil && !os.IsNotExist(ierr) {
				err = ierr
			}
			continue
		}
		st, _ := info.Sys().(*syscall.Stat_t)
		entries = append(entries, entry{d.Name(), info, st})
	}
	return entries, err
}

func (s *scan) readTop(root string) []*top {
	entries, _ := read(root)
	tops := make([]*top, len(entries))
	for i, e := range entries {
		tops[i] = &top{name: e.name, dir: e.info.IsDir()}
	}
	return tops
}

func (s *scan) snapshot(tops []*top) Progress {
	p := Progress{Files: s.files.Load(), Allocated: s.allocated.Load(), Top: make([]TopProgress, len(tops))}
	p.Current, _ = s.current.Load().(string)
	for i, t := range tops {
		p.Top[i] = TopProgress{Name: t.name, Dir: t.dir, Allocated: t.allocated.Load(), Done: t.done.Load()}
	}
	return p
}

// walkTop walks the root, with one running total for each direct child.
func (s *scan) walkTop(root string, node *Node, tops []*top) {
	byName := make(map[string]*top, len(tops))
	for _, t := range tops {
		byName[t.name] = t
	}
	s.walkInto(root, node, func(name string) *top {
		if t := byName[name]; t != nil {
			return t
		}
		// The entry appeared after the first listing.
		return &top{name: name}
	}, nil)
	for _, t := range tops {
		t.done.Store(true)
	}
}

// walkInto fills node from the directory at path. bucket is not nil only
// for the root, where each child gets its own running total; below the root
// all sizes go to the total that the caller passes down.
func (s *scan) walkInto(path string, node *Node, bucket func(string) *top, total *top) {
	if s.ctx.Err() != nil {
		node.Incomplete = true
		return
	}
	s.current.Store(path)
	entries, err := read(path)
	if err != nil {
		node.Incomplete = true
		node.Err = err.Error()
	}

	var wg sync.WaitGroup
	var files []*Node
	for _, e := range entries {
		t := total
		if bucket != nil {
			t = bucket(e.name)
		}
		child := &Node{Name: e.name, Dir: e.info.IsDir()}
		full := filepath.Join(path, e.name)
		if !child.Dir {
			s.measureFile(full, e, child, t)
			if bucket != nil && t != nil {
				t.done.Store(true)
			}
			if child.Files > 0 {
				files = append(files, child)
			}
			continue
		}
		node.Children = append(node.Children, child)
		switch {
		case full == s.skipPath:
			child.Skipped = "other volumes"
		case e.st != nil && !s.devices[uint64(e.st.Dev)]:
			child.Skipped = "another volume"
		case e.st != nil && e.st.Flags&sfDataless != 0:
			child.Skipped = "not on this Mac"
		}
		if child.Skipped != "" {
			if bucket != nil && t != nil {
				t.done.Store(true)
			}
			continue
		}
		finish := func() {
			s.walkInto(full, child, nil, t)
			if bucket != nil && t != nil {
				t.done.Store(true)
			}
		}
		select {
		case s.slots <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-s.slots }()
				finish()
			}()
		default:
			// The pool is full: walk in place, so that the depth of the
			// tree cannot block the scan.
			finish()
		}
	}
	wg.Wait()

	for _, c := range node.Children {
		node.Allocated += c.Allocated
		node.Logical += c.Logical
		node.Files += c.Files
		node.Incomplete = node.Incomplete || c.Incomplete
	}
	sort.SliceStable(files, func(i, j int) bool { return larger(files[i], files[j]) })
	for i, f := range files {
		node.Allocated += f.Allocated
		node.Logical += f.Logical
		node.Files++
		if i < s.opts.FilesPerDir {
			node.Children = append(node.Children, f)
		} else {
			node.OtherFiles++
			node.OtherAllocated += f.Allocated
		}
	}
	sort.SliceStable(node.Children, func(i, j int) bool { return larger(node.Children[i], node.Children[j]) })
}

func larger(a, b *Node) bool {
	if a.Allocated != b.Allocated {
		return a.Allocated > b.Allocated
	}
	if a.Logical != b.Logical {
		return a.Logical > b.Logical
	}
	return a.Name < b.Name
}

// measureFile counts one entry that is not a directory. A file with more
// than one name counts once for the scan.
func (s *scan) measureFile(full string, e entry, node *Node, total *top) {
	if e.st != nil && e.st.Nlink > 1 && e.info.Mode().IsRegular() {
		id := identity{uint64(e.st.Dev), uint64(e.st.Ino)}
		s.mu.Lock()
		seen := s.links[id]
		s.links[id] = true
		s.mu.Unlock()
		if seen {
			return
		}
	}
	node.Files = 1
	node.Logical = e.info.Size()
	if e.st != nil {
		node.Allocated = int64(e.st.Blocks) * 512
	}
	s.files.Add(1)
	s.allocated.Add(node.Allocated)
	if total != nil {
		total.allocated.Add(node.Allocated)
	}
	if !e.info.Mode().IsRegular() {
		return
	}
	s.mu.Lock()
	if len(s.largest) < s.opts.LargestFiles {
		heap.Push(&s.largest, File{full, node.Allocated, node.Logical})
	} else if smallest := s.largest[0]; node.Allocated > smallest.Allocated || (node.Allocated == smallest.Allocated && node.Logical > smallest.Logical) {
		s.largest[0] = File{full, node.Allocated, node.Logical}
		heap.Fix(&s.largest, 0)
	}
	s.mu.Unlock()
}

// fileHeap is a min-heap, so that the smallest of the kept files is on top.
type fileHeap []File

func (h fileHeap) Len() int { return len(h) }
func (h fileHeap) Less(i, j int) bool {
	if h[i].Allocated != h[j].Allocated {
		return h[i].Allocated < h[j].Allocated
	}
	return h[i].Logical < h[j].Logical
}
func (h fileHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *fileHeap) Push(x any)   { *h = append(*h, x.(File)) }
func (h *fileHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
