package app

import (
	"fmt"
	"os"
	"strings"
)

// Run 是统一入口，负责把参数分发给 daemon、ctl 和迁移子命令。
func Run(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runDaemon(args)
	}

	switch args[0] {
	case "daemon":
		return runDaemon(args[1:])
	case "downloadd":
		return runDaemon(args[1:])
	case "aria2c":
		return runDaemon(args[1:])
	case "ctl":
		return runCtl(args[1:])
	case "downloadctl":
		return runCtl(args[1:])
	case "migrate-from-aria2":
		return runMigrate(args[1:])
	default:
		if looksLikeStartupInput(args[0]) {
			return runDaemon(args)
		}
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func looksLikeStartupInput(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "://") || strings.HasPrefix(strings.ToLower(trimmed), "magnet:") {
		return true
	}
	if strings.HasSuffix(strings.ToLower(trimmed), ".torrent") {
		return true
	}
	if _, err := os.Stat(trimmed); err == nil {
		return true
	}
	return false
}
