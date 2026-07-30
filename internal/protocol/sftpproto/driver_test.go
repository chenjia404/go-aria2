package sftpproto

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestAdd_IndexOut(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	item, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "sftp://user:pass@host.example/remote/original.bin",
		SaveDir: t.TempDir(),
		Options: map[string]string{"index-out": "custom.bin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "custom.bin" {
		t.Fatalf("name: %q", item.Name)
	}
	if item.Files[0].Path == "" || !endsWith(item.Files[0].Path, "custom.bin") {
		t.Fatalf("path: %q", item.Files[0].Path)
	}
}

func endsWith(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
