package aria2

import (
	"encoding/json"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestToBitTorrentResponseAnnounceListNested(t *testing.T) {
	item := &task.Task{
		Protocol: task.ProtocolBT,
		Name:     "x",
		Meta: map[string]string{
			"bt.mode":     "single",
			"bt.trackers": "http://a/announce\nhttp://b/announce",
		},
	}
	bt := toBitTorrentResponse(item)
	raw, err := json.Marshal(bt["announceList"])
	if err != nil {
		t.Fatal(err)
	}
	// 必须是 JSON 二维数组，不能是 ["url","url"]
	if string(raw) != `[["http://a/announce"],["http://b/announce"]]` {
		t.Fatalf("unexpected announceList JSON: %s", raw)
	}
}
