package sftpproto

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// ChangeURI 增删 SFTP 任务的镜像 URI。
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
	if fileIndex > len(st.task.Files) {
		d.mu.Unlock()
		return 0, 0, fmt.Errorf("fileIndex out of range")
	}

	file := &st.task.Files[fileIndex-1]
	uris := append([]string(nil), file.URIs...)
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
				if !isSFTPChangeURIValid(uri) {
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
				if !isSFTPChangeURIValid(uri) {
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

	file.URIs = uris
	st.endpoints = endpointsFromURIs(uris)
	if st.active >= len(st.endpoints) {
		st.active = 0
	}
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()
	return delCount, addCount, nil
}

func isSFTPChangeURIValid(uri string) bool {
	uri = strings.TrimSpace(uri)
	return strings.HasPrefix(strings.ToLower(uri), "sftp://")
}

func endpointsFromURIs(uris []string) []endpoint {
	out := make([]endpoint, 0, len(uris))
	for _, raw := range uris {
		if ep, err := parseEndpoint(raw); err == nil {
			out = append(out, ep)
		}
	}
	return out
}
