package common

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// ResolveBoolOption 解析 aria2 风格布尔选项，未知值返回 fallback。
func ResolveBoolOption(opts map[string]string, key string, fallback bool) bool {
	if opts == nil {
		return fallback
	}
	value, ok := opts[key]
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		return fallback
	}
}

// PauseRequestedFromChangeOption 解析 changeOption 中的 pause 字段。
func PauseRequestedFromChangeOption(opts map[string]string) (pause bool, ok bool) {
	value, found := opts["pause"]
	if !found {
		return false, false
	}
	return ResolveBoolOption(map[string]string{"pause": value}, "pause", false), true
}

// ShouldRejectExistingFile 在 allow-overwrite=false 且 continue=false 时拒绝已存在文件。
func ShouldRejectExistingFile(opts map[string]string, existingSize int64) bool {
	if existingSize <= 0 {
		return false
	}
	allowOverwrite := ResolveBoolOption(opts, "allow-overwrite", false)
	continueDownloads := ResolveBoolOption(opts, "continue", true)
	return !allowOverwrite && !continueDownloads
}

// ShouldResetExistingFile 在 allow-overwrite=true 且 continue=false 时从头下载。
func ShouldResetExistingFile(opts map[string]string, existingSize int64) bool {
	if existingSize <= 0 {
		return false
	}
	allowOverwrite := ResolveBoolOption(opts, "allow-overwrite", false)
	continueDownloads := ResolveBoolOption(opts, "continue", true)
	return allowOverwrite && !continueDownloads
}

// ShouldResumePartial 是否从本地部分文件续传。
func ShouldResumePartial(opts map[string]string, existingSize, total int64) bool {
	if existingSize <= 0 {
		return false
	}
	if !ResolveBoolOption(opts, "continue", true) {
		return false
	}
	if total > 0 && existingSize >= total {
		return false
	}
	return true
}

// SleepBetweenMirrors 在切换镜像 URI 前等待 retry-wait 秒（aria2 语义）。
func SleepBetweenMirrors(ctx context.Context, opts map[string]string) error {
	wait := ParseTimeoutSeconds(opts, "retry-wait")
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// ParseTimeoutSeconds 从选项中解析秒级超时（connect-timeout、timeout 等）。
func ParseTimeoutSeconds(opts map[string]string, keys ...string) time.Duration {
	if opts == nil {
		return 0
	}
	for _, key := range keys {
		raw, ok := opts[key]
		if !ok {
			continue
		}
		secs, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || secs <= 0 {
			continue
		}
		return time.Duration(secs) * time.Second
	}
	return 0
}
