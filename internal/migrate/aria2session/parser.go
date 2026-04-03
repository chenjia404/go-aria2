package aria2session

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Aria2SessionTask 描述 aria2 save-session 文件中的一个任务块。
type Aria2SessionTask struct {
	URI      string
	GID      string
	Dir      string
	Out      string
	Paused   bool
	Checksum string
	Metalink string
	Options  map[string]string
}

// ParseAria2Session 读取 aria2 的 save-session 文件。
// 规则：
// - 非缩进行视为新任务的 URI
// - 缩进行视为当前任务的 option
// - 忽略空行和 # 注释
func ParseAria2Session(path string) ([]Aria2SessionTask, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return ParseAria2SessionReader(file)
}

// ParseAria2SessionReader 便于测试的 Reader 版本。
func ParseAria2SessionReader(r io.Reader) ([]Aria2SessionTask, error) {
	scanner := bufio.NewScanner(r)
	var (
		tasks   []Aria2SessionTask
		current *Aria2SessionTask
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
		current = &Aria2SessionTask{
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
