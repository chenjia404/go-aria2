package aria2

import (
	"fmt"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 参考 aria2 RpcMethodTest.cc 中 GatherProgressCommon / TellWaiting / Pause 相关用例。

func TestRpcMethod_GatherProgressCommon(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{DefaultDir: saveDir})
	uri := "http://localhost/aria2.tar.bz2"
	gid := env.MustGID("aria2.addUri", []any{uri}, map[string]any{"dir": saveDir, "pause": "true"})

	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			item.FollowingGID = "gid-leader"
			item.BelongsToGID = "gid-parent"
			item.FollowedByGIDs = []string{"gid-child-1", "gid-child-2"}
		}
	}

	status := env.MustCall("aria2.tellStatus", gid).(map[string]any)
	if status["dir"] != saveDir {
		t.Fatalf("dir: %#v", status["dir"])
	}
	files, ok := status["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files: %#v", status["files"])
	}
	uris, ok := files[0]["uris"].([]map[string]any)
	if !ok || len(uris) != 1 || uris[0]["uri"] != uri {
		t.Fatalf("file uris: %#v", files[0]["uris"])
	}
	followedBy := status["followedBy"].([]string)
	if len(followedBy) != 2 || status["following"] != "gid-leader" || status["belongsTo"] != "gid-parent" {
		t.Fatalf("related downloads: %#v", status)
	}

	filtered := env.MustCall("aria2.tellStatus", gid, []any{"gid"}).(map[string]any)
	if len(filtered) != 1 || filtered["gid"] != gid {
		t.Fatalf("keys filter should only return gid: %#v", filtered)
	}
}

func TestRpcMethod_TellWaiting_NegativeOffset(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true})
	for i := 0; i < 4; i++ {
		env.MustGID("aria2.addUri", []any{fmt.Sprintf("http://example.com/%d", i)})
	}

	mid := env.MustCall("aria2.tellWaiting", 1, 2).([]map[string]any)
	if len(mid) != 2 {
		t.Fatalf("expected 2 at offset 1, got %#v", mid)
	}
	last := env.MustCall("aria2.tellWaiting", -1, 2).([]map[string]any)
	if len(last) != 1 {
		t.Fatalf("expected 1 at negative offset, got %#v", last)
	}
	empty := env.MustCall("aria2.tellWaiting", 100, 10).([]map[string]any)
	if len(empty) != 0 {
		t.Fatalf("expected empty beyond range, got %#v", empty)
	}
}

func TestRpcMethod_PauseAllAndUnpauseAll(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{DefaultDir: saveDir, MaxConcurrent: 3})
	gids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		gids = append(gids, env.MustGID("aria2.addUri", []any{fmt.Sprintf("http://example.com/%d", i)}, map[string]any{"dir": saveDir}))
	}

	if raw := env.MustCall("aria2.pauseAll"); raw != "OK" {
		t.Fatalf("pauseAll: %#v", raw)
	}
	for _, gid := range gids {
		if env.Status(gid)["status"] != "paused" {
			t.Fatalf("expected paused after pauseAll for %s", gid)
		}
	}

	if raw := env.MustCall("aria2.unpauseAll"); raw != "OK" {
		t.Fatalf("unpauseAll: %#v", raw)
	}
	active := env.MustCall("aria2.tellActive", 0, 10).([]map[string]any)
	if len(active) == 0 {
		t.Fatal("expected at least one active task after unpauseAll")
	}
}

func TestRpcMethod_Authorize_RejectsBadToken(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	env.Service = NewService(env.Service.manager, "secret-token")

	rpcErr := env.ExpectRPCError("aria2.getVersion", "token:wrong")
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params for bad token, got %#v", rpcErr)
	}
	if raw := env.MustCall("aria2.getVersion", "token:secret-token"); raw == nil {
		t.Fatal("expected version with valid token")
	}
}

func TestRpcMethod_TellStopped_IncludesCompletedTask(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/done"}, map[string]any{"pause": "true"})
	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			item.Status = task.StatusComplete
			item.FollowingGID = "gid-leader"
		}
	}

	stopped := env.MustCall("aria2.tellStopped", 0, 10).([]map[string]any)
	if len(stopped) == 0 || stopped[0]["gid"] != gid {
		t.Fatalf("tellStopped: %#v", stopped)
	}
	if stopped[0]["following"] != "gid-leader" {
		t.Fatalf("expected following on stopped task, got %#v", stopped[0])
	}
}
