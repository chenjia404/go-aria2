package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// 参考 aria2 RpcMethodTest.cc::testGetOption 与 getUris 语义。

func TestRpcMethod_GetOption_OmitsPause(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{DefaultDir: saveDir})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{
		"dir":   saveDir,
		"pause": "true",
		"out":   "sample.bin",
	})

	opts := env.Option(gid)
	if opts["dir"] != saveDir || opts["out"] != "sample.bin" {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if _, ok := opts["pause"]; ok {
		t.Fatalf("getOption should not include pause: %#v", opts)
	}
}

func TestRpcMethod_GetUris_WaitingStatus(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	uri := "http://example.com/a"
	gid := env.MustGID("aria2.addUri", []any{uri}, map[string]any{"pause": "true"})

	uris := env.MustCall("aria2.getUris", gid).([]map[string]any)
	if len(uris) != 1 || uris[0]["uri"] != uri || uris[0]["status"] != "waiting" {
		t.Fatalf("getUris: %#v", uris)
	}

	status := env.Status(gid)
	files := status["files"].([]map[string]any)
	fileURIs := files[0]["uris"].([]map[string]any)
	if len(fileURIs) != 1 || fileURIs[0]["status"] != "waiting" {
		t.Fatalf("tellStatus file uris: %#v", fileURIs)
	}
}

func TestRpcMethod_GetPeers_EmptyForHTTP(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})

	peers := env.MustCall("aria2.getPeers", gid).([]map[string]any)
	if len(peers) != 0 {
		t.Fatalf("HTTP getPeers should be empty: %#v", peers)
	}
}

func TestUriStatusForTask(t *testing.T) {
	t.Parallel()

	if uriStatusForTask(task.StatusPaused) != "waiting" {
		t.Fatal("paused should map to waiting")
	}
	if uriStatusForTask(task.StatusActive) != "used" {
		t.Fatal("active should map to used")
	}
}
