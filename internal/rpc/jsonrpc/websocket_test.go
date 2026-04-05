package jsonrpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/gorilla/websocket"
)

func TestWebSocketJSONRPCGetVersion(t *testing.T) {
	t.Parallel()

	mgr := manager.New(manager.Options{})
	h := testHandler{
		invoke: func(_ context.Context, method string, params []any) (any, error) {
			if method == "aria2.getVersion" {
				return map[string]any{"version": "1.0.0"}, nil
			}
			return nil, NewError(CodeMethodNotFound, "method not found")
		},
	}
	srv := NewServer(h, Options{
		AllowOriginAll: true,
		WebSocket: &WebSocketOptions{
			Manager: mgr,
			Secret:  "",
		},
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r)
	}))
	defer ts.Close()

	u := "ws" + strings.TrimPrefix(ts.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "aria2.getVersion", "params": []any{}}
	if err := c.WriteJSON(req); err != nil {
		t.Fatalf("write: %v", err)
	}
	var resp map[string]any
	if err := c.ReadJSON(&resp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp["jsonrpc"] != "2.0" || resp["id"] != float64(1) {
		t.Fatalf("unexpected response: %#v", resp)
	}
	res, ok := resp["result"].(map[string]any)
	if !ok || res["version"] != "1.0.0" {
		t.Fatalf("unexpected result: %#v", resp)
	}
}

func TestAuthorizeWebSocket(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "http://x/?token=ok", nil)
	if !authorizeWebSocket(r, "ok") {
		t.Fatal("query token")
	}
	r = httptest.NewRequest(http.MethodGet, "http://x/", nil)
	r.Header.Set("Authorization", "token:secret")
	if !authorizeWebSocket(r, "secret") {
		t.Fatal("authorization header")
	}
	r = httptest.NewRequest(http.MethodGet, "http://x/", nil)
	if authorizeWebSocket(r, "x") {
		t.Fatal("missing token should fail")
	}
}

func TestProcessBodyNotificationNoResponse(t *testing.T) {
	t.Parallel()

	h := testHandler{invoke: func(context.Context, string, []any) (any, error) {
		t.Fatal("invoke should not be called for notification")
		return nil, nil
	}}
	body := []byte(`{"jsonrpc":"2.0","method":"aria2.ping"}`)
	out, ok := ProcessBody(context.Background(), h, body)
	if ok || out != nil {
		t.Fatalf("expected no response, got ok=%v out=%s", ok, out)
	}
}

func TestProcessBodyBatchWithNullSlots(t *testing.T) {
	t.Parallel()

	var calls int
	h := testHandler{invoke: func(_ context.Context, method string, _ []any) (any, error) {
		calls++
		if method != "aria2.getVersion" {
			return nil, NewError(CodeMethodNotFound, "nope")
		}
		return map[string]any{"version": "x"}, nil
	}}
	body := []byte(`[{"jsonrpc":"2.0","method":"foo"},{"jsonrpc":"2.0","id":1,"method":"aria2.getVersion","params":[]}]`)
	out, ok := ProcessBody(context.Background(), h, body)
	if !ok {
		t.Fatal("expected response")
	}
	var arr []any
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 2 || arr[0] != nil {
		t.Fatalf("expected [null, result], got %s", string(out))
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
}
