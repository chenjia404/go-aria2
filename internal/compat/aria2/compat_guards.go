package aria2

import (
	"fmt"

	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

func errNoActiveDownload(gid string) error {
	return jsonrpc.NewError(jsonrpc.CodeInvalidParams, fmt.Sprintf("No active download for GID#%s", gid))
}

func errCannotPauseNow(gid string) error {
	return jsonrpc.NewError(jsonrpc.CodeInvalidParams, fmt.Sprintf("GID#%s cannot be paused now", gid))
}

func taskCanBePaused(item *task.Task) bool {
	switch item.Status {
	case task.StatusActive, task.StatusWaiting:
		return true
	default:
		return false
	}
}
