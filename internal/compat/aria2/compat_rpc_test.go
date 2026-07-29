package aria2

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

func TestAddUriMapsOutOptionToFileName(t *testing.T) {
	t.Parallel()

	driver := newRPCStubDriver()
	saveDir := t.TempDir()
	mgr := manager.New(manager.Options{DefaultDir: saveDir})
	mgr.RegisterDriver(driver)
	service := NewService(mgr, "")

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/download.bin"},
		map[string]any{"dir": saveDir, "out": "custom-name.bin", "pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gid := rawGID.(string)

	status, err := service.Invoke(context.Background(), "aria2.tellStatus", []any{gid})
	if err != nil {
		t.Fatalf("tellStatus: %v", err)
	}
	statusMap := status.(map[string]any)
	files := statusMap["files"].([]map[string]any)
	if len(files) == 0 || files[0]["path"] != filepath.Join(saveDir, "custom-name.bin") {
		t.Fatalf("expected custom output name, got %#v", files)
	}
}

func TestChangeOptionReturnsOK(t *testing.T) {
	t.Parallel()

	driver := newRPCStubDriver()
	saveDir := t.TempDir()
	service := NewService(manager.New(manager.Options{DefaultDir: saveDir}), "")
	service.manager.RegisterDriver(driver)

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/download.bin"},
		map[string]any{"dir": saveDir, "pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gid := rawGID.(string)

	raw, err := service.Invoke(context.Background(), "aria2.changeOption", []any{
		gid,
		map[string]any{"max-download-limit": "1024"},
	})
	if err != nil {
		t.Fatalf("changeOption: %v", err)
	}
	if raw != "OK" {
		t.Fatalf("expected OK, got %#v", raw)
	}
}

func TestMulticallContinuesOnSubcallFailure(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{DefaultDir: t.TempDir()}), "secret")

	raw, err := service.Invoke(context.Background(), "system.multicall", []any{
		[]any{
			map[string]any{
				"methodName": "aria2.ping",
				"params":     []any{"token:secret"},
			},
			map[string]any{
				"methodName": "aria2.tellStatus",
				"params":     []any{"token:secret", "missing-gid"},
			},
			map[string]any{
				"methodName": "aria2.ping",
				"params":     []any{"token:secret"},
			},
		},
	})
	if err != nil {
		t.Fatalf("multicall: %v", err)
	}
	results, ok := raw.([]any)
	if !ok || len(results) != 3 {
		t.Fatalf("unexpected multicall payload: %#v", raw)
	}
	if got, _ := results[0].([]any); len(got) != 1 || got[0] != "pong" {
		t.Fatalf("unexpected first result: %#v", results[0])
	}
	if errObj, ok := results[1].(map[string]any); !ok || errObj["message"] == "" {
		t.Fatalf("expected error object for failed subcall, got %#v", results[1])
	}
	if got, _ := results[2].([]any); len(got) != 1 || got[0] != "pong" {
		t.Fatalf("unexpected third result: %#v", results[2])
	}
}

func TestMulticallRejectsInvalidSubcallToken(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{DefaultDir: t.TempDir()}), "secret")
	raw, err := service.Invoke(context.Background(), "system.multicall", []any{
		[]any{
			map[string]any{
				"methodName": "aria2.ping",
				"params":     []any{"token:wrong"},
			},
		},
	})
	if err != nil {
		t.Fatalf("multicall: %v", err)
	}
	results := raw.([]any)
	errObj, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", results[0])
	}
	if errObj["code"] != jsonrpc.CodeInvalidParams {
		t.Fatalf("unexpected error code: %#v", errObj)
	}
}
