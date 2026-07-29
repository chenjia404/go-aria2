package aria2

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// 参考 aria2 RpcMethodTest.cc::testChangeUri：多文件条目增删 URI 与返回值计数。

func setupChangeUriMultiFileTask(t *testing.T, env *rpcTestEnv) string {
	t.Helper()
	gid := env.MustGID("aria2.addUri", []any{"http://example.org/placeholder"}, map[string]any{"pause": "true"})
	for _, item := range env.Driver.tasks {
		if item.GID != gid {
			continue
		}
		item.Files = []task.File{
			{Index: 1, Path: "file-0", URIs: []string{}},
			{Index: 2, Path: "file-1", URIs: []string{
				"http://example.org/aria2.tar.bz2",
				"http://example.org/mustremove1",
				"http://example.org/mustremove2",
			}},
			{Index: 3, Path: "file-2", URIs: []string{}},
		}
	}
	return gid
}

func mustIntPair(t *testing.T, raw any) (int, int) {
	t.Helper()
	pair, ok := raw.([]any)
	if !ok || len(pair) != 2 {
		t.Fatalf("expected [delCount, addCount], got %#v", raw)
	}
	del, ok := pair[0].(int)
	if !ok {
		t.Fatalf("delCount type: %#v", pair[0])
	}
	add, ok := pair[1].(int)
	if !ok {
		t.Fatalf("addCount type: %#v", pair[1])
	}
	return del, add
}

func fileURIs(env *rpcTestEnv, gid string, fileIndex int) []string {
	for _, item := range env.Driver.tasks {
		if item.GID != gid {
			continue
		}
		if fileIndex < 1 || fileIndex > len(item.Files) {
			return nil
		}
		return append([]string(nil), item.Files[fileIndex-1].URIs...)
	}
	return nil
}

func TestRpcMethod_ChangeUri_Success(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{})
	gid := setupChangeUriMultiFileTask(t, env)

	raw := env.MustCall("aria2.changeUri", gid, 2,
		[]any{
			"http://example.org/mustremove1",
			"http://example.org/mustremove2",
			"http://example.org/notexist",
		},
		[]any{
			"http://example.org/added1",
			"http://example.org/added2",
			"baduri",
			"http://example.org/added3",
		},
	)
	del, add := mustIntPair(t, raw)
	if del != 2 || add != 3 {
		t.Fatalf("changeUri counts: del=%d add=%d", del, add)
	}

	want := []string{
		"http://example.org/aria2.tar.bz2",
		"http://example.org/added1",
		"http://example.org/added2",
		"http://example.org/added3",
	}
	if got := fileURIs(env, gid, 2); len(got) != len(want) {
		t.Fatalf("file[1] uris: %#v", got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("file[1] uris[%d]: got %q want %q (all=%#v)", i, got[i], want[i], got)
			}
		}
	}
	if len(fileURIs(env, gid, 1)) != 0 || len(fileURIs(env, gid, 3)) != 0 {
		t.Fatalf("other files should stay empty")
	}

	raw = env.MustCall("aria2.changeUri", gid, 2,
		[]any{},
		[]any{
			"http://example.org/added1-1",
			"http://example.org/added1-2",
		},
		2,
	)
	del, add = mustIntPair(t, raw)
	if del != 0 || add != 2 {
		t.Fatalf("changeUri with position counts: del=%d add=%d", del, add)
	}
	uris := fileURIs(env, gid, 2)
	if len(uris) != 6 || uris[2] != "http://example.org/added1-1" || uris[3] != "http://example.org/added1-2" {
		t.Fatalf("inserted uris: %#v", uris)
	}

	raw = env.MustCall("aria2.changeUri", gid, 1,
		[]any{},
		[]any{
			"http://example.org/added1-1",
			"http://example.org/added1-2",
		},
		1000,
	)
	del, add = mustIntPair(t, raw)
	if del != 0 || add != 2 {
		t.Fatalf("changeUri far position counts: del=%d add=%d", del, add)
	}
	if got := fileURIs(env, gid, 1); len(got) != 2 || got[0] != "http://example.org/added1-1" {
		t.Fatalf("file[0] uris: %#v", got)
	}
}

func TestRpcMethod_AddUri_WithPosition(t *testing.T) {
	t.Parallel()

	env := newRPCTestEnv(t, manager.Options{StartPaused: true})
	env.MustGID("aria2.addUri", []any{"http://uri1"})
	second := env.MustGID("aria2.addUri", []any{"http://uri2"}, map[string]any{}, 0)

	waiting := env.MustCall("aria2.tellWaiting", 0, 10).([]map[string]any)
	if len(waiting) < 2 || waiting[0]["gid"] != second {
		t.Fatalf("expected second task at queue head, got %#v", waiting)
	}
}
