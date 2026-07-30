package ed2k

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/monkeyWie/goed2k/protocol"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// ChangeURI 为 ED2K 任务增删镜像 ed2k:// URI，语义对齐 aria2.changeUri。
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
		d.mu.Unlock()
		return 0, 0, fmt.Errorf("fileIndex out of range")
	}

	uris := append([]string(nil), st.uris...)
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
			for _, raw := range addURIs {
				if !isED2KChangeURIValid(raw) {
					continue
				}
				link, err := parseLink(raw)
				if err != nil || !hashMatchesLink(st.hash, link) {
					continue
				}
				if containsURI(uris, link.SourceURI) {
					continue
				}
				uris = append(uris, link.SourceURI)
				d.connectLinkSources(link)
				addCount++
			}
		} else {
			pos := position
			if pos > len(uris) {
				pos = len(uris)
			}
			for _, raw := range addURIs {
				if !isED2KChangeURIValid(raw) {
					continue
				}
				link, err := parseLink(raw)
				if err != nil || !hashMatchesLink(st.hash, link) {
					continue
				}
				if containsURI(uris, link.SourceURI) {
					continue
				}
				uris = append(append(append([]string(nil), uris[:pos]...), link.SourceURI), uris[pos:]...)
				d.connectLinkSources(link)
				addCount++
				pos++
			}
		}
	}
	if len(uris) == 0 {
		d.mu.Unlock()
		return 0, 0, fmt.Errorf("uri list cannot be empty")
	}

	st.uris = uris
	d.mu.Unlock()
	_ = time.Now()
	return delCount, addCount, nil
}

func isED2KChangeURIValid(uri string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "ed2k://")
}

func containsURI(uris []string, target string) bool {
	for _, uri := range uris {
		if uri == target {
			return true
		}
	}
	return false
}

func hashMatchesLink(hash protocol.Hash, link *link) bool {
	if link == nil {
		return false
	}
	return strings.EqualFold(hash.String(), link.Hash)
}

func (d *Driver) connectLinkSources(link *link) {
	if d == nil || d.client == nil || link == nil || len(link.Sources) == 0 {
		return
	}
	_ = d.client.ConnectServers(link.Sources...)
}
