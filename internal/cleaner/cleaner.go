package cleaner

import (
	"github.com/wanggang316/handbyte/internal/scanner"
	"time"
)

// Mode selects the deletion strategy.
type Mode int

const (
	// ModeTrash moves the target into ~/.Trash. Default and safest.
	ModeTrash Mode = iota
	// ModeRemove unlinks the target directly. Faster, unrecoverable.
	ModeRemove
	// ModeDryRun reports what would happen but changes nothing.
	ModeDryRun
)

// Result is the outcome of cleaning one path.
type Result struct {
	ID            string
	Destination   string
	Message       string
	JournalError  string
	TrashIdentity scanner.Identity
	FreeDelta     *int64
	Path          string
	Freed         int64 // Estimated permanently removed bytes, not a volume delta.
	Previewed     int64
	Moved         int64
	Status        string
	Err           error `json:"-"`
	StartedAt     time.Time
	EndedAt       time.Time
}

// Progress reports incremental status while cleaning a batch.
type Progress struct {
	Run        uint64
	Current    string
	Completed  int
	Total      int
	BytesFreed int64
	LastResult *Result
	Done       bool
}
