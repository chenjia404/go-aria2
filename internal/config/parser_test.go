package config

import (
	"strings"
	"testing"
)

func TestParseExtendedOptions(t *testing.T) {
	configText := `
# comment
enable-rpc=true
rpc-listen-port=6801
rpc-listen-all=yes
rpc-allow-origin-all=1
rpc-max-request-size=2048
enable-websocket=true
pause=true
continue=false
daemon=false
log=./downloadd.log
log-level=debug
data-dir=./run
http-user-agent=demo-agent
http-referer=https://example.com
http-proxy=http://proxy.local:8080
https-proxy=https://secure-proxy.local:8443
all-proxy=socks5://proxy.local:1080
max-connection-per-server=8
split=16
check-certificate=no
unknown-key=value
`

	cfg, err := Parse(strings.NewReader(configText))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !cfg.EnableRPC || cfg.RPCListenPort != 6801 {
		t.Fatalf("unexpected RPC settings: %+v", cfg)
	}
	if !cfg.RPCListenAll || !cfg.RPCAllowOriginAll || cfg.RPCMaxRequestSize != 2048 {
		t.Fatalf("unexpected extended RPC settings: %+v", cfg)
	}
	if !cfg.EnableWebSocket || !cfg.Pause || cfg.ContinueDownloads {
		t.Fatalf("unexpected phase2 flags: %+v", cfg)
	}
	if cfg.DataDir != "./run" || cfg.HTTPUserAgent != "demo-agent" || cfg.HTTPReferer != "https://example.com" {
		t.Fatalf("unexpected http settings: %+v", cfg)
	}
	if cfg.HTTPProxy != "http://proxy.local:8080" || cfg.HTTPSProxy != "https://secure-proxy.local:8443" || cfg.AllProxy != "socks5://proxy.local:1080" {
		t.Fatalf("unexpected proxy settings: %+v", cfg)
	}
	if cfg.MaxConnectionPerServer != 8 || cfg.Split != 16 || cfg.CheckCertificate {
		t.Fatalf("unexpected connection settings: %+v", cfg)
	}
	if cfg.LogPath != "./downloadd.log" || cfg.LogLevel != "debug" {
		t.Fatalf("unexpected log settings: %+v", cfg)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("expected one warning, got %d", len(cfg.Warnings))
	}
}
