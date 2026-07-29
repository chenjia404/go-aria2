package aria2

import (
	"fmt"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// 参考 aria2 RpcMethodTest.cc::testPause：pause/unpause/pauseAll/unpauseAll/forcePauseAll 序列。

func TestRpcMethod_PauseLifecycle(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{MaxConcurrent: 3})
	gids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		gids = append(gids, env.MustGID("aria2.addUri", []any{fmt.Sprintf("http://url%d", i+1)}))
	}

	if raw := env.MustCall("aria2.pause", gids[0]); raw != gids[0] {
		t.Fatalf("pause: %#v", raw)
	}
	if env.Status(gids[0])["status"] != "paused" {
		t.Fatal("expected first task paused")
	}

	if raw := env.MustCall("aria2.unpause", gids[0]); raw != gids[0] {
		t.Fatalf("unpause: %#v", raw)
	}
	if env.Status(gids[0])["status"] == "paused" {
		t.Fatal("expected first task unpaused")
	}

	if raw := env.MustCall("aria2.pauseAll"); raw != "OK" {
		t.Fatalf("pauseAll: %#v", raw)
	}
	for _, gid := range gids {
		if env.Status(gid)["status"] != "paused" {
			t.Fatalf("expected all paused after pauseAll, gid %s status %v", gid, env.Status(gid)["status"])
		}
	}

	if raw := env.MustCall("aria2.unpauseAll"); raw != "OK" {
		t.Fatalf("unpauseAll: %#v", raw)
	}
	for _, gid := range gids {
		if env.Status(gid)["status"] == "paused" {
			t.Fatalf("expected all unpaused after unpauseAll, gid %s still paused", gid)
		}
	}

	if raw := env.MustCall("aria2.forcePauseAll"); raw != "OK" {
		t.Fatalf("forcePauseAll: %#v", raw)
	}
	for _, gid := range gids {
		if env.Status(gid)["status"] != "paused" {
			t.Fatalf("expected all paused after forcePauseAll, gid %s status %v", gid, env.Status(gid)["status"])
		}
	}
}

func TestRpcMethod_ChangePosition_Success(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true})
	gids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		gids = append(gids, env.MustGID("aria2.addUri", []any{fmt.Sprintf("http://example.com/%d", i)}, map[string]any{"pause": "true"}))
	}

	pos := env.MustCall("aria2.changePosition", gids[1], 0, "POS_SET")
	if pos != 0 {
		t.Fatalf("changePosition POS_SET: got %#v want 0", pos)
	}
	waiting := env.MustCall("aria2.tellWaiting", 0, 10).([]map[string]any)
	if len(waiting) < 2 || waiting[0]["gid"] != gids[1] {
		t.Fatalf("expected reordered queue, got %#v", waiting)
	}
}

func TestRpcMethod_SystemListNotifications(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	raw := env.MustCall("system.listNotifications")
	notifications, ok := raw.([]string)
	if !ok || len(notifications) == 0 {
		t.Fatalf("listNotifications: %#v", raw)
	}
	required := []string{
		"aria2.onDownloadStart",
		"aria2.onDownloadPause",
		"aria2.onDownloadStop",
		"aria2.onDownloadComplete",
		"aria2.onDownloadError",
		"aria2.onBtDownloadComplete",
	}
	for _, name := range required {
		found := false
		for _, item := range notifications {
			if item == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing notification %s in %#v", name, notifications)
		}
	}
}
