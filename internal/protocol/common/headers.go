package common

import (
	"net/http"
	"strings"
)

// ParseHeaderOption 解析 aria2 header 选项（多行 Header: value）。
func ParseHeaderOption(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			continue
		}
		out[name] = value
	}
	return out
}

// ApplyCustomHeaders 将自定义头写入请求，不覆盖已显式设置的键。
func ApplyCustomHeaders(req *http.Request, headers map[string]string) {
	if req == nil || len(headers) == 0 {
		return
	}
	for key, value := range headers {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if req.Header.Get(key) != "" {
			continue
		}
		req.Header.Set(key, value)
	}
}
