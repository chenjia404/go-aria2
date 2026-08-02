package text

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestRouteKind_FTPAndSFTP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		uri      string
		protocol task.Protocol
	}{
		{"ftp://example.com/file.bin", task.Protocol("ftp")},
		{"sftp://user@host/path/file.bin", task.Protocol("sftp")},
	}
	for _, tc := range cases {
		got, err := routeKind(tc.uri)
		if err != nil {
			t.Fatalf("routeKind(%q): %v", tc.uri, err)
		}
		if got != tc.protocol {
			t.Fatalf("routeKind(%q) = %q, want %q", tc.uri, got, tc.protocol)
		}
	}
}

func TestEntryToTask_FTPURI(t *testing.T) {
	t.Parallel()

	item, err := entryToTask(Entry{
		URI: "ftp://example.com/downloads/file.bin",
		Dir: "/data",
		Out: "file.bin",
	})
	if err != nil {
		t.Fatalf("entryToTask: %v", err)
	}
	if item.Protocol != task.Protocol("ftp") {
		t.Fatalf("protocol: %q", item.Protocol)
	}
	if len(item.Files) != 1 || item.Files[0].URIs[0] != "ftp://example.com/downloads/file.bin" {
		t.Fatalf("files: %+v", item.Files)
	}
}
