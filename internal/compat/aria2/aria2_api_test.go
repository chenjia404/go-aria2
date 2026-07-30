package aria2

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 以下测试参考 aria2 官方 test/Aria2ApiTest.cc，验证 C API / RPC 层语义一致性。

func TestAria2Api_AddUri(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	uri := "http://localhost/1"
	gid := env.MustGID("aria2.addUri", []any{uri}, map[string]any{"pause": "true"})

	status := env.Status(gid)
	if status["status"] != "paused" {
		t.Fatalf("expected paused status after add with pause, got %#v", status["status"])
	}

	files, ok := status["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("expected one file entry, got %#v", status["files"])
	}
	uris, ok := files[0]["uris"].([]map[string]any)
	if !ok || len(uris) != 1 || uris[0]["uri"] != uri {
		t.Fatalf("unexpected file uris: %#v", files[0]["uris"])
	}

	rawFiles := env.MustCall("aria2.getFiles", gid)
	fileList, ok := rawFiles.([]map[string]any)
	if !ok || len(fileList) != 1 {
		t.Fatalf("getFiles: %#v", rawFiles)
	}
}

func TestAria2Api_AddMetalink(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	raw := env.MustCall("aria2.addMetalink", sampleMetalinkBase64(), map[string]any{"pause": "true"})
	gids := mustStringSlice(t, raw)
	if len(gids) != 2 {
		t.Fatalf("expected 2 gids from metalink, got %#v", raw)
	}
	for _, gid := range gids {
		if env.Status(gid)["status"] != "paused" {
			t.Fatalf("expected paused task for gid %s", gid)
		}
	}
}

func TestAria2Api_AddTorrent(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addTorrent", sampleTorrentBase64(), map[string]any{"pause": "true"})
	status := env.Status(gid)
	if status["status"] != "paused" {
		t.Fatalf("expected paused torrent task, got %#v", status["status"])
	}
	if bittorrent, ok := status["bittorrent"].(map[string]any); !ok || bittorrent == nil {
		t.Fatalf("expected bittorrent section in tellStatus, got %#v", status["bittorrent"])
	}
}

func TestAria2Api_RemovePause(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{MaxConcurrent: 1})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"})
	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			item.Status = task.StatusWaiting
		}
	}

	env.ExpectError("aria2.pause", "0")
	env.MustCall("aria2.pause", gid)
	if env.Status(gid)["status"] != "paused" {
		t.Fatalf("expected paused after pause")
	}

	env.ExpectError("aria2.unpause", "0")
	env.MustCall("aria2.unpause", gid)
	if env.Status(gid)["status"] == "paused" {
		t.Fatalf("expected non-paused after unpause")
	}

	env.ExpectError("aria2.remove", "0")
	env.MustCall("aria2.remove", gid)
	if _, err := env.Call("aria2.tellStatus", gid); err == nil {
		t.Fatalf("expected tellStatus to fail after remove")
	}
}

func TestAria2Api_ChangePosition(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true, MaxConcurrent: 1})
	const n = 10
	gids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		gids = append(gids, env.MustGID("aria2.addUri", []any{"http://localhost/"}))
	}

	env.ExpectError("aria2.changePosition", "0", -2, "POS_CUR")
	if pos := env.MustCall("aria2.changePosition", gids[4], -2, "POS_CUR"); pos != 2 {
		t.Fatalf("POS_CUR expected 2, got %#v", pos)
	}
	if pos := env.MustCall("aria2.changePosition", gids[4], 5, "POS_SET"); pos != 5 {
		t.Fatalf("POS_SET expected 5, got %#v", pos)
	}
	if pos := env.MustCall("aria2.changePosition", gids[4], -2, "POS_END"); pos != 7 {
		t.Fatalf("POS_END expected 7, got %#v", pos)
	}
}

func TestAria2Api_ChangeOption(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	uri := "http://localhost/1"
	gid := env.MustGID("aria2.addUri",
		[]any{uri},
		map[string]any{"dir": "mydownload", "pause": "true"},
	)

	opts := env.Option(gid)
	if opts["dir"] != "mydownload" {
		t.Fatalf("expected dir=mydownload, got %#v", opts)
	}
	if opts["unknown"] != "" {
		t.Fatalf("unknown option should be empty, got %#v", opts["unknown"])
	}

	env.ExpectError("aria2.changeOption", "0", map[string]any{"dir": "newlocation"})
	if raw := env.MustCall("aria2.changeOption", gid, map[string]any{"dir": "newlocation"}); raw != "OK" {
		t.Fatalf("changeOption should return OK, got %#v", raw)
	}
	if env.Option(gid)["dir"] != "newlocation" {
		t.Fatalf("expected updated dir in getOption")
	}
}

func TestAria2Api_ChangeGlobalOption(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	rpcErr := env.ExpectRPCError("aria2.changeGlobalOption", map[string]any{"file-allocation": "none"})
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("file-allocation should be rejected as unimplemented, got %#v", rpcErr)
	}

	env.ExpectError("aria2.changeGlobalOption", map[string]any{"file-allocation": "foo"})
}

func TestAria2Api_InvalidFileAllocationOnAdd(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	badOpt := map[string]any{"file-allocation": "foo", "pause": "true"}

	env.ExpectError("aria2.addUri", []any{"http://localhost/1"}, badOpt)
	env.ExpectError("aria2.addTorrent", sampleTorrentBase64(), badOpt)
	env.ExpectError("aria2.addMetalink", sampleMetalinkBase64(), badOpt)
}

func TestAria2Api_ChangeOptionRejectsBadFileAllocation(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri",
		[]any{"http://localhost/1"},
		map[string]any{"dir": "mydownload", "pause": "true"},
	)

	env.ExpectError("aria2.changeOption", gid, map[string]any{"file-allocation": "foo"})
	rpcErr := env.ExpectRPCError("aria2.changeOption", gid, map[string]any{"file-allocation": "none"})
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("file-allocation none should be unimplemented, got %#v", rpcErr)
	}
}

func TestAria2Api_HiddenOptionsNotReturned(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	env := newRPCTestEnv(t, manager.Options{
		GlobalOptions: map[string]string{
			"dir":               saveDir,
			"startup-idle-time": "60",
		},
	})
	if got := mustStringMap(t, env.MustCall("aria2.getGlobalOption"))["startup-idle-time"]; got != "" {
		t.Fatalf("hidden global option should not be returned, got %q", got)
	}

	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"pause": "true"})
	for _, item := range env.Driver.tasks {
		if item.GID == gid {
			if item.Options == nil {
				item.Options = map[string]string{}
			}
			item.Options["startup-idle-time"] = "60"
		}
	}
	if got := env.Option(gid)["startup-idle-time"]; got != "" {
		t.Fatalf("hidden task option should not be returned, got %q", got)
	}
}

func TestAria2Api_DownloadResultErrorStatus(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri",
		[]any{"http://example.org/timeout"},
		map[string]any{"dir": "mydownload", "pause": "true"},
	)

	for _, item := range env.Driver.tasks {
		item.Status = task.StatusError
		item.ErrorCode = "19"
		item.ErrorMessage = "timeout"
		item.Options["dir"] = "mydownload"
	}

	status := env.Status(gid)
	if status["status"] != "error" {
		t.Fatalf("expected error status, got %#v", status["status"])
	}
	if status["errorCode"] != "19" {
		t.Fatalf("unexpected errorCode: %#v", status["errorCode"])
	}
	if env.Option(gid)["dir"] != "mydownload" {
		t.Fatalf("expected dir preserved on error task")
	}
}

func TestAria2Api_TellActiveWaitingStopped(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true, MaxConcurrent: 1})
	waitingGID := env.MustGID("aria2.addUri", []any{"http://localhost/waiting"})
	activeGID := env.MustGID("aria2.addUri", []any{"http://localhost/active"})
	for _, item := range env.Driver.tasks {
		if item.GID == activeGID {
			item.Status = task.StatusActive
		}
	}

	active := env.MustCall("aria2.tellActive", 0, 10).([]map[string]any)
	if len(active) != 1 || active[0]["gid"] != activeGID {
		t.Fatalf("tellActive: %#v", active)
	}

	waiting := env.MustCall("aria2.tellWaiting", 0, 10).([]map[string]any)
	if len(waiting) == 0 || waiting[0]["gid"] != waitingGID {
		t.Fatalf("tellWaiting: %#v", waiting)
	}

	for _, item := range env.Driver.tasks {
		if item.GID == waitingGID {
			item.Status = task.StatusComplete
		}
	}
	stopped := env.MustCall("aria2.tellStopped", 0, 10).([]map[string]any)
	if len(stopped) == 0 || stopped[0]["gid"] != waitingGID {
		t.Fatalf("tellStopped: %#v", stopped)
	}
}

func TestAria2Api_GetUrisAndChangeUri(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	uri := "http://example.com/a"
	gid := env.MustGID("aria2.addUri", []any{uri}, map[string]any{"pause": "true"})

	uris := env.MustCall("aria2.getUris", gid).([]map[string]any)
	if len(uris) != 1 || uris[0]["uri"] != uri {
		t.Fatalf("getUris: %#v", uris)
	}

	env.MustCall("aria2.changeUri", gid, 1, []any{uri}, []any{"http://example.com/b"}, 0)
	uris = env.MustCall("aria2.getUris", gid).([]map[string]any)
	if len(uris) != 1 || uris[0]["uri"] != "http://example.com/b" {
		t.Fatalf("changeUri: %#v", uris)
	}
}

func TestAria2Api_GetGlobalStatFields(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true})
	env.MustGID("aria2.addUri", []any{"http://localhost/1"})
	for _, item := range env.Driver.tasks {
		item.Status = task.StatusActive
	}

	stat := env.MustCall("aria2.getGlobalStat").(map[string]any)
	for _, key := range []string{"numActive", "numWaiting", "numStopped", "numStoppedTotal", "downloadSpeed", "uploadSpeed"} {
		if stat[key] == nil {
			t.Fatalf("missing %s in global stat: %#v", key, stat)
		}
	}
}

func TestAria2Api_SystemMethodsListed(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	methods := mustStringSlice(t, env.MustCall("system.listMethods"))
	notifications := mustStringSlice(t, env.MustCall("system.listNotifications"))

	requiredMethods := []string{
		"aria2.addUri", "aria2.addTorrent", "aria2.addMetalink",
		"aria2.remove", "aria2.pause", "aria2.unpause",
		"aria2.tellStatus", "aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped",
		"aria2.getOption", "aria2.changeOption", "aria2.changePosition", "aria2.changeUri",
		"aria2.getGlobalStat", "aria2.forceShutdown",
		"system.multicall",
	}
	for _, method := range requiredMethods {
		if !containsString(methods, method) {
			t.Fatalf("missing method %s in %#v", method, methods)
		}
	}
	if len(notifications) == 0 {
		t.Fatalf("expected notifications")
	}
}

func TestAria2Api_AuthWithSecret(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	env.Service = NewService(env.Service.manager, "secret-token")

	if _, err := env.Call("aria2.ping"); err == nil {
		t.Fatalf("expected missing token error")
	}
	if raw := env.MustCall("aria2.ping", "token:secret-token"); raw != "pong" {
		t.Fatalf("expected pong, got %#v", raw)
	}
	if _, err := env.Call("system.listMethods"); err != nil {
		t.Fatalf("listMethods should work without token: %v", err)
	}
}

func TestAria2JsonRPC_WireFormat(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	server := jsonrpc.NewServer(env.Service, jsonrpc.Options{})
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

	call := func(body string) map[string]any {
		t.Helper()
		resp, err := http.Post(ts.URL, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	ping := call(`{"jsonrpc":"2.0","id":"1","method":"aria2.ping","params":[]}`)
	if ping["error"] != nil {
		t.Fatalf("ping error: %#v", ping["error"])
	}
	if ping["result"] != "pong" {
		t.Fatalf("ping result: %#v", ping["result"])
	}

	add := call(`{"jsonrpc":"2.0","id":"2","method":"aria2.addUri","params":[["http://localhost/1"],{"pause":"true"}]}`)
	if add["error"] != nil {
		t.Fatalf("addUri error: %#v", add["error"])
	}
	gid, ok := add["result"].(string)
	if !ok || gid == "" {
		t.Fatalf("addUri result: %#v", add["result"])
	}

	statusBody := `{"jsonrpc":"2.0","id":"3","method":"aria2.tellStatus","params":["` + gid + `"]}`
	status := call(statusBody)
	if status["error"] != nil {
		t.Fatalf("tellStatus error: %#v", status["error"])
	}
	result, ok := status["result"].(map[string]any)
	if !ok || result["gid"] != gid {
		t.Fatalf("tellStatus result: %#v", status["result"])
	}
}

func TestAria2JsonRPC_BatchRequest(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	server := jsonrpc.NewServer(env.Service, jsonrpc.Options{})
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

	body := `[{"jsonrpc":"2.0","id":"1","method":"aria2.ping","params":[]},{"jsonrpc":"2.0","id":"2","method":"aria2.getVersion","params":[]}]`
	resp, err := http.Post(ts.URL, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	var batch []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 responses, got %#v", batch)
	}
	if batch[0]["result"] != "pong" {
		t.Fatalf("first result: %#v", batch[0])
	}
	version, ok := batch[1]["result"].(map[string]any)
	if !ok || version["version"] == nil {
		t.Fatalf("second result: %#v", batch[1])
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// 确保 Service 满足 jsonrpc.Handler，便于在测试中直接挂载 HTTP Server。
var _ jsonrpc.Handler = (*Service)(nil)

func TestAria2ServiceImplementsHandler(t *testing.T) {
	t.Parallel()
	var handler jsonrpc.Handler = NewService(manager.New(manager.Options{DefaultDir: t.TempDir()}), "")
	_, err := handler.Invoke(context.Background(), "aria2.ping", nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}
