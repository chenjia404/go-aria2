package common

import (
	"strconv"
	"strings"
)

// NewTaskDownloadLimiter 解析任务选项中的下载限速。
// 优先 max-download-limit，其次 max-overall-download-limit；均无则返回 base。
func NewTaskDownloadLimiter(opts map[string]string, base *ByteLimiter) *ByteLimiter {
	if opts == nil {
		return base
	}
	if value, ok := opts["max-download-limit"]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			if parsed <= 0 {
				return nil
			}
			return NewByteLimiter(parsed)
		}
	}
	if value, ok := opts["max-overall-download-limit"]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			if parsed <= 0 {
				return nil
			}
			if base != nil {
				return base
			}
			return NewByteLimiter(parsed)
		}
	}
	return base
}

// NewTaskUploadLimiter 解析任务选项中的上传限速（max-upload-limit）。
func NewTaskUploadLimiter(opts map[string]string, base *ByteLimiter) *ByteLimiter {
	if opts == nil {
		return base
	}
	if value, ok := opts["max-upload-limit"]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			if parsed <= 0 {
				return nil
			}
			return NewByteLimiter(parsed)
		}
	}
	if value, ok := opts["max-overall-upload-limit"]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			if parsed <= 0 {
				return nil
			}
			if base != nil {
				return base
			}
			return NewByteLimiter(parsed)
		}
	}
	return base
}
