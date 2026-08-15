package app

import (
	"strings"
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

func TestParseDaemonArgsDropInAria2Flags(t *testing.T) {
	t.Parallel()

	opts, err := parseDaemonArgs([]string{
		"--conf-path", "aria2.conf",
		"--listen-port", "6881-6999",
		"--dht-listen-port=6881-6999",
		"--max-overall-download-limit=2M",
		"--max-download-limit", "1M",
		"--min-split-size=8M",
		"--max-tries", "8",
		"--lowest-speed-limit=10K",
		"--http-user", "alice",
		"--http-passwd", "secret",
		"--disk-cache", "64M",
		"--enable-peer-exchange=true",
		"--not-a-real-aria2-option=1",
		"--quiet",
		"--no-conf",
	})
	if err != nil {
		t.Fatalf("parseDaemonArgs: %v", err)
	}
	if !opts.noConf {
		t.Fatal("expected --no-conf")
	}
	if !opts.confSeen {
		t.Fatal("expected --conf-path seen")
	}
	if opts.values["listen-port"] != 6881 {
		t.Fatalf("listen-port: %#v", opts.values["listen-port"])
	}
	if opts.values["max-overall-download-limit"] != int64(2*1024*1024) {
		t.Fatalf("overall dl: %#v", opts.values["max-overall-download-limit"])
	}
	if opts.values["max-download-limit"] != int64(1024*1024) {
		t.Fatalf("dl: %#v", opts.values["max-download-limit"])
	}
	if opts.values["http-user"] != "alice" {
		t.Fatalf("http-user: %#v", opts.values["http-user"])
	}
	if len(opts.unknownWarnings) != 1 || !strings.Contains(opts.unknownWarnings[0], "not-a-real-aria2-option") {
		t.Fatalf("expected only truly unknown option warned, got %#v", opts.unknownWarnings)
	}

	cfg := config.Default()
	if err := applyDaemonCLIOptions(cfg, opts); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if cfg.ListenPort != 6881 || cfg.MaxTries != 8 || cfg.HTTPUser != "alice" || !cfg.Quiet {
		t.Fatalf("applied: %+v", cfg)
	}
	if cfg.MinSplitSize != 8*1024*1024 || cfg.LowestSpeedLimit != 10*1024 {
		t.Fatalf("sizes: min=%d lowest=%d", cfg.MinSplitSize, cfg.LowestSpeedLimit)
	}
}
