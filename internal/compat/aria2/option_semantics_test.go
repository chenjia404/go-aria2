package aria2

import "testing"

func TestStoreOnlyOptionsAreKnownOptions(t *testing.T) {
	t.Parallel()

	samples := map[string]string{
		"bt-request-peer-speed-limit": "100K",
		"bt-enable-lpd":               "true",
		"bt-remove-unselected-file":   "false",
	"bt-detach-seed-only":         "false",
	"max-upload-limit":            "50K",
	"max-download-limit":          "100K",
	}
	for key := range storeOnlyOptions {
		sample, ok := samples[key]
		if !ok {
			t.Fatalf("missing sample value for storeOnly option %q", key)
		}
		if err := validateKnownOption(key, sample); err != nil {
			t.Fatalf("storeOnly option %q should pass validation: %v", key, err)
		}
	}
}

func TestStartupOnlyOptionsAreGlobalDisallowed(t *testing.T) {
	t.Parallel()

	for key := range startupOnlyOptions {
		if _, ok := globalDisallowedOptions[key]; !ok {
			t.Fatalf("startup-only option %q should be in globalDisallowedOptions", key)
		}
	}
}

func TestUnimplementedOptionsEmpty(t *testing.T) {
	t.Parallel()

	if len(unimplementedOptions) > 0 {
		t.Fatalf("unexpected unimplemented options: %#v", unimplementedOptions)
	}
}
