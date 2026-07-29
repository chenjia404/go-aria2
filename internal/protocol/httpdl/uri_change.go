package httpdl

import (
	"context"
	"fmt"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// ChangeURI 增删 HTTP 任务的镜像 URI，语义对齐 aria2.changeUri。
func (d *Driver) ChangeURI(ctx context.Context, taskID string, fileIndex int, delURIs, addURIs []string, position int) error {
	_ = ctx
	if fileIndex < 1 {
		return fmt.Errorf("fileIndex must be >= 1")
	}

	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	if fileIndex > len(st.task.Files) {
		d.mu.Unlock()
		return fmt.Errorf("fileIndex out of range")
	}

	file := &st.task.Files[fileIndex-1]
	uris := append([]string(nil), file.URIs...)
	if len(delURIs) > 0 {
		delSet := make(map[string]struct{}, len(delURIs))
		for _, uri := range delURIs {
			delSet[uri] = struct{}{}
		}
		filtered := uris[:0]
		for _, uri := range uris {
			if _, remove := delSet[uri]; !remove {
				filtered = append(filtered, uri)
			}
		}
		uris = filtered
	}
	if len(addURIs) > 0 {
		if position < 0 || position > len(uris) {
			uris = append(uris, addURIs...)
		} else {
			uris = append(append(append([]string(nil), uris[:position]...), addURIs...), uris[position:]...)
		}
	}
	if len(uris) == 0 {
		d.mu.Unlock()
		return fmt.Errorf("uri list cannot be empty")
	}

	file.URIs = uris
	st.sourceURLs = append([]string(nil), uris...)
	if st.sourceURL == "" || !containsURI(uris, st.sourceURL) {
		st.sourceURL = uris[0]
		st.sourceIndex = 0
	} else {
		for i, uri := range uris {
			if uri == st.sourceURL {
				st.sourceIndex = i
				break
			}
		}
	}
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()
	return nil
}

func containsURI(uris []string, target string) bool {
	for _, uri := range uris {
		if uri == target {
			return true
		}
	}
	return false
}
