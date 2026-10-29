package cleaner

import (
	"context"
	"testing"
)

func TestPreviewDoesNotClaimFreedBytes(t *testing.T) {
	results := Clean(context.Background(), []string{"/fixture/cache", "/fixture/cache"}, map[string]int64{"/fixture/cache": 1024}, ModeDryRun, nil)
	if len(results) != 1 {
		t.Fatalf("duplicate actions: %d", len(results))
	}
	if results[0].Freed != 0 {
		t.Fatalf("preview claimed %d bytes freed", results[0].Freed)
	}
}
