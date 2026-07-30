package common

import (
	"context"
	"testing"
	"time"
)

func TestResolveBoolOption(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"yes", true},
		{"1", true},
		{"false", false},
		{"no", false},
		{"0", false},
	} {
		got := ResolveBoolOption(map[string]string{"pause": tc.value}, "pause", true)
		if got != tc.want {
			t.Fatalf("pause=%q: got %v want %v", tc.value, got, tc.want)
		}
	}
	if ResolveBoolOption(map[string]string{"pause": "maybe"}, "pause", false) {
		t.Fatal("unknown value should use fallback")
	}
}

func TestDownloadFilePolicy(t *testing.T) {
	t.Parallel()

	opts := map[string]string{"allow-overwrite": "false", "continue": "false"}
	if !ShouldRejectExistingFile(opts, 100) {
		t.Fatal("expected reject")
	}
	if ShouldResumePartial(opts, 100, 1000) {
		t.Fatal("continue=false should not resume")
	}

	opts = map[string]string{"allow-overwrite": "true", "continue": "false"}
	if !ShouldResetExistingFile(opts, 100) {
		t.Fatal("expected reset")
	}

	opts = map[string]string{"continue": "true"}
	if !ShouldResumePartial(opts, 50, 100) {
		t.Fatal("expected resume")
	}
}

func TestParseTimeoutSeconds(t *testing.T) {
	t.Parallel()

	d := ParseTimeoutSeconds(map[string]string{"connect-timeout": "30"}, "connect-timeout")
	if d != 30*time.Second {
		t.Fatalf("got %v", d)
	}
	if ParseTimeoutSeconds(nil, "timeout") != 0 {
		t.Fatal("expected zero")
	}
}

func TestSleepBetweenMirrors(t *testing.T) {
	t.Parallel()

	start := time.Now()
	if err := SleepBetweenMirrors(context.Background(), map[string]string{"retry-wait": "1"}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("expected ~1s wait, got %v", elapsed)
	}
}
