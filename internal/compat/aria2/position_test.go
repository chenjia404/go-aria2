package aria2

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

func TestAddUriHonorsQueuePosition(t *testing.T) {
	t.Parallel()

	driver := newRPCStubDriver()
	saveDir := t.TempDir()
	mgr := manager.New(manager.Options{DefaultDir: saveDir, MaxConcurrent: 1})
	mgr.RegisterDriver(driver)
	service := NewService(mgr, "")

	if _, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/first.bin"},
		map[string]any{"dir": saveDir, "pause": "true"},
	}); err != nil {
		t.Fatalf("first addUri: %v", err)
	}

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/second.bin"},
		map[string]any{"dir": saveDir, "pause": "true"},
		0,
	})
	if err != nil {
		t.Fatalf("second addUri: %v", err)
	}
	secondGID, _ := rawGID.(string)

	waiting, err := service.Invoke(context.Background(), "aria2.tellWaiting", []any{0, 10})
	if err != nil {
		t.Fatalf("tellWaiting: %v", err)
	}
	items, ok := waiting.([]map[string]any)
	if !ok || len(items) < 2 {
		t.Fatalf("expected at least 2 waiting tasks, got %#v", waiting)
	}
	if items[0]["gid"] != secondGID {
		t.Fatalf("expected second task at position 0, got %#v", items)
	}
}
