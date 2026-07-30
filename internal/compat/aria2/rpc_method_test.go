package aria2

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
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
	raw := env.MustCall("aria2.changeGlobalOption", map[string]any{
		"enable-rpc":      "true",
		"file-allocation": "none",
	})
	opts := mustStringMap(t, raw)
	if opts == nil {
		t.Fatal("expected global options map")
	}
	if _, ok := opts["enable-rpc"]; ok {
		t.Fatal("enable-rpc should be filtered from changeGlobalOption result")
	}
	if opts["file-allocation"] != "none" {
		t.Fatalf("file-allocation should be applied: %#v", opts)
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

func TestRpcMethod_TellWaiting_Pagination(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{StartPaused: true})
	for i := 0; i < 4; i++ {
		env.MustGID("aria2.addUri", []any{fmt.Sprintf("http://example.com/%d", i)})
	}
	waiting := env.MustCall("aria2.tellWaiting", 1, 2).([]map[string]any)
	if len(waiting) != 2 {
		t.Fatalf("expected 2 waiting tasks, got %#v", waiting)
	}
	empty := env.MustCall("aria2.tellWaiting", 100, 10).([]map[string]any)
	if len(empty) != 0 {
		t.Fatalf("expected empty slice for offset beyond range, got %#v", empty)
	}
}

func TestRpcMethod_AddMetalink_LinksBatchDownloads(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	gids := mustStringSlice(t, env.MustCall("aria2.addMetalink", sampleMetalinkBase64(), map[string]any{"pause": "true"}))
	if len(gids) != 2 {
		t.Fatalf("expected 2 gids, got %#v", gids)
	}
	leader := env.Status(gids[0])
	followedBy, ok := leader["followedBy"].([]string)
	if !ok || len(followedBy) != 1 || followedBy[0] != gids[1] {
		t.Fatalf("leader followedBy: %#v", leader["followedBy"])
	}
	follower := env.Status(gids[1])
	if follower["following"] != gids[0] || follower["belongsTo"] != gids[0] {
		t.Fatalf("follower links: %#v", follower)
	}
}

func TestRpcMethod_AddTorrent_SaveUploadMetadata(t *testing.T) {
	t.Parallel()
	saveDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{DefaultDir: saveDir})
	env.MustGID("aria2.addTorrent", sampleTorrentBase64(), map[string]any{
		"dir":                      saveDir,
		"rpc-save-upload-metadata": "true",
		"pause":                    "true",
	})
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".torrent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected saved .torrent in %s, got %#v", saveDir, entries)
	}
}

func TestRpcMethod_GetSessionInfo(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	info := env.MustCall("aria2.getSessionInfo").(map[string]any)
	if info["sessionId"] == nil || info["sessionId"] == "" {
		t.Fatalf("expected sessionId, got %#v", info)
	}
}

func TestRpcMethod_ChangeOption_RejectsInvalidSplit(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"pause": "true"})
	env.ExpectError("aria2.changeOption", gid, map[string]any{"split": "bad"})
}

func TestRpcMethod_GatherStoppedDownloadBTMetadata(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addTorrent", sampleTorrentBase64(), map[string]any{"pause": "true"})
	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			item.Protocol = task.ProtocolBT
			item.Name = "test.bin"
			item.Meta = map[string]string{
				"bt.creationDate": "1712123456",
			}
			item.Status = task.StatusComplete
		}
	}
	status := env.Status(gid)
	bt, ok := status["bittorrent"].(map[string]any)
	if !ok {
		t.Fatalf("expected bittorrent section, got %#v", status["bittorrent"])
	}
	if bt["creationDate"] != int64(1712123456) {
		t.Fatalf("creationDate: %#v", bt["creationDate"])
	}
}

func TestRpcMethod_SystemMulticall_Success(t *testing.T) {
	t.Parallel()
	env := newRPCTestEnv(t, manager.Options{})
	calls := []any{
		map[string]any{"methodName": "aria2.ping", "params": []any{}},
		map[string]any{"methodName": "aria2.getVersion", "params": []any{}},
	}
	raw := env.MustCall("system.multicall", calls)
	results, ok := raw.([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("unexpected multicall result: %#v", raw)
	}
}
