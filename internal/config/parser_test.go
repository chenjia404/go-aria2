package config

import (
	"strings"
	"testing"
	"time"
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
user-agent=demo-agent
http-referer=https://example.com
http-proxy=http://proxy.local:8080
https-proxy=https://secure-proxy.local:8443
all-proxy=socks5://proxy.local:1080
no-proxy=localhost,127.0.0.1
max-download-limit=123
max-connection-per-server=8
split=16
check-certificate=no
allow-overwrite=true
auto-file-renaming=false
bt-tracker=udp://tracker-good
bt-exclude-tracker=udp://tracker-bad
bt-force-encryption=true
bt-load-saved-metadata=true
bt-require-crypto=true
bt-save-metadata=true
bt-min-crypto-level=arc4
enable-dht6=false
dht-file-path=/tmp/dht.dat
dht-file-path6=/tmp/dht6.dat
dht-listen-port=26701
follow-metalink=false
follow-torrent=false
pause-metadata=true
seed-time=90
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
	if cfg.HTTPProxy != "http://proxy.local:8080" || cfg.HTTPSProxy != "https://secure-proxy.local:8443" || cfg.AllProxy != "socks5://proxy.local:1080" || cfg.NoProxy != "localhost,127.0.0.1" {
		t.Fatalf("unexpected proxy settings: %+v", cfg)
	}
	if cfg.MaxConnectionPerServer != 8 || cfg.Split != 16 || cfg.CheckCertificate || cfg.MaxDownloadLimit != 123 {
		t.Fatalf("unexpected connection settings: %+v", cfg)
	}
	if !cfg.AllowOverwrite {
		t.Fatalf("expected allow-overwrite enabled: %+v", cfg)
	}
	if cfg.AutoFileRenaming {
		t.Fatalf("expected auto-file-renaming disabled: %+v", cfg)
	}
	if !cfg.BTForceEncryption {
		t.Fatalf("expected bt-force-encryption enabled: %+v", cfg)
	}
	if !cfg.BTRequireCrypto {
		t.Fatalf("expected bt-require-crypto enabled: %+v", cfg)
	}
	if cfg.BTMinCryptoLevel != "arc4" {
		t.Fatalf("expected bt-min-crypto-level parsed: %+v", cfg)
	}
	if cfg.BTTracker != "udp://tracker-good" || cfg.BTExcludeTracker != "udp://tracker-bad" {
		t.Fatalf("expected bt tracker settings parsed: %+v", cfg)
	}
	if !cfg.BTLoadSavedMetadata || !cfg.BTSaveMetadata {
		t.Fatalf("expected bt metadata flags parsed: %+v", cfg)
	}
	if cfg.EnableDHT6 || cfg.DHTListenPort != 26701 || cfg.DHTFilePath != "/tmp/dht.dat" || cfg.DHTFilePath6 != "/tmp/dht6.dat" {
		t.Fatalf("expected dht settings parsed: %+v", cfg)
	}
	if cfg.FollowMetalink {
		t.Fatalf("expected follow-metalink disabled: %+v", cfg)
	}
	if cfg.FollowTorrent {
		t.Fatalf("expected follow-torrent disabled: %+v", cfg)
	}
	if !cfg.PauseMetadata {
		t.Fatalf("expected pause-metadata enabled: %+v", cfg)
	}
	if cfg.SeedTime != 90*time.Minute {
		t.Fatalf("expected seed-time parsed: %+v", cfg)
	}
	if cfg.LogPath != "./downloadd.log" || cfg.LogLevel != "debug" {
		t.Fatalf("unexpected log settings: %+v", cfg)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("expected one warning, got %d: %#v", len(cfg.Warnings), cfg.Warnings)
	}
}

func TestParseTypicalAria2Conf(t *testing.T) {
	t.Parallel()

	configText := `
dir="/downloads" # 下载目录
disk-cache=64M
file-allocation=none
continue=true
max-concurrent-downloads=10
max-connection-per-server=16
min-split-size=8M
split=16
disable-ipv6=false
input-file=/config/aria2.session
save-session=/config/aria2.session
save-session-interval=60
enable-rpc=true
rpc-allow-origin-all=true
rpc-listen-all=true
rpc-listen-port=6800
rpc-max-request-size=2M
follow-torrent=true
bt-max-peers=0
enable-dht=true
enable-dht6=true
dht-listen-port=6881-6999
listen-port=6881-6999
enable-peer-exchange=true
bt-enable-lpd=true
bt-request-peer-speed-limit=1M
max-overall-download-limit=2M
max-overall-upload-limit=200K
max-download-limit=1M
seed-ratio=1.0
force-save=true
bt-hash-check-seed=true
bt-seed-unverified=true
bt-save-metadata=true
http-user=alice
http-passwd=secret
ftp-user=bob
ftp-passwd=ftp-secret
header=Cookie: a=1
header=Accept: */*
max-tries=8
lowest-speed-limit=10K
rpc-save-upload-metadata=true
quiet=false
`

	cfg, err := Parse(strings.NewReader(configText))
	if err != nil {
		t.Fatalf("typical aria2.conf should parse: %v", err)
	}
	if cfg.Dir != "/downloads" {
		t.Fatalf("dir: %q", cfg.Dir)
	}
	if cfg.ListenPort != 6881 || cfg.DHTListenPort != 6881 {
		t.Fatalf("port range: listen=%d dht=%d", cfg.ListenPort, cfg.DHTListenPort)
	}
	if cfg.MinSplitSize != 8*1024*1024 {
		t.Fatalf("min-split-size: %d", cfg.MinSplitSize)
	}
	if cfg.MaxOverallDownloadLimit != 2*1024*1024 || cfg.MaxOverallUploadLimit != 200*1024 || cfg.MaxDownloadLimit != 1024*1024 {
		t.Fatalf("speed limits: %+v", cfg)
	}
	if cfg.RPCMaxRequestSize != 2*1024*1024 {
		t.Fatalf("rpc-max-request-size: %d", cfg.RPCMaxRequestSize)
	}
	if cfg.BTMaxPeers != 0 {
		t.Fatalf("bt-max-peers=0 should be kept, got %d", cfg.BTMaxPeers)
	}
	if cfg.HTTPUser != "alice" || cfg.HTTPPasswd != "secret" || cfg.FTPUser != "bob" {
		t.Fatalf("auth: %+v", cfg)
	}
	if !strings.Contains(cfg.Header, "Cookie: a=1") || !strings.Contains(cfg.Header, "Accept: */*") {
		t.Fatalf("header: %q", cfg.Header)
	}
	if cfg.MaxTries != 8 || cfg.LowestSpeedLimit != 10*1024 {
		t.Fatalf("retry/speed: tries=%d lowest=%d", cfg.MaxTries, cfg.LowestSpeedLimit)
	}
	if cfg.InputFile != "/config/aria2.session" || !cfg.RPCSaveUploadMetadata {
		t.Fatalf("input/rpc-save: %+v", cfg)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("typical aria2 options should not warn: %#v", cfg.Warnings)
	}
}

func TestSanitizeConfigValue(t *testing.T) {
	t.Parallel()

	if got := SanitizeConfigValue(`true # rpc`); got != "true" {
		t.Fatalf("comment: %q", got)
	}
	if got := SanitizeConfigValue(`"/downloads"`); got != "/downloads" {
		t.Fatalf("quotes: %q", got)
	}
	if got := SanitizeConfigValue(`'true'`); got != "true" {
		t.Fatalf("single quotes: %q", got)
	}
}

func TestParsePortSpec(t *testing.T) {
	t.Parallel()

	got, err := ParsePortSpec("6881-6999")
	if err != nil || got != 6881 {
		t.Fatalf("range: %d %v", got, err)
	}
	got, err = ParsePortSpec("16881")
	if err != nil || got != 16881 {
		t.Fatalf("single: %d %v", got, err)
	}
	if _, err := ParsePortSpec("bad"); err == nil {
		t.Fatal("expected error")
	}
}
