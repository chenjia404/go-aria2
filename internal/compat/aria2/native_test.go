package aria2

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestNativeGetVersionAndProtocolStats(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{DefaultDir: "./downloads"}), "")

	rawVersion, err := service.Invoke(context.Background(), "native.getVersion", nil)
	if err != nil {
		t.Fatalf("native.getVersion: %v", err)
	}
	version, ok := rawVersion.(map[string]any)
	if !ok || version["nativeRpc"] != true {
		t.Fatalf("unexpected native version: %#v", rawVersion)
	}

	rawStats, err := service.Invoke(context.Background(), "native.getProtocolStats", nil)
	if err != nil {
		t.Fatalf("native.getProtocolStats: %v", err)
	}
	stats, ok := rawStats.(map[string]any)
	if !ok || stats["protocols"] == nil {
		t.Fatalf("unexpected stats: %#v", rawStats)
	}
}

func TestNativeGetTaskMetaAndImportSession(t *testing.T) {
	t.Parallel()

	sessionPath := filepath.Join(t.TempDir(), "session.json")
	store := session.NewFileStore(sessionPath)
	mgr := manager.New(manager.Options{DefaultDir: "./downloads", Store: store})
	mgr.RegisterDriver(newRPCStubDriver())

	service := NewService(mgr, "")
	service.SetSessionPath(sessionPath)

	created, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI:     "http://example.com/file.bin",
		SaveDir: t.TempDir(),
		Meta:    map[string]string{"custom.key": "value"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	rawMeta, err := service.Invoke(context.Background(), "native.getTaskMeta", []any{created.GID})
	if err != nil {
		t.Fatalf("native.getTaskMeta: %v", err)
	}
	meta, ok := rawMeta.(map[string]string)
	if !ok || meta["custom.key"] != "value" {
		t.Fatalf("unexpected meta: %#v", rawMeta)
	}

	exportPath := filepath.Join(t.TempDir(), "export-session.json")
	if _, err := service.Invoke(context.Background(), "native.exportSession", []any{exportPath}); err != nil {
		t.Fatalf("native.exportSession: %v", err)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("export file missing: %v", err)
	}

	importMgr := manager.New(manager.Options{DefaultDir: "./downloads"})
	importMgr.RegisterDriver(newRPCStubDriver())
	importService := NewService(importMgr, "")
	rawImport, err := importService.Invoke(context.Background(), "native.importSession", []any{exportPath})
	if err != nil {
		t.Fatalf("native.importSession: %v", err)
	}
	result, ok := rawImport.(map[string]any)
	if !ok {
		t.Fatalf("unexpected import result type: %#v", rawImport)
	}
	switch imported := result["imported"].(type) {
	case int:
		if imported != 1 {
			t.Fatalf("unexpected import count: %#v", rawImport)
		}
	case float64:
		if imported != 1 {
			t.Fatalf("unexpected import count: %#v", rawImport)
		}
	default:
		t.Fatalf("unexpected import result: %#v", rawImport)
	}
}

func TestNativeMethodsListed(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{}), "")
	raw, err := service.Invoke(context.Background(), "system.listMethods", nil)
	if err != nil {
		t.Fatalf("listMethods: %v", err)
	}
	methods, ok := raw.([]string)
	if !ok {
		t.Fatalf("unexpected methods payload: %#v", raw)
	}
	found := false
	for _, method := range methods {
		if method == "native.getVersion" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("native.getVersion not listed: %#v", methods)
	}
}
