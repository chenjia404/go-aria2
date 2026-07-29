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
		"file-allocation": "none",
		"enable-rpc":      "100K",
		"out":             "foo.bin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := filtered["out"]; ok {
		t.Fatal("task-only option should be filtered")
	}
	if _, ok := filtered["enable-rpc"]; ok {
		t.Fatal("disallowed global option should be filtered")
	}
	if filtered["file-allocation"] != "none" {
		t.Fatalf("expected file-allocation preserved, got %#v", filtered)
	}
}

func TestFilterHiddenOptions(t *testing.T) {
	t.Parallel()

	filtered := filterHiddenOptions(map[string]string{
		"dir":               "/tmp",
		"startup-idle-time": "60",
	})
	if _, ok := filtered["startup-idle-time"]; ok {
		t.Fatal("hidden option should be removed")
	}
	if filtered["dir"] != "/tmp" {
		t.Fatalf("expected dir preserved, got %#v", filtered)
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
}
