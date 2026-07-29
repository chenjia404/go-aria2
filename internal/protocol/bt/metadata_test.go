package bt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func TestMetadataFilePath(t *testing.T) {
	t.Parallel()

	got := metadataFilePath("/tmp/dl", "ABCD")
	if got != filepath.Join("/tmp/dl", "abcd.torrent") {
		t.Fatalf("unexpected metadata path: %s", got)
	}
}

func TestTryLoadSavedMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	infoHash := "0123456789abcdef0123456789abcdef01234567"
	path := filepath.Join(dir, infoHash+".torrent")
	payload := []byte("d8:announce0:e")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	got, err := tryLoadSavedMetadata(dir, infoHash)
	if err != nil {
		t.Fatalf("tryLoadSavedMetadata: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestMaybeLoadMagnetFromSavedMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "sample.bin"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	mi := metainfo.MetaInfo{}
	mi.SetDefaults()
	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatalf("build torrent info: %v", err)
	}
	info.Name = name
	rawInfo, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	mi.InfoBytes = rawInfo
	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		t.Fatalf("write metainfo: %v", err)
	}

	infoHash := mi.HashInfoBytes().HexString()
	metaPath := filepath.Join(dir, infoHash+".torrent")
	if err := os.WriteFile(metaPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	magnet := "magnet:?xt=urn:btih:" + infoHash
	spec, loadedRaw, ok := maybeLoadMagnetFromSavedMetadata(magnet, dir, map[string]string{"bt-load-saved-metadata": "true"})
	if !ok || spec == nil {
		t.Fatal("expected saved metadata to be loaded")
	}
	if len(loadedRaw) == 0 {
		t.Fatal("expected raw torrent payload")
	}
	if spec.InfoHash.HexString() != infoHash {
		t.Fatalf("unexpected info hash: %s", spec.InfoHash.HexString())
	}
}

func TestShouldSaveAndPauseMetadataFlags(t *testing.T) {
	t.Parallel()

	if !shouldSaveTorrentMetadata(map[string]string{"bt-save-metadata": "true"}, "magnet") {
		t.Fatal("expected save metadata for magnet")
	}
	if shouldSaveTorrentMetadata(map[string]string{"bt-save-metadata": "false"}, "magnet") {
		t.Fatal("expected save metadata disabled")
	}
	if !shouldPauseAfterMetadata(map[string]string{"pause-metadata": "true"}, "magnet") {
		t.Fatal("expected pause after metadata")
	}
}
