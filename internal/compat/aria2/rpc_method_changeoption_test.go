package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// 参考 aria2 RpcMethodTest.cc::testChangeOption，验证速度限制规范化与 BT 选项写入。

func TestRpcMethod_ChangeOption_NormalizesSpeedLimits(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addUri", []any{"http://localhost/1"}, map[string]any{"pause": "true"})

	if raw := env.MustCall("aria2.changeOption", gid, map[string]any{
		"max-download-limit": "100K",
		"max-upload-limit":   "50K",
	}); raw != "OK" {
		t.Fatalf("changeOption: %#v", raw)
	}

	opts := env.Option(gid)
	if opts["max-download-limit"] != "102400" {
		t.Fatalf("max-download-limit: %#v", opts["max-download-limit"])
	}
	if opts["max-upload-limit"] != "51200" {
		t.Fatalf("max-upload-limit: %#v", opts["max-upload-limit"])
	}
}

func TestRpcMethod_ChangeOption_StoresBTOptionsWithoutDriverEffect(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := env.MustGID("aria2.addTorrent", sampleTorrentBase64(), nil, map[string]any{"pause": "true"})

	if raw := env.MustCall("aria2.changeOption", gid, map[string]any{
		"bt-max-peers":                "100",
		"bt-request-peer-speed-limit": "300K",
	}); raw != "OK" {
		t.Fatalf("changeOption: %#v", raw)
	}

	opts := env.Option(gid)
	if opts["bt-max-peers"] != "100" {
		t.Fatalf("bt-max-peers: %#v", opts["bt-max-peers"])
	}
	if opts["bt-request-peer-speed-limit"] != "307200" {
		t.Fatalf("bt-request-peer-speed-limit: %#v", opts["bt-request-peer-speed-limit"])
	}
}

func TestRpcMethod_ChangeGlobalOption_NormalizesSpeedLimits(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	changed := mustStringMap(t, env.MustCall("aria2.changeGlobalOption", map[string]any{
		"max-overall-download-limit": "200K",
		"max-overall-upload-limit":   "100K",
	}))
	if changed["max-overall-download-limit"] != "204800" {
		t.Fatalf("max-overall-download-limit: %#v", changed["max-overall-download-limit"])
	}
	if changed["max-overall-upload-limit"] != "102400" {
		t.Fatalf("max-overall-upload-limit: %#v", changed["max-overall-upload-limit"])
	}
}
