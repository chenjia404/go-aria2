package common

import (
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestApplyTransferProgressUpdatesSpeed(t *testing.T) {
	t.Parallel()

	item := &task.Task{Status: task.StatusActive}
	var lastBytes int64
	lastTick := time.Now().Add(-time.Second)
	ApplyTransferProgress(item, 4096, 8192, &lastBytes, &lastTick)
	if item.DownloadSpeed <= 0 {
		t.Fatalf("expected positive download speed, got %d", item.DownloadSpeed)
	}
	if item.CompletedLength != 4096 || item.TotalLength != 8192 {
		t.Fatalf("progress: completed=%d total=%d", item.CompletedLength, item.TotalLength)
	}
	if item.Connections != 1 {
		t.Fatalf("connections: %d", item.Connections)
	}
}
