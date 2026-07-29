package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 参考 aria2 RpcMethodTest.cc 中 getOption / changeUri_fail 等与 GID 相关的用例。

func TestRpcMethod_TellStatus_InvalidGID(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	rpcErr := env.ExpectRPCError("aria2.tellStatus", "0123456789abcdef")
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params, got %#v", rpcErr)
	}
}

func TestRpcMethod_GetOption_ReturnsTaskDir(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{DefaultDir: saveDir})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"dir": saveDir, "pause": "true"})

	opts := mustStringMap(t, env.MustCall("aria2.getOption", gid))
	if opts["dir"] != saveDir {
		t.Fatalf("dir: %#v", opts["dir"])
	}
	if opts["pause"] != "true" {
		t.Fatalf("pause: %#v", opts["pause"])
	}
}

func TestRpcMethod_GetOption_StoppedTask(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	otherDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{DefaultDir: saveDir})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/done"}, map[string]any{"dir": otherDir, "pause": "true"})
	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			item.Status = task.StatusComplete
		}
	}

	opts := mustStringMap(t, env.MustCall("aria2.getOption", gid))
	if opts["dir"] != otherDir {
		t.Fatalf("stopped task dir: %#v", opts["dir"])
	}
}

func TestRpcMethod_GetOption_InvalidGID(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	rpcErr := env.ExpectRPCError("aria2.getOption", "0123456789abcdef")
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params, got %#v", rpcErr)
	}
}

func TestRpcMethod_ChangeUri_InvalidGID(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	rpcErr := env.ExpectRPCError("aria2.changeUri", "0123456789abcdef", 1, []any{}, []any{}, 0)
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params, got %#v", rpcErr)
	}
}

func TestRpcMethod_ChangeUri_RejectsStringFileIndex(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})
	env.ExpectError("aria2.changeUri", gid, "0", []any{}, []any{}, 0)
}

func TestRpcMethod_ChangeUri_RejectsAddURIsNotList(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})
	rpcErr := env.ExpectRPCError("aria2.changeUri", gid, 1, []any{}, "http://url", 0)
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params for non-list addURIs, got %#v", rpcErr)
	}
}
