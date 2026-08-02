package bt

import "testing"

func TestTorrentIntegritySnapshot_NilTorrent(t *testing.T) {
	t.Parallel()

	verified, pending := torrentIntegritySnapshot(nil)
	if verified != 0 || pending {
		t.Fatalf("nil torrent: verified=%d pending=%v", verified, pending)
	}
}

func TestRunCheckIntegrityIfNeeded_RequiresOption(t *testing.T) {
	t.Parallel()

	d := &Driver{opts: Options{CheckIntegrity: false}}
	st := &state{}
	d.runCheckIntegrityIfNeeded("task-1", st)
	if st.integrityRunning {
		t.Fatal("expected no integrity run without check-integrity option")
	}
}
