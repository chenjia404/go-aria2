package aria2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	jsonrpcserver "github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
	"github.com/gorilla/websocket"
)

// 验证 WebSocket 能收到与 aria2 一致的任务生命周期通知。

func TestWebSocketNotification_OnDownloadPause(t *testing.T) {
	t.Parallel()

	mgr := manager.New(manager.Options{DefaultDir: t.TempDir()})
	driver := newRPCStubDriver()
	mgr.RegisterDriver(driver)
	svc := NewService(mgr, "")

	srv := jsonrpcserver.NewServer(svc, jsonrpcserver.Options{
		AllowOriginAll: true,
		WebSocket: &jsonrpcserver.WebSocketOptions{
			Manager: mgr,
		},
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r)
	}))
	defer ts.Close()

	u := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	gid, err := svc.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/ws-notify"},
		map[string]any{},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gidStr, _ := gid.(string)

	if _, err := svc.Invoke(context.Background(), "aria2.pause", []any{gidStr}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			continue
		}
		if msg["method"] == "aria2.onDownloadPause" {
			params, _ := msg["params"].([]any)
			if len(params) == 1 {
				if ev, ok := params[0].(map[string]any); ok && ev["gid"] == gidStr {
					return
				}
			}
		}
	}
	t.Fatal("expected aria2.onDownloadPause notification")
}

func TestWebSocketNotification_AddPausedSkipsStart(t *testing.T) {
	t.Parallel()

	mgr := manager.New(manager.Options{DefaultDir: t.TempDir(), StartPaused: true})
	driver := newRPCStubDriver()
	mgr.RegisterDriver(driver)
	svc := NewService(mgr, "")

	srv := jsonrpcserver.NewServer(svc, jsonrpcserver.Options{
		AllowOriginAll: true,
		WebSocket: &jsonrpcserver.WebSocketOptions{
			Manager: mgr,
		},
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r)
	}))
	defer ts.Close()

	u := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := svc.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/paused-add"},
		map[string]any{"pause": "true"},
	}); err != nil {
		t.Fatalf("addUri: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err == nil {
		if msg["method"] == "aria2.onDownloadStart" {
			t.Fatalf("paused add should not emit onDownloadStart: %#v", msg)
		}
	}
}
