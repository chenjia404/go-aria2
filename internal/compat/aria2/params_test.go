package aria2

import (
	"encoding/base64"
	"testing"
)

func TestDecodeTorrentPayloadVariants(t *testing.T) {
	t.Parallel()

	payload := []byte{0x64, 0x38, 0x3a, 0x61, 0x6e, 0x6e, 0x6f, 0x75, 0x6e, 0x63, 0x65}
	encoded := base64.StdEncoding.EncodeToString(payload)

	cases := []struct {
		name  string
		value any
	}{
		{"base64", encoded},
		{"buffer-json", map[string]any{
			"type": "Buffer",
			"data": []any{float64(100), float64(56), float64(58), float64(97), float64(110), float64(110), float64(111), float64(117), float64(110), float64(99), float64(101)},
		}},
		{"byte-array", []any{float64(100), float64(56), float64(58), float64(97), float64(110), float64(110), float64(111), float64(117), float64(110), float64(99), float64(101)}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeTorrentPayload(tc.value)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if string(got) != string(payload) {
				t.Fatalf("unexpected payload: %q", got)
			}
		})
	}
}

func TestParseAddTorrentParams(t *testing.T) {
	t.Parallel()

	payload := []byte("torrent-data")
	encoded := base64.StdEncoding.EncodeToString(payload)

	twoArg, uris, opts, pos, err := parseAddTorrentParams([]any{
		encoded,
		map[string]any{"dir": "/tmp", "pause": true},
	})
	if err != nil {
		t.Fatalf("two-arg parse failed: %v", err)
	}
	if string(twoArg) != string(payload) || len(uris) != 0 || opts["dir"] != "/tmp" || opts["pause"] != "true" || pos != -1 {
		t.Fatalf("unexpected two-arg result: payload=%q uris=%v opts=%v pos=%d", twoArg, uris, opts, pos)
	}

	threeArgNull, uris2, opts2, pos2, err := parseAddTorrentParams([]any{
		encoded,
		nil,
		map[string]any{"dir": "/data"},
	})
	if err != nil {
		t.Fatalf("three-arg null parse failed: %v", err)
	}
	if string(threeArgNull) != string(payload) || len(uris2) != 0 || opts2["dir"] != "/data" || pos2 != -1 {
		t.Fatalf("unexpected three-arg null result: payload=%q uris=%v opts=%v pos=%d", threeArgNull, uris2, opts2, pos2)
	}

	threeArgURIs, uris3, opts3, pos3, err := parseAddTorrentParams([]any{
		encoded,
		[]any{"http://seed.example/a", "http://seed.example/b"},
		map[string]any{"out": "file.bin"},
	})
	if err != nil {
		t.Fatalf("three-arg uris parse failed: %v", err)
	}
	if len(uris3) != 2 || opts3["out"] != "file.bin" || pos3 != -1 {
		t.Fatalf("unexpected three-arg uris result: uris=%v opts=%v pos=%d", uris3, opts3, pos3)
	}
	_ = threeArgURIs

	withPosition, _, opts4, pos4, err := parseAddTorrentParams([]any{
		encoded,
		map[string]any{"dir": "/queue"},
		0,
	})
	if err != nil {
		t.Fatalf("position parse failed: %v", err)
	}
	if string(withPosition) != string(payload) || opts4["dir"] != "/queue" || pos4 != 0 {
		t.Fatalf("unexpected position result: opts=%v pos=%d", opts4, pos4)
	}
}
