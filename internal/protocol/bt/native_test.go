package bt

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestGetBTTrackersFromMagnetTask(t *testing.T) {
	t.Parallel()

	driver, err := New(Options{DataDir: t.TempDir(), ListenPort: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer driver.Close()

	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI: "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=udp%3A%2F%2Ftracker.example.com%3A80",
		SaveDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	trackers, err := driver.GetBTTrackers(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetBTTrackers: %v", err)
	}
	raw, ok := trackers["trackers"].([]string)
	if !ok || len(raw) == 0 {
		t.Fatalf("expected trackers, got %#v", trackers)
	}
}
