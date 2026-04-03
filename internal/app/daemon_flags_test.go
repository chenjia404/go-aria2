package app

import (
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/config"
)

func TestParseDaemonArgsAria2Aliases(t *testing.T) {
	t.Parallel()

	opts, err := parseDaemonArgs([]string{
		"-d", "/downloads",
		"-j", "4",
		"--user-agent", "ua-test",
		"--no-proxy", "localhost,127.0.0.1",
		"--rpc-secret", "secret",
		"--enable-rpc",
		"--allow-overwrite",
		"--auto-file-renaming",
		"--bt-tracker", "udp://tracker-good",
		"--bt-exclude-tracker", "udp://tracker-bad",
		"--bt-force-encryption",
		"--bt-load-saved-metadata",
		"--bt-require-crypto",
		"--bt-save-metadata",
		"--bt-min-crypto-level", "arc4",
		"--dht-file-path", "/tmp/dht.dat",
		"--dht-file-path6", "/tmp/dht6.dat",
		"--dht-listen-port", "26701",
		"--enable-dht6",
		"--follow-metalink",
		"--follow-torrent",
		"--listen-port", "6888",
		"--max-download-limit", "123",
		"--pause-metadata",
		"--seed-time", "60",
		"--seed-ratio", "1.5",
	})
	if err != nil {
		t.Fatalf("parseDaemonArgs returned error: %v", err)
	}

	cfg := config.Default()
	if err := applyDaemonCLIOptions(cfg, opts); err != nil {
		t.Fatalf("applyDaemonCLIOptions returned error: %v", err)
	}

	if cfg.Dir != "/downloads" {
		t.Fatalf("expected dir override, got %q", cfg.Dir)
	}
	if cfg.MaxConcurrentDownloads != 4 {
		t.Fatalf("expected concurrency override, got %d", cfg.MaxConcurrentDownloads)
	}
	if cfg.RPCSecret != "secret" {
		t.Fatalf("expected rpc secret override, got %q", cfg.RPCSecret)
	}
	if !cfg.EnableRPC {
		t.Fatalf("expected rpc enabled")
	}
	if !cfg.AllowOverwrite {
		t.Fatalf("expected allow-overwrite override")
	}
	if !cfg.AutoFileRenaming {
		t.Fatalf("expected auto-file-renaming override")
	}
	if cfg.BTTracker != "udp://tracker-good" {
		t.Fatalf("expected bt-tracker override, got %q", cfg.BTTracker)
	}
	if cfg.BTExcludeTracker != "udp://tracker-bad" {
		t.Fatalf("expected bt-exclude-tracker override, got %q", cfg.BTExcludeTracker)
	}
	if !cfg.BTForceEncryption {
		t.Fatalf("expected bt-force-encryption override")
	}
	if !cfg.BTLoadSavedMetadata || !cfg.BTSaveMetadata {
		t.Fatalf("expected bt saved metadata flags override")
	}
	if !cfg.BTRequireCrypto {
		t.Fatalf("expected bt-require-crypto override")
	}
	if cfg.BTMinCryptoLevel != "arc4" {
		t.Fatalf("expected bt-min-crypto-level override, got %q", cfg.BTMinCryptoLevel)
	}
	if !cfg.EnableDHT6 || cfg.DHTListenPort != 26701 {
		t.Fatalf("expected dht overrides, got %+v", cfg)
	}
	if cfg.DHTFilePath != "/tmp/dht.dat" || cfg.DHTFilePath6 != "/tmp/dht6.dat" {
		t.Fatalf("expected dht file path overrides, got %+v", cfg)
	}
	if !cfg.FollowMetalink {
		t.Fatalf("expected follow-metalink override")
	}
	if !cfg.FollowTorrent {
		t.Fatalf("expected follow-torrent override")
	}
	if cfg.ListenPort != 6888 {
		t.Fatalf("expected listen port override, got %d", cfg.ListenPort)
	}
	if cfg.MaxDownloadLimit != 123 {
		t.Fatalf("expected max-download-limit override, got %d", cfg.MaxDownloadLimit)
	}
	if !cfg.PauseMetadata {
		t.Fatalf("expected pause-metadata override")
	}
	if cfg.SeedTime != 60*time.Minute {
		t.Fatalf("expected seed-time override, got %v", cfg.SeedTime)
	}
	if cfg.SeedRatio != 1.5 {
		t.Fatalf("expected seed ratio override, got %v", cfg.SeedRatio)
	}
	if cfg.NoProxy != "localhost,127.0.0.1" {
		t.Fatalf("expected no-proxy override, got %q", cfg.NoProxy)
	}
	if cfg.HTTPUserAgent != "ua-test" {
		t.Fatalf("expected user-agent alias override, got %q", cfg.HTTPUserAgent)
	}
}
