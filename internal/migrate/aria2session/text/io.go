package text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// WriteFile 将任务快照写入 aria2 文本 save-session 格式。
func WriteFile(path string, tasks []*task.Task) error {
	if path == "" {
		return nil
	}
	data, err := Format(tasks)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Format 生成 aria2 save-session 文本内容。
func Format(tasks []*task.Task) ([]byte, error) {
	var b strings.Builder
	wrote := 0
	for _, item := range tasks {
		entry, ok := taskToEntry(item)
		if !ok {
			continue
		}
		if wrote > 0 {
			b.WriteByte('\n')
		}
		if err := writeEntry(&b, entry); err != nil {
			return nil, err
		}
		wrote++
	}
	return []byte(b.String()), nil
}

func writeEntry(b *strings.Builder, entry Entry) error {
	uri := strings.TrimSpace(entry.URI)
	if uri == "" {
		return fmt.Errorf("empty session URI")
	}
	b.WriteString(uri)
	b.WriteByte('\n')

	keys := make([]string, 0, len(entry.Options)+6)
	for k := range entry.Options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := entry.Options[key]
		if strings.TrimSpace(value) == "" {
			continue
		}
		fmt.Fprintf(b, " %s=%s\n", key, value)
	}
	return nil
}

func taskToEntry(item *task.Task) (Entry, bool) {
	if item == nil || item.Status == task.StatusRemoved {
		return Entry{}, false
	}
	if strings.EqualFold(item.Meta["bt.sessionDetached"], "true") {
		return Entry{}, false
	}
	if !shouldExportTask(item) {
		return Entry{}, false
	}
	uri := taskSourceURI(item)
	if uri == "" {
		return Entry{}, false
	}

	opts := cloneMap(item.Options)
	for k, v := range item.LocalOptions {
		opts[k] = v
	}
	if item.SaveDir != "" {
		opts["dir"] = item.SaveDir
	}
	if item.Name != "" {
		opts["out"] = item.Name
	}
	switch item.Status {
	case task.StatusPaused:
		opts["pause"] = "true"
	default:
		delete(opts, "pause")
	}
	if checksum := strings.TrimSpace(item.Meta["aria2.checksum"]); checksum != "" {
		opts["checksum"] = checksum
	}
	if item.GID != "" {
		opts["gid"] = strings.ToLower(item.GID)
	}

	return Entry{
		URI:     uri,
		GID:     strings.ToLower(item.GID),
		Dir:     item.SaveDir,
		Out:     item.Name,
		Paused:  item.Status == task.StatusPaused,
		Options: opts,
	}, true
}

func shouldExportTask(item *task.Task) bool {
	switch item.Status {
	case task.StatusWaiting, task.StatusPaused, task.StatusActive, task.StatusError:
		return true
	case task.StatusComplete:
		return item.Seeder
	default:
		return false
	}
}

func taskSourceURI(item *task.Task) string {
	if item == nil {
		return ""
	}
	for _, key := range []string{"bt.source.uri", "http.sourceURL", "ed2k.sourceURI", "ftp.sourceURL", "file.sourceURI"} {
		if u := strings.TrimSpace(item.Meta[key]); u != "" {
			return u
		}
	}
	if len(item.Files) > 0 {
		for _, u := range item.Files[0].URIs {
			if strings.TrimSpace(u) != "" {
				return u
			}
		}
	}
	return ""
}

// TasksFromBytes 解析 aria2 文本 session 内容为内部任务。
func TasksFromBytes(data []byte) ([]*task.Task, error) {
	entries, err := ParseReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	out := make([]*task.Task, 0, len(entries))
	for _, entry := range entries {
		preview, err := entryToTask(entry)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.URI, err)
		}
		out = append(out, preview)
	}
	return out, nil
}

// IsTextSession 判断内容是否为 aria2 文本 save-session（非 JSON）。
func IsTextSession(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '[', '{':
		return false
	default:
		return true
	}
}

// TasksFromReader 按内容嗅探 JSON 或 aria2 文本格式加载任务。
func TasksFromReader(r io.Reader) ([]*task.Task, bool, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, false, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, false, nil
	}
	if IsTextSession(data) {
		tasks, err := TasksFromBytes(data)
		return tasks, true, err
	}
	var tasks []*task.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, false, err
	}
	for i := range tasks {
		tasks[i] = tasks[i].Clone()
	}
	return tasks, false, nil
}
