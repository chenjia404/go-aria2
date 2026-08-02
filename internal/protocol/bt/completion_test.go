package bt

import (
	"testing"
)

func TestOptionEnabled(t *testing.T) {
	t.Parallel()

	if !optionEnabled(map[string]string{"bt-detach-seed-only": "true"}, "bt-detach-seed-only") {
		t.Fatal("expected true")
	}
	if optionEnabled(map[string]string{"bt-detach-seed-only": "false"}, "bt-detach-seed-only") {
		t.Fatal("expected false")
	}
}

func TestApplyBTRateLimiters(t *testing.T) {
	t.Parallel()

	st := &state{}
	applyBTRateLimiters(st, map[string]string{
		"max-download-limit": "8192",
		"max-upload-limit":   "4096",
	})
	if st.downloadLimiter == nil || st.uploadLimiter == nil {
		t.Fatalf("expected limiters, dl=%v ul=%v", st.downloadLimiter, st.uploadLimiter)
	}
}

func TestSessionDetached(t *testing.T) {
	t.Parallel()

	d := &Driver{tasks: map[string]*state{"t1": {sessionDetached: true}}}
	if !d.SessionDetached("t1") {
		t.Fatal("expected detached")
	}
	if d.SessionDetached("missing") {
		t.Fatal("expected not detached for missing task")
	}
}
