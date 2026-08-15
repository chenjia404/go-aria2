package ftp

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

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

func TestApplyFTPAuthFromOptions(t *testing.T) {
	t.Parallel()

	input := task.AddTaskInput{
		URIs:    []string{"ftp://127.0.0.1/file.bin"},
		Options: map[string]string{"ftp-user": "bob", "ftp-passwd": "secret"},
	}
	eps, err := collectEndpoints(input)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	applyFTPAuth(eps, input.Options)
	if eps[0].user != "bob" || eps[0].password != "secret" {
		t.Fatalf("auth: %+v", eps[0])
	}

	input = task.AddTaskInput{
		URIs:    []string{"ftp://alice:pw@127.0.0.1/file.bin"},
		Options: map[string]string{"ftp-user": "bob", "ftp-passwd": "secret"},
	}
	eps, err = collectEndpoints(input)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	applyFTPAuth(eps, input.Options)
	if eps[0].user != "alice" {
		t.Fatalf("URI user should win, got %q", eps[0].user)
	}
}
