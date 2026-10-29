package ui

import (
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
	"testing"
)

func TestLateProgressDoesNotChangeFinalResult(t *testing.T) {
	m := New(scanner.Default(), cleaner.ModeDryRun, 0)
	r := cleaner.Result{Path: "/fixture/cache"}
	m.Update(cleanDoneMsg{results: []cleaner.Result{r}})
	m.Update(cleanProgressMsg{LastResult: &r})
	if len(m.cleanRes) != 1 {
		t.Fatalf("final count changed to %d", len(m.cleanRes))
	}
}
