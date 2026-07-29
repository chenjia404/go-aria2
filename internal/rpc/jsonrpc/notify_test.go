package jsonrpc

import (
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestAria2NotificationsForEvent_AddPausedSkipsStart(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{}
	ev := manager.Event{
		Type: manager.EventTaskAdded,
		Task: &task.Task{GID: "gid-1", Status: task.StatusPaused},
		Time: time.Now(),
	}
	if got := aria2NotificationsForEvent(ev, prev); len(got) != 0 {
		t.Fatalf("paused add should not notify start: %#v", got)
	}
}

func TestAria2NotificationsForEvent_AddWaitingSkipsStart(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{}
	ev := manager.Event{
		Type: manager.EventTaskAdded,
		Task: &task.Task{GID: "gid-1", Status: task.StatusWaiting},
		Time: time.Now(),
	}
	if got := aria2NotificationsForEvent(ev, prev); len(got) != 0 {
		t.Fatalf("waiting add should not notify start: %#v", got)
	}
}

func TestAria2NotificationsForEvent_AddActiveStarts(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{}
	ev := manager.Event{
		Type: manager.EventTaskAdded,
		Task: &task.Task{GID: "gid-1", Status: task.StatusActive},
		Time: time.Now(),
	}
	got := aria2NotificationsForEvent(ev, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onDownloadStart" {
		t.Fatalf("active add: %#v", got)
	}
}

func TestAria2NotificationsForEvent_PauseAndResume(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{}
	gid := "gid-1"
	item := &task.Task{GID: gid, Status: task.StatusActive}

	pauseEv := manager.Event{Type: manager.EventTaskUpdated, Task: &task.Task{GID: gid, Status: task.StatusPaused}}
	got := aria2NotificationsForEvent(pauseEv, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onDownloadPause" {
		t.Fatalf("pause: %#v", got)
	}

	resumeEv := manager.Event{Type: manager.EventTaskUpdated, Task: item}
	item.Status = task.StatusActive
	got = aria2NotificationsForEvent(resumeEv, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onDownloadStart" {
		t.Fatalf("resume: %#v", got)
	}
}

func TestAria2NotificationsForEvent_RemoveStops(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{ "gid-1": {Status: task.StatusActive} }
	ev := manager.Event{
		Type: manager.EventTaskRemoved,
		Task: &task.Task{GID: "gid-1", Status: task.StatusRemoved},
	}
	got := aria2NotificationsForEvent(ev, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onDownloadStop" {
		t.Fatalf("remove: %#v", got)
	}
	if _, ok := prev["gid-1"]; ok {
		t.Fatal("prev entry should be cleared after remove")
	}
}

func TestAria2NotificationsForEvent_BTSeedingComplete(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{}
	gid := "gid-bt"
	seedDone := manager.Event{
		Type: manager.EventTaskUpdated,
		Task: &task.Task{GID: gid, Status: task.StatusComplete, Protocol: task.ProtocolBT, Seeder: true},
	}
	got := aria2NotificationsForEvent(seedDone, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onBtDownloadComplete" {
		t.Fatalf("bt seeding complete: %#v", got)
	}

	seedStop := manager.Event{
		Type: manager.EventTaskUpdated,
		Task: &task.Task{GID: gid, Status: task.StatusComplete, Protocol: task.ProtocolBT, Seeder: false},
	}
	got = aria2NotificationsForEvent(seedStop, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onDownloadComplete" {
		t.Fatalf("bt post-seed complete: %#v", got)
	}
}

func TestAria2NotificationsForEvent_HTTPComplete(t *testing.T) {
	t.Parallel()

	prev := map[string]taskSnap{}
	ev := manager.Event{
		Type: manager.EventTaskUpdated,
		Task: &task.Task{GID: "gid-http", Status: task.StatusComplete, Protocol: task.ProtocolHTTP},
	}
	got := aria2NotificationsForEvent(ev, prev)
	if len(got) != 1 || got[0]["method"] != "aria2.onDownloadComplete" {
		t.Fatalf("http complete: %#v", got)
	}
}
