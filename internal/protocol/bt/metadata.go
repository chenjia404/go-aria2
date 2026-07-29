package bt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	torrentlib "github.com/anacrolix/torrent"

	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

func metadataFilePath(saveDir, infoHash string) string {
	infoHash = strings.ToLower(strings.TrimSpace(infoHash))
	if infoHash == "" {
		return ""
	}
	return filepath.Join(saveDir, infoHash+".torrent")
}

func tryLoadSavedMetadata(saveDir, infoHash string) ([]byte, error) {
	path := metadataFilePath(saveDir, infoHash)
	if path == "" {
		return nil, fmt.Errorf("missing info hash")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty metadata file")
	}
	return data, nil
}

func specFromSavedMetadata(saveDir, infoHash string) (*torrentlib.TorrentSpec, []byte, error) {
	payload, err := tryLoadSavedMetadata(saveDir, infoHash)
	if err != nil {
		return nil, nil, err
	}
	mi, raw, err := loadMetaInfo(payload)
	if err != nil {
		return nil, nil, err
	}
	spec, err := torrentlib.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		return nil, nil, err
	}
	return spec, raw, nil
}

func saveTorrentMetadata(tor *torrentlib.Torrent, saveDir string) error {
	if tor == nil || tor.Info() == nil {
		return fmt.Errorf("torrent metadata not ready")
	}
	saveDir = strings.TrimSpace(saveDir)
	if saveDir == "" {
		return fmt.Errorf("missing save directory")
	}
	path := metadataFilePath(saveDir, tor.InfoHash().HexString())
	if path == "" {
		return fmt.Errorf("missing info hash")
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	mi := tor.Metainfo()
	return mi.Write(f)
}

func maybeLoadMagnetFromSavedMetadata(uri, saveDir string, opts map[string]string) (*torrentlib.TorrentSpec, []byte, bool) {
	if !common.OptionBool(opts, "bt-load-saved-metadata", true) {
		return nil, nil, false
	}
	spec, err := torrentlib.TorrentSpecFromMagnetUri(uri)
	if err != nil {
		return nil, nil, false
	}
	infoHash := spec.InfoHash.HexString()
	loadedSpec, raw, err := specFromSavedMetadata(saveDir, infoHash)
	if err != nil {
		return nil, nil, false
	}
	return loadedSpec, raw, true
}

func shouldSaveTorrentMetadata(opts map[string]string, kind string) bool {
	return kind == "magnet" && common.OptionBool(opts, "bt-save-metadata", true)
}

func shouldPauseAfterMetadata(opts map[string]string, kind string) bool {
	return kind == "magnet" && common.OptionBool(opts, "pause-metadata", false)
}
