package common

import "testing"

func TestEffectiveSegmentCount_MinSplitSize(t *testing.T) {
	t.Parallel()

	const twentyMiB = 20 * 1024 * 1024
	// 20MiB file, min-split 10M -> max 2 segments (20M/10M)
	got := EffectiveSegmentCount(twentyMiB, 0, 5, 0, 10*1024*1024)
	if got != 2 {
		t.Fatalf("got %d want 2", got)
	}
	// 20MiB file, min-split 15M -> only 1 segment (20M/15M = 1)
	got = EffectiveSegmentCount(twentyMiB, 0, 5, 0, 15*1024*1024)
	if got != 1 {
		t.Fatalf("got %d want 1", got)
	}
}

func TestBuildDownloadRanges_PieceLength(t *testing.T) {
	t.Parallel()

	ranges := BuildDownloadRanges(0, 5*1024*1024, 5, 1024*1024)
	if len(ranges) != 5 {
		t.Fatalf("expected 5 ranges, got %d", len(ranges))
	}
	if ranges[0][0] != 0 || ranges[0][1] != 1024*1024-1 {
		t.Fatalf("first range: %#v", ranges[0])
	}
}

func TestParseIndexOut(t *testing.T) {
	t.Parallel()

	m, err := ParseIndexOut("custom.bin")
	if err != nil || m[1] != "custom.bin" {
		t.Fatalf("default index 1: %#v %v", m, err)
	}
	m, err = ParseIndexOut("1=a.bin,2=b.bin")
	if err != nil || m[1] != "a.bin" || m[2] != "b.bin" {
		t.Fatalf("multi mapping: %#v %v", m, err)
	}
}
