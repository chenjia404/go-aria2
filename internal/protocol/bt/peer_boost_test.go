package bt

import "testing"

func TestEffectiveRequestPeerThreshold(t *testing.T) {
	t.Parallel()

	if got := effectiveRequestPeerThreshold(nil, defaultRequestPeerSpeedLimit); got != defaultRequestPeerSpeedLimit {
		t.Fatalf("expected default %d, got %d", defaultRequestPeerSpeedLimit, got)
	}
	if got := effectiveRequestPeerThreshold(map[string]string{"bt-request-peer-speed-limit": "102400"}, 0); got != 102400 {
		t.Fatalf("unexpected task override: %d", got)
	}
	if got := effectiveRequestPeerThreshold(map[string]string{
		"bt-request-peer-speed-limit": "204800",
		"max-download-limit":          "102400",
	}, 0); got != 102400 {
		t.Fatalf("expected capped by max-download-limit, got %d", got)
	}
	if got := effectiveRequestPeerThreshold(map[string]string{"bt-request-peer-speed-limit": "0"}, 0); got != 0 {
		t.Fatalf("expected zero disables threshold, got %d", got)
	}
}

func TestResolveBTMaxPeers(t *testing.T) {
	t.Parallel()

	if got := resolveBTMaxPeers(map[string]string{"bt-max-peers": "42"}, 10); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := resolveBTMaxPeers(nil, 55); got != 55 {
		t.Fatalf("expected driver default 55, got %d", got)
	}
	if got := resolveBTMaxPeers(map[string]string{"bt-max-peers": "0"}, 55); got != 0 {
		t.Fatalf("bt-max-peers=0 should mean unlimited, got %d", got)
	}
}
