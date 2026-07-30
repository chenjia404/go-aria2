package ftp

import "testing"

func TestParseEndpoint_LocalFTPURI(t *testing.T) {
	t.Parallel()

	ep, err := parseEndpoint("ftp://127.0.0.1:12345/file.bin")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ep.remote != "file.bin" {
		t.Fatalf("remote: %q", ep.remote)
	}
}
