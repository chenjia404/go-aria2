package sftpproto

import (
	"context"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestDriverRespectsMaxDownloadLimit_SFTP(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 24*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	srv := newTestSFTPServer(t, payload)
	defer srv.close()

	saveDir := t.TempDir()
	driver := New()
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     srv.uri(),
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"max-download-limit": "8192"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	start := time.Now()
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitSFTPComplete(t, driver, created.ID, len(payload))
	elapsed := time.Since(start)

	if elapsed < 2*time.Second {
		t.Fatalf("download finished too quickly (%v), rate limit likely not applied", elapsed)
	}
}

func TestDriverChangeOptionUpdatesDownloadLimit_SFTP(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 16*1024)
	srv := newTestSFTPServer(t, payload)
	defer srv.close()

	saveDir := t.TempDir()
	driver := New()
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     srv.uri(),
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.ChangeOption(context.Background(), created.ID, map[string]string{
		"max-download-limit": "4096",
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
