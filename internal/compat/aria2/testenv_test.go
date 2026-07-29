package aria2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// rpcTestEnv 封装 aria2 兼容性测试常用的 Service + stub 驱动环境。
type rpcTestEnv struct {
	t       *testing.T
	SaveDir string
	Driver  *rpcStubDriver
	Service *Service
}

func newRPCTestEnv(t *testing.T, opts manager.Options) *rpcTestEnv {
	t.Helper()
	if opts.DefaultDir == "" {
		opts.DefaultDir = t.TempDir()
	}
	if opts.GlobalOptions == nil {
		opts.GlobalOptions = map[string]string{"dir": opts.DefaultDir}
	} else if opts.GlobalOptions["dir"] == "" {
		opts.GlobalOptions["dir"] = opts.DefaultDir
	}
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = 10
	}

	driver := newRPCStubDriver()
	mgr := manager.New(opts)
	mgr.RegisterDriver(driver)
	svc := NewService(mgr, "")

	return &rpcTestEnv{
		t:       t,
		SaveDir: opts.DefaultDir,
		Driver:  driver,
		Service: svc,
	}
}

func (e *rpcTestEnv) Call(method string, params ...any) (any, error) {
	e.t.Helper()
	return e.Service.Invoke(context.Background(), method, params)
}

func (e *rpcTestEnv) MustCall(method string, params ...any) any {
	e.t.Helper()
	raw, err := e.Call(method, params...)
	if err != nil {
		e.t.Fatalf("%s: %v", method, err)
	}
	return raw
}

func (e *rpcTestEnv) MustGID(method string, params ...any) string {
	e.t.Helper()
	raw := e.MustCall(method, params...)
	gid, ok := raw.(string)
	if !ok || gid == "" {
		e.t.Fatalf("%s: expected gid string, got %#v", method, raw)
	}
	return gid
}

func (e *rpcTestEnv) Status(gid string) map[string]any {
	e.t.Helper()
	raw := e.MustCall("aria2.tellStatus", gid)
	status, ok := raw.(map[string]any)
	if !ok {
		e.t.Fatalf("tellStatus: unexpected payload %#v", raw)
	}
	return status
}

func (e *rpcTestEnv) Option(gid string) map[string]string {
	e.t.Helper()
	raw := e.MustCall("aria2.getOption", gid)
	opts, ok := raw.(map[string]string)
	if !ok {
		e.t.Fatalf("getOption: unexpected payload %#v", raw)
	}
	return opts
}

func (e *rpcTestEnv) ExpectError(method string, params ...any) {
	e.t.Helper()
	_, err := e.Call(method, params...)
	if err == nil {
		e.t.Fatalf("%s: expected error", method)
	}
}

func (e *rpcTestEnv) ExpectRPCError(method string, params ...any) *jsonrpc.RPCError {
	e.t.Helper()
	_, err := e.Call(method, params...)
	if err == nil {
		e.t.Fatalf("%s: expected error", method)
	}
	rpcErr, ok := err.(*jsonrpc.RPCError)
	if !ok {
		// aria2 对无效 GID 等场景返回 fault，go-aria2 部分路径仍返回普通 error。
		return nil
	}
	return rpcErr
}

func mustStringMap(t *testing.T, raw any) map[string]string {
	t.Helper()
	m, ok := raw.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %#v", raw)
	}
	return m
}

func mustStringSlice(t *testing.T, raw any) []string {
	t.Helper()
	s, ok := raw.([]string)
	if !ok {
		t.Fatalf("expected []string, got %#v", raw)
	}
	return s
}

// sampleTorrentPayload 与 internal/protocol/bt/parser_test.go 使用相同的最小合法 torrent。
func sampleTorrentPayload() []byte {
	return []byte("d8:announce14:http://tracker13:creation datei1712123456e4:infod6:lengthi123e4:name8:test.bin12:piece lengthi262144e6:pieces20:12345678901234567890ee")
}

func sampleTorrentBase64() string {
	return base64.StdEncoding.EncodeToString(sampleTorrentPayload())
}

func sampleMetalinkXML() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="a.bin">
    <url>http://example.com/a.bin</url>
  </file>
  <file name="b.bin">
    <url>http://example.com/b.bin</url>
  </file>
</metalink>`
}

func sampleMetalinkBase64() string {
	return base64.StdEncoding.EncodeToString([]byte(sampleMetalinkXML()))
}

func jsonRoundTrip(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}
