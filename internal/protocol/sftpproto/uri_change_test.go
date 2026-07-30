package sftpproto

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeURI_SFTPMirrors(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	item, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "sftp://user:pass@mirror1.example/file.bin",
		URIs:    []string{"sftp://user:pass@mirror2.example/file.bin"},
		SaveDir: t.TempDir(),
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}

	del, add, err := driver.ChangeURI(context.Background(), item.ID, 1,
		[]string{"sftp://user:pass@mirror2.example/file.bin"},
		[]string{"sftp://user:pass@mirror3.example/file.bin", "baduri"},
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
}
