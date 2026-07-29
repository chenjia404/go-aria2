package aria2

import (
	"encoding/json"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestToStatusResponseRelatedDownloads(t *testing.T) {
	t.Parallel()

	item := &task.Task{
		GID:            "gid-leader",
		Status:         task.StatusComplete,
		FollowingGID:   "gid-parent",
		BelongsToGID:   "gid-parent",
		FollowedByGIDs: []string{"gid-child-1", "gid-child-2"},
		Files:          []task.File{{Index: 1, Path: "a.bin"}},
	}
	resp := toStatusResponse(item, nil)
	followedBy, ok := resp["followedBy"].([]string)
	if !ok || len(followedBy) != 2 {
		t.Fatalf("followedBy: %#v", resp["followedBy"])
	}
	if resp["following"] != "gid-parent" || resp["belongsTo"] != "gid-parent" {
		t.Fatalf("unexpected links: %#v", resp)
	}

	filtered := toStatusResponse(item, []string{"gid"})
	if len(filtered) != 1 || filtered["gid"] != "gid-leader" {
		t.Fatalf("keys filter should only return gid: %#v", filtered)
	}
}

func TestToBitTorrentResponseGatherMetadata(t *testing.T) {
	t.Parallel()

	item := &task.Task{
		Protocol: task.ProtocolBT,
		Name:     "aria2-test",
		Meta: map[string]string{
			"bt.mode":         "multi",
			"bt.comment":      "REDNOAH.COM RULES",
			"bt.createdBy":    "aria2",
			"bt.creationDate": "1123456789",
			"bt.trackers":     "http://tracker1\nhttp://tracker2\nhttp://tracker3",
		},
	}
	bt := toBitTorrentResponse(item)
	if bt["mode"] != "multi" {
		t.Fatalf("mode: %#v", bt["mode"])
	}
	if bt["comment"] != "REDNOAH.COM RULES" {
		t.Fatalf("comment: %#v", bt["comment"])
	}
	creationDate, ok := bt["creationDate"].(int64)
	if !ok || creationDate != 1123456789 {
		t.Fatalf("creationDate should be int64, got %#v", bt["creationDate"])
	}
	info := bt["info"].(map[string]any)
	if info["name"] != "aria2-test" {
		t.Fatalf("info.name: %#v", info["name"])
	}
	announceList := bt["announceList"].([][]string)
	if len(announceList) != 3 || announceList[0][0] != "http://tracker1" {
		t.Fatalf("announceList: %#v", announceList)
	}

	status := toStatusResponse(item, nil)
	btSection, ok := status["bittorrent"].(map[string]any)
	if !ok || btSection["creationDate"] != int64(1123456789) {
		t.Fatalf("tellStatus bittorrent section: %#v", status["bittorrent"])
	}
}

func TestToBitTorrentResponseDefaultsModeSingle(t *testing.T) {
	t.Parallel()

	item := &task.Task{Protocol: task.ProtocolBT, Name: "x"}
	bt := toBitTorrentResponse(item)
	if bt["mode"] != "single" {
		t.Fatalf("expected default mode single, got %#v", bt["mode"])
	}
}

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

func TestToStatusResponseErrorCodeAndBTFields(t *testing.T) {
	t.Parallel()

	item := &task.Task{
		GID:      "abc",
		Status:   task.StatusActive,
		Protocol: task.ProtocolBT,
		Meta: map[string]string{
			"bt.totalPieces": "16",
			"bitfield":       "ff00",
		},
	}
	resp := toStatusResponse(item, nil)
	if resp["errorCode"] != "0" {
		t.Fatalf("expected errorCode 0, got %#v", resp["errorCode"])
	}
	if resp["numPieces"] != "16" {
		t.Fatalf("expected numPieces, got %#v", resp["numPieces"])
	}
	if resp["bitfield"] != "ff00" {
		t.Fatalf("expected bitfield, got %#v", resp["bitfield"])
	}
}
