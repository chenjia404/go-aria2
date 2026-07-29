package ed2k

import (
	"context"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// GetED2KSources 返回 ED2K 任务已知源地址（native.getEd2kSources）。
// 合并链接内静态源（s=）与 goed2k 运行时发现的 peer 端点。
func (d *Driver) GetED2KSources(ctx context.Context, taskID string) ([]string, error) {
	_ = ctx
	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)

	add := func(addr string) {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			return
		}
		if _, ok := seen[addr]; ok {
			return
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}

	_, staticSources := parseLinkExtras(st.uri)
	for _, part := range staticSources {
		add(part)
	}

	handle := d.client.FindTransfer(st.hash)
	if handle.IsValid() {
		for _, peer := range handle.GetPeersInfo() {
			add(peer.Endpoint.String())
		}
	}

	return out, nil
}
