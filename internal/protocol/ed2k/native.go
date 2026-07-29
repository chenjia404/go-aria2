package ed2k

import (
	"context"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// GetED2KSources 返回 ED2K 任务已知源地址（native.getEd2kSources）。
func (d *Driver) GetED2KSources(ctx context.Context, taskID string) ([]string, error) {
	_ = ctx
	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}

	item, err := d.snapshot("", taskID)
	if err != nil {
		return nil, err
	}
	if raw := strings.TrimSpace(item.Meta["ed2k.sources"]); raw != "" {
		parts := strings.Split(raw, "\n")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out, nil
	}
	if strings.TrimSpace(st.uri) != "" {
		return []string{st.uri}, nil
	}
	return []string{}, nil
}
