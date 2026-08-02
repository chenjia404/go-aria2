package config

import "github.com/chenjia404/go-aria2/internal/core/session"

// ApplyAria2CompatMode 在 aria2-compat-mode=true 时启用与 aria2 更接近的运行时默认行为。
func ApplyAria2CompatMode(cfg *Config) {
	if cfg == nil || !cfg.Aria2CompatMode {
		return
	}
	cfg.RPCStrictAuth = true
}

// Aria2SessionExportPath 返回与 JSON session 并存的 aria2 文本 save-session 路径。
func Aria2SessionExportPath(jsonSessionPath string) string {
	return session.CompanionExportPath(jsonSessionPath)
}
