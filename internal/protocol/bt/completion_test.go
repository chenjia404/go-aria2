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

func TestTaskOptionEnabled_DriverDefault(t *testing.T) {
	t.Parallel()

	d := &Driver{opts: Options{DetachSeedOnly: true}}
	st := &state{options: map[string]string{}}
	if !d.taskOptionEnabled(st, "bt-detach-seed-only") {
		t.Fatal("expected driver default detach-seed-only")
	}
	if d.taskOptionEnabled(st, "bt-remove-unselected-file") {
		t.Fatal("expected remove-unselected false by default")
	}
}

func TestDriverCloseStopsRateLimitLoop(t *testing.T) {
	t.Parallel()

	dataDir := mustTempDir(t)
	defer removeDirEventually(t, dataDir)
	driver, err := New(Options{DataDir: dataDir, ListenPort: 0})
	if err != nil {
		t.Fatalf("new bt driver: %v", err)
	}
	if err := driver.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-driver.stopCh:
	default:
		t.Fatal("expected stopCh closed after Close")
	}
}
