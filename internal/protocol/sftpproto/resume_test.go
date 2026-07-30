package sftpproto

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func waitSFTPComplete(t *testing.T, driver *Driver, taskID string, payloadLen int) {
	t.Helper()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		status, err := driver.TellStatus(context.Background(), taskID)
		if err != nil {
			t.Fatalf("TellStatus: %v", err)
		}
		switch status.Status {
		case task.StatusComplete:
			if status.CompletedLength != int64(payloadLen) {
				t.Fatalf("completed length: got %d want %d", status.CompletedLength, payloadLen)
			}
			if len(status.Files) > 0 && status.Files[0].CompletedLength != int64(payloadLen) {
				t.Fatalf("file completed length: got %d want %d", status.Files[0].CompletedLength, payloadLen)
			}
			return
		case task.StatusError:
			t.Fatalf("download failed: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for SFTP download")
}

func TestDriverResumesPartialDownload_SFTP(t *testing.T) {
	t.Parallel()

	payload := []byte("hello sftp world")
	srv := newTestSFTPServer(t, payload)
	defer srv.close()

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.bin")
	if err := os.WriteFile(outputPath, payload[:5], 0o644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     srv.uri(),
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"continue": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitSFTPComplete(t, driver, created.ID, len(payload))

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("content: got %q want %q", data, payload)
	}
}

func TestDriverRestartsWhenContinueFalse_SFTP(t *testing.T) {
	t.Parallel()

	payload := []byte("hello sftp world")
	srv := newTestSFTPServer(t, payload)
	defer srv.close()

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.bin")
	if err := os.WriteFile(outputPath, payload[:5], 0o644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     srv.uri(),
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"continue": "false", "allow-overwrite": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitSFTPComplete(t, driver, created.ID, len(payload))

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("content: got %q want %q", data, payload)
	}
}

func TestDriverAdvanceSyncsFileProgress_SFTP(t *testing.T) {
	t.Parallel()

	payload := []byte("sync check")
	srv := newTestSFTPServer(t, payload)
	defer srv.close()

	saveDir := t.TempDir()
	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     srv.uri(),
		SaveDir: saveDir,
		Name:    "file.bin",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitSFTPComplete(t, driver, created.ID, len(payload))

	files, err := driver.GetFiles(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files: %#v", files)
	}
	if files[0].CompletedLength != int64(len(payload)) || files[0].Length != int64(len(payload)) {
		t.Fatalf("file progress: %#v", files[0])
	}
}
