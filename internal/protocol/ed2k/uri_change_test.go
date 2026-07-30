package ed2k

import (
	"context"
	"strings"
	"testing"

	"github.com/monkeyWie/goed2k/protocol"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeURI_ED2KURIs(t *testing.T) {
	t.Parallel()

	hash, err := protocol.HashFromString("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	uri := "ed2k://|file|demo.bin|123|" + hash.String() + "|/"

	driver := &Driver{
		tasks: map[string]*state{
			"task-1": {
				hash: hash,
				uris: []string{uri},
			},
		},
	}

	addURI := "ed2k://|file|demo.bin|123|" + hash.String() + "|s=1.2.3.4:4662|/"
	del, add, err := driver.ChangeURI(context.Background(), "task-1", 1, nil, []string{addURI}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if del != 0 || add != 1 {
		t.Fatalf("counts del=%d add=%d", del, add)
	}
	st := driver.tasks["task-1"]
	if len(st.uris) != 2 {
		t.Fatalf("uris: %#v", st.uris)
	}

	badHash := strings.Repeat("a", 32)
	badURI := "ed2k://|file|demo.bin|123|" + badHash + "|/"
	_, _, err = driver.ChangeURI(context.Background(), "task-1", 1, nil, []string{badURI}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.uris) != 2 {
		t.Fatalf("bad hash uri should be skipped: %#v", st.uris)
	}
}

func TestChangeURI_DeleteAndEmptyList(t *testing.T) {
	t.Parallel()

	hash, err := protocol.HashFromString("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	uri1 := "ed2k://|file|demo.bin|123|" + hash.String() + "|/"
	uri2 := "ed2k://|file|demo.bin|123|" + hash.String() + "|s=1.2.3.4:4662|/"

	driver := &Driver{
		tasks: map[string]*state{
			"task-1": {
				hash: hash,
				uris: []string{uri1, uri2},
			},
		},
	}

	del, add, err := driver.ChangeURI(context.Background(), "task-1", 1, []string{uri2}, nil, -1)
	if err != nil {
		t.Fatal(err)
	}
	if del != 1 || add != 0 {
		t.Fatalf("counts del=%d add=%d", del, add)
	}
	if len(driver.tasks["task-1"].uris) != 1 {
		t.Fatalf("uris: %#v", driver.tasks["task-1"].uris)
	}

	_, _, err = driver.ChangeURI(context.Background(), "task-1", 1, []string{uri1}, nil, -1)
	if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("expected empty list error, got %v", err)
	}
}

func TestCollectLinks_Dedupes(t *testing.T) {
	t.Parallel()

	hash := strings.Repeat("b", 32)
	uri := "ed2k://|file|a.bin|1|" + hash + "|/"
	links, err := collectLinks(task.AddTaskInput{
		URI:  uri,
		URIs: []string{uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
}
