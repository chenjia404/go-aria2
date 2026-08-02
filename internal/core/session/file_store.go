package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/migrate/aria2session/text"
)

// FileStore 将任务快照保存为本地 JSON 文件，可选同步导出 aria2 文本 save-session。
type FileStore struct {
	path            string
	aria2ExportPath string
}

// NewFileStore 创建一个基于文件的 session store。
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// SetAria2ExportPath 启用向 aria2 文本 save-session 路径的双写（空字符串关闭）。
func (s *FileStore) SetAria2ExportPath(path string) {
	if s == nil {
		return
	}
	s.aria2ExportPath = path
}

// Aria2ExportPath 返回 aria2 文本 session 导出路径。
func (s *FileStore) Aria2ExportPath() string {
	if s == nil {
		return ""
	}
	return s.aria2ExportPath
}

// Load 从磁盘加载任务快照。自动识别 JSON 与 aria2 文本 save-session。
func (s *FileStore) Load(ctx context.Context) ([]*task.Task, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	tasks, err := s.loadPath(ctx, s.path)
	if err == nil {
		return tasks, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	companion := loadCompanionPath(s.path)
	if companion == "" || companion == s.path {
		return nil, nil
	}
	tasks, err = s.loadPath(ctx, companion)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return tasks, err
}

func (s *FileStore) loadPath(ctx context.Context, path string) ([]*task.Task, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}

	tasks, _, err := text.TasksFromReader(bytes.NewReader(data))
	return tasks, err
}

// Save 通过临时文件 + 原子替换方式落盘，避免写入中断导致 session 损坏。
func (s *FileStore) Save(ctx context.Context, tasks []*task.Task) error {
	if s == nil || s.path == "" {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	snapshot := make([]*task.Task, 0, len(tasks))
	for _, item := range tasks {
		snapshot = append(snapshot, item.Clone())
	}

	if err := s.saveJSON(ctx, s.path, snapshot); err != nil {
		return err
	}
	if s.aria2ExportPath == "" {
		return nil
	}
	return text.WriteFile(s.aria2ExportPath, snapshot)
}

func (s *FileStore) saveJSON(ctx context.Context, path string, tasks []*task.Task) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// CompanionExportPath 返回与 JSON session 路径对应的 aria2 文本 export 路径。
func CompanionExportPath(jsonPath string) string {
	if jsonPath == "" {
		return ""
	}
	ext := filepath.Ext(jsonPath)
	if ext == ".json" || ext == ".JSON" {
		return jsonPath[:len(jsonPath)-len(ext)]
	}
	return jsonPath + ".aria2"
}

func loadCompanionPath(jsonPath string) string {
	if jsonPath == "" {
		return ""
	}
	ext := filepath.Ext(jsonPath)
	if ext != ".json" && ext != ".JSON" {
		return ""
	}
	return jsonPath[:len(jsonPath)-len(ext)]
}

// LoadFile 便于测试：从任意 reader 加载（JSON 或 aria2 文本）。
func LoadFile(ctx context.Context, r io.Reader) ([]*task.Task, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	tasks, _, err := text.TasksFromReader(r)
	return tasks, err
}
