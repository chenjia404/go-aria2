package aria2

import (
	"errors"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

func TestMapManagerRPCError(t *testing.T) {
	t.Parallel()

	rpcErr, ok := mapManagerRPCError(manager.ErrTaskNotFound).(*jsonrpc.RPCError)
	if !ok || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("task not found: %#v", rpcErr)
	}

	rpcErr, ok = mapManagerRPCError(errors.New("unknown position mode: bad")).(*jsonrpc.RPCError)
	if !ok || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("bad position mode: %#v", rpcErr)
	}
}

func TestRpcMethod_ChangePosition_BadKeywordReturnsInvalidParams(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"pause": "true"})

	_, err := env.Call("aria2.changePosition", gid, 0, "bad keyword")
	if err == nil {
		t.Fatal("expected error for bad keyword")
	}
	rpcErr, ok := err.(*jsonrpc.RPCError)
	if !ok || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected invalid params, got %v", err)
	}
}

func TestRpcMethod_ChangeUri_RejectsNonListURIs(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://example.com/a"}, map[string]any{"pause": "true"})
	env.ExpectError("aria2.changeUri", gid, 1, []any{}, "http://url", 0)
}
