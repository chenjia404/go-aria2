package aria2

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
)

func TestRpcMethod_ChangeUri_ED2K(t *testing.T) {
	t.Parallel()

	saveDir := t.TempDir()
	driver, err := ed2k.New(ed2k.Options{
		StatePath:    filepath.Join(saveDir, "ed2k-state"),
		EnableDHT:    false,
		EnableServer: false,
	})
	if err != nil {
		t.Fatalf("ed2k.New: %v", err)
	}
	t.Cleanup(func() { _ = driver.Close() })

	mgr := manager.New(manager.Options{DefaultDir: saveDir})
	mgr.RegisterDriver(driver)
	svc := NewServiceWithConfig(mgr, ServiceConfig{ED2KEnabled: true})

	hash := "0123456789abcdef0123456789abcdef"
	primary := "ed2k://|file|demo.bin|11|" + hash + "|/"
	rawGID, err := svc.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{primary},
		map[string]any{"dir": saveDir, "pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gid, ok := rawGID.(string)
	if !ok || gid == "" {
		t.Fatalf("gid: %#v", rawGID)
	}

	mirror := "ed2k://|file|demo.bin|11|" + hash + "|s=1.2.3.4:4662|/"
	raw, err := svc.Invoke(context.Background(), "aria2.changeUri", []any{
		gid, 1, []any{}, []any{mirror}, -1,
	})
	if err != nil {
		t.Fatalf("changeUri: %v", err)
	}
	del, add := mustIntPair(t, raw)
	if del != 0 || add != 1 {
		t.Fatalf("changeUri counts: del=%d add=%d", del, add)
	}

	status, err := svc.Invoke(context.Background(), "aria2.tellStatus", []any{gid})
	if err != nil {
		t.Fatalf("tellStatus: %v", err)
	}
	statusMap := status.(map[string]any)
	files := statusMap["files"].([]map[string]any)
	if len(files) != 1 {
		t.Fatalf("files: %#v", files)
	}
	uris := files[0]["uris"].([]map[string]any)
	if len(uris) != 2 {
		t.Fatalf("expected 2 uris, got %#v", uris)
	}

	badMirror := "ed2k://|file|demo.bin|11|" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + "|s=9.9.9.9:4662|/"
	raw, err = svc.Invoke(context.Background(), "aria2.changeUri", []any{
		gid, 1, []any{}, []any{badMirror}, -1,
	})
	if err != nil {
		t.Fatalf("changeUri bad hash: %v", err)
	}
	del, add = mustIntPair(t, raw)
	if del != 0 || add != 0 {
		t.Fatalf("bad hash should be skipped: del=%d add=%d", del, add)
	}

	raw, err = svc.Invoke(context.Background(), "aria2.changeUri", []any{
		gid, 1, []any{mirror}, []any{}, -1,
	})
	if err != nil {
		t.Fatalf("changeUri delete: %v", err)
	}
	del, add = mustIntPair(t, raw)
	if del != 1 || add != 0 {
		t.Fatalf("delete counts: del=%d add=%d", del, add)
	}
}
