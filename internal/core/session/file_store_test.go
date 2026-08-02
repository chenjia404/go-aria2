package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestFileStore_LoadAria2TextSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	aria2Text := "https://example.com/sample.bin\n gid=0123456789abcdef\n dir=/tmp\n"
	if err := os.WriteFile(path, []byte(aria2Text), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	store := NewFileStore(path)
	tasks, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Meta["aria2.import"] != "true" {
		t.Fatal("expected aria2 import meta")
	}
}

func TestFileStore_SaveDualWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "session.json")
	exportPath := filepath.Join(dir, "session")
	store := NewFileStore(jsonPath)
	store.SetAria2ExportPath(exportPath)

	item := &task.Task{
		GID:      "0123456789abcdef",
		Protocol: task.ProtocolHTTP,
		Status:   task.StatusWaiting,
		SaveDir:  "/tmp",
		Name:     "sample.bin",
		Meta: map[string]string{
			"http.sourceURL": "https://example.com/sample.bin",
		},
		Options: map[string]string{"dir": "/tmp", "out": "sample.bin"},
		Files: []task.File{{
			URIs: []string{"https://example.com/sample.bin"},
		}},
	}

	if err := store.Save(context.Background(), []*task.Task{item}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("json session missing: %v", err)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("aria2 export missing: %v", err)
	}
}
