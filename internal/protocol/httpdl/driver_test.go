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

func TestDriverRejectsExistingFileWhenOverwriteDisabled(t *testing.T) {
	t.Parallel()

	payload := []byte("hello world")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.txt")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	driver := New(Options{UserAgent: "test-agent"})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs:    []string{server.URL + "/file.txt"},
		SaveDir: saveDir,
		Name:    "file.txt",
		Options: map[string]string{
			"continue":        "false",
			"allow-overwrite": "false",
		},
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
		if status.Status == task.StatusError {
			if status.ErrorMessage == "" {
				t.Fatalf("expected error message when overwrite is disabled")
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("final TellStatus returned error: %v", err)
	}
	if status.Status != task.StatusError {
		t.Fatalf("unexpected status: %+v", status)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing file should be preserved, got %q", string(data))
	}
}

func TestDriverAutoRenamesExistingFile(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.txt")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	driver := New(Options{UserAgent: "test-agent"})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs:    []string{"https://example.com/file.txt"},
		SaveDir: saveDir,
		Name:    "file.txt",
		Options: map[string]string{
			"continue":           "false",
			"allow-overwrite":    "false",
			"auto-file-renaming": "true",
		},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if created.Name != "file.1.txt" {
		t.Fatalf("expected renamed file name, got %q", created.Name)
	}
	if len(created.Files) != 1 || created.Files[0].Path != filepath.Join(saveDir, "file.1.txt") {
		t.Fatalf("expected renamed output path, got %+v", created.Files)
	}
}

func TestDriverFailoverToMirrorURL(t *testing.T) {
	t.Parallel()

	payload := []byte("mirror-payload-data")
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badServer.Close()

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer goodServer.Close()

	driver := New(Options{UserAgent: "test-agent"})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs: []string{
			badServer.URL + "/missing.bin",
			goodServer.URL + "/file.bin",
		},
		SaveDir: t.TempDir(),
		Name:    "file.bin",
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if len(created.Files[0].URIs) != 2 {
		t.Fatalf("expected 2 mirror URIs, got %#v", created.Files[0].URIs)
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
		if status.Status == task.StatusError {
			t.Fatalf("download failed before mirror failover: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TellStatus returned error: %v", err)
	}
	if status.Status != task.StatusComplete {
		t.Fatalf("expected complete after mirror failover, got %+v", status)
	}
	data, err := os.ReadFile(created.Files[0].Path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected payload: %q", data)
	}
}

func TestChunkedDownloadFailoverToMirror(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 8192)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badServer.Close()

	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			w.Header().Set("Accept-Ranges", "bytes")
			return
		case http.MethodGet:
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var start, end int
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if start < 0 || end < start || end >= len(payload) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			segment := payload[start : end+1]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
			w.Header().Set("Content-Length", strconv.Itoa(len(segment)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(segment)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer goodServer.Close()

	driver := New(Options{UserAgent: "test-agent", Split: 4})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs: []string{
			badServer.URL + "/missing.bin",
			goodServer.URL + "/file.bin",
		},
		SaveDir: t.TempDir(),
		Name:    "file.bin",
		Options: map[string]string{"split": "4"},
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
		if status.Status == task.StatusError {
			t.Fatalf("chunked download failed before mirror failover: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, err := driver.TellStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("TellStatus returned error: %v", err)
	}
	if status.Status != task.StatusComplete {
		t.Fatalf("expected complete after chunked mirror failover, got %+v", status)
	}
	data, err := os.ReadFile(created.Files[0].Path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected chunked payload length=%d", len(data))
	}
}

func TestBuildClientAppliesTimeouts(t *testing.T) {
	t.Parallel()

	client := buildClient(Options{}, map[string]string{
		"connect-timeout": "15",
		"timeout":         "60",
	})
	if client.Timeout != 60*time.Second {
		t.Fatalf("client timeout: %v", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil {
		t.Fatal("expected transport with DialContext")
	}
}

func TestHTTPBasicAuthFromOptions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("ok-auth"))
	}))
	defer server.Close()

	saveDir := t.TempDir()
	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URIs:    []string{server.URL + "/private.bin"},
		SaveDir: saveDir,
		Name:    "private.bin",
		Options: map[string]string{"http-user": "alice", "http-passwd": "secret"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := driver.TellStatus(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("TellStatus: %v", err)
		}
		if status.Status == task.StatusComplete {
			break
		}
		if status.Status == task.StatusError {
			t.Fatalf("auth download failed: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, err := os.ReadFile(filepath.Join(saveDir, "private.bin"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "ok-auth" {
		t.Fatalf("payload: %q", data)
	}
}

func TestResolveMaxTries(t *testing.T) {
	t.Parallel()

	if got := resolveMaxTries(nil); got != 1 {
		t.Fatalf("nil: %d", got)
	}
	if got := resolveMaxTries(map[string]string{"max-tries": "8"}); got != 8 {
		t.Fatalf("8: %d", got)
	}
	if got := resolveMaxTries(map[string]string{"max-tries": "0"}); got != 1<<20 {
		t.Fatalf("unlimited: %d", got)
	}
}
