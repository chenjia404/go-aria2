package bt

import (
	"testing"

	torrentlib "github.com/anacrolix/torrent"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func TestEffectiveOptsForSessionRestore_prefersLocalOverStaleMerged(t *testing.T) {
	global := map[string]string{"bt-tracker": "udp://global/announce"}
	saved := &task.Task{
		Options: map[string]string{"bt-tracker": "udp://old-saved-merge/announce"},
		LocalOptions: map[string]string{
			"bt-tracker": "udp://per-task/announce",
		},
	}
	eff := effectiveOptsForSessionRestore(saved, global)
	if eff["bt-tracker"] != "udp://per-task/announce" {
		t.Fatalf("task LocalOptions should override global: got %q", eff["bt-tracker"])
	}
}

func TestEffectiveOptsForSessionRestore_legacyUsesOptions(t *testing.T) {
	global := map[string]string{"bt-tracker": "udp://global/announce"}
	saved := &task.Task{
		Options: map[string]string{"bt-tracker": "udp://merged-at-save/announce"},
	}
	eff := effectiveOptsForSessionRestore(saved, global)
	if eff["bt-tracker"] != "udp://merged-at-save/announce" {
		t.Fatalf("legacy: use saved.Options: got %q", eff["bt-tracker"])
	}
}

func TestApplyBTTrackerOpts_excludePrefix(t *testing.T) {
	spec := &torrentlib.TorrentSpec{
		Trackers: [][]string{
			{"udp://bad/announce", "udp://good/announce"},
		},
	}
	src := addSource{}
	applyBTTrackerOpts(spec, &src, map[string]string{
		"bt-exclude-tracker": "udp://bad",
	})
	if len(spec.Trackers) != 1 || len(spec.Trackers[0]) != 1 || spec.Trackers[0][0] != "udp://good/announce" {
		t.Fatalf("unexpected trackers: %#v", spec.Trackers)
	}
	if len(src.Trackers) != 1 || src.Trackers[0] != "udp://good/announce" {
		t.Fatalf("unexpected source trackers: %#v", src.Trackers)
	}
}

func TestApplyBTTrackerOpts_appendExtra(t *testing.T) {
	spec := &torrentlib.TorrentSpec{
		Trackers: [][]string{{"udp://a/announce"}},
	}
	src := addSource{}
	applyBTTrackerOpts(spec, &src, map[string]string{
		"bt-tracker": "udp://b/announce, udp://c/announce",
	})
	if len(spec.Trackers) != 3 {
		t.Fatalf("want 3 tiers, got %#v", spec.Trackers)
	}
	wantFlat := []string{"udp://a/announce", "udp://b/announce", "udp://c/announce"}
	if len(src.Trackers) != len(wantFlat) {
		t.Fatalf("source trackers %#v", src.Trackers)
	}
	for i, w := range wantFlat {
		if src.Trackers[i] != w {
			t.Fatalf("idx %d want %q got %q", i, w, src.Trackers[i])
		}
	}
}

func TestApplyBTTrackerOpts_dedupAndExcludeExtra(t *testing.T) {
	spec := &torrentlib.TorrentSpec{
		Trackers: [][]string{{"udp://keep/announce"}},
	}
	src := addSource{}
	applyBTTrackerOpts(spec, &src, map[string]string{
		"bt-tracker":         "udp://keep/announce, udp://new/announce",
		"bt-exclude-tracker": "udp://skip",
	})
	// duplicate keep not added; udp://skip/... would be excluded if present
	if len(spec.Trackers) != 2 {
		t.Fatalf("want 2 tiers, got %#v", spec.Trackers)
	}
	if spec.Trackers[1][0] != "udp://new/announce" {
		t.Fatalf("second tier: %#v", spec.Trackers[1])
	}
	if len(src.Trackers) != 2 {
		t.Fatalf("source trackers %#v", src.Trackers)
	}
}

func TestApplyBTTrackerOpts_excludeBlocksAppended(t *testing.T) {
	spec := &torrentlib.TorrentSpec{Trackers: [][]string{{"udp://ok/announce"}}}
	src := addSource{}
	applyBTTrackerOpts(spec, &src, map[string]string{
		"bt-tracker":         "udp://blocked/path",
		"bt-exclude-tracker": "udp://blocked",
	})
	if len(spec.Trackers) != 1 {
		t.Fatalf("blocked extra should not appear: %#v", spec.Trackers)
	}
}
