package manager

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

type btNativeDriver interface {
	GetBTTrackers(ctx context.Context, taskID string) (map[string]any, error)
	ReannounceTorrent(ctx context.Context, taskID string) error
	ForcePieceCheck(ctx context.Context, taskID string) error
}

type ed2kNativeDriver interface {
	GetED2KSources(ctx context.Context, taskID string) ([]string, error)
	RecheckEd2kFile(ctx context.Context, taskID string) error
}

// ProtocolStats 汇总各协议任务数量与速率。
type ProtocolStats struct {
	ByProtocol map[string]map[string]any
}

// GetNativeTask 返回任务的完整内部视图（native.getTask）。
func (m *Manager) GetNativeTask(ctx context.Context, gid string) (*task.Task, error) {
	item, err := m.TellStatus(ctx, gid)
	if err != nil {
		return nil, err
	}
	_, stored, _, err := m.lookupByGID(gid)
	if err != nil {
		return item, nil
	}
	if len(stored.Meta) > 0 {
		if item.Meta == nil {
			item.Meta = map[string]string{}
		}
		for k, v := range stored.Meta {
			item.Meta[k] = v
		}
	}
	if stored.LocalOptions != nil {
		item.LocalOptions = cloneOptions(stored.LocalOptions)
	}
	return item, nil
}

// GetNativeTaskMeta 返回任务 Meta 映射（native.getTaskMeta）。
func (m *Manager) GetNativeTaskMeta(ctx context.Context, gid string) (map[string]string, error) {
	_, current, _, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	return cloneOptions(current.Meta), nil
}

// GetProtocolStats 返回协议级统计（native.getProtocolStats）。
func (m *Manager) GetProtocolStats() ProtocolStats {
	stats := ProtocolStats{ByProtocol: map[string]map[string]any{}}
	for _, item := range m.SnapshotTasks() {
		if item == nil {
			continue
		}
		key := string(item.Protocol)
		bucket := stats.ByProtocol[key]
		if bucket == nil {
			bucket = map[string]any{
				"tasks":         0,
				"downloadSpeed": int64(0),
				"uploadSpeed":   int64(0),
			}
			stats.ByProtocol[key] = bucket
		}
		bucket["tasks"] = bucket["tasks"].(int) + 1
		bucket["downloadSpeed"] = bucket["downloadSpeed"].(int64) + item.DownloadSpeed
		bucket["uploadSpeed"] = bucket["uploadSpeed"].(int64) + item.UploadSpeed
	}
	return stats
}

// ImportSessionFrom 从 go-aria2 session JSON 导入任务（native.importSession）。
func (m *Manager) ImportSessionFrom(ctx context.Context, path string) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, fmt.Errorf("session path is required")
	}
	tasks, err := session.NewFileStore(path).Load(ctx)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, stored := range tasks {
		if stored == nil || stored.Status == task.StatusRemoved {
			continue
		}
		input, err := addInputFromStoredTask(stored)
		if err != nil {
			return imported, err
		}
		if _, err := m.Add(ctx, input); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func (m *Manager) GetBTTrackers(ctx context.Context, gid string) (map[string]any, error) {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	if current.Protocol != task.ProtocolBT {
		return nil, fmt.Errorf("task %s is not a bt task", gid)
	}
	native, ok := driver.(btNativeDriver)
	if !ok {
		return nil, fmt.Errorf("bt native API not available")
	}
	return native.GetBTTrackers(ctx, taskID)
}

func (m *Manager) ReannounceTorrent(ctx context.Context, gid string) error {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return err
	}
	if current.Protocol != task.ProtocolBT {
		return fmt.Errorf("task %s is not a bt task", gid)
	}
	native, ok := driver.(btNativeDriver)
	if !ok {
		return fmt.Errorf("bt native API not available")
	}
	return native.ReannounceTorrent(ctx, taskID)
}

func (m *Manager) ForcePieceCheck(ctx context.Context, gid string) error {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return err
	}
	if current.Protocol != task.ProtocolBT {
		return fmt.Errorf("task %s is not a bt task", gid)
	}
	native, ok := driver.(btNativeDriver)
	if !ok {
		return fmt.Errorf("bt native API not available")
	}
	return native.ForcePieceCheck(ctx, taskID)
}

func (m *Manager) GetED2KSources(ctx context.Context, gid string) ([]string, error) {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return nil, err
	}
	if current.Protocol != task.ProtocolED2K {
		return nil, fmt.Errorf("task %s is not an ed2k task", gid)
	}
	native, ok := driver.(ed2kNativeDriver)
	if !ok {
		return nil, fmt.Errorf("ed2k native API not available")
	}
	return native.GetED2KSources(ctx, taskID)
}

func (m *Manager) RecheckEd2kFile(ctx context.Context, gid string) error {
	taskID, current, driver, err := m.lookupByGID(gid)
	if err != nil {
		return err
	}
	if current.Protocol != task.ProtocolED2K {
		return fmt.Errorf("task %s is not an ed2k task", gid)
	}
	native, ok := driver.(ed2kNativeDriver)
	if !ok {
		return fmt.Errorf("ed2k native API not available")
	}
	return native.RecheckEd2kFile(ctx, taskID)
}

func addInputFromStoredTask(stored *task.Task) (task.AddTaskInput, error) {
	if stored == nil {
		return task.AddTaskInput{}, fmt.Errorf("nil task")
	}
	input := task.AddTaskInput{
		GID:     stored.GID,
		SaveDir: stored.SaveDir,
		Name:    stored.Name,
		Meta:    cloneOptions(stored.Meta),
	}
	if stored.LocalOptions != nil {
		input.Options = cloneOptions(stored.LocalOptions)
	} else {
		input.Options = cloneOptions(stored.Options)
	}

	switch stored.Protocol {
	case task.ProtocolBT:
		switch stored.Meta["bt.source.kind"] {
		case "magnet":
			input.URI = stored.Meta["bt.source.uri"]
		case "torrent-bytes", "torrent-url":
			raw, err := base64.StdEncoding.DecodeString(stored.Meta["bt.source.torrentBase64"])
			if err != nil {
				return task.AddTaskInput{}, err
			}
			input.Torrent = raw
			input.URI = stored.Meta["bt.source.uri"]
		default:
			return task.AddTaskInput{}, fmt.Errorf("unsupported bt session source for import")
		}
	case task.Protocol("file"):
		if uri := strings.TrimSpace(stored.Meta["file.sourceURI"]); uri != "" {
			input.URI = uri
		} else if len(stored.Files) > 0 && len(stored.Files[0].URIs) > 0 {
			input.URI = stored.Files[0].URIs[0]
		} else {
			return task.AddTaskInput{}, fmt.Errorf("file task has no URI to import")
		}
	case task.ProtocolHTTP:
		seen := map[string]struct{}{}
		for _, file := range stored.Files {
			for _, uri := range file.URIs {
				if uri == "" {
					continue
				}
				if _, ok := seen[uri]; ok {
					continue
				}
				seen[uri] = struct{}{}
				input.URIs = append(input.URIs, uri)
			}
		}
		if len(input.URIs) == 0 {
			return task.AddTaskInput{}, fmt.Errorf("http task has no URIs to import")
		}
	case task.ProtocolED2K:
		if uri := strings.TrimSpace(stored.Meta["ed2k.sourceURI"]); uri != "" {
			input.URI = uri
		} else if len(stored.Files) > 0 && len(stored.Files[0].URIs) > 0 {
			input.URI = stored.Files[0].URIs[0]
		} else {
			return task.AddTaskInput{}, fmt.Errorf("ed2k task has no URI to import")
		}
	default:
		return task.AddTaskInput{}, fmt.Errorf("unsupported protocol %q", stored.Protocol)
	}
	return input, nil
}
