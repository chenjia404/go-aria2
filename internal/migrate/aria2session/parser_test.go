package aria2session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestParseAria2Session(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session.txt")
	content := "\n# comment\nmagnet:?xt=urn:btih:aaaa\n gid=0123456789abcdef\n dir=/downloads\n out=movie.mkv\n pause=true\n checksum=sha1=abcdef\n metalink=http://example.com/movie.meta4\n\nhttp://example.com/file.torrent\n dir=/torrents\n\ned2k://|file|sample.iso|123|abcdef|/\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	tasks, err := ParseAria2Session(path)
	if err != nil {
		t.Fatalf("ParseAria2Session returned error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].URI == "" || tasks[0].GID != "0123456789abcdef" || tasks[0].Dir != "/downloads" || tasks[0].Out != "movie.mkv" {
		t.Fatalf("unexpected first task: %#v", tasks[0])
	}
	if !tasks[0].Paused || tasks[0].Checksum != "sha1=abcdef" || tasks[0].Metalink != "http://example.com/movie.meta4" {
		t.Fatalf("unexpected first task flags: %#v", tasks[0])
	}
	if tasks[1].Options["dir"] != "/torrents" {
		t.Fatalf("unexpected second task: %#v", tasks[1])
	}
	if tasks[2].URI[:7] != "ed2k://" {
		t.Fatalf("unexpected third task: %#v", tasks[2])
	}
}

func TestPreviewTaskAndGID(t *testing.T) {
	t.Parallel()

	item := Aria2SessionTask{
		URI:    "http://example.com/file.bin",
		GID:    "0123456789abcdef",
		Dir:    "/downloads",
		Out:    "file.bin",
		Paused: true,
		Options: map[string]string{
			"checksum": "sha1=deadbeef",
		},
	}
	preview, err := ImportAria2Tasks([]Aria2SessionTask{item})
	if err != nil {
		t.Fatalf("ImportAria2Tasks preview returned error: %v", err)
	}
	if len(preview) != 1 {
		t.Fatalf("expected one preview task, got %d", len(preview))
	}
	if preview[0].Protocol != task.ProtocolHTTP || preview[0].GID != "0123456789abcdef" {
		t.Fatalf("unexpected preview task: %#v", preview[0])
	}
	if preview[0].Status != task.StatusPaused {
		t.Fatalf("expected paused preview task: %#v", preview[0])
	}
	if preview[0].Meta["aria2.checksum"] != "sha1=deadbeef" || preview[0].Meta["aria2.paused"] != "true" {
		t.Fatalf("unexpected preview meta: %#v", preview[0].Meta)
	}
}

func TestBuildAddInputCarriesSessionFields(t *testing.T) {
	t.Parallel()

	input, err := buildAddInput(Aria2SessionTask{
		URI:      "magnet:?xt=urn:btih:aaaa",
		GID:      "0123456789abcdef",
		Paused:   true,
		Checksum: "sha1=deadbeef",
		Metalink: "http://example.com/movie.meta4",
	}, false)
	if err != nil {
		t.Fatalf("buildAddInput returned error: %v", err)
	}
	if input.GID != "0123456789abcdef" {
		t.Fatalf("expected preserved gid, got %#v", input.GID)
	}
	if input.Options["pause"] != "true" {
		t.Fatalf("expected pause option, got %#v", input.Options)
	}
	if input.Meta["aria2.checksum"] != "sha1=deadbeef" || input.Meta["aria2.metalink"] != "http://example.com/movie.meta4" {
		t.Fatalf("unexpected meta mapping: %#v", input.Meta)
	}
}
