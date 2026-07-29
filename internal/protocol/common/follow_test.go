package common

import "testing"

func TestShouldFollowTorrentURL(t *testing.T) {
	t.Parallel()

	if !ShouldFollowTorrentURL(map[string]string{}) {
		t.Fatal("expected default follow true")
	}
	if ShouldFollowTorrentURL(map[string]string{"follow-torrent": "false"}) {
		t.Fatal("expected follow-torrent=false")
	}
	if ShouldFollowTorrentURL(map[string]string{"follow-metalink": "false"}) {
		t.Fatal("expected follow-metalink=false")
	}
}
