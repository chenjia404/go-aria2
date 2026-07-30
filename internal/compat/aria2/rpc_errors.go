package aria2

import (
	"errors"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// mapManagerRPCError 将 manager 层错误映射为 aria2 风格 RPC 错误码。
func mapManagerRPCError(err error) error {
	if err == nil {
		return nil
	}
	var rpcErr *jsonrpc.RPCError
	if errors.As(err, &rpcErr) {
		return err
	}
	if errors.Is(err, manager.ErrTaskNotFound) {
		return jsonrpc.NewError(jsonrpc.CodeInvalidParams, err.Error())
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unknown position mode"),
		strings.Contains(msg, "not in queue"),
		strings.Contains(msg, "not supported"),
		strings.Contains(msg, "cannot remove download result"),
		strings.Contains(msg, "No active download for GID#"),
		strings.Contains(msg, "cannot be paused now"),
		strings.Contains(msg, "cannot be unpaused now"):
		return jsonrpc.NewError(jsonrpc.CodeInvalidParams, msg)
	default:
		return err
	}
}
