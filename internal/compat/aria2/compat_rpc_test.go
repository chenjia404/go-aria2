package aria2

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
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
