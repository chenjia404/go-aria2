package aria2session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRPCClientCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "aria2.getVersion" {
			t.Fatalf("unexpected method: %s", req.Method)
		}
		if len(req.Params) != 1 || req.Params[0] != "token:secret" {
			t.Fatalf("unexpected params: %#v", req.Params)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"version":"1.36.0"}}`))
	}))
	defer server.Close()

	client := NewRPCClient(server.URL, "secret")
	raw, err := client.Call(context.Background(), "aria2.getVersion")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["version"] != "1.36.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestFetchAria2SessionTasksFromRPC(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		switch req.Method {
		case "aria2.tellActive":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":[{"gid":"abcd1234abcd1234","status":"active","files":[{"uris":[{"uri":"http://example.com/file.bin"}]}]}]}`))
		case "aria2.tellWaiting":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":[]}`))
		case "aria2.tellStopped":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":[]}`))
		case "aria2.getOption":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"dir":"/tmp","out":"file.bin"}}`))
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
	}))
	defer server.Close()

	tasks, err := FetchAria2SessionTasksFromRPC(context.Background(), server.URL, "")
	if err != nil {
		t.Fatalf("FetchAria2SessionTasksFromRPC: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %#v", tasks)
	}
	if tasks[0].URI != "http://example.com/file.bin" {
		t.Fatalf("unexpected uri: %#v", tasks[0])
	}
	if tasks[0].Dir != "/tmp" || tasks[0].Out != "file.bin" {
		t.Fatalf("unexpected options mapping: %#v", tasks[0])
	}
	if tasks[0].Options["aria2.import.source"] != "aria2-rpc" {
		t.Fatalf("expected aria2-rpc import source, got %#v", tasks[0].Options)
	}
}

func TestSessionTaskFromRPCPaused(t *testing.T) {
	t.Parallel()

	item, err := sessionTaskFromRPC(map[string]any{
		"gid":    "abcd1234abcd1234",
		"status": "paused",
		"files": []any{
			map[string]any{
				"uris": []any{
					map[string]any{"uri": "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"},
				},
			},
		},
	}, map[string]string{"dir": "/data"})
	if err != nil {
		t.Fatalf("sessionTaskFromRPC: %v", err)
	}
	if !item.Paused {
		t.Fatal("expected paused task")
	}
}
