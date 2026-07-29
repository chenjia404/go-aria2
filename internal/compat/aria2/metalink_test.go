package aria2

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

func TestAddMetalinkCreatesMultipleTasks(t *testing.T) {
	t.Parallel()

	service := NewService(manager.New(manager.Options{GlobalOptions: map[string]string{"dir": t.TempDir()}}), "")
	driver := newRPCStubDriver()
	service.manager.RegisterDriver(driver)

	payload := base64.StdEncoding.EncodeToString([]byte(`<?xml version="1.0" encoding="utf-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="a.bin">
    <url>http://example.com/a.bin</url>
  </file>
  <file name="b.bin">
    <url>http://example.com/b.bin</url>
  </file>
</metalink>`))

	raw, err := service.Invoke(context.Background(), "aria2.addMetalink", []any{
		payload,
		map[string]any{"dir": t.TempDir()},
	})
	if err != nil {
		t.Fatalf("addMetalink returned error: %v", err)
	}
	gids, ok := raw.([]string)
	if !ok || len(gids) != 2 {
		t.Fatalf("expected 2 gids, got %#v", raw)
	}
}
