package text

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func entryToTask(item Entry) (*task.Task, error) {
	kind, err := routeKind(item.URI)
	if err != nil {
		return nil, err
	}
	saveDir := item.Dir
	name := item.Out
	if name == "" {
		name = derivePreviewName(item.URI)
	}
	if saveDir != "" && name != "" {
		name = filepath.Base(name)
	}

	t := &task.Task{
		ID:       generateOrReuseGID(item),
		GID:      generateOrReuseGID(item),
		Protocol: kind,
		Name:     name,
		Status:   task.StatusWaiting,
		SaveDir:  saveDir,
		Options:  cloneMap(item.Options),
		Meta: map[string]string{
			"aria2.import":        "true",
			"aria2.import.source": importSource(item),
		},
	}
	if t.SaveDir != "" {
		t.Options["dir"] = t.SaveDir
	}
	if item.Out != "" {
		t.Options["out"] = item.Out
	}
	if item.Paused || parseBoolValue(item.Options["pause"]) || parseBoolValue(item.Options["paused"]) {
		t.Status = task.StatusPaused
		t.Options["pause"] = "true"
		t.Meta["aria2.paused"] = "true"
	}
	if item.Checksum != "" {
		t.Options["checksum"] = item.Checksum
		t.Meta["aria2.checksum"] = item.Checksum
	}
	if item.Metalink != "" {
		t.Options["metalink"] = item.Metalink
		t.Meta["aria2.metalink"] = item.Metalink
	}
	if checksum := strings.TrimSpace(item.Options["checksum"]); checksum != "" && t.Meta["aria2.checksum"] == "" {
		t.Meta["aria2.checksum"] = checksum
	}
	if metalink := strings.TrimSpace(item.Options["metalink"]); metalink != "" && t.Meta["aria2.metalink"] == "" {
		t.Meta["aria2.metalink"] = metalink
	}
	if t.Protocol == task.ProtocolBT {
		t.Meta["bt.resume.mode"] = "fast"
	}
	if t.Protocol == task.ProtocolED2K {
		t.Meta["ed2k.import"] = "true"
	}
	if t.Protocol == task.ProtocolHTTP || t.Protocol == task.Protocol("file") || t.Protocol == task.Protocol("ftp") || t.Protocol == task.Protocol("sftp") {
		t.Files = []task.File{{
			Index:    0,
			Path:     filepath.Join(saveDir, name),
			Selected: true,
			URIs:     []string{item.URI},
		}}
	}
	return t, nil
}

func routeKind(uri string) (task.Protocol, error) {
	lower := strings.ToLower(strings.TrimSpace(uri))
	switch {
	case strings.HasPrefix(lower, "magnet:"):
		return task.ProtocolBT, nil
	case strings.HasPrefix(lower, "ed2k://"):
		return task.ProtocolED2K, nil
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		if strings.HasSuffix(lower, ".torrent") {
			return task.ProtocolBT, nil
		}
		return task.ProtocolHTTP, nil
	case strings.HasPrefix(lower, "file://"):
		return task.Protocol("file"), nil
	case strings.HasPrefix(lower, "ftp://"):
		return task.Protocol("ftp"), nil
	case strings.HasPrefix(lower, "sftp://"):
		return task.Protocol("sftp"), nil
	default:
		return "", fmt.Errorf("unsupported aria2 session uri: %s", uri)
	}
}

func generateOrReuseGID(item Entry) string {
	if gid := strings.TrimSpace(item.GID); gid != "" && len(gid) == 16 {
		return strings.ToLower(gid)
	}
	if gid := strings.TrimSpace(item.Options["gid"]); gid != "" && len(gid) == 16 {
		return strings.ToLower(gid)
	}
	sum := sha1.Sum([]byte(strings.Join([]string{item.URI, item.Dir, item.Out}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func derivePreviewName(uri string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "magnet:") {
		return "magnet-task"
	}
	if idx := strings.LastIndex(uri, "/"); idx >= 0 && idx < len(uri)-1 {
		return uri[idx+1:]
	}
	return filepath.Base(uri)
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func importSource(item Entry) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.URI)), "magnet:") {
		return "magnet"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.URI)), "ed2k://") {
		return "ed2k"
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.URI)), ".torrent") {
		return "torrent-url"
	}
	return "uri"
}
