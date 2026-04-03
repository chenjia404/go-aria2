package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectStartupJobsMergesCliDefaults(t *testing.T) {
	t.Parallel()

	opts := daemonCLIOptions{
		startup: map[string]string{
			"dir":   "/downloads",
			"out":   "movie.mkv",
			"gid":   "0123456789abcdef",
			"split": "4",
		},
		uris: []string{"https://example.com/file.torrent"},
	}

	jobs, err := collectStartupJobs(opts)
	if err != nil {
		t.Fatalf("collectStartupJobs returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	if jobs[0].Options["dir"] != "/downloads" || jobs[0].Options["out"] != "movie.mkv" || jobs[0].Options["gid"] != "0123456789abcdef" {
		t.Fatalf("unexpected startup job options: %#v", jobs[0].Options)
	}
}

func TestParseStartupInputFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")
	content := "\nhttps://example.com/a.bin\tdir=/downloads\n out=a.bin\nhttps://example.com/b.bin\n gid=0123456789abcdef\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write input file: %v", err)
	}

	jobs, err := parseStartupInputFile(path)
	if err != nil {
		t.Fatalf("parseStartupInputFile returned error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if len(jobs[0].URIs) != 2 {
		t.Fatalf("expected first job to have 2 uris, got %#v", jobs[0].URIs)
	}
	if jobs[0].Options["out"] != "a.bin" {
		t.Fatalf("expected out option on first job, got %#v", jobs[0].Options)
	}
	if jobs[1].Options["gid"] != "0123456789abcdef" {
		t.Fatalf("expected gid option on second job, got %#v", jobs[1].Options)
	}
}
