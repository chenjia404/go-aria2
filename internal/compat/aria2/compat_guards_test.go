package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestErrNoActiveDownload(t *testing.T) {
	t.Parallel()

	err := errNoActiveDownload("abc123")
	if err == nil || err.Error() != "No active download for GID#abc123" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskCanBePaused(t *testing.T) {
	t.Parallel()

	if !taskCanBePaused(&task.Task{Status: task.StatusWaiting}) {
		t.Fatal("waiting should be pausable")
	}
	if !taskCanBePaused(&task.Task{Status: task.StatusActive}) {
		t.Fatal("active should be pausable")
	}
	if taskCanBePaused(&task.Task{Status: task.StatusPaused}) {
		t.Fatal("paused should not be pausable")
	}
}
