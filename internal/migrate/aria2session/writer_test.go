package aria2session

import (
	"strings"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestFormatAria2Session_RoundTrip(t *testing.T) {
	t.Parallel()

	original := `https://example.com/file.bin
 gid=0123456789abcdef
 dir=/downloads
 pause=true
 out=file.bin
`
	entries, err := ParseAria2SessionReader(strings.NewReader(original))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	item, err := previewTask(entries[0])
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	item.Meta = map[string]string{
		"http.sourceURL": "https://example.com/file.bin",
	}

	data, err := FormatAria2Session([]*task.Task{item})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	round, err := ParseAria2SessionReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("roundtrip parse: %v", err)
	}
	if len(round) != 1 {
		t.Fatalf("expected 1 roundtrip entry, got %d", len(round))
	}
	if round[0].URI != "https://example.com/file.bin" {
		t.Fatalf("uri mismatch: %q", round[0].URI)
	}
	if !round[0].Paused {
		t.Fatal("expected paused in roundtrip")
	}
}
