package bt

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestChangeURI_BTWebSeeds(t *testing.T) {
	t.Parallel()

	driver, err := New(Options{DataDir: t.TempDir(), EnableDHT: false})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()

	item, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
		URIs:    []string{"http://example.com/seed.bin"},
		SaveDir: t.TempDir(),
		Options: map[string]string{"pause": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}

	del, add, err := driver.ChangeURI(context.Background(), item.ID, 1,
		[]string{"http://example.com/seed.bin"},
		[]string{"http://example.com/new-seed.bin", "bad"},
		-1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if del != 1 || add != 1 {
		t.Fatalf("counts del=%d add=%d", del, add)
	}

	status, err := driver.TellStatus(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Files) == 0 || len(status.Files[0].URIs) == 0 {
		t.Fatalf("expected uris in files: %#v", status.Files)
	}
}
