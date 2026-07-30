package httpdl

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeOptionUpdatesTaskDownloadLimiter(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "http://example.com/file.bin",
		SaveDir: t.TempDir(),
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := driver.ChangeOption(context.Background(), created.ID, map[string]string{
		"max-download-limit": "8192",
	}); err != nil {
		t.Fatalf("ChangeOption: %v", err)
	}

	driver.mu.Lock()
	limiter := driver.tasks[created.ID].limiter
	driver.mu.Unlock()
	if limiter == nil {
		t.Fatal("expected limiter after changeOption")
	}
}
