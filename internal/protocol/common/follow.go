package common

import "strings"

// OptionBool 解析 aria2 风格布尔选项，缺省返回 defaultValue。
func OptionBool(opts map[string]string, key string, defaultValue bool) bool {
	if opts == nil {
		return defaultValue
	}
	value, ok := opts[key]
	if !ok || strings.TrimSpace(value) == "" {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

// ShouldFollowTorrentURL 判断是否应把 HTTP(S) .torrent URL 交给 BT 驱动解析。
func ShouldFollowTorrentURL(opts map[string]string) bool {
	if !OptionBool(opts, "follow-metalink", true) {
		return false
	}
	return OptionBool(opts, "follow-torrent", true)
}
