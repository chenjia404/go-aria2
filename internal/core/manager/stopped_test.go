package manager

import (
	"context"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestPaginateStoppedSupportsNegativeOffset(t *testing.T) {
	t.Parallel()

	driver := newStubDriver()
	mgr := New(Options{DefaultDir: "./downloads"})
	mgr.RegisterDriver(driver)

	base := time.Now().Add(-time.Minute)
	for i, gid := range []string{"gid-1", "gid-2", "gid-3"} {
		item := &task.Task{
			ID:        "task-" + gid,
			GID:       gid,
			Protocol:  task.ProtocolHTTP,
			Status:    task.StatusComplete,
			SaveDir:   "./downloads",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
			UpdatedAt: base.Add(time.Duration(i) * time.Second),
		}
		driver.tasks[item.ID] = item.Clone()
		mgr.mu.Lock()
		mgr.tasks[item.ID] = item.Clone()
		mgr.driverByTaskID[item.ID] = driver
		mgr.mu.Unlock()
	}

	stopped, err := mgr.TellStopped(context.Background(), -1, 1)
	if err != nil {
		t.Fatalf("TellStopped: %v", err)
	}
	if len(stopped) != 1 || stopped[0].GID != "gid-3" {
		t.Fatalf("expected most recent stopped task, got %#v", stopped)
	}
}
