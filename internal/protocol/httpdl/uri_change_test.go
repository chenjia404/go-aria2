package httpdl

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeURIInsertAndDelete(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	item, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "http://a.example/file.bin",
		URIs:    []string{"http://b.example/file.bin"},
		SaveDir: t.TempDir(),
		Name:    "file.bin",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := driver.ChangeURI(context.Background(), item.ID, 1, []string{"http://a.example/file.bin"}, []string{"http://c.example/file.bin"}, 0); err != nil {
		t.Fatalf("ChangeURI: %v", err)
	}
	files, err := driver.GetFiles(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	want := []string{"http://c.example/file.bin", "http://b.example/file.bin"}
	if len(files) != 1 || len(files[0].URIs) != len(want) {
		t.Fatalf("unexpected files: %#v", files)
	}
	for i, uri := range want {
		if files[0].URIs[i] != uri {
			t.Fatalf("unexpected uris: %#v", files[0].URIs)
		}
	}
}
