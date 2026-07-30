package aria2

import (
	"testing"
)

func TestParseSpeedLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"100K", 102400},
		{"50K", 51200},
		{"1M", 1024 * 1024},
	}
	for _, tc := range cases {
		got, err := parseSpeedLimit(tc.in)
		if err != nil {
			t.Fatalf("parseSpeedLimit(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseSpeedLimit(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	if _, err := parseSpeedLimit("badvalue"); err == nil {
		t.Fatal("expected error for badvalue")
	}
}

func TestValidateKnownOption_FileAllocation(t *testing.T) {
	t.Parallel()

	if err := validateKnownOption("file-allocation", "none"); err != nil {
		t.Fatalf("valid value rejected: %v", err)
	}
	if err := validateKnownOption("file-allocation", "foo"); err == nil {
		t.Fatal("expected error for invalid file-allocation")
	}
}

func TestPrepareChangeTaskOptions_FiltersGlobalOnly(t *testing.T) {
	t.Parallel()

	filtered, err := prepareChangeTaskOptions(map[string]string{
		"dir":                        "/tmp",
		"max-overall-download-limit": "100K",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := filtered["max-overall-download-limit"]; ok {
		t.Fatal("global-only option should be filtered")
	}
	if filtered["dir"] != "/tmp" {
		t.Fatalf("expected dir preserved, got %#v", filtered)
	}
}

func TestPrepareChangeGlobalOptions_FiltersDisallowed(t *testing.T) {
	t.Parallel()

	filtered, err := prepareChangeGlobalOptions(map[string]string{
		"file-allocation":            "none",
		"enable-rpc":                 "100K",
		"out":                        "foo.bin",
		"max-overall-download-limit": "badvalue",
	})
	if err == nil {
		t.Fatal("expected error for bad max-overall-download-limit")
	}

	filtered, err = prepareChangeGlobalOptions(map[string]string{
		"max-overall-download-limit": "100K",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filtered["max-overall-download-limit"] != "102400" {
		t.Fatalf("expected max-overall-download-limit preserved, got %#v", filtered)
	}

	filtered, err = prepareChangeGlobalOptions(map[string]string{"file-allocation": "none"})
	if err != nil {
		t.Fatalf("file-allocation should be accepted: %v", err)
	}
	if filtered["file-allocation"] != "none" {
		t.Fatalf("expected file-allocation preserved, got %#v", filtered)
	}
}

func TestPrepareChangeGlobalOptions_FiltersAllDisallowed(t *testing.T) {
	t.Parallel()

	input := map[string]string{
		"file-allocation": "none",
		"dir":             "/tmp/global",
	}
	for key := range globalDisallowedOptions {
		input[key] = "ignored"
	}
	for _, key := range []string{"out", "pause", "max-download-limit", "select-file"} {
		input[key] = "ignored"
	}

	filtered, err := prepareChangeGlobalOptions(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filtered["file-allocation"] != "none" {
		t.Fatalf("expected file-allocation preserved, got %#v", filtered)
	}
	if filtered["dir"] != "/tmp/global" {
		t.Fatalf("expected dir preserved, got %#v", filtered)
	}
	for key := range globalDisallowedOptions {
		if _, ok := filtered[key]; ok {
			t.Fatalf("globalDisallowed option %q should be filtered", key)
		}
	}
	for _, key := range []string{"out", "pause", "max-download-limit", "select-file"} {
		if _, ok := filtered[key]; ok {
			t.Fatalf("task-only option %q should be filtered", key)
		}
	}
}

func TestIsTaskOnlyOption(t *testing.T) {
	t.Parallel()

	taskOnly := []string{"out", "pause", "gid", "index-out", "select-file", "split", "max-download-limit", "max-upload-limit"}
	for _, key := range taskOnly {
		if !isTaskOnlyOption(key) {
			t.Fatalf("%q should be task-only", key)
		}
	}
	for _, key := range []string{"dir", "file-allocation", "max-overall-download-limit"} {
		if isTaskOnlyOption(key) {
			t.Fatalf("%q should not be task-only", key)
		}
	}
}

func TestFilterHiddenOptions(t *testing.T) {
	t.Parallel()

	filtered := filterHiddenOptions(map[string]string{
		"dir":               "/tmp",
		"startup-idle-time": "60",
		"pause":             "true",
	})
	if _, ok := filtered["startup-idle-time"]; ok {
		t.Fatal("hidden option should be removed")
	}
	if _, ok := filtered["pause"]; ok {
		t.Fatal("pause should be hidden from getOption")
	}
	if filtered["dir"] != "/tmp" {
		t.Fatalf("expected dir preserved, got %#v", filtered)
	}
}

func TestValidateKnownOption_PositiveIntOptions(t *testing.T) {
	t.Parallel()

	if err := validateKnownOption("split", "16"); err != nil {
		t.Fatalf("valid split rejected: %v", err)
	}
	if err := validateKnownOption("split", "bad"); err == nil {
		t.Fatal("expected error for invalid split")
	}
	if err := validateKnownOption("seed-ratio", "1.5"); err != nil {
		t.Fatalf("valid seed-ratio rejected: %v", err)
	}
	if err := validateKnownOption("seed-ratio", "foo"); err == nil {
		t.Fatal("expected error for invalid seed-ratio")
	}
	if err := validateKnownOption("seed-time", "3600"); err != nil {
		t.Fatalf("valid seed-time rejected: %v", err)
	}
	if err := validateKnownOption("connect-timeout", "bad"); err == nil {
		t.Fatal("expected error for invalid connect-timeout")
	}
	if err := validateKnownOption("bt-request-peer-speed-limit", "300K"); err != nil {
		t.Fatalf("valid bt-request-peer-speed-limit rejected: %v", err)
	}
	if err := validateKnownOption("checksum", "sha-1=deadbeef"); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := validateKnownOption("checksum", "bad"); err == nil {
		t.Fatal("expected error for invalid checksum")
	}
}

func TestIsValidURI(t *testing.T) {
	t.Parallel()

	if !isValidURI("http://localhost/1") {
		t.Fatal("expected http uri to be valid")
	}
	if !isValidURI("magnet:?xt=urn:btih:abc") {
		t.Fatal("expected magnet uri to be valid")
	}
	if isValidURI("not uri") {
		t.Fatal("expected plain text to be invalid")
	}
	if !isValidURI("ftp://example.com/a") {
		t.Fatal("expected ftp uri to be valid")
	}
	if !isValidURI("sftp://example.com/a") {
		t.Fatal("expected sftp uri to be valid")
	}
}
