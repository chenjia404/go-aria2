package app

import (
	"testing"

	"github.com/chenjia404/go-aria2/internal/config"
)

func TestParseDaemonArgsAria2Aliases(t *testing.T) {
	t.Parallel()

	opts, err := parseDaemonArgs([]string{
		"-d", "/downloads",
		"-j", "4",
		"--rpc-secret", "secret",
		"--enable-rpc",
		"--listen-port", "6888",
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
	if cfg.ListenPort != 6888 {
		t.Fatalf("expected listen port override, got %d", cfg.ListenPort)
	}
	if cfg.SeedRatio != 1.5 {
		t.Fatalf("expected seed ratio override, got %v", cfg.SeedRatio)
	}
}
