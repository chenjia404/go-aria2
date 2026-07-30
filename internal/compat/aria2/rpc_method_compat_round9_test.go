package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

func TestRejectUnimplementedOptions(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"file-allocation": "none",
		"header":          "X-Test: 1",
		"min-split-size":  "16",
		"index-out":       "out.bin",
		"piece-length":    "16",
	}
	for key, value := range cases {
		err := validateAddOptions(map[string]string{key: value, "pause": "true"})
		if err == nil {
			t.Fatalf("expected unimplemented error for %s", key)
		}
		rpcErr, ok := err.(*jsonrpc.RPCError)
		if !ok || rpcErr.Code != jsonrpc.CodeInvalidParams {
			t.Fatalf("%s: unexpected error %#v", key, err)
		}
	}
}

func TestOptionAliasRefererAndUserAgent(t *testing.T) {
	t.Parallel()

	opts := map[string]string{
		"referer":    "http://example.com/",
		"user-agent": "test-agent",
		"pause":      "true",
	}
	if err := validateAddOptions(opts); err != nil {
		t.Fatalf("validateAddOptions: %v", err)
	}
	if opts["http-referer"] != "http://example.com/" {
		t.Fatalf("referer alias: %#v", opts)
	}
	if opts["http-user-agent"] != "test-agent" {
		t.Fatalf("user-agent alias: %#v", opts)
	}
	if _, ok := opts["referer"]; ok {
		t.Fatal("referer should be normalized away")
	}
}

func TestValidateAddURIScheme(t *testing.T) {
	t.Parallel()

	for _, uri := range []string{
		"http://localhost/1",
		"https://example.com/x",
		"ed2k://|file|a.bin|1|hash|/",
		"magnet:?xt=urn:btih:abc",
	} {
		if err := validateAddURIScheme(uri); err != nil {
			t.Fatalf("%q should be supported: %v", uri, err)
		}
	}

	for _, uri := range []string{"ftp://example.com/a", "sftp://example.com/a", "not uri"} {
		if err := validateAddURIScheme(uri); err == nil {
			t.Fatalf("%q should be rejected", uri)
		}
	}
}

func TestRpcMethod_AddUri_UnsupportedScheme(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	rpcErr := env.ExpectRPCError("aria2.addUri", []any{"ftp://example.com/a.bin"})
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params, got %#v", rpcErr)
	}
}

func TestRpcMethod_StrictAuth_RequiresTokenForListMethods(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	env.Service = NewServiceWithConfig(env.Service.manager, ServiceConfig{
		RPCSecret:  "secret-token",
		StrictAuth: true,
	})

	rpcErr := env.ExpectRPCError("system.listMethods")
	if rpcErr == nil || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("strict auth listMethods: %#v", rpcErr)
	}
	if raw := env.MustCall("system.listMethods", "token:secret-token"); raw == nil {
		t.Fatal("expected listMethods with token")
	}
}
