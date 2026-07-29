package bt

import "testing"

func TestNeedsSeparateDHTPort(t *testing.T) {
	t.Parallel()

	if !needsSeparateDHTPort(Options{EnableDHT: true, ListenPort: 6881, DHTListenPort: 26701}) {
		t.Fatal("expected separate dht port")
	}
	if needsSeparateDHTPort(Options{EnableDHT: true, ListenPort: 6881, DHTListenPort: 6881}) {
		t.Fatal("same port should not need separate dht")
	}
	if needsSeparateDHTPort(Options{EnableDHT: false, ListenPort: 6881, DHTListenPort: 26701}) {
		t.Fatal("disabled dht should not need separate port")
	}
}
