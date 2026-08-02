package aria2session

import (
	"io"

	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/migrate/aria2session/text"
)

// Aria2SessionTask 描述 aria2 save-session 文件中的一个任务块。
type Aria2SessionTask = text.Entry

// ParseAria2Session 读取 aria2 的 save-session 文件。
func ParseAria2Session(path string) ([]Aria2SessionTask, error) {
	return text.ParseFile(path)
}

// ParseAria2SessionReader 便于测试的 Reader 版本。
func ParseAria2SessionReader(r io.Reader) ([]Aria2SessionTask, error) {
	return text.ParseReader(r)
}

// WriteAria2Session 将任务快照写入 aria2 文本 save-session 格式。
func WriteAria2Session(path string, tasks []*task.Task) error {
	return text.WriteFile(path, tasks)
}

// TasksFromReader 按内容嗅探 JSON 或 aria2 文本格式加载任务。
func TasksFromReader(r io.Reader) ([]*task.Task, bool, error) {
	return text.TasksFromReader(r)
}

// FormatAria2Session 生成 aria2 save-session 文本内容。
func FormatAria2Session(tasks []*task.Task) ([]byte, error) {
	return text.Format(tasks)
}

// TasksFromAria2SessionBytes 解析 aria2 文本 session 内容。
func TasksFromAria2SessionBytes(data []byte) ([]*task.Task, error) {
	return text.TasksFromBytes(data)
}

// IsAria2SessionText 判断内容是否为 aria2 文本 save-session（非 JSON）。
func IsAria2SessionText(data []byte) bool {
	return text.IsTextSession(data)
}

func previewTask(item Aria2SessionTask) (*task.Task, error) {
	return text.PreviewTask(item)
}

func parseBoolValue(value string) bool {
	return text.ParseBoolValue(value)
}
