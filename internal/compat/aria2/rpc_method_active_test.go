package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 参考 aria2 对 getServers / pause / forcePause / tellActive 的语义约束。

func TestRpcMethod_GetServers_RequiresActiveDownload(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})

	rpcErr := env.ExpectRPCError("aria2.getServers", gid)
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params for paused task, got %#v", rpcErr)
	}

	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			item.Status = task.StatusActive
		}
	}
	servers := env.MustCall("aria2.getServers", gid).([]map[string]any)
	if len(servers) != 1 {
		t.Fatalf("expected servers for active task, got %#v", servers)
	}
}

func TestRpcMethod_Pause_RejectsAlreadyPaused(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})

	rpcErr := env.ExpectRPCError("aria2.pause", gid)
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected cannot pause now, got %#v", rpcErr)
	}
	rpcErr = env.ExpectRPCError("aria2.forcePause", gid)
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected forcePause cannot pause now, got %#v", rpcErr)
	}
}

func TestRpcMethod_Pause_AllowsWaitingTask(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{MaxConcurrent: 1})
	first := env.MustGID("aria2.addUri", []any{"http://example.com/1"})
	second := env.MustGID("aria2.addUri", []any{"http://example.com/2"})
	for _, item := range env.Driver.tasks {
		switch item.GID {
		case first:
			item.Status = task.StatusActive
		case second:
			item.Status = task.StatusWaiting
		}
	}

	if raw := env.MustCall("aria2.pause", second); raw != second {
		t.Fatalf("pause waiting: %#v", raw)
	}
	if env.Status(second)["status"] != "paused" {
		t.Fatalf("expected paused after pause, got %#v", env.Status(second))
	}
}

func TestRpcMethod_TellActive_OnlyActiveTasks(t *testing.T) {
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

	active := env.MustCall("aria2.tellActive").([]map[string]any)
	if len(active) != 1 || active[0]["gid"] != activeGID {
		t.Fatalf("tellActive: %#v", active)
	}
}
