package ftp

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeURI_FTPMirrors(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	item, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "ftp://mirror1.example/file.bin",
		URIs:    []string{"ftp://mirror2.example/file.bin"},
		SaveDir: t.TempDir(),
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}

	del, add, err := driver.ChangeURI(context.Background(), item.ID, 1,
		[]string{"ftp://mirror2.example/file.bin"},
		[]string{"ftp://mirror3.example/file.bin", "baduri"},
		-1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if del != 1 || add != 1 {
		t.Fatalf("counts del=%d add=%d", del, add)
	}

	status, err := driver.TellStatus(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Files) != 1 || len(status.Files[0].URIs) != 2 {
		t.Fatalf("uris: %#v", status.Files[0].URIs)
	}
	if status.Files[0].URIs[0] != "ftp://mirror1.example/file.bin" {
		t.Fatalf("primary uri: %q", status.Files[0].URIs[0])
	}
}

func TestAdd_IndexOut(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	item, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "ftp://mirror.example/original.bin",
		SaveDir: t.TempDir(),
		Options: map[string]string{"index-out": "renamed.bin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "renamed.bin" {
		t.Fatalf("name: %q", item.Name)
	}
	if item.Files[0].Path == "" || !endsWith(item.Files[0].Path, "renamed.bin") {
		t.Fatalf("path: %q", item.Files[0].Path)
	}
}

func endsWith(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
