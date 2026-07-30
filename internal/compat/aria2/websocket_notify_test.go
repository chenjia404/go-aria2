package aria2

import (
	"context"
	"net"
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

// readWSUntil 在超时前持续读取 WebSocket 消息，直到 match 返回 true。
// gorilla/websocket 在 ReadJSON 失败后不可重复读取同一连接，因此不能在超时后 continue 重试。
// 每收到一条非匹配消息会刷新读超时，避免先到的 onDownloadStart 等通知耗尽总等待时间。
func readWSUntil(t *testing.T, conn *websocket.Conn, timeout time.Duration, match func(msg map[string]any) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("websocket notification not received before timeout")
		}
		if err := conn.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var msg map[string]any
		err := conn.ReadJSON(&msg)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("websocket notification not received before timeout")
			}
			t.Fatalf("websocket read: %v", err)
		}
		if match(msg) {
			return
		}
	}
}

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
		map[string]any{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gidStr, _ := gid.(string)

	if _, err := svc.Invoke(context.Background(), "aria2.unpause", []any{gidStr}); err != nil {
		t.Fatalf("unpause: %v", err)
	}
	readWSUntil(t, conn, 5*time.Second, func(msg map[string]any) bool {
		return msg["method"] == "aria2.onDownloadStart"
	})

	if _, err := svc.Invoke(context.Background(), "aria2.pause", []any{gidStr}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	readWSUntil(t, conn, 5*time.Second, func(msg map[string]any) bool {
		if msg["method"] != "aria2.onDownloadPause" {
			return false
		}
		params, _ := msg["params"].([]any)
		if len(params) != 1 {
			return false
		}
		ev, ok := params[0].(map[string]any)
		return ok && ev["gid"] == gidStr
	})
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

func TestWebSocketNotification_OnDownloadStart(t *testing.T) {
	t.Parallel()

	mgr := manager.New(manager.Options{DefaultDir: t.TempDir(), MaxConcurrent: 3})
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
		[]any{"http://example.com/ws-start"},
		map[string]any{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gidStr, _ := gid.(string)

	if _, err := svc.Invoke(context.Background(), "aria2.unpause", []any{gidStr}); err != nil {
		t.Fatalf("unpause: %v", err)
	}

	readWSUntil(t, conn, 5*time.Second, func(msg map[string]any) bool {
		if msg["method"] != "aria2.onDownloadStart" {
			return false
		}
		params, _ := msg["params"].([]any)
		if len(params) != 1 {
			return false
		}
		ev, ok := params[0].(map[string]any)
		return ok && ev["gid"] == gidStr
	})
}

func TestWebSocketNotification_OnDownloadStop(t *testing.T) {
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
		[]any{"http://example.com/ws-stop"},
		map[string]any{"pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gidStr, _ := gid.(string)

	if _, err := svc.Invoke(context.Background(), "aria2.remove", []any{gidStr}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	readWSUntil(t, conn, 5*time.Second, func(msg map[string]any) bool {
		if msg["method"] != "aria2.onDownloadStop" {
			return false
		}
		params, _ := msg["params"].([]any)
		if len(params) != 1 {
			return false
		}
		ev, ok := params[0].(map[string]any)
		return ok && ev["gid"] == gidStr
	})
}
