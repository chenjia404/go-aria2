package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// 参考 aria2 RpcMethodTest.cc::testSystemMulticall：混合成功与失败的子调用。

func TestRpcMethod_SystemMulticall_MixedResults(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	calls := []any{
		map[string]any{
			"methodName": "aria2.addUri",
			"params":     []any{[]any{"http://localhost/0"}},
		},
		map[string]any{
			"methodName": "aria2.addUri",
			"params":     []any{[]any{"http://localhost/1"}},
		},
		map[string]any{
			"methodName": "not exists",
			"params":     []any{},
		},
		"not struct",
		map[string]any{
			"methodName": "system.multicall",
			"params":     []any{},
		},
		map[string]any{
			"methodName": "aria2.getVersion",
		},
		map[string]any{
			"methodName": "aria2.getVersion",
			"params":     []any{},
		},
	}

	raw := env.MustCall("system.multicall", calls)
	results, ok := raw.([]any)
	if !ok || len(results) != 7 {
		t.Fatalf("expected 7 multicall results, got %#v", raw)
	}

	gid0, ok := results[0].([]any)
	if !ok || len(gid0) != 1 {
		t.Fatalf("addUri[0] result: %#v", results[0])
	}
	if _, ok := gid0[0].(string); !ok || gid0[0] == "" {
		t.Fatalf("addUri[0] gid: %#v", gid0[0])
	}
	gid1, ok := results[1].([]any)
	if !ok || len(gid1) != 1 {
		t.Fatalf("addUri[1] result: %#v", results[1])
	}
	if _, ok := gid1[0].(string); !ok || gid1[0] == "" {
		t.Fatalf("addUri[1] gid: %#v", gid1[0])
	}

	for i := 2; i <= 4; i++ {
		errObj, ok := results[i].(map[string]any)
		if !ok {
			t.Fatalf("expected error object at index %d, got %#v", i, results[i])
		}
		if errObj["message"] == "" {
			t.Fatalf("expected error message at index %d: %#v", i, errObj)
		}
	}
	if code, _ := results[2].(map[string]any)["code"].(int); code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("not exists method code: %#v", results[2])
	}

	for i := 5; i <= 6; i++ {
		versionWrap, ok := results[i].([]any)
		if !ok || len(versionWrap) != 1 {
			t.Fatalf("getVersion[%d] result: %#v", i, results[i])
		}
		version, ok := versionWrap[0].(map[string]any)
		if !ok || version["version"] == "" {
			t.Fatalf("getVersion[%d] payload: %#v", i, versionWrap[0])
		}
	}

	waitingGIDs := map[string]struct{}{gid0[0].(string): {}, gid1[0].(string): {}}
	for gid := range waitingGIDs {
		status := env.Status(gid)
		if status["gid"] != gid {
			t.Fatalf("tellStatus gid mismatch: %#v", status)
		}
	}
}
