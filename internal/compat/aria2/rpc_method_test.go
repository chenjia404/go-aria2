package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 以下测试参考 aria2 官方 test/RpcMethodTest.cc，覆盖 RPC 参数校验与错误路径。

func TestRpcMethod_AddUri_WithoutUri(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addUri")
}

func TestRpcMethod_AddUri_NotUri(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addUri", []any{"not uri"})
}

func TestRpcMethod_AddUri_WithBadOption(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addUri",
		[]any{"http://localhost"},
		map[string]any{"file-allocation": "badvalue"},
	)
}

func TestRpcMethod_AddUri_WithBadPosition(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addUri",
		[]any{"http://localhost/"},
		map[string]any{},
		-1,
	)
}

func TestRpcMethod_AddTorrent_WithoutTorrent(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addTorrent")
}

func TestRpcMethod_AddTorrent_NotBase64Torrent(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addTorrent", "not torrent")
}

func TestRpcMethod_AddMetalink_WithoutMetalink(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addMetalink")
}

func TestRpcMethod_AddMetalink_NotBase64Metalink(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.addMetalink", "not metalink")
}

func TestRpcMethod_ChangeOption_WithBadOption(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"pause": "true"})
	env.ExpectError("aria2.changeOption", gid, map[string]any{"max-download-limit": "badvalue"})
}

func TestRpcMethod_ChangeOption_WithNotAllowedOption(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"pause": "true"})
	raw := env.MustCall("aria2.changeOption", gid, map[string]any{"max-overall-download-limit": "100K"})
	if raw != "OK" {
		t.Fatalf("expected OK when global-only option is ignored, got %#v", raw)
	}
}

func TestRpcMethod_ChangeOption_WithoutGid(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.changeOption")
}

func TestRpcMethod_ChangeGlobalOption_WithBadOption(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.changeGlobalOption", map[string]any{"max-overall-download-limit": "badvalue"})
}

func TestRpcMethod_ChangeGlobalOption_WithNotAllowedOption(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	raw := env.MustCall("aria2.changeGlobalOption", map[string]any{"enable-rpc": "100K"})
	opts := mustStringMap(t, raw)
	if opts == nil {
		t.Fatal("expected global options map")
	}
}

func TestRpcMethod_TellStatus_WithoutGid(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.tellStatus")
}

func TestRpcMethod_TellWaiting_Fail(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.tellWaiting")
}

func TestRpcMethod_ChangePosition_Fail(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("aria2.changePosition")
	env.ExpectError("aria2.changePosition", "1", 2, "bad keyword")
}

func TestRpcMethod_ChangeUri_Fail(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})
	// fileIndex 0 无效（aria2 从 1 开始）
	env.ExpectError("aria2.changeUri", gid, 0, []any{}, []any{}, 0)
}

func TestRpcMethod_NoSuchMethod(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	_, err := env.Call("make.hamburger")
	if err == nil {
		t.Fatal("expected method not found error")
	}
	rpcErr, ok := err.(*jsonrpc.RPCError)
	if !ok || rpcErr.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("expected method not found, got %v", err)
	}
}

func TestRpcMethod_SystemMulticall_Fail(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	env.ExpectError("system.multicall")
}

func TestRpcMethod_ChangeGlobalOption_AcceptsSpeedSuffix(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	changed := mustStringMap(t, env.MustCall("aria2.changeGlobalOption", map[string]any{
		"max-overall-download-limit": "100K",
	}))
	if changed["max-overall-download-limit"] != "102400" {
		t.Fatalf("expected normalized speed limit 102400, got %#v", changed)
	}
}
