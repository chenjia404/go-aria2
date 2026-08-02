//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 与真实 aria2 daemon 并行调用 go-aria2，比对 RPC 响应结构与关键字段。

const (
	sampleTorrentB64 = "ZDg6YW5ub3VuY2UxNDpodHRwOi8vdHJhY2tlcjEzOmNyZWF0aW9uIGRhdGVpMTcxMjEyMzQ1NmU0OmluZm9kNjpsZW5ndGhpMTIzZTQ6bmFtZTg6dGVzdC5iaW4xMjpwaWVjZSBsZW5ndGhpMjYyMTQ0ZTY6cGllY2VzMjA6MTIzNDU2Nzg5MDEyMzQ1Njc4OTBlZQ=="
	sampleMetalinkB64 = "PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4KPG1ldGFsaW5rIHhtbG5zPSJ1cm46aWV0ZjpwYXJhbXM6eG1sOm5zOm1ldGFsaW5rIj4KICA8ZmlsZSBuYW1lPSJhLmJpbiI+CiAgICA8dXJsPmh0dHA6Ly9leGFtcGxlLmNvbS9hLmJpbjwvdXJsPgogIDwvZmlsZT4KICA8ZmlsZSBuYW1lPSJiLmJpbiI+CiAgICA8dXJsPmh0dHA6Ly9leGFtcGxlLmNvbS9iLmJpbjwvdXJsPgogIDwvZmlsZT4KPC9tZXRhbGluaz4="
)

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
		"aria2.remove", "aria2.pause", "aria2.unpause", "aria2.pauseAll", "aria2.unpauseAll",
		"aria2.removeDownloadResult", "aria2.purgeDownloadResult",
		"aria2.tellStatus", "aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped",
		"aria2.getOption", "aria2.changeOption", "aria2.changePosition", "aria2.changeUri",
		"aria2.getFiles", "aria2.getPeers", "aria2.getServers", "aria2.getUris",
		"aria2.getGlobalOption", "aria2.changeGlobalOption", "aria2.getGlobalStat",
		"aria2.getVersion", "aria2.saveSession", "system.multicall",
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

	params := []any{[]any{"http://example.com/pauseall.bin"}, map[string]any{"pause": "true"}}
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

func TestCompareAria2_AddTorrentTellStatus(t *testing.T) {
	work := t.TempDir()
	secret := "compare-torrent"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{sampleTorrentB64, []any{}, map[string]any{"pause": "true"}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addTorrent", params)), "aria2 addTorrent")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addTorrent", params)), "go addTorrent")

	aria2Status := decodeJSON[map[string]any](t, mustCall(t, ctx, aria2, "aria2.tellStatus", aria2GID), "aria2 status")
	goStatus := decodeJSON[map[string]any](t, mustCall(t, ctx, goAria, "aria2.tellStatus", goGID), "go status")

	for _, key := range []string{"gid", "status", "bittorrent"} {
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
}

func TestCompareAria2_AddMetalink(t *testing.T) {
	work := t.TempDir()
	secret := "compare-metalink"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{sampleMetalinkB64, map[string]any{"pause": "true"}}
	aria2GIDs := decodeJSON[[]string](t, first(rawCall(t, ctx, aria2, goAria, "aria2.addMetalink", params)), "aria2 metalink")
	goGIDs := decodeJSON[[]string](t, second(rawCall(t, ctx, aria2, goAria, "aria2.addMetalink", params)), "go metalink")
	if len(aria2GIDs) != 2 || len(goGIDs) != 2 {
		t.Fatalf("expected 2 gids: aria2=%#v go=%#v", aria2GIDs, goGIDs)
	}
}

func TestCompareAria2_UnpauseAll(t *testing.T) {
	work := t.TempDir()
	secret := "compare-unpauseall"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/unpauseall.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if got := mustString(t, mustCall(t, ctx, d, "aria2.pauseAll"), d.name+" pauseAll"); got != "OK" {
			t.Fatalf("%s pauseAll returned %q", d.name, got)
		}
		status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" paused")
		if status["status"] != "paused" {
			t.Fatalf("%s expected paused after pauseAll, got %v", d.name, status["status"])
		}
		if got := mustString(t, mustCall(t, ctx, d, "aria2.unpauseAll"), d.name+" unpauseAll"); got != "OK" {
			t.Fatalf("%s unpauseAll returned %q", d.name, got)
		}
		status = decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" unpaused")
		if status["status"] != "active" {
			t.Fatalf("%s expected active after unpauseAll, got %v", d.name, status["status"])
		}
	}
}

func TestCompareAria2_TellStopped(t *testing.T) {
	work := t.TempDir()
	secret := "compare-stopped"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/stopped.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if got := mustString(t, mustCall(t, ctx, d, "aria2.remove", gid), d.name+" remove"); got != gid {
			t.Fatalf("%s remove returned %q", d.name, got)
		}
		// aria2 在 remove 后会把任务从内存移除，tellStopped 通常为空（与官方手册一致）。
		stopped := decodeJSON[[]map[string]any](t, mustCall(t, ctx, d, "aria2.tellStopped", 0, 10), d.name+" tellStopped")
		if len(stopped) != 0 {
			t.Fatalf("%s tellStopped should be empty after remove, got %#v", d.name, stopped)
		}
	}
}

func TestCompareAria2_PurgeDownloadResult(t *testing.T) {
	work := t.TempDir()
	secret := "compare-purge"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	for _, d := range []*daemonHandle{aria2, goAria} {
		if got := mustString(t, mustCall(t, ctx, d, "aria2.purgeDownloadResult"), d.name+" purgeDownloadResult"); got != "OK" {
			t.Fatalf("%s purgeDownloadResult returned %q", d.name, got)
		}
		stopped := decodeJSON[[]map[string]any](t, mustCall(t, ctx, d, "aria2.tellStopped", 0, 10), d.name+" tellStopped after purge")
		if len(stopped) != 0 {
			t.Fatalf("%s tellStopped after purge: want empty, got %#v", d.name, stopped)
		}
	}
}

func TestCompareAria2_GetSessionInfoAndListNotifications(t *testing.T) {
	work := t.TempDir()
	secret := "compare-session"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	aria2Info := decodeJSON[map[string]any](t, first(rawCall(t, ctx, aria2, goAria, "aria2.getSessionInfo", nil)), "aria2 session")
	goInfo := decodeJSON[map[string]any](t, second(rawCall(t, ctx, aria2, goAria, "aria2.getSessionInfo", nil)), "go session")
	for _, key := range []string{"sessionId"} {
		if aria2Info[key] == nil || aria2Info[key] == "" {
			t.Fatalf("aria2 getSessionInfo missing %q: %#v", key, aria2Info)
		}
		if goInfo[key] == nil || goInfo[key] == "" {
			t.Fatalf("go-aria2 getSessionInfo missing %q: %#v", key, goInfo)
		}
	}

	aria2Notes := decodeJSON[[]string](t, first(rawCall(t, ctx, aria2, goAria, "system.listNotifications", nil)), "aria2 notifications")
	goNotes := decodeJSON[[]string](t, second(rawCall(t, ctx, aria2, goAria, "system.listNotifications", nil)), "go notifications")
	for _, method := range []string{
		"aria2.onDownloadStart",
		"aria2.onDownloadComplete",
		"aria2.onDownloadError",
	} {
		if !contains(aria2Notes, method) {
			t.Fatalf("aria2 missing notification %s", method)
		}
		if !contains(goNotes, method) {
			t.Fatalf("go-aria2 missing notification %s", method)
		}
	}
}

func TestCompareAria2_GetOption(t *testing.T) {
	work := t.TempDir()
	secret := "compare-getoption"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/getoption.bin"}, map[string]any{"pause": "true", "out": "getoption.bin"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		opts := decodeJSON[map[string]string](t, mustCall(t, ctx, d, "aria2.getOption", gid), d.name+" getOption")
		if opts["out"] != "getoption.bin" {
			t.Fatalf("%s out mismatch: %#v", d.name, opts)
		}
		status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" tellStatus")
		if status["status"] != "paused" {
			t.Fatalf("%s expected paused status, got %#v", d.name, status["status"])
		}
	}
}

func TestCompareAria2_ShutdownReturnsOK(t *testing.T) {
	t.Run("aria2", func(t *testing.T) {
		work := t.TempDir()
		d := startAria2Daemon(t, work, freeListenPort(t), "compare-shutdown-aria2")
		ctx := context.Background()
		if got := mustString(t, mustCall(t, ctx, d, "aria2.shutdown"), "aria2 shutdown"); got != "OK" {
			t.Fatalf("aria2 shutdown returned %q", got)
		}
	})
	t.Run("go-aria2", func(t *testing.T) {
		work := t.TempDir()
		d := startGoAria2Daemon(t, work, freeListenPort(t), "compare-shutdown-go")
		ctx := context.Background()
		if got := mustString(t, mustCall(t, ctx, d, "aria2.shutdown"), "go shutdown"); got != "OK" {
			t.Fatalf("go-aria2 shutdown returned %q", got)
		}
	})
}

func TestCompareAria2_RemoveDownloadResult(t *testing.T) {
	work := t.TempDir()
	secret := "compare-remove-result"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/remove-result.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		// waiting/paused 任务调用 removeDownloadResult 应失败（aria2 与 go-aria2 一致）。
		if _, err := d.call(ctx, "aria2.removeDownloadResult", []any{gid}); err == nil {
			t.Fatalf("%s removeDownloadResult should fail for paused task", d.name)
		}
	}
}

func TestCompareAria2_GetFiles(t *testing.T) {
	work := t.TempDir()
	secret := "compare-getfiles"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/getfiles.bin"}, map[string]any{"pause": "true", "out": "getfiles.bin"}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 addUri")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go addUri")

	aria2Files := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.getFiles", aria2GID), "aria2 getFiles")
	goFiles := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.getFiles", goGID), "go getFiles")
	if len(aria2Files) != 1 || len(goFiles) != 1 {
		t.Fatalf("expected one file entry: aria2=%#v go=%#v", aria2Files, goFiles)
	}
	for _, key := range []string{"index", "path", "length", "completedLength", "selected", "uris"} {
		if aria2Files[0][key] == nil {
			t.Fatalf("aria2 getFiles missing %q: %#v", key, aria2Files[0])
		}
		if goFiles[0][key] == nil {
			t.Fatalf("go-aria2 getFiles missing %q: %#v", key, goFiles[0])
		}
	}
	if aria2Files[0]["index"] != goFiles[0]["index"] {
		t.Fatalf("index mismatch: aria2=%v go=%v", aria2Files[0]["index"], goFiles[0]["index"])
	}
}

func TestCompareAria2_GetPeersAndGetUris(t *testing.T) {
	work := t.TempDir()
	secret := "compare-peers-uris"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/peers-uris.bin"}, map[string]any{"pause": "true"}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 addUri")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go addUri")

	aria2Peers := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.getPeers", aria2GID), "aria2 getPeers")
	goPeers := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.getPeers", goGID), "go getPeers")
	if len(aria2Peers) != 0 || len(goPeers) != 0 {
		t.Fatalf("HTTP task getPeers should be empty: aria2=%#v go=%#v", aria2Peers, goPeers)
	}

	aria2URIs := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.getUris", aria2GID), "aria2 getUris")
	goURIs := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.getUris", goGID), "go getUris")
	if len(aria2URIs) != 1 || len(goURIs) != 1 {
		t.Fatalf("expected one uri entry: aria2=%#v go=%#v", aria2URIs, goURIs)
	}
	if aria2URIs[0]["status"] != "waiting" || goURIs[0]["status"] != "waiting" {
		t.Fatalf("paused task uri status should be waiting: aria2=%#v go=%#v", aria2URIs[0], goURIs[0])
	}
}

func TestCompareAria2_GetOptionOmitsPause(t *testing.T) {
	work := t.TempDir()
	secret := "compare-getoption-pause"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/no-pause-opt.bin"}, map[string]any{"pause": "true", "out": "no-pause-opt.bin"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		opts := decodeJSON[map[string]string](t, mustCall(t, ctx, d, "aria2.getOption", gid), d.name+" getOption")
		if opts["out"] != "no-pause-opt.bin" {
			t.Fatalf("%s out mismatch: %#v", d.name, opts)
		}
		if _, ok := opts["pause"]; ok {
			t.Fatalf("%s getOption should not include pause: %#v", d.name, opts)
		}
	}
}

func TestCompareAria2_TellActiveEmptyWhenPaused(t *testing.T) {
	work := t.TempDir()
	secret := "compare-tellactive-empty"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/paused-active.bin"}, map[string]any{"pause": "true"}}
	mustCallSlice(t, ctx, aria2, "aria2.addUri", params)
	mustCallSlice(t, ctx, goAria, "aria2.addUri", params)

	aria2Active := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.tellActive"), "aria2 tellActive")
	goActive := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.tellActive"), "go tellActive")
	if len(aria2Active) != 0 || len(goActive) != 0 {
		t.Fatalf("tellActive should be empty for paused tasks: aria2=%#v go=%#v", aria2Active, goActive)
	}
}

func TestCompareAria2_GetServersFailsForNonActive(t *testing.T) {
	work := t.TempDir()
	secret := "compare-getservers-nonactive"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/nonactive-servers.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if _, err := d.call(ctx, "aria2.getServers", []any{gid}); err == nil {
			t.Fatalf("%s getServers should fail for paused task", d.name)
		}
	}
}

func TestCompareAria2_BTGetPeersEmptyWhenPaused(t *testing.T) {
	work := t.TempDir()
	secret := "compare-bt-peers"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{sampleTorrentB64, []any{}, map[string]any{"pause": "true"}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addTorrent", params)), "aria2 addTorrent")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addTorrent", params)), "go addTorrent")

	aria2Peers := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.getPeers", aria2GID), "aria2 getPeers")
	goPeers := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.getPeers", goGID), "go getPeers")
	if len(aria2Peers) != 0 || len(goPeers) != 0 {
		t.Fatalf("paused BT getPeers should be empty: aria2=%#v go=%#v", aria2Peers, goPeers)
	}
}

func TestCompareAria2_PauseRejectsAlreadyPaused(t *testing.T) {
	work := t.TempDir()
	secret := "compare-pause-reject"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/paused.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if _, err := d.call(ctx, "aria2.pause", []any{gid}); err == nil {
			t.Fatalf("%s pause should fail for already paused task", d.name)
		}
		if _, err := d.call(ctx, "aria2.forcePause", []any{gid}); err == nil {
			t.Fatalf("%s forcePause should fail for already paused task", d.name)
		}
	}
}

func TestCompareAria2_UnpausePausedTask(t *testing.T) {
	work := t.TempDir()
	secret := "compare-unpause-paused"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/unpause-paused.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if got := mustString(t, mustCall(t, ctx, d, "aria2.unpause", gid), d.name+" unpause"); got != gid {
			t.Fatalf("%s unpause returned %q", d.name, got)
		}
	}
}

func TestCompareAria2_UnpauseRejectsNonPaused(t *testing.T) {
	work := t.TempDir()
	secret := "compare-unpause-reject"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	ts := startSlowDownloadServer(t)
	slowURL := ts.URL + "/slow.bin"
	for _, d := range []*daemonHandle{aria2, goAria} {
		mustCall(t, ctx, d, "aria2.changeGlobalOption", map[string]any{"max-concurrent-downloads": "1"})
	}

	pauseParams := []any{[]any{slowURL}, map[string]any{"pause": "true"}}
	aria2Active := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", pauseParams)), "aria2 active gid")
	goActive := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", pauseParams)), "go active gid")
	mustCall(t, ctx, aria2, "aria2.unpause", aria2Active)
	mustCall(t, ctx, goAria, "aria2.unpause", goActive)

	waitUntilTaskStatus(t, ctx, aria2, aria2Active, "active", 20*time.Second)
	waitUntilTaskStatus(t, ctx, goAria, goActive, "active", 20*time.Second)

	aria2Waiting := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", []any{[]any{slowURL}})), "aria2 waiting gid")
	goWaiting := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", []any{[]any{slowURL}})), "go waiting gid")
	for _, pair := range []struct {
		d   *daemonHandle
		gid string
	}{
		{aria2, aria2Waiting},
		{goAria, goWaiting},
	} {
		status := decodeJSON[map[string]any](t, mustCall(t, ctx, pair.d, "aria2.tellStatus", pair.gid), pair.d.name+" waiting status")
		if status["status"] != "waiting" {
			t.Fatalf("%s expected waiting, got %#v", pair.d.name, status["status"])
		}
	}

	if _, err := aria2.call(ctx, "aria2.unpause", []any{aria2Active}); err == nil {
		t.Fatal("aria2 unpause should fail for active task")
	}
	if _, err := goAria.call(ctx, "aria2.unpause", []any{goActive}); err == nil {
		t.Fatal("go-aria2 unpause should fail for active task")
	}
	if _, err := aria2.call(ctx, "aria2.unpause", []any{aria2Waiting}); err == nil {
		t.Fatal("aria2 unpause should fail for waiting task")
	}
	if _, err := goAria.call(ctx, "aria2.unpause", []any{goWaiting}); err == nil {
		t.Fatal("go-aria2 unpause should fail for waiting task")
	}

	ts.CloseClientConnections()
	forceRemoveDownloads(t, ctx, aria2, aria2Active, aria2Waiting)
	forceRemoveDownloads(t, ctx, goAria, goActive, goWaiting)
}

func TestCompareAria2_GetServersActiveDownload(t *testing.T) {
	work := t.TempDir()
	secret := "compare-getservers-active"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	ts := startSlowDownloadServer(t)
	slowURL := ts.URL + "/slow.bin"
	for _, d := range []*daemonHandle{aria2, goAria} {
		mustCall(t, ctx, d, "aria2.changeGlobalOption", map[string]any{"max-concurrent-downloads": "1"})
	}

	pauseParams := []any{[]any{slowURL}, map[string]any{"pause": "true"}}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", pauseParams)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", pauseParams)), "go gid")
	mustCall(t, ctx, aria2, "aria2.unpause", aria2GID)
	mustCall(t, ctx, goAria, "aria2.unpause", goGID)

	waitUntilTaskStatus(t, ctx, aria2, aria2GID, "active", 20*time.Second)
	waitUntilTaskStatus(t, ctx, goAria, goGID, "active", 20*time.Second)

	aria2Servers := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.getServers", aria2GID), "aria2 getServers")
	goServers := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.getServers", goGID), "go getServers")
	if len(aria2Servers) == 0 || len(goServers) == 0 {
		t.Fatalf("getServers should return data for active downloads: aria2=%#v go=%#v", aria2Servers, goServers)
	}
	for _, key := range []string{"index", "servers"} {
		if aria2Servers[0][key] == nil || goServers[0][key] == nil {
			t.Fatalf("missing %q in getServers: aria2=%#v go=%#v", key, aria2Servers[0], goServers[0])
		}
	}

	ts.CloseClientConnections()
	forceRemoveDownloads(t, ctx, aria2, aria2GID)
	forceRemoveDownloads(t, ctx, goAria, goGID)
}

func TestCompareAria2_ChangeUri_HTTP(t *testing.T) {
	work := t.TempDir()
	secret := "compare-changeuri"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{
		[]any{
			"http://example.com/changeuri-1.bin",
			"http://example.com/changeuri-2.bin",
		},
		map[string]any{"pause": "true"},
	}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go gid")

	delURIs := []any{"http://example.com/changeuri-2.bin"}
	addURIs := []any{"http://example.com/changeuri-added.bin", "baduri"}
	compareChangeURICounts(t, ctx, aria2, goAria, aria2GID, goGID, 1, delURIs, addURIs)

	aria2URIs := decodeJSON[[]map[string]any](t, mustCall(t, ctx, aria2, "aria2.getUris", aria2GID), "aria2 getUris")
	goURIs := decodeJSON[[]map[string]any](t, mustCall(t, ctx, goAria, "aria2.getUris", goGID), "go getUris")
	if len(aria2URIs) == 0 || len(goURIs) == 0 {
		t.Fatalf("getUris empty: aria2=%#v go=%#v", aria2URIs, goURIs)
	}
	aria2List := uriListFromGetUris(aria2URIs[0])
	goList := uriListFromGetUris(goURIs[0])
	if len(aria2List) != len(goList) {
		t.Fatalf("uri count mismatch: aria2=%#v go=%#v", aria2List, goList)
	}
	for i := range aria2List {
		if aria2List[i] != goList[i] {
			t.Fatalf("uri[%d] mismatch: aria2=%q go=%q", i, aria2List[i], goList[i])
		}
	}
}

func TestCompareAria2_SaveSession(t *testing.T) {
	work := t.TempDir()
	secret := "compare-savesession"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/savesession.bin"}, map[string]any{"pause": "true"}}
	mustCallSlice(t, ctx, aria2, "aria2.addUri", params)
	mustCallSlice(t, ctx, goAria, "aria2.addUri", params)

	for _, d := range []*daemonHandle{aria2, goAria} {
		if got := mustString(t, mustCall(t, ctx, d, "aria2.saveSession"), d.name+" saveSession"); got != "OK" {
			t.Fatalf("%s saveSession returned %q", d.name, got)
		}
	}
}

func TestGoAria2_FileURIAccepted(t *testing.T) {
	work := t.TempDir()
	secret := "compare-file-uri"
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	src := filepath.Join(work, "source.bin")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloadDir := filepath.Join(work, "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatal(err)
	}

	gid := mustString(t, mustCallSlice(t, ctx, goAria, "aria2.addUri", []any{
		[]any{"file://" + src},
		map[string]any{"dir": downloadDir, "pause": "true"},
	}), "addUri file://")
	if gid == "" {
		t.Fatal("expected gid")
	}
}

func TestGoAria2_FileAllocationAccepted(t *testing.T) {
	work := t.TempDir()
	secret := "compare-file-alloc"
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	gid := mustString(t, mustCallSlice(t, ctx, goAria, "aria2.addUri", []any{
		[]any{"http://example.com/a.bin"},
		map[string]any{"pause": "true", "file-allocation": "none"},
	}), "addUri with file-allocation")
	if gid == "" {
		t.Fatal("expected gid")
	}
}

func TestGoAria2_StrictAuthRequiresToken(t *testing.T) {
	work := t.TempDir()
	secret := "compare-strict-auth"
	goAria := startGoAria2DaemonWithExtra(t, work, freeListenPort(t), secret, "rpc-strict-auth=true")
	ctx := context.Background()

	if _, err := goAria.callUnauthenticated(ctx, "system.listMethods", nil); err == nil {
		t.Fatal("strict auth should reject listMethods without token")
	}
	if raw, err := goAria.callUnauthenticated(ctx, "system.listMethods", []any{"token:" + secret}); err != nil {
		t.Fatalf("listMethods with token: %v", err)
	} else if raw == nil {
		t.Fatal("listMethods with token should succeed")
	}
}

func TestGoAria2_SplitOptionsAccepted(t *testing.T) {
	work := t.TempDir()
	secret := "compare-split-opts"
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	gid := mustString(t, mustCallSlice(t, ctx, goAria, "aria2.addUri", []any{
		[]any{"http://example.com/a.bin"},
		map[string]any{
			"pause":          "true",
			"min-split-size": "1M",
			"piece-length":   "512K",
			"index-out":      "renamed.bin",
		},
	}), "addUri with split options")
	if gid == "" {
		t.Fatal("expected gid")
	}
}

func TestGoAria2_XMLRPCGetVersion(t *testing.T) {
	work := t.TempDir()
	secret := "compare-xmlrpc"
	port := freeListenPort(t)
	goAria := startGoAria2Daemon(t, work, port, secret)
	ctx := context.Background()

	// JSON-RPC 冒烟确认 daemon 正常。
	_ = mustCall(t, ctx, goAria, "aria2.getVersion", "token:"+secret)

	body := fmt.Sprintf(`<?xml version="1.0"?><methodCall>
<methodName>aria2.getVersion</methodName>
<params><param><value><string>token:%s</string></value></param></params>
</methodCall>`, secret)
	resp, err := http.Post(goAria.baseURL+"/xmlrpc", "text/xml", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("xmlrpc status: %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !strings.Contains(text, "<methodResponse>") || !strings.Contains(text, "version") {
		t.Fatalf("unexpected xmlrpc response: %s", text)
	}
}

func TestCompareAria2_ForceRemove(t *testing.T) {
	work := t.TempDir()
	secret := "compare-forceremove"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{[]any{"http://example.com/forceremove.bin"}, map[string]any{"pause": "true"}}
	for _, d := range []*daemonHandle{aria2, goAria} {
		gid := mustString(t, mustCallSlice(t, ctx, d, "aria2.addUri", params), d.name+" addUri")
		if got := mustString(t, mustCall(t, ctx, d, "aria2.forceRemove", gid), d.name+" forceRemove"); got != gid {
			t.Fatalf("%s forceRemove returned %q", d.name, got)
		}
		if _, err := d.call(ctx, "aria2.tellStatus", []any{gid}); err == nil {
			status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" after forceRemove")
			if status["status"] != "removed" {
				t.Fatalf("%s tellStatus after forceRemove: %#v", d.name, status)
			}
		}
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

func compareChangeURICounts(t *testing.T, ctx context.Context, aria2, goAria *daemonHandle, aria2GID, goGID string, fileIndex int, delURIs, addURIs []any, extra ...any) {
	t.Helper()
	aParams := append([]any{aria2GID, fileIndex, delURIs, addURIs}, extra...)
	gParams := append([]any{goGID, fileIndex, delURIs, addURIs}, extra...)
	aRaw := mustCall(t, ctx, aria2, "aria2.changeUri", aParams...)
	gRaw := mustCall(t, ctx, goAria, "aria2.changeUri", gParams...)

	var aPair, gPair []float64
	if err := json.Unmarshal(aRaw, &aPair); err != nil || len(aPair) != 2 {
		t.Fatalf("aria2 changeUri result: %s (%v)", string(aRaw), err)
	}
	if err := json.Unmarshal(gRaw, &gPair); err != nil || len(gPair) != 2 {
		t.Fatalf("go-aria2 changeUri result: %s (%v)", string(gRaw), err)
	}
	if int(aPair[0]) != int(gPair[0]) || int(aPair[1]) != int(gPair[1]) {
		t.Fatalf("changeUri counts mismatch: aria2=[%d,%d] go=[%d,%d]", int(aPair[0]), int(aPair[1]), int(gPair[0]), int(gPair[1]))
	}
}

func uriListFromGetUris(item map[string]any) []string {
	raw, ok := item["uris"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if uri, ok := m["uri"].(string); ok {
			out = append(out, uri)
		}
	}
	return out
}

func TestGoAria2_GetVersionProtocols(t *testing.T) {
	work := t.TempDir()
	secret := "compare-version-protos"
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	ver := decodeJSON[map[string]any](t, mustCall(t, ctx, goAria, "aria2.getVersion"), "getVersion")
	enabled, ok := ver["enabledProtocols"].([]any)
	if !ok {
		t.Fatalf("enabledProtocols: %#v", ver["enabledProtocols"])
	}
	for _, proto := range []string{"ftp", "sftp"} {
		if !containsAny(enabled, proto) {
			t.Fatalf("enabledProtocols missing %q: %#v", proto, enabled)
		}
	}
	if containsAny(enabled, "ed2k") {
		t.Fatalf("ed2k should not be enabled when ed2k-enable=false: %#v", enabled)
	}
	supported, ok := ver["supportedProtocols"].([]any)
	if !ok {
		t.Fatalf("supportedProtocols: %#v", ver["supportedProtocols"])
	}
	if !containsAny(supported, "magnet") {
		t.Fatalf("supportedProtocols should list magnet: %#v", supported)
	}
}

func containsAny(items []any, target string) bool {
	for _, item := range items {
		if s, ok := item.(string); ok && s == target {
			return true
		}
	}
	return false
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func TestCompareAria2_ChangeUri_FTP(t *testing.T) {
	work := t.TempDir()
	secret := "compare-changeuri-ftp"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{
		[]any{
			"ftp://mirror1.example/changeuri.bin",
			"ftp://mirror2.example/changeuri.bin",
		},
		map[string]any{"pause": "true"},
	}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go gid")

	delURIs := []any{"ftp://mirror2.example/changeuri.bin"}
	addURIs := []any{"ftp://mirror3.example/changeuri.bin", "baduri"}
	compareChangeURICounts(t, ctx, aria2, goAria, aria2GID, goGID, 1, delURIs, addURIs)
}

func TestCompareAria2_ChangeUri_SFTP(t *testing.T) {
	work := t.TempDir()
	secret := "compare-changeuri-sftp"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{
		[]any{
			"sftp://mirror1.example/changeuri.bin",
			"sftp://mirror2.example/changeuri.bin",
		},
		map[string]any{"pause": "true"},
	}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addUri", params)), "go gid")

	delURIs := []any{"sftp://mirror2.example/changeuri.bin"}
	addURIs := []any{"sftp://mirror3.example/changeuri.bin", "baduri"}
	compareChangeURICounts(t, ctx, aria2, goAria, aria2GID, goGID, 1, delURIs, addURIs)
}

func TestCompareAria2_ChangeUri_BT(t *testing.T) {
	work := t.TempDir()
	secret := "compare-changeuri-bt"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := []any{
		sampleTorrentB64,
		[]any{"http://example.com/bt-seed-1.bin", "http://example.com/bt-seed-2.bin"},
		map[string]any{"pause": "true"},
	}
	aria2GID := mustString(t, first(rawCall(t, ctx, aria2, goAria, "aria2.addTorrent", params)), "aria2 gid")
	goGID := mustString(t, second(rawCall(t, ctx, aria2, goAria, "aria2.addTorrent", params)), "go gid")

	delURIs := []any{"http://example.com/bt-seed-2.bin"}
	addURIs := []any{"http://example.com/bt-seed-added.bin", "baduri"}
	compareChangeURICounts(t, ctx, aria2, goAria, aria2GID, goGID, 1, delURIs, addURIs)
}

func TestCompareAria2_ChangeGlobalOption_MaxOverallDownloadLimit(t *testing.T) {
	work := t.TempDir()
	secret := "compare-changeglobal-dl"
	aria2 := startAria2Daemon(t, work, freeListenPort(t), secret)
	goAria := startGoAria2Daemon(t, work, freeListenPort(t), secret)
	ctx := context.Background()

	params := map[string]any{"max-overall-download-limit": "100K"}
	for _, d := range []*daemonHandle{aria2, goAria} {
		mustCall(t, ctx, d, "aria2.changeGlobalOption", params)
	}

	aria2Global := decodeJSON[map[string]string](t, mustCall(t, ctx, aria2, "aria2.getGlobalOption", nil), "aria2 getGlobalOption")
	goGlobal := decodeJSON[map[string]string](t, mustCall(t, ctx, goAria, "aria2.getGlobalOption", nil), "go getGlobalOption")
	if aria2Global["max-overall-download-limit"] != goGlobal["max-overall-download-limit"] {
		t.Fatalf("getGlobalOption mismatch: aria2=%#v go=%#v",
			aria2Global["max-overall-download-limit"], goGlobal["max-overall-download-limit"])
	}
	if goGlobal["max-overall-download-limit"] != "102400" {
		t.Fatalf("expected normalized 102400, got aria2=%#v go=%#v",
			aria2Global["max-overall-download-limit"], goGlobal["max-overall-download-limit"])
	}
}

func TestGoAria2_WebSocketAuthAndNotify(t *testing.T) {
	work := t.TempDir()
	secret := "compare-ws-auth"
	goAria := startGoAria2DaemonWithExtra(t, work, freeListenPort(t), secret, "enable-websocket=true")
	ctx := context.Background()

	wsBase := "ws" + strings.TrimPrefix(goAria.baseURL, "http")
	_, resp, err := websocket.DefaultDialer.Dial(wsBase+"/jsonrpc", nil)
	if err == nil {
		t.Fatal("websocket without token should fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %#v err=%v", resp, err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsBase+"/jsonrpc?token="+secret, nil)
	if err != nil {
		t.Fatalf("websocket dial with token: %v", err)
	}
	defer conn.Close()

	gid := mustString(t, mustCallSlice(t, ctx, goAria, "aria2.addUri", []any{
		[]any{"http://example.com/ws-daemon.bin"},
		map[string]any{"pause": "true"},
	}), "addUri")
	if _, err := goAria.call(ctx, "aria2.unpause", []any{gid}); err != nil {
		t.Fatalf("unpause: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("expected aria2.onDownloadStart over websocket")
			}
			t.Fatalf("websocket read: %v", err)
		}
		if msg["method"] == "aria2.onDownloadStart" {
			params, _ := msg["params"].([]any)
			if len(params) == 1 {
				if ev, ok := params[0].(map[string]any); ok && ev["gid"] == gid {
					return
				}
			}
		}
	}
}
