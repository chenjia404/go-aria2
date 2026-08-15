package config

import (
	"fmt"
	"strconv"
	"strings"
)

// SanitizeConfigValue 去掉 aria2.conf 行内注释与成对引号，便于直接复用现有配置。
func SanitizeConfigValue(value string) string {
	return unquoteValue(stripInlineComment(value))
}

func stripInlineComment(s string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return strings.TrimSpace(s)
}

func unquoteValue(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ParsePortSpec 解析 aria2 端口或端口区间，区间取起始端口。
// 例如 6881、6881-6999。
func ParsePortSpec(value string) (int, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, fmt.Errorf("empty port")
	}
	if i := strings.IndexByte(text, '-'); i > 0 {
		text = strings.TrimSpace(text[:i])
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return n, nil
}

// ParseSpeedBytes 解析 aria2 风格速度/大小（支持 K/M/G 后缀，基数 1024）。
func ParseSpeedBytes(value string) (int64, error) {
	return parseSpeedBytes(value)
}

// knownIgnoredOptions 是 aria2 常见但当前仅接受、不改变语义的选项。
// 直接替换 aria2 时这些项不应导致启动失败，也不应刷 unknown warning。
var knownIgnoredOptions = map[string]struct{}{
	"allow-piece-length-change":        {},
	"async-dns":                        {},
	"async-dns-server":                 {},
	"auto-save-interval":               {},
	"bt-external-ip":                   {},
	"bt-hash-check-seed":               {},
	"bt-lpd-interface":                 {},
	"bt-max-open-files":                {},
	"bt-metadata-only":                 {},
	"bt-prioritize-piece":              {},
	"bt-seed-unverified":               {},
	"bt-stop-timeout":                  {},
	"bt-tracker-connect-timeout":       {},
	"bt-tracker-interval":              {},
	"bt-tracker-timeout":               {},
	"ca-certificate":                   {},
	"certificate":                      {},
	"conditional-get":                  {},
	"console-log-level":                {},
	"content-disposition-default-utf8": {},
	"cookie":                           {},
	"deferred-input":                   {},
	"dht-entry-point":                  {},
	"dht-entry-point6":                 {},
	"dht-listen-addr6":                 {},
	"dht-message-timeout":              {},
	"disable-ipv6":                     {},
	"disk-cache":                       {},
	"download-result":                  {},
	"dry-run":                          {},
	"dscp":                             {},
	"enable-color":                     {},
	"enable-http-keep-alive":           {},
	"enable-http-pipelining":           {},
	"enable-mmap":                      {},
	"enable-peer-exchange":             {},
	"event-poll":                       {},
	"force-sequential":                 {},
	"ftp-pasv":                         {},
	"ftp-proxy":                        {},
	"ftp-proxy-passwd":                 {},
	"ftp-proxy-user":                   {},
	"ftp-reuse-connection":             {},
	"ftp-type":                         {},
	"hash-check-only":                  {},
	"http-accept-gzip":                 {},
	"http-auth-challenge":              {},
	"http-no-cache":                    {},
	"http-proxy-passwd":                {},
	"http-proxy-user":                  {},
	"http-want-digest":                 {},
	"human-readable":                   {},
	"keep-unfinished-download-result":  {},
	"load-cookies":                     {},
	"max-download-result":              {},
	"max-file-not-found":               {},
	"max-mmap-limit":                   {},
	"max-resume-failure-tries":         {},
	"metalink-base-uri":                {},
	"metalink-enable-unique-protocol":  {},
	"metalink-language":                {},
	"metalink-location":                {},
	"metalink-os":                      {},
	"metalink-preferred-protocol":      {},
	"metalink-version":                 {},
	"min-tls-version":                  {},
	"netrc-path":                       {},
	"no-file-allocation-limit":         {},
	"no-netrc":                         {},
	"optimize-concurrent-downloads":    {},
	"parameterized-uri":                {},
	"peer-agent":                       {},
	"peer-id-prefix":                   {},
	"private-key":                      {},
	"realtime-chunk-checksum":          {},
	"remote-time":                      {},
	"reuse-uri":                        {},
	"rlimit-nofile":                    {},
	"rpc-certificate":                  {},
	"rpc-passwd":                       {},
	"rpc-private-key":                  {},
	"rpc-secure":                       {},
	"rpc-user":                         {},
	"save-cookies":                     {},
	"save-not-found":                   {},
	"server-stat-if":                   {},
	"server-stat-of":                   {},
	"server-stat-timeout":              {},
	"show-console-readout":             {},
	"socket-recv-buffer-size":          {},
	"ssh-host-key-md":                  {},
	"stop":                             {},
	"stop-with-process":                {},
	"stream-piece-selector":            {},
	"summary-interval":                 {},
	"truncate-console-readout":         {},
	"uri-selector":                     {},
	"use-head":                         {},
	"always-resume":                    {},
	"all-proxy-passwd":                 {},
	"all-proxy-user":                   {},
}

// IsKnownIgnoredOption 报告该键是否为已识别但暂不落地语义的 aria2 选项。
func IsKnownIgnoredOption(key string) bool {
	_, ok := knownIgnoredOptions[strings.ToLower(strings.TrimSpace(key))]
	return ok
}
