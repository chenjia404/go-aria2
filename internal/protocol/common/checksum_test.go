package common

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestVerifyTaskChecksumSHA256(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	content := []byte("checksum-test")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sum := sha256.Sum256(content)

	item := &task.Task{
		Name:    "file.bin",
		SaveDir: dir,
		Files: []task.File{{
			Index: 1,
			Path:  path,
		}},
		Options: map[string]string{
			"checksum": "sha-256=" + hex.EncodeToString(sum[:]),
		},
	}

	checked, matched, _, err := VerifyTaskChecksum(item)
	if err != nil {
		t.Fatalf("VerifyTaskChecksum: %v", err)
	}
	if !checked || !matched {
		t.Fatalf("expected checksum match, checked=%v matched=%v", checked, matched)
	}
}
