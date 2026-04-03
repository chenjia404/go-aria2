package httpdl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestDriverResumesPartialDownload(t *testing.T) {
	t.Parallel()

	payload := []byte("hello world")
	var mu sync.Mutex
	var sawRange string
	var unexpectedRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentRange := r.Header.Get("Range")
		mu.Lock()
		sawRange = currentRange
		mu.Unlock()
		if currentRange == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload)
			return
		}
		if currentRange != "bytes=5-" {
			mu.Lock()
			unexpectedRange = currentRange
			mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", "bytes 5-10/11")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[5:])
	}))
	defer server.Close()

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.txt")
	if err := os.WriteFile(outputPath, payload[:5], 0o644); err != nil {
		t.Fatalf("write partial file: %v", err)
	}

	driver := New(Options{UserAgent: "test-agent"})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs:    []string{server.URL + "/file.txt"},
		SaveDir: saveDir,
		Name:    "file.txt",
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := driver.TellStatus(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("TellStatus returned error: %v", err)
		}
		if status.Status == task.StatusComplete {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("final TellStatus returned error: %v", err)
	}
	if status.Status != task.StatusComplete {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.CompletedLength != int64(len(payload)) || status.TotalLength != int64(len(payload)) {
		t.Fatalf("unexpected lengths: %+v", status)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected file content: %q", string(data))
	}
	mu.Lock()
	rangeSeen := sawRange
	badRange := unexpectedRange
	mu.Unlock()

	if rangeSeen == "" {
		t.Fatalf("expected range request on resume")
	}
	if badRange != "" {
		t.Fatalf("unexpected range request: %q", badRange)
	}
}

func TestDriverChunkedDownload(t *testing.T) {
	t.Parallel()

	payload := []byte("abcdefghijkl")
	var mu sync.Mutex
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.Header().Set("Accept-Ranges", "bytes")
			return
		case http.MethodGet:
			rng := r.Header.Get("Range")
			mu.Lock()
			ranges = append(ranges, rng)
			mu.Unlock()
			if rng == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var start, end int
			if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if start < 0 || end >= len(payload) || start > end {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[start : end+1])
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	saveDir := t.TempDir()
	driver := New(Options{UserAgent: "test-agent", Split: 3})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs:    []string{server.URL + "/chunked.bin"},
		SaveDir: saveDir,
		Name:    "chunked.bin",
		Options: map[string]string{"split": "3"},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := driver.TellStatus(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("TellStatus returned error: %v", err)
		}
		if status.Status == task.StatusComplete {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("final TellStatus returned error: %v", err)
	}
	if status.Status != task.StatusComplete {
		t.Fatalf("unexpected status: %+v", status)
	}

	data, err := os.ReadFile(filepath.Join(saveDir, "chunked.bin"))
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected file content: %q", string(data))
	}

	mu.Lock()
	gotRanges := append([]string(nil), ranges...)
	mu.Unlock()
	if len(gotRanges) < 3 {
		t.Fatalf("expected multiple range requests, got %#v", gotRanges)
	}
}
