package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 参考 aria2：unpause 仅对 paused 任务有效。

func TestRpcMethod_Unpause_OnlyPausedTask(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})

	if raw := env.MustCall("aria2.unpause", gid); raw != gid {
		t.Fatalf("unpause paused: %#v", raw)
	}
	if env.Status(gid)["status"] == "paused" {
		t.Fatal("expected unpaused status")
	}
}

func TestRpcMethod_Unpause_RejectsWaitingAndActive(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{MaxConcurrent: 1})
	activeGID := env.MustGID("aria2.addUri", []any{"http://example.com/active"})
	waitingGID := env.MustGID("aria2.addUri", []any{"http://example.com/waiting"})
	for _, item := range env.Driver.tasks {
		switch item.GID {
		case activeGID:
			item.Status = task.StatusActive
		case waitingGID:
			item.Status = task.StatusWaiting
		}
	}

	for _, gid := range []string{activeGID, waitingGID} {
		rpcErr := env.ExpectRPCError("aria2.unpause", gid)
		if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
			t.Fatalf("unpause %s: expected cannot unpaused now, got %#v", gid, rpcErr)
		}
	}
}
