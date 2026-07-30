package ftp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestDriverRespectsMaxDownloadLimit_FTP(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 24*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	mock, addr := newResumeFTPMock(t, payload)
	defer mock.close()

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	uri := "ftp://" + host + ":" + port + "/" + mock.remote

	saveDir := t.TempDir()
	driver := New()
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     uri,
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"max-download-limit": "8192"}, // 8 KiB/s
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	start := time.Now()
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		status, err := driver.TellStatus(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("TellStatus: %v", err)
		}
		if status.Status == task.StatusComplete {
			break
		}
		if status.Status == task.StatusError {
			t.Fatalf("download failed: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("final TellStatus: %v", err)
	}
	if status.Status != task.StatusComplete {
		t.Fatalf("timed out waiting for download, status=%s", status.Status)
	}
	elapsed := time.Since(start)

	// 24KiB at 8KiB/s 约需 3s；留出测试环境余量。
	if elapsed < 2*time.Second {
		t.Fatalf("download finished too quickly (%v), rate limit likely not applied", elapsed)
	}
}
