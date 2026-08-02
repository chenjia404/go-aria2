package aria2session

import (
	"context"
	"fmt"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/migrate/aria2session/text"
)

// Logger 是迁移流程使用的最小日志接口�?
type Logger interface {
	Printf(format string, v ...any)
}

// ImportError 收集单个任务导入失败原因�?
type ImportError struct {
	Errors []error
}

// Error 汇总导入失败信息�?
func (e *ImportError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return ""
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	var b strings.Builder
	b.WriteString("multiple import errors:")
	for _, err := range e.Errors {
		b.WriteString("\n- ")
		b.WriteString(err.Error())
	}
	return b.String()
}

// Importer �?aria2 session 任务导入当前 manager�?
type Importer struct {
	Manager *manager.Manager
	Logger  Logger
	Strict  bool
}

// ImportAria2Tasks �?session 任务转换为内部任务预览�?
func ImportAria2Tasks(tasks []Aria2SessionTask) ([]*task.Task, error) {
	out := make([]*task.Task, 0, len(tasks))
	for _, item := range tasks {
		preview, err := previewTask(item)
		if err != nil {
			return nil, err
		}
		out = append(out, preview)
	}
	return out, nil
}

// ImportAria2Tasks 执行实际导入�?
func (i *Importer) ImportAria2Tasks(ctx context.Context, tasks []Aria2SessionTask) ([]*task.Task, error) {
	if i == nil || i.Manager == nil {
		return nil, fmt.Errorf("manager is required")
	}

	success := make([]*task.Task, 0, len(tasks))
	var errs []error
	for _, item := range tasks {
		if i.Logger != nil {
			i.Logger.Printf("[INFO] Importing task: %s", text.DisplayName(item))
		}

		input, err := buildAddInput(item, i.Strict)
		if err != nil {
			errs = append(errs, err)
			if i.Logger != nil {
				i.Logger.Printf("[ERROR] skip task %s: %v", text.DisplayName(item), err)
			}
			continue
		}

		created, err := i.Manager.Add(ctx, input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", text.DisplayName(item), err))
			if i.Logger != nil {
				i.Logger.Printf("[ERROR] import failed for %s: %v", text.DisplayName(item), err)
			}
			continue
		}

		if checked, matched, actual, err := verifyTaskChecksum(created); err != nil {
			errs = append(errs, fmt.Errorf("%s checksum validation failed: %w", text.DisplayName(item), err))
			if i.Logger != nil {
				i.Logger.Printf("[ERROR] checksum validation failed for %s: %v", text.DisplayName(item), err)
			}
		} else if checked {
			expected := checksumRaw(created)
			if matched {
				if i.Logger != nil {
					i.Logger.Printf("[INFO] Checksum verified for %s: %s", text.DisplayName(item), actual)
				}
			} else {
				err := fmt.Errorf("%s checksum mismatch: expected %s got %s", text.DisplayName(item), expected, actual)
				errs = append(errs, err)
				if i.Logger != nil {
					i.Logger.Printf("[ERROR] %v", err)
				}
			}
		}

		success = append(success, created)
		if i.Logger != nil {
			i.Logger.Printf("[INFO] Task imported successfully: gid=%s name=%s", created.GID, created.Name)
		}
	}

	if len(errs) > 0 {
		return success, &ImportError{Errors: errs}
	}
	return success, nil
}

func buildAddInput(item Aria2SessionTask, strict bool) (task.AddTaskInput, error) {
	kind, err := text.RouteKind(item.URI)
	if err != nil {
		return task.AddTaskInput{}, err
	}

	opts := text.CloneOptions(item.Options)
	gid := strings.TrimSpace(item.GID)
	if gid == "" {
		gid = strings.TrimSpace(opts["gid"])
	}
	if gid != "" {
		gid = strings.ToLower(gid)
		if len(gid) != 16 {
			gid = ""
		}
	}
	if item.Dir != "" {
		opts["dir"] = item.Dir
	}
	if item.Out != "" {
		opts["out"] = item.Out
	}
	if item.Paused || text.ParseBoolValue(opts["pause"]) || text.ParseBoolValue(opts["paused"]) {
		opts["pause"] = "true"
	}
	if item.Checksum != "" {
		opts["checksum"] = item.Checksum
	}
	if item.Metalink != "" {
		opts["metalink"] = item.Metalink
	}
	if strict {
		opts["bt.resume.mode"] = "strict"
	} else {
		opts["bt.resume.mode"] = "fast"
	}
	delete(opts, "gid")
	delete(opts, "paused")

	input := task.AddTaskInput{
		GID:     text.GenerateGID(item),
		URI:     item.URI,
		SaveDir: item.Dir,
		Name:    item.Out,
		Options: opts,
		Meta: map[string]string{
			"aria2.import":        "true",
			"aria2.import.source": importSource(item),
		},
	}

	switch kind {
	case task.ProtocolBT:
		input.URIs = []string{item.URI}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.URI)), "magnet:") {
			input.Torrent = nil
		}
	case task.ProtocolED2K:
		input.URIs = []string{item.URI}
	case task.ProtocolHTTP:
		input.URIs = []string{item.URI}
	}
	if gid != "" {
		input.GID = gid
	}
	if item.Checksum != "" {
		input.Meta["aria2.checksum"] = item.Checksum
	}
	if item.Metalink != "" {
		input.Meta["aria2.metalink"] = item.Metalink
	}
	if checksum := strings.TrimSpace(item.Options["checksum"]); checksum != "" && input.Meta["aria2.checksum"] == "" {
		input.Meta["aria2.checksum"] = checksum
	}
	if metalink := strings.TrimSpace(item.Options["metalink"]); metalink != "" && input.Meta["aria2.metalink"] == "" {
		input.Meta["aria2.metalink"] = metalink
	}
	if item.Paused || text.ParseBoolValue(item.Options["pause"]) || text.ParseBoolValue(item.Options["paused"]) {
		input.Meta["aria2.paused"] = "true"
	}
	return input, nil
}

// ImportED2KTask 导入单个 ED2K 任务。
func ImportED2KTask(ctx context.Context, mgr *manager.Manager, item Aria2SessionTask) (*task.Task, error) {
	if mgr == nil {
		return nil, fmt.Errorf("manager is required")
	}
	input, err := buildAddInput(item, false)
	if err != nil {
		return nil, err
	}
	return mgr.Add(ctx, input)
}

// ImportFromAria2RPC 从运行中的 aria2 JSON-RPC 拉取任务并导入 manager。
func ImportFromAria2RPC(ctx context.Context, importer *Importer, endpoint, secret string) ([]*task.Task, error) {
	if importer == nil || importer.Manager == nil {
		return nil, fmt.Errorf("importer with manager is required")
	}
	tasks, err := FetchAria2SessionTasksFromRPC(ctx, endpoint, secret)
	if err != nil {
		return nil, err
	}
	return importer.ImportAria2Tasks(ctx, tasks)
}
