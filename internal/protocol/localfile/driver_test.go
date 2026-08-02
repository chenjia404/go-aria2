package localfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestParseFileURI(t *testing.T) {
	t.Parallel()

	path, err := parseFileURI("file:///tmp/sample.bin")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if path != "/tmp/sample.bin" {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestDriver_CopyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(src, []byte("hello file"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	saveDir := filepath.Join(dir, "downloads")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	drv := New()
	item, err := drv.Add(context.Background(), task.AddTaskInput{
		URI:     "file://" + src,
		SaveDir: saveDir,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := drv.Start(context.Background(), item.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := drv.TellStatus(context.Background(), item.ID)
		if err != nil {
			t.Fatalf("tellStatus: %v", err)
		}
		switch status.Status {
		case task.StatusComplete:
			out := filepath.Join(saveDir, "source.txt")
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if string(data) != "hello file" {
				t.Fatalf("unexpected content: %q", data)
			}
			return
		case task.StatusError:
			t.Fatalf("task error: %s", status.ErrorMessage)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("copy did not complete in time")
}
