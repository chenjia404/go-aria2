package aria2session

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
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
			i.Logger.Printf("[INFO] Importing task: %s", displayName(item))
		}

		input, err := buildAddInput(item, i.Strict)
		if err != nil {
			errs = append(errs, err)
			if i.Logger != nil {
				i.Logger.Printf("[ERROR] skip task %s: %v", displayName(item), err)
			}
			continue
		}

		created, err := i.Manager.Add(ctx, input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", displayName(item), err))
			if i.Logger != nil {
				i.Logger.Printf("[ERROR] import failed for %s: %v", displayName(item), err)
			}
			continue
		}

		if checked, matched, actual, err := verifyTaskChecksum(created); err != nil {
			errs = append(errs, fmt.Errorf("%s checksum validation failed: %w", displayName(item), err))
			if i.Logger != nil {
				i.Logger.Printf("[ERROR] checksum validation failed for %s: %v", displayName(item), err)
			}
		} else if checked {
			expected := checksumRaw(created)
			if matched {
				if i.Logger != nil {
					i.Logger.Printf("[INFO] Checksum verified for %s: %s", displayName(item), actual)
				}
			} else {
				err := fmt.Errorf("%s checksum mismatch: expected %s got %s", displayName(item), expected, actual)
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
	kind, err := routeKind(item.URI)
	if err != nil {
		return task.AddTaskInput{}, err
	}

	opts := cloneMap(item.Options)
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
	if item.Paused || parseBoolValue(opts["pause"]) || parseBoolValue(opts["paused"]) {
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
		GID:     generateOrReuseGID(item),
		URI:     item.URI,
		SaveDir: item.Dir,
		Name:    item.Out,
		Options: opts,
		Meta: map[string]string{
			"aria2.import":        "true",
			"aria2.import.source": "save-session",
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
	if item.Paused || parseBoolValue(item.Options["pause"]) || parseBoolValue(item.Options["paused"]) {
		input.Meta["aria2.paused"] = "true"
	}
	return input, nil
}

func previewTask(item Aria2SessionTask) (*task.Task, error) {
	kind, err := routeKind(item.URI)
	if err != nil {
		return nil, err
	}
	saveDir := item.Dir
	name := item.Out
	if name == "" {
		name = derivePreviewName(item.URI)
	}
	if saveDir != "" && name != "" {
		name = filepath.Base(name)
	}

	t := &task.Task{
		ID:       generateOrReuseGID(item),
		GID:      generateOrReuseGID(item),
		Protocol: kind,
		Name:     name,
		Status:   task.StatusWaiting,
		SaveDir:  saveDir,
		Options:  cloneMap(item.Options),
		Meta: map[string]string{
			"aria2.import":        "true",
			"aria2.import.source": "save-session",
		},
	}
	if t.SaveDir != "" {
		t.Options["dir"] = t.SaveDir
	}
	if item.Out != "" {
		t.Options["out"] = item.Out
	}
	if item.Paused || parseBoolValue(item.Options["pause"]) || parseBoolValue(item.Options["paused"]) {
		t.Status = task.StatusPaused
		t.Options["pause"] = "true"
		t.Meta["aria2.paused"] = "true"
	}
	if item.Checksum != "" {
		t.Options["checksum"] = item.Checksum
		t.Meta["aria2.checksum"] = item.Checksum
	}
	if item.Metalink != "" {
		t.Options["metalink"] = item.Metalink
		t.Meta["aria2.metalink"] = item.Metalink
	}
	if checksum := strings.TrimSpace(item.Options["checksum"]); checksum != "" && t.Meta["aria2.checksum"] == "" {
		t.Meta["aria2.checksum"] = checksum
	}
	if metalink := strings.TrimSpace(item.Options["metalink"]); metalink != "" && t.Meta["aria2.metalink"] == "" {
		t.Meta["aria2.metalink"] = metalink
	}
	if t.Protocol == task.ProtocolBT {
		t.Meta["bt.resume.mode"] = "fast"
	}
	if t.Protocol == task.ProtocolED2K {
		t.Meta["ed2k.import"] = "true"
	}
	if t.Protocol == task.ProtocolHTTP {
		t.Files = []task.File{{
			Index:    0,
			Path:     filepath.Join(saveDir, name),
			Selected: true,
			URIs:     []string{item.URI},
		}}
	}
	return t, nil
}

func routeKind(uri string) (task.Protocol, error) {
	lower := strings.ToLower(strings.TrimSpace(uri))
	switch {
	case strings.HasPrefix(lower, "magnet:"):
		return task.ProtocolBT, nil
	case strings.HasPrefix(lower, "ed2k://"):
		return task.ProtocolED2K, nil
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		if strings.HasSuffix(lower, ".torrent") {
			return task.ProtocolBT, nil
		}
		return task.ProtocolHTTP, nil
	default:
		return "", fmt.Errorf("unsupported aria2 session uri: %s", uri)
	}
}

func generateOrReuseGID(item Aria2SessionTask) string {
	if gid := strings.TrimSpace(item.GID); gid != "" {
		if len(gid) == 16 {
			return strings.ToLower(gid)
		}
	}
	if gid := strings.TrimSpace(item.Options["gid"]); gid != "" {
		if len(gid) == 16 {
			return strings.ToLower(gid)
		}
	}

	sum := sha1.Sum([]byte(strings.Join([]string{item.URI, item.Dir, item.Out}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func derivePreviewName(uri string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "magnet:") {
		return "magnet-task"
	}
	if idx := strings.LastIndex(uri, "/"); idx >= 0 && idx < len(uri)-1 {
		return uri[idx+1:]
	}
	return filepath.Base(uri)
}

func displayName(item Aria2SessionTask) string {
	if item.Out != "" {
		return item.Out
	}
	return derivePreviewName(item.URI)
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ImportED2KTask 预留给后�?RPC/批量迁移使用�?
func ImportED2KTask(ctx context.Context, mgr *manager.Manager, item Aria2SessionTask) (*task.Task, error) {
	_ = ctx
	_ = mgr
	_ = item
	return nil, fmt.Errorf("not implemented")
}

// ImportFromAria2RPC 预留给后续通过 aria2 RPC 直连导入时使用�?
func ImportFromAria2RPC(ctx context.Context, endpoint, secret string) error {
	_ = ctx
	_ = endpoint
	_ = secret
	return fmt.Errorf("not implemented")
}
