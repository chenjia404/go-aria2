package manager

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestForcePauseAllPausesActiveAndWaiting(t *testing.T) {
	t.Parallel()

	driver := newStubDriver()
	mgr := New(Options{DefaultDir: t.TempDir(), MaxConcurrent: 2})
	mgr.RegisterDriver(driver)

	activeGID := addStubTask(t, mgr, driver, "active", task.StatusActive)
	waitingGID := addStubTask(t, mgr, driver, "waiting", task.StatusWaiting)

	if err := mgr.ForcePauseAll(context.Background()); err != nil {
		t.Fatalf("ForcePauseAll: %v", err)
	}
	if driver.tasks["task-active"].Status != task.StatusPaused {
		t.Fatalf("expected active task paused, got %s", driver.tasks["task-active"].Status)
	}
	if driver.tasks["task-waiting"].Status != task.StatusPaused {
		t.Fatalf("expected waiting task paused, got %s", driver.tasks["task-waiting"].Status)
	}
	_ = activeGID
	_ = waitingGID
}

func TestLinkBatchDownloadsSetsRelatedGIDs(t *testing.T) {
	t.Parallel()

	mgr := New(Options{DefaultDir: t.TempDir()})
	gids := []string{"gid-leader", "gid-follow-1", "gid-follow-2"}
	for _, gid := range gids {
		id := "task-" + gid
		mgr.mu.Lock()
		mgr.tasks[id] = &task.Task{ID: id, GID: gid, Status: task.StatusWaiting}
		mgr.mu.Unlock()
	}
	mgr.LinkBatchDownloads(gids)

	leader := mgr.tasks["task-gid-leader"]
	if len(leader.FollowedByGIDs) != 2 || leader.FollowedByGIDs[0] != "gid-follow-1" {
		t.Fatalf("unexpected followedBy on leader: %#v", leader.FollowedByGIDs)
	}
	follower := mgr.tasks["task-gid-follow-1"]
	if follower.FollowingGID != "gid-leader" || follower.BelongsToGID != "gid-leader" {
		t.Fatalf("unexpected follower links: following=%q belongsTo=%q", follower.FollowingGID, follower.BelongsToGID)
	}
}

func addStubTask(t *testing.T, mgr *Manager, driver *stubDriver, name string, status task.Status) string {
	t.Helper()
	item := &task.Task{
		ID:       "task-" + name,
		GID:      "gid-" + name,
		Protocol: task.ProtocolHTTP,
		Status:   status,
		SaveDir:  t.TempDir(),
	}
	driver.tasks[item.ID] = item.Clone()
	mgr.mu.Lock()
	mgr.tasks[item.ID] = item.Clone()
	mgr.driverByTaskID[item.ID] = driver
	mgr.mu.Unlock()
	return item.GID
}