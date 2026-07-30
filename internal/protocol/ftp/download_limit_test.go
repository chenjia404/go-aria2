package ftp

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestSetDownloadLimitUpdatesLimiter(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	driver.SetDownloadLimit(1024)
	if driver.limiter == nil {
		t.Fatal("expected limiter after SetDownloadLimit")
	}
	driver.SetDownloadLimit(0)
	if driver.limiter != nil {
		t.Fatalf("expected nil limiter after clearing limit")
	}
}

func TestTaskUsesSharedOverallLimiter(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	driver.SetDownloadLimit(2048)

	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "ftp://user:pass@127.0.0.1:21/file.bin",
		SaveDir: t.TempDir(),
		Options: map[string]string{"max-overall-download-limit": "2048"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	driver.mu.Lock()
	st := driver.tasks[created.ID]
	driver.mu.Unlock()
	if st == nil {
		t.Fatal("missing task state")
	}
	if st.limiter != driver.limiter {
		t.Fatalf("expected shared overall limiter, got %p driver=%p", st.limiter, driver.limiter)
	}
}
