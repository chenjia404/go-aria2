package bt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent/metainfo"
)

// SaveUploadedTorrentMetadata 将 RPC 上传的 torrent 原始数据保存到 dir/<infohash>.torrent（aria2 rpc-save-upload-metadata）。
func SaveUploadedTorrentMetadata(saveDir string, payload []byte) error {
	saveDir = filepath.Clean(saveDir)
	if saveDir == "" {
		return fmt.Errorf("missing save directory")
	}
	if len(payload) == 0 {
		return fmt.Errorf("empty torrent payload")
	}
	mi, err := metainfo.Load(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("parse torrent: %w", err)
	}
	path := metadataFilePath(saveDir, mi.HashInfoBytes().HexString())
	if path == "" {
		return fmt.Errorf("missing info hash")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}
