//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 与真实 aria2 daemon 并行调用 go-aria2，比对 RPC 响应结构与关键字段。

func TestCompareAria2_PingAndVersion(t *testing.T) {
	work := t.TempDir()
	secret := "compare-secret"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	// 官方 aria2 无 aria2.ping，仅验证 go-aria2 扩展。
	if got := mustString(t, mustCall(t, ctx, goAria, "aria2.ping"), "go ping"); got != "pong" {
		t.Fatalf("go-aria2 ping: %q", got)
	}
	if _, err := aria2.call(ctx, "aria2.ping", nil); err == nil {
		t.Fatal("aria2 should not implement aria2.ping")
	}

	aria2Ver := decodeJSON[map[string]any](t, first(rawCall(t, ctx, aria2, goAria, "aria2.getVersion", nil)), "aria2 version")
	goVer := decodeJSON[map[string]any](t, second(rawCall(t, ctx, aria2, goAria, "aria2.getVersion", nil)), "go version")

	for _, key := range []string{"version", "enabledFeatures"} {
		if _, ok := goVer[key]; !ok {
			t.Fatalf("go-aria2 getVersion missing %q: %#v", key, goVer)
		}
		if _, ok := aria2Ver[key]; !ok {
			t.Fatalf("aria2 getVersion missing %q: %#v", key, aria2Ver)
		}
	}
}

func TestCompareAria2_ListMethods(t *testing.T) {
	work := t.TempDir()
	secret := "compare-methods"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	aria2Methods := decodeJSON[[]string](t, first(rawCall(t, ctx, aria2, goAria, "system.listMethods", nil)), "aria2 methods")
	goMethods := decodeJSON[[]string](t, second(rawCall(t, ctx, aria2, goAria, "system.listMethods", nil)), "go methods")

	required := []string{
		"aria2.addUri", "aria2.addTorrent", "aria2.addMetalink",
		"aria2.remove", "aria2.pause", "aria2.unpause",
		"aria2.tellStatus", "aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped",
		"aria2.getOption", "aria2.changeOption", "aria2.changePosition",
		"aria2.getGlobalOption", "aria2.changeGlobalOption", "aria2.getGlobalStat",
		"aria2.getVersion", "system.multicall",
	}
	for _, method := range required {
		if !contains(aria2Methods, method) {
			t.Fatalf("aria2 missing method %s", method)
		}
		if !contains(goMethods, method) {
			t.Fatalf("go-aria2 missing method %s", method)
		}
	}
	if !contains(goMethods, "aria2.ping") {
		t.Fatal("go-aria2 should expose aria2.ping")
	}
}

func TestCompareAria2_AddUriTellStatus(t *testing.T) {
	work := t.TempDir()
	secret := "compare-adduri"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("integration payload"))
	}))
	t.Cleanup(ts.Close)

	opts := map[string]any{"pause": "true", "out": "sample.bin"}
	params := []any{[]any{ts.URL + "/file.bin"}, opts}

	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 addUri")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go addUri")

	aria2Status := decodeJSON[map[string]any](t, mustCall(t, ctx, aria2, "aria2.tellStatus", aria2GID), "aria2 status")
	goStatus := decodeJSON[map[string]any](t, mustCall(t, ctx, goAria, "aria2.tellStatus", goGID), "go status")

	for _, key := range []string{"gid", "status", "totalLength", "completedLength", "downloadSpeed", "uploadSpeed", "files"} {
		if aria2Status[key] == nil {
			t.Fatalf("aria2 tellStatus missing %q", key)
		}
		if goStatus[key] == nil {
			t.Fatalf("go-aria2 tellStatus missing %q", key)
		}
	}
	if aria2Status["status"] != goStatus["status"] {
		t.Fatalf("status mismatch: aria2=%v go=%v", aria2Status["status"], goStatus["status"])
	}

	aria2Opts := decodeJSON[map[string]string](t, mustCall(t, ctx, aria2, "aria2.getOption", aria2GID), "aria2 option")
	goOpts := decodeJSON[map[string]string](t, mustCall(t, ctx, goAria, "aria2.getOption", goGID), "go option")
	if aria2Opts["out"] != goOpts["out"] {
		t.Fatalf("out mismatch: aria2=%q go=%q", aria2Opts["out"], goOpts["out"])
	}
}

func TestCompareAria2_ChangeOptionReturnsOK(t *testing.T) {
	work := t.TempDir()
	secret := "compare-changeopt"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	gidParams := []any{[]any{"http://example.com/sample.bin"}, map[string]any{"pause": "true"}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", gidParams)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", gidParams)), "go gid")

	aria2OK := mustString(t, mustCall(t, ctx, aria2, "aria2.changeOption", aria2GID, map[string]any{"max-download-limit": "1024"}), "aria2 changeOption")
	goOK := mustString(t, mustCall(t, ctx, goAria, "aria2.changeOption", goGID, map[string]any{"max-download-limit": "1024"}), "go changeOption")
	if aria2OK != "OK" || goOK != "OK" {
		t.Fatalf("changeOption: aria2=%q go=%q", aria2OK, goOK)
	}
}

func TestCompareAria2_PauseUnpauseRemove(t *testing.T) {
	work := t.TempDir()
	secret := "compare-lifecycle"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/lifecycle.bin"}, map[string]any{}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go gid")

	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := aria2GID
		if d.name == "go-aria2" {
			gid = goGID
		}
		if got := mustString(t, mustCall(t, ctx, d, "aria2.pause", gid), d.name+" pause"); got != gid {
			t.Fatalf("%s pause returned %q", d.name, got)
		}
		status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" paused")
		if status["status"] != "paused" {
			t.Fatalf("%s expected paused, got %v", d.name, status["status"])
		}
		if got := mustString(t, mustCall(t, ctx, d, "aria2.unpause", gid), d.name+" unpause"); got != gid {
			t.Fatalf("%s unpause returned %q", d.name, got)
		}
		if got := mustString(t, mustCall(t, ctx, d, "aria2.remove", gid), d.name+" remove"); got != gid {
			t.Fatalf("%s remove returned %q", d.name, got)
		}
		raw, err := d.call(ctx, "aria2.tellStatus", []any{gid})
		if err == nil {
			status := decodeJSON[map[string]any](t, raw, d.name+" after remove")
			if status["status"] != "removed" {
				t.Fatalf("%s tellStatus after remove: expected error or status=removed, got %#v", d.name, status)
			}
		}
	}
}

func TestCompareAria2_GlobalStatAndMulticall(t *testing.T) {
	work := t.TempDir()
	secret := "compare-global"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	aria2Stat := decodeJSON[map[string]any](t, first(rawCall(t, ctx, aria2, goAria, "aria2.getGlobalStat", nil)), "aria2 stat")
	goStat := decodeJSON[map[string]any](t, second(rawCall(t, ctx, aria2, goAria, "aria2.getGlobalStat", nil)), "go stat")
	for _, key := range []string{"numActive", "numWaiting", "numStopped", "numStoppedTotal", "downloadSpeed", "uploadSpeed"} {
		if aria2Stat[key] == nil || goStat[key] == nil {
			t.Fatalf("missing global stat key %q: aria2=%#v go=%#v", key, aria2Stat, goStat)
		}
	}

	multiCalls := []any{
		map[string]any{"methodName": "aria2.getVersion", "params": []any{"token:" + secret}},
		map[string]any{"methodName": "aria2.getGlobalStat", "params": []any{"token:" + secret}},
	}
	aria2Multi := decodeJSON[[]any](t, mustCallSlice(t, ctx, aria2, "system.multicall", []any{multiCalls}), "aria2 multicall")
	goMulti := decodeJSON[[]any](t, mustCallSlice(t, ctx, goAria, "system.multicall", []any{multiCalls}), "go multicall")
	if len(aria2Multi) != 2 || len(goMulti) != 2 {
		t.Fatalf("multicall length mismatch: aria2=%d go=%d", len(aria2Multi), len(goMulti))
	}
}

func TestCompareAria2_ChangePosition(t *testing.T) {
	work := t.TempDir()
	secret := "compare-position"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	const n = 5
	aria2GIDs := make([]string, 0, n)
	goGIDs := make([]string, 0, n)
	for i := range n {
		params := []any{[]any{fmt.Sprintf("http://example.com/queue-%d.bin", i)}, map[string]any{"pause": "true"}}
		aria2GIDs = append(aria2GIDs, mustString(t, mustCallSlice(t, ctx, aria2, "aria2.addUri", params), "aria2 add"))
		goGIDs = append(goGIDs, mustString(t, mustCallSlice(t, ctx, goAria, "aria2.addUri", params), "go add"))
	}

	compareIntResult(t, ctx, aria2, goAria, "aria2.changePosition", aria2GIDs[2], -1, "POS_CUR", goGIDs[2])
	compareIntResult(t, ctx, aria2, goAria, "aria2.changePosition", aria2GIDs[2], 0, "POS_SET", goGIDs[2])
}

func TestCompareAria2_PauseAllAndForcePauseAll(t *testing.T) {
	work := t.TempDir()
	secret := "compare-pauseall"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/pauseall.bin"}, map[string]any{}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if got := mustString(t, mustCall(t, ctx, d, "aria2.pauseAll"), d.name+" pauseAll"); got != "OK" {
			t.Fatalf("%s pauseAll returned %q", d.name, got)
		}
		status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" paused")
		if status["status"] != "paused" {
			t.Fatalf("%s expected paused after pauseAll, got %v", d.name, status["status"])
		}
		if got := mustString(t, mustCall(t, ctx, d, "aria2.forcePauseAll"), d.name+" forcePauseAll"); got != "OK" {
			t.Fatalf("%s forcePauseAll returned %q", d.name, got)
		}
	}
}

func TestCompareAria2_TellWaiting(t *testing.T) {
	work := t.TempDir()
	secret := "compare-waiting"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	for i := range 3 {
		params := []any{[]any{fmt.Sprintf("http://example.com/wait-%d.bin", i)}, map[string]any{"pause": "true"}}
		mustCallSlice(t, ctx, aria2, "aria2.addUri", params)
		mustCallSlice(t, ctx, goAria, "aria2.addUri", params)
	}

	aria2Waiting := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.tellWaiting", 0, 10), "aria2 waiting")
	goWaiting := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.tellWaiting", 0, 10), "go waiting")
	if len(aria2Waiting) < 3 || len(goWaiting) < 3 {
		t.Fatalf("expected at least 3 waiting: aria2=%d go=%d", len(aria2Waiting), len(goWaiting))
	}
}

func rawCall(t *testing.T, ctx context.Context, aria2, goAria *daemonHandle, method string, params []any) (json.RawMessage, json.RawMessage) {
	t.Helper()
	a, err := aria2.call(ctx, method, params)
	if err != nil {
		t.Fatalf("aria2 %s: %v\nlog:\n%s", method, err, aria2.log.String())
	}
	g, err := goAria.call(ctx, method, params)
	if err != nil {
		t.Fatalf("go-aria2 %s: %v\nlog:\n%s", method, err, goAria.log.String())
	}
	return a, g
}

func mustCallSlice(t *testing.T, ctx context.Context, d *daemonHandle, method string, params []any) json.RawMessage {
	t.Helper()
	raw, err := d.call(ctx, method, params)
	if err != nil {
		t.Fatalf("%s %s: %v\nlog:\n%s", d.name, method, err, d.log.String())
	}
	return raw
}

func mustCall(t *testing.T, ctx context.Context, d *daemonHandle, method string, params ...any) json.RawMessage {
	t.Helper()
	raw, err := d.call(ctx, method, params)
	if err != nil {
		t.Fatalf("%s %s: %v\nlog:\n%s", d.name, method, err, d.log.String())
	}
	return raw
}

func first(a, _ json.RawMessage) json.RawMessage  { return a }
func second(_, b json.RawMessage) json.RawMessage { return b }

func compareIntResult(t *testing.T, ctx context.Context, aria2, goAria *daemonHandle, method, aria2GID string, pos int, how, goGID string) {
	t.Helper()
	aRaw := mustCall(t, ctx, aria2, method, aria2GID, pos, how)
	gRaw := mustCall(t, ctx, goAria, method, goGID, pos, how)

	var aVal, gVal float64
	if err := json.Unmarshal(aRaw, &aVal); err != nil {
		t.Fatalf("aria2 %s result: %s (%v)", method, string(aRaw), err)
	}
	if err := json.Unmarshal(gRaw, &gVal); err != nil {
		t.Fatalf("go-aria2 %s result: %s (%v)", method, string(gRaw), err)
	}
	if int(aVal) != int(gVal) {
		t.Fatalf("%s position mismatch: aria2=%d go=%d", method, int(aVal), int(gVal))
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
