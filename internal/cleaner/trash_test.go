package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wanggang316/handbyte/internal/scanner"
)

func TestNativeTrashCollisionAndRestore(t *testing.T) {
	if os.Getenv("HANDBYTE_NATIVE_TEST") != "1" {
		t.Skip("requires native macOS Trash access")
	}
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	root, err := scanner.Physical(root)
	if err != nil {
		t.Fatal(err)
	}
	var destinations []string
	for _, parent := range []string{"first", "second"} {
		path := filepath.Join(root, parent, "handbyte-disposable-cache")
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "fixture"), []byte(parent), 0600); err != nil {
			t.Fatal(err)
		}
		destination, err := moveToTrash(path)
		if err != nil {
			t.Fatal(err)
		}
		destinations = append(destinations, destination)
		id, err := scanner.CaptureIdentity(destination)
		if err != nil {
			t.Fatal(err)
		}
		r := Result{ID: parent, Path: path, Destination: destination, Status: "trashed", TrashIdentity: id, EndedAt: time.Now()}
		if err := appendJournal(r); err != nil {
			t.Fatal(err)
		}
	}
	if destinations[0] == destinations[1] {
		t.Fatal("Trash names collided")
	}
	first := filepath.Join(root, "first", "handbyte-disposable-cache")
	if err := os.Mkdir(first, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), "first"); err == nil {
		t.Fatal("restore overwrote existing directory")
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if err := Restore(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, id, "handbyte-disposable-cache", "fixture"))
		if err != nil || string(data) != id {
			t.Fatalf("restored data mismatch: %s %v", data, err)
		}
	}
	history, err := History()
	if err != nil || len(history) != 2 {
		t.Fatalf("history: %+v %v", history, err)
	}
	for _, r := range history {
		if r.Status != "restored" {
			t.Fatal(r)
		}
	}
}
