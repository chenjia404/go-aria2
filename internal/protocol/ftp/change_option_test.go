package ftp

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeOptionPauseNo_Unpauses(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "ftp://user:pass@127.0.0.1:21/file.bin",
		SaveDir: t.TempDir(),
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := driver.ChangeOption(context.Background(), created.ID, map[string]string{"pause": "no"}); err != nil {
		t.Fatalf("ChangeOption: %v", err)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TellStatus: %v", err)
	}
	if status.Status == task.StatusPaused {
		t.Fatalf("expected unpause after pause=no, still paused")
	}
}
