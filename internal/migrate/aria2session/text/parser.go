package text

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// Entry 描述 aria2 save-session 文件中的一个任务块。
type Entry struct {
	URI      string
	GID      string
	Dir      string
	Out      string
	Paused   bool
	Checksum string
	Metalink string
	Options  map[string]string
}

// ParseFile 读取 aria2 的 save-session 文件。
func ParseFile(path string) ([]Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ParseReader(file)
}

// ParseReader 解析 aria2 save-session 文本。
func ParseReader(r io.Reader) ([]Entry, error) {
	scanner := bufio.NewScanner(r)
	var (
		tasks   []Entry
		current *Entry
		lineNo  int
		hadTask bool
	)

	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if isOptionLine(raw) {
			if current == nil {
				return nil, fmt.Errorf("line %d: option without task URI", lineNo)
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				return nil, fmt.Errorf("line %d: invalid option format", lineNo)
			}
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			if current.Options == nil {
				current.Options = make(map[string]string)
			}
			current.Options[key] = value
			switch key {
			case "gid":
				current.GID = strings.ToLower(value)
			case "dir":
				current.Dir = value
			case "out":
				current.Out = value
			case "pause", "paused":
				current.Paused = parseBoolValue(value)
			case "checksum":
				current.Checksum = value
			case "metalink":
				current.Metalink = value
			}
			continue
		}

		if hadTask {
			tasks = append(tasks, *current)
		}
		current = &Entry{
			URI:     line,
			Options: map[string]string{},
		}
		hadTask = true
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		tasks = append(tasks, *current)
	}
	return tasks, nil
}

func isOptionLine(raw string) bool {
	if raw == "" {
		return false
	}
	return raw[0] == ' ' || raw[0] == '\t'
}

func parseBoolValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ParseBoolValue 解析 aria2 布尔选项字符串。
func ParseBoolValue(value string) bool {
	return parseBoolValue(value)
}

// PreviewTask 将 aria2 session 条目转换为内部任务预览。
func PreviewTask(item Entry) (*task.Task, error) {
	return entryToTask(item)
}

// RouteKind 识别 URI 对应协议。
func RouteKind(uri string) (task.Protocol, error) {
	return routeKind(uri)
}

// GenerateGID 为 session 条目生成或复用 GID。
func GenerateGID(item Entry) string {
	return generateOrReuseGID(item)
}

// CloneOptions 复制选项 map。
func CloneOptions(src map[string]string) map[string]string {
	return cloneMap(src)
}

// DisplayName 返回任务展示名。
func DisplayName(item Entry) string {
	if item.Out != "" {
		return item.Out
	}
	return derivePreviewName(item.URI)
}
