package bt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// ChangeURI 为 BT 任务增删 web seed / 镜像 URI，语义对齐 aria2.changeUri。
func (d *Driver) ChangeURI(ctx context.Context, taskID string, fileIndex int, delURIs, addURIs []string, position int) (int, int, error) {
	_ = ctx
	if fileIndex < 1 {
		return 0, 0, fmt.Errorf("fileIndex must be >= 1")
	}

	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return 0, 0, manager.ErrTaskNotFound
	}
	if fileIndex > 1 {
		info := st.torrent.Info()
		if info != nil && len(info.UpvertedFiles()) > 1 && fileIndex > len(info.UpvertedFiles()) {
			d.mu.Unlock()
			return 0, 0, fmt.Errorf("fileIndex out of range")
		}
	}

	uris := d.btURIsForFile(st, fileIndex)
	delCount := 0
	if len(delURIs) > 0 {
		delSet := make(map[string]struct{}, len(delURIs))
		for _, uri := range delURIs {
			delSet[uri] = struct{}{}
		}
		filtered := uris[:0]
		for _, uri := range uris {
			if _, remove := delSet[uri]; remove {
				delCount++
				continue
			}
			filtered = append(filtered, uri)
		}
		uris = filtered
	}

	addCount := 0
	if len(addURIs) > 0 {
		if position < 0 {
			for _, uri := range addURIs {
				if !isBTChangeURIValid(uri) {
					continue
				}
				uris = append(uris, uri)
				addCount++
			}
		} else {
			pos := position
			if pos > len(uris) {
				pos = len(uris)
			}
			for _, uri := range addURIs {
				if !isBTChangeURIValid(uri) {
					continue
				}
				uris = append(append(append([]string(nil), uris[:pos]...), uri), uris[pos:]...)
				addCount++
				pos++
			}
		}
	}
	if len(uris) == 0 {
		d.mu.Unlock()
		return 0, 0, fmt.Errorf("uri list cannot be empty")
	}

	st.webSeeds = rebuildWebSeeds(st.source.URI, uris)
	tor := st.torrent
	d.mu.Unlock()

	if tor != nil && addCount > 0 {
		toAdd := make([]string, 0, addCount)
		for _, uri := range addURIs {
			if isWebSeedURL(uri) {
				toAdd = append(toAdd, uri)
			}
		}
		if len(toAdd) > 0 {
			tor.AddWebSeeds(toAdd)
		}
	}
	_ = time.Now()
	return delCount, addCount, nil
}

func isBTChangeURIValid(uri string) bool {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(uri), "magnet:?") {
		return true
	}
	return strings.Contains(uri, "://")
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func rebuildWebSeeds(sourceURI string, uris []string) []string {
	out := make([]string, 0, len(uris))
	for _, uri := range uris {
		if uri == "" || uri == sourceURI {
			continue
		}
		if isWebSeedURL(uri) {
			out = append(out, uri)
		}
	}
	return dedupeStrings(out)
}
