package manager

import (
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestInsertTaskAtQueuePosition(t *testing.T) {
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

	if err := mgr.insertTaskAtQueuePosition("gid-3", 0); err != nil {
		t.Fatalf("insertTaskAtQueuePosition: %v", err)
	}
	if mgr.nextWaitingTaskID() != third.ID {
		t.Fatalf("expected %s at queue head, got %s", third.ID, mgr.nextWaitingTaskID())
	}
}
