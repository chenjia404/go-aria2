package ftp

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestGetServersUsesTaskDownloadSpeed(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "ftp://user:pass@127.0.0.1:21/a.bin",
		SaveDir: t.TempDir(),
		URIs:    []string{"ftp://user:pass@127.0.0.1:21/b.bin"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	driver.mu.Lock()
	st := driver.tasks[created.ID]
	st.task.DownloadSpeed = 12345
	st.active = 0
	driver.mu.Unlock()

	servers, err := driver.GetServers(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetServers: %v", err)
	}
	if len(servers) != 1 || len(servers[0].Servers) != 2 {
		t.Fatalf("servers: %#v", servers)
	}
	if servers[0].Servers[0].DownloadSpeed != 12345 {
		t.Fatalf("active mirror speed: got %d want 12345", servers[0].Servers[0].DownloadSpeed)
	}
	if servers[0].Servers[1].DownloadSpeed != 0 {
		t.Fatalf("inactive mirror speed: got %d want 0", servers[0].Servers[1].DownloadSpeed)
	}
}
