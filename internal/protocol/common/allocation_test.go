package common

import (
	"path/filepath"
	"testing"
)

func TestParseFileAllocation(t *testing.T) {
	t.Parallel()

	if got := ParseFileAllocation(map[string]string{"file-allocation": "prealloc"}); got != FileAllocationPrealloc {
		t.Fatalf("got %q", got)
	}
	if got := ParseFileAllocation(map[string]string{"file-allocation": "bad"}); got != FileAllocationNone {
		t.Fatalf("got %q", got)
	}
}

func TestPrepareDownloadFile_Prealloc(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "pre.bin")
	file, offset, err := PrepareDownloadFile(path, FileAllocationPrealloc, 0, 128, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if offset != 0 {
		t.Fatalf("offset=%d", offset)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 128 {
		t.Fatalf("size=%d", info.Size())
	}
}

func TestParseHeaderOption(t *testing.T) {
	t.Parallel()

	headers := ParseHeaderOption("X-A: 1\nX-B:two\nbadline")
	if headers["X-A"] != "1" || headers["X-B"] != "two" {
		t.Fatalf("%#v", headers)
	}
	if _, ok := headers["badline"]; ok {
		t.Fatal("invalid line should be ignored")
	}
}
