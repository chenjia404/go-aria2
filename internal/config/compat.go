package config

import (
	"os"
	"path/filepath"

	"github.com/chenjia404/go-aria2/internal/core/session"
)

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

// ShouldWriteAria2TextSession 判断 save-session 路径是否应按 aria2 文本格式回写。
// 非 .json 后缀时与原生 aria2 互换，避免覆盖原 session 文件。
func ShouldWriteAria2TextSession(path string) bool {
	ext := filepath.Ext(path)
	return ext != ".json" && ext != ".JSON"
}

// ResolveDefaultConfigPath 按 aria2 顺序查找默认配置：当前目录、XDG、~/.aria2。
func ResolveDefaultConfigPath() string {
	if fileExists("aria2.conf") {
		return "aria2.conf"
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		p := filepath.Join(xdg, "aria2", "aria2.conf")
		if fileExists(p) {
			return p
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		p := filepath.Join(home, ".aria2", "aria2.conf")
		if fileExists(p) {
			return p
		}
	}
	return "aria2.conf"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
