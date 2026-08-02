package aria2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	jsonrpcserver "github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
	"github.com/gorilla/websocket"
)

// 验证 WebSocket 能收到与 aria2 一致的任务生命周期通知。
//
// 连接建立后立即启动后台读协程，避免服务端 WriteMessage 因客户端未读而阻塞；
// 同包大量 t.Parallel 用例并发时易在 CI 上争抢 CPU，因此 WebSocket 测试串行执行。

const wsNotifyTimeout = 15 * time.Second

var wsNotifyTestMu sync.Mutex

type wsNotifyReader struct {
	msgs  chan map[string]any
	errCh chan error
}

func startWSNotifyReader(conn *websocket.Conn) *wsNotifyReader {
	r := &wsNotifyReader{
		msgs:  make(chan map[string]any, 32),
		errCh: make(chan error, 1),
	}
	go func() {
		for {
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				select {
				case r.errCh <- err:
				default:
				}
				return
			}
			r.msgs <- msg
		}
	}()
	// 给读协程时间进入 ReadJSON，避免首条通知在客户端尚未读时阻塞服务端写。
	time.Sleep(20 * time.Millisecond)
	return r
}

func (r *wsNotifyReader) waitMatch(t *testing.T, timeout time.Duration, match func(map[string]any) bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("websocket notification not received before timeout")
		}
		select {
		case msg := <-r.msgs:
			if match(msg) {
				return
			}
		case err := <-r.errCh:
			t.Fatalf("websocket read: %v", err)
		case <-time.After(remaining):
			t.Fatal("websocket notification not received before timeout")
		}
	}
}

func TestWebSocketNotification_OnDownloadPause(t *testing.T) {
	wsNotifyTestMu.Lock()
	defer wsNotifyTestMu.Unlock()

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
	reader := startWSNotifyReader(conn)

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
	reader.waitMatch(t, wsNotifyTimeout, func(msg map[string]any) bool {
		return msg["method"] == "aria2.onDownloadStart"
	})

	if _, err := svc.Invoke(context.Background(), "aria2.pause", []any{gidStr}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	reader.waitMatch(t, wsNotifyTimeout, func(msg map[string]any) bool {
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
	wsNotifyTestMu.Lock()
	defer wsNotifyTestMu.Unlock()

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
	reader := startWSNotifyReader(conn)

	if _, err := svc.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/paused-add"},
		map[string]any{"pause": "true"},
	}); err != nil {
		t.Fatalf("addUri: %v", err)
	}

	select {
	case msg := <-reader.msgs:
		if msg["method"] == "aria2.onDownloadStart" {
			t.Fatalf("paused add should not emit onDownloadStart: %#v", msg)
		}
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWebSocketNotification_OnDownloadStart(t *testing.T) {
	wsNotifyTestMu.Lock()
	defer wsNotifyTestMu.Unlock()

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
	reader := startWSNotifyReader(conn)

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
	reader.waitMatch(t, wsNotifyTimeout, func(msg map[string]any) bool {
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
	wsNotifyTestMu.Lock()
	defer wsNotifyTestMu.Unlock()

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
	reader := startWSNotifyReader(conn)

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
	reader.waitMatch(t, wsNotifyTimeout, func(msg map[string]any) bool {
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
