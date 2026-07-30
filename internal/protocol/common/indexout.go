package common

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseIndexOut 解析 aria2 index-out 选项。
// 支持 "path"（默认 index=1）、"1=path"、"1=a,2=b" 等形式。
func ParseIndexOut(raw string) (map[int]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	out := map[int]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx, path, ok := strings.Cut(part, "=")
		if !ok {
			if len(out) > 0 {
				return nil, fmt.Errorf("invalid index-out value: %q", raw)
			}
			out[1] = part
			continue
		}
		index, err := strconv.Atoi(strings.TrimSpace(idx))
		if err != nil || index < 1 {
			return nil, fmt.Errorf("invalid index-out index: %q", part)
		}
		path = strings.TrimSpace(path)
		if path == "" {
			return nil, fmt.Errorf("invalid index-out path: %q", part)
		}
		out[index] = path
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid index-out value: %q", raw)
	}
	return out, nil
}

// ResolveIndexOutName 返回指定 1-based 文件索引的输出名，未设置时返回 fallback。
func ResolveIndexOutName(opts map[string]string, fileIndex int, fallback string) string {
	if opts == nil || fileIndex < 1 {
		return fallback
	}
	raw, ok := opts["index-out"]
	if !ok {
		return fallback
	}
	mapping, err := ParseIndexOut(raw)
	if err != nil || len(mapping) == 0 {
		return fallback
	}
	if path, ok := mapping[fileIndex]; ok {
		return path
	}
	return fallback
}
