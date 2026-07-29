package bt

import (
	"testing"

	torrentlib "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/mse"
)

func TestApplyBTCryptoOptionsForceEncryption(t *testing.T) {
	t.Parallel()

	cfg := torrentlib.NewDefaultClientConfig()
	applyBTCryptoOptions(cfg, CryptoOptions{ForceEncryption: true})

	if cfg.CryptoProvides != mse.CryptoMethodRC4 {
		t.Fatalf("expected RC4 only, got %v", cfg.CryptoProvides)
	}
	if !cfg.HeaderObfuscationPolicy.RequirePreferred {
		t.Fatal("expected required obfuscation")
	}
	if cfg.CryptoSelector(mse.AllSupportedCrypto) != mse.CryptoMethodRC4 {
		t.Fatal("selector should prefer RC4")
	}
}

func TestApplyBTCryptoOptionsHandshakeLevel(t *testing.T) {
	t.Parallel()

	cfg := torrentlib.NewDefaultClientConfig()
	applyBTCryptoOptions(cfg, CryptoOptions{MinCryptoLevel: "handshake"})

	if !cfg.HeaderObfuscationPolicy.RequirePreferred {
		t.Fatal("expected handshake level to require obfuscation")
	}
}

func TestApplyBTCryptoOptionsArc4Level(t *testing.T) {
	t.Parallel()

	cfg := torrentlib.NewDefaultClientConfig()
	applyBTCryptoOptions(cfg, CryptoOptions{MinCryptoLevel: "arc4"})

	if cfg.CryptoProvides != mse.CryptoMethodRC4 {
		t.Fatalf("expected RC4 for arc4 level, got %v", cfg.CryptoProvides)
	}
}
