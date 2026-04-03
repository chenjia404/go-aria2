package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// FileStore 将任务快照保存为本地 JSON 文件�?
type FileStore struct {
	path string
}

// NewFileStore 创建一个基于文件的 session store�?
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Load 从磁盘加载任务快照。文件不存在时返回空列表�?
func (s *FileStore) Load(ctx context.Context) ([]*task.Task, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var tasks []*task.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}

	for i := range tasks {
		tasks[i] = tasks[i].Clone()
	}
	return tasks, nil
}

// Save 通过临时文件 + 原子替换方式落盘，避免写入中断导�?session 损坏�?
func (s *FileStore) Save(ctx context.Context, tasks []*task.Task) error {
	if s == nil || s.path == "" {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	snapshot := make([]*task.Task, 0, len(tasks))
	for _, item := range tasks {
		snapshot = append(snapshot, item.Clone())
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
