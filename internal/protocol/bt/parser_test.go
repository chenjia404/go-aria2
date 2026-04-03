package bt

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestParseAddInputMagnet(t *testing.T) {
	result, err := parseAddInput(context.Background(), task.AddTaskInput{
		URIs: []string{"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=demo.iso&tr=http://tracker/announce&xl=123"},
	})
	if err != nil {
		t.Fatalf("parseAddInput returned error: %v", err)
	}

	if result.Spec.InfoHash.HexString() != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("unexpected info hash: %s", result.Spec.InfoHash.HexString())
	}
	if result.Source.DisplayName != "demo.iso" || result.Source.TotalLength != 123 {
		t.Fatalf("unexpected magnet source: %+v", result.Source)
	}
	if len(result.Source.Trackers) != 1 || result.Source.Trackers[0] != "http://tracker/announce" {
		t.Fatalf("unexpected trackers: %+v", result.Source.Trackers)
	}
}

func TestParseAddInputTorrentBytesAndRestore(t *testing.T) {
	payload := []byte("d8:announce14:http://tracker13:creation datei1712123456e4:infod6:lengthi123e4:name8:test.bin12:piece lengthi262144e6:pieces20:12345678901234567890ee")

	result, err := parseAddInput(context.Background(), task.AddTaskInput{
		Torrent: payload,
	})
	if err != nil {
		t.Fatalf("parseAddInput returned error: %v", err)
	}

	if result.Spec.DisplayName != "test.bin" {
		t.Fatalf("unexpected torrent name: %s", result.Spec.DisplayName)
	}
	if result.Source.Kind != "torrent-bytes" || result.Source.TorrentBase64 == "" {
		t.Fatalf("unexpected source payload: %+v", result.Source)
	}

	restored, err := restoreSource(map[string]string{
		"bt.source.kind":          result.Source.Kind,
		"bt.source.torrentBase64": result.Source.TorrentBase64,
	})
	if err != nil {
		t.Fatalf("restoreSource returned error: %v", err)
	}
	if restored.Spec.DisplayName != "test.bin" {
		t.Fatalf("unexpected restored display name: %s", restored.Spec.DisplayName)
	}
}
