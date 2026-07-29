package aria2

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestForceShutdownAlias(t *testing.T) {
	t.Parallel()

	var forced atomic.Bool
	service := NewService(manager.New(manager.Options{DefaultDir: t.TempDir()}), "")
	service.SetShutdownHook(func(force bool) {
		forced.Store(force)
	})

	raw, err := service.Invoke(context.Background(), "aria2.forceShutdown", nil)
	if err != nil {
		t.Fatalf("forceShutdown: %v", err)
	}
	if raw != "OK" {
		t.Fatalf("expected OK, got %#v", raw)
	}
	time.Sleep(10 * time.Millisecond)
	if !forced.Load() {
		t.Fatalf("expected force shutdown hook to run with force=true")
	}
}

func TestGetGlobalStatIncludesNumStoppedTotal(t *testing.T) {
	t.Parallel()

	driver := newRPCStubDriver()
	mgr := manager.New(manager.Options{DefaultDir: t.TempDir()})
	mgr.RegisterDriver(driver)
	service := NewService(mgr, "")

	rawGID, err := service.Invoke(context.Background(), "aria2.addUri", []any{
		[]any{"http://example.com/download.bin"},
		map[string]any{"dir": t.TempDir(), "pause": "true"},
	})
	if err != nil {
		t.Fatalf("addUri: %v", err)
	}
	gid := rawGID.(string)
	for _, item := range driver.tasks {
		item.Status = task.StatusComplete
	}

	if _, err := service.Invoke(context.Background(), "aria2.removeDownloadResult", []any{gid}); err != nil {
		t.Fatalf("removeDownloadResult: %v", err)
	}

	raw, err := service.Invoke(context.Background(), "aria2.getGlobalStat", nil)
	if err != nil {
		t.Fatalf("getGlobalStat: %v", err)
	}
	stat := raw.(map[string]any)
	if stat["numStoppedTotal"] != "1" {
		t.Fatalf("expected numStoppedTotal=1, got %#v", stat)
	}
}
