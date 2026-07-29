package manager

import (
	"context"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangePositionMovesWaitingTask(t *testing.T) {
	t.Parallel()

	driver := newStubDriver()
	mgr := New(Options{DefaultDir: "./downloads", MaxConcurrent: 1})
	mgr.RegisterDriver(driver)

	base := time.Now().Add(-time.Minute)
	first := &task.Task{
		ID: "task-1", GID: "gid-1", Protocol: task.ProtocolHTTP,
		Status: task.StatusWaiting, SaveDir: "./downloads", CreatedAt: base,
	}
	second := &task.Task{
		ID: "task-2", GID: "gid-2", Protocol: task.ProtocolHTTP,
		Status: task.StatusWaiting, SaveDir: "./downloads", CreatedAt: base.Add(time.Second),
	}
	third := &task.Task{
		ID: "task-3", GID: "gid-3", Protocol: task.ProtocolHTTP,
		Status: task.StatusWaiting, SaveDir: "./downloads", CreatedAt: base.Add(2 * time.Second),
	}
	for _, item := range []*task.Task{first, second, third} {
		driver.tasks[item.ID] = item.Clone()
		mgr.mu.Lock()
		mgr.tasks[item.ID] = item.Clone()
		mgr.driverByTaskID[item.ID] = driver
		mgr.mu.Unlock()
	}

	newPos, err := mgr.ChangePosition(context.Background(), third.GID, 0, "POS_SET")
	if err != nil {
		t.Fatalf("ChangePosition: %v", err)
	}
	if newPos != 0 {
		t.Fatalf("expected new position 0, got %d", newPos)
	}
	if next := mgr.nextWaitingTaskID(); next != third.ID {
		t.Fatalf("expected %s to be first in queue, got %s", third.ID, next)
	}
}

func TestResolveQueueIndexModes(t *testing.T) {
	t.Parallel()

	if got, err := resolveQueueIndex(2, 5, 0, "POS_SET"); err != nil || got != 0 {
		t.Fatalf("POS_SET: got %d err %v", got, err)
	}
	if got, err := resolveQueueIndex(1, 5, -1, "POS_CUR"); err != nil || got != 0 {
		t.Fatalf("POS_CUR: got %d err %v", got, err)
	}
	if got, err := resolveQueueIndex(1, 5, 0, "POS_END"); err != nil || got != 4 {
		t.Fatalf("POS_END: got %d err %v", got, err)
	}
}
