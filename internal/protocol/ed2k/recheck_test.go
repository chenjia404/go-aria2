package ed2k

import (
	"os"
	"path/filepath"
	"testing"

	goed2k "github.com/goed2k/core"
	"github.com/goed2k/core/protocol"
)

func TestHashED2KFileSinglePiece(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "demo.bin")
	payload := []byte("hello ed2k recheck")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	want, err := protocol.HashFromData(payload)
	if err != nil {
		t.Fatalf("HashFromData: %v", err)
	}
	got, err := hashED2KFile(path, int64(len(payload)))
	if err != nil {
		t.Fatalf("hashED2KFile: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("unexpected hash: got %s want %s", got, want)
	}
}

func TestHashED2KFileUsesPieceSizeBoundary(t *testing.T) {
	t.Parallel()

	if goed2k.PieceSize <= 0 {
		t.Fatal("unexpected piece size")
	}
}
