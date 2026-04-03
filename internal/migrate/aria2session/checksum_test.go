package aria2session

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestVerifyTaskChecksum(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	content := []byte("hello checksum\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	sum := sha1.Sum(content)
	item := &task.Task{
		Files: []task.File{{Path: path}},
		Meta: map[string]string{
			"aria2.checksum": "sha1=" + hex.EncodeToString(sum[:]),
		},
	}

	checked, matched, actual, err := verifyTaskChecksum(item)
	if err != nil {
		t.Fatalf("verifyTaskChecksum returned error: %v", err)
	}
	if !checked || !matched {
		t.Fatalf("expected checksum match, got checked=%v matched=%v actual=%s", checked, matched, actual)
	}

	item.Meta["aria2.checksum"] = "sha1=deadbeef"
	checked, matched, actual, err = verifyTaskChecksum(item)
	if err != nil {
		t.Fatalf("verifyTaskChecksum returned error: %v", err)
	}
	if !checked || matched || actual == "" {
		t.Fatalf("expected checksum mismatch, got checked=%v matched=%v actual=%s", checked, matched, actual)
	}
}

func TestVerifyTaskChecksumSkipsMissingFile(t *testing.T) {
	t.Parallel()

	item := &task.Task{
		SaveDir: t.TempDir(),
		Name:    "missing.bin",
		Meta: map[string]string{
			"aria2.checksum": "sha1=deadbeef",
		},
	}

	checked, matched, actual, err := verifyTaskChecksum(item)
	if err != nil {
		t.Fatalf("verifyTaskChecksum returned error: %v", err)
	}
	if checked || matched || actual != "" {
		t.Fatalf("expected skipped checksum check, got checked=%v matched=%v actual=%s", checked, matched, actual)
	}
}
