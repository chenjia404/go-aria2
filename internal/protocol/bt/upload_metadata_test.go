package bt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveUploadedTorrentMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	payload := []byte("d8:announce14:http://tracker13:creation datei1712123456e4:infod6:lengthi123e4:name8:test.bin12:piece lengthi262144e6:pieces20:12345678901234567890ee")
	if err := SaveUploadedTorrentMetadata(dir, payload); err != nil {
		t.Fatalf("SaveUploadedTorrentMetadata: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || filepath.Ext(entries[0].Name()) != ".torrent" {
		t.Fatalf("expected one .torrent file, got %#v", entries)
	}
}
