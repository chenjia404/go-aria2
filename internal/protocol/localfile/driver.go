package localfile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

type state struct {
	task       *task.Task
	sourcePath string
	sourceURI  string
	outputPath string
	cancel     context.CancelFunc
	running    bool
	paused     bool
	removed    bool
	fileAlloc  common.FileAllocationMode
	lastTick   time.Time
	lastBytes  int64
}

// Driver 实现 aria2 兼容的 file:// 本地文件复制下载。
type Driver struct {
	mu    sync.RWMutex
	tasks map[string]*state
}

// New 创建 file:// 驱动。
func New() *Driver {
	return &Driver{tasks: make(map[string]*state)}
}

func (d *Driver) Name() string { return "file" }

func (d *Driver) CanHandle(input task.AddTaskInput) bool {
	for _, uri := range append([]string{input.URI}, input.URIs...) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "file://") {
			return true
		}
	}
	return false
}

func (d *Driver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx
	sourceURI, sourcePath, err := resolveSource(input)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("source file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source path is a directory: %s", sourcePath)
	}

	name := deriveName(sourcePath, input.Name)
	name = common.ResolveIndexOutName(input.Options, 1, name)
	outputPath := filepath.Join(input.SaveDir, name)
	if shouldAutoRename(input.Options, outputPath) {
		renamedPath, renamedName, err := nextAvailablePath(outputPath)
		if err != nil {
			return nil, err
		}
		outputPath = renamedPath
		name = renamedName
	}

	total := info.Size()
	item := &task.Task{
		ID:              newID(),
		Protocol:        task.Protocol("file"),
		Name:            name,
		Status:          task.StatusWaiting,
		SaveDir:         input.SaveDir,
		TotalLength:     total,
		CompletedLength: 0,
		Files: []task.File{{
			Index:    1,
			Path:     outputPath,
			Length:   total,
			Selected: true,
			URIs:     []string{sourceURI},
		}},
		Options: cloneMap(input.Options),
		Meta: map[string]string{
			"file.sourceURI":  sourceURI,
			"file.sourcePath": sourcePath,
			"file.outputPath": outputPath,
		},
	}

	d.mu.Lock()
	d.tasks[item.ID] = &state{
		task:       item.Clone(),
		sourcePath: sourcePath,
		sourceURI:  sourceURI,
		outputPath: outputPath,
		fileAlloc:  common.ParseFileAllocation(input.Options),
		lastTick:   time.Now(),
	}
	d.mu.Unlock()
	return item.Clone(), nil
}

func (d *Driver) Start(ctx context.Context, taskID string) error {
	_ = ctx
	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	if st.running {
		d.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	st.running = true
	st.paused = false
	st.task.Status = task.StatusActive
	st.task.DownloadSpeed = 0
	st.task.Connections = 1
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()

	go d.copy(runCtx, taskID)
	return nil
}

func (d *Driver) Pause(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.running = false
	st.paused = true
	st.task.Status = task.StatusPaused
	st.task.DownloadSpeed = 0
	st.task.Connections = 0
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()
	return nil
}

func (d *Driver) Remove(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	st.removed = true
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.running = false
	st.task.Status = task.StatusRemoved
	path := st.outputPath
	d.mu.Unlock()
	if force && path != "" {
		_ = os.Remove(path)
	}
	return nil
}

func (d *Driver) PurgeLocalState(taskID string) {
	d.mu.Lock()
	delete(d.tasks, taskID)
	d.mu.Unlock()
}

func (d *Driver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil {
		return nil, manager.ErrTaskNotFound
	}
	now := time.Now()
	if st.running && !st.lastTick.IsZero() {
		elapsed := now.Sub(st.lastTick).Seconds()
		if elapsed > 0 {
			delta := st.task.CompletedLength - st.lastBytes
			if delta > 0 {
				st.task.DownloadSpeed = int64(float64(delta) / elapsed)
			}
			st.lastTick = now
			st.lastBytes = st.task.CompletedLength
		}
	} else {
		st.task.DownloadSpeed = 0
	}
	return st.task.Clone(), nil
}

func (d *Driver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	item, err := d.TellStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.CloneFiles(item.Files), nil
}

func (d *Driver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	if len(opts) == 0 {
		return nil
	}
	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	if st.task.Options == nil {
		st.task.Options = map[string]string{}
	}
	for k, v := range opts {
		st.task.Options[k] = v
	}
	st.fileAlloc = common.ParseFileAllocation(st.task.Options)
	if _, ok := opts["index-out"]; ok {
		name := common.ResolveIndexOutName(st.task.Options, 1, st.task.Name)
		st.task.Name = name
		st.outputPath = filepath.Join(st.task.SaveDir, name)
		st.task.Files[0].Path = st.outputPath
		st.task.Meta["file.outputPath"] = st.outputPath
	}
	shouldPause, hasPause := common.PauseRequestedFromChangeOption(opts)
	d.mu.Unlock()
	if hasPause {
		if shouldPause {
			return d.Pause(ctx, taskID, false)
		}
		return d.Start(ctx, taskID)
	}
	return nil
}

func (d *Driver) LoadSessionTasks(ctx context.Context, tasks []*task.Task, globalOptions map[string]string) error {
	_ = ctx
	_ = globalOptions
	for _, saved := range tasks {
		if saved == nil || saved.Protocol != task.Protocol("file") {
			continue
		}
		sourcePath := strings.TrimSpace(saved.Meta["file.sourcePath"])
		sourceURI := strings.TrimSpace(saved.Meta["file.sourceURI"])
		outputPath := strings.TrimSpace(saved.Meta["file.outputPath"])
		if outputPath == "" && len(saved.Files) > 0 {
			outputPath = saved.Files[0].Path
		}
		if sourcePath == "" && sourceURI != "" {
			if p, err := parseFileURI(sourceURI); err == nil {
				sourcePath = p
			}
		}
		if sourcePath == "" || outputPath == "" {
			continue
		}
		d.mu.Lock()
		d.tasks[saved.ID] = &state{
			task:       saved.Clone(),
			sourcePath: sourcePath,
			sourceURI:  sourceURI,
			outputPath: outputPath,
			paused:     saved.Status == task.StatusPaused,
			running:    saved.Status == task.StatusActive,
			fileAlloc:  common.ParseFileAllocation(saved.Options),
			lastTick:   time.Now(),
			lastBytes:  saved.CompletedLength,
		}
		d.mu.Unlock()
		if saved.Status == task.StatusActive {
			_ = d.Start(context.Background(), saved.ID)
		}
	}
	return nil
}

func (d *Driver) copy(ctx context.Context, taskID string) {
	defer func() {
		d.mu.Lock()
		if st := d.tasks[taskID]; st != nil {
			st.running = false
			if st.cancel != nil {
				st.cancel()
				st.cancel = nil
			}
		}
		d.mu.Unlock()
	}()

	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(st.outputPath), 0o755); err != nil {
		d.fail(taskID, err)
		return
	}

	srcInfo, err := os.Stat(st.sourcePath)
	if err != nil {
		d.fail(taskID, err)
		return
	}
	total := srcInfo.Size()

	localSize := int64(0)
	if info, statErr := os.Stat(st.outputPath); statErr == nil {
		localSize = info.Size()
	}
	if common.ShouldRejectExistingFile(st.task.Options, localSize) {
		d.fail(taskID, fmt.Errorf("target file already exists and allow-overwrite=false: %s", st.outputPath))
		return
	}
	if common.ShouldResetExistingFile(st.task.Options, localSize) {
		_ = os.Remove(st.outputPath)
		localSize = 0
	}

	resumePartial := common.ShouldResumePartial(st.task.Options, localSize, total)
	dst, offset, err := common.PrepareDownloadFile(st.outputPath, st.fileAlloc, localSize, total, resumePartial)
	if err != nil {
		d.fail(taskID, err)
		return
	}
	defer dst.Close()

	src, err := os.Open(st.sourcePath)
	if err != nil {
		d.fail(taskID, err)
		return
	}
	defer src.Close()
	if offset > 0 {
		if _, err := src.Seek(offset, io.SeekStart); err != nil {
			d.fail(taskID, err)
			return
		}
	}

	d.mu.Lock()
	if cur := d.tasks[taskID]; cur != nil {
		cur.task.TotalLength = total
		cur.task.CompletedLength = offset
		cur.lastBytes = offset
		cur.lastTick = time.Now()
		if len(cur.task.Files) > 0 {
			cur.task.Files[0].Length = total
			cur.task.Files[0].CompletedLength = offset
		}
	}
	d.mu.Unlock()

	buf := make([]byte, 256*1024)
	written := offset
	for {
		if ctx.Err() != nil {
			d.pauseAfterCancel(taskID)
			return
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				d.fail(taskID, writeErr)
				return
			}
			written += int64(n)
			d.mu.Lock()
			if cur := d.tasks[taskID]; cur != nil {
				cur.task.CompletedLength = written
				if len(cur.task.Files) > 0 {
					cur.task.Files[0].CompletedLength = written
				}
			}
			d.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			d.fail(taskID, readErr)
			return
		}
	}
	d.complete(taskID)
}

func (d *Driver) complete(taskID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil {
		return
	}
	st.running = false
	st.task.Status = task.StatusComplete
	st.task.CompletedLength = st.task.TotalLength
	st.task.DownloadSpeed = 0
	st.task.Connections = 0
	st.task.UpdatedAt = time.Now()
	if len(st.task.Files) > 0 {
		st.task.Files[0].CompletedLength = st.task.Files[0].Length
	}
}

func (d *Driver) fail(taskID string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil {
		return
	}
	st.running = false
	st.task.Status = task.StatusError
	st.task.ErrorCode = "1"
	st.task.ErrorMessage = err.Error()
	st.task.DownloadSpeed = 0
	st.task.Connections = 0
	st.task.UpdatedAt = time.Now()
}

func (d *Driver) pauseAfterCancel(taskID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		return
	}
	st.running = false
	st.paused = true
	st.task.Status = task.StatusPaused
	st.task.DownloadSpeed = 0
	st.task.Connections = 0
	st.task.UpdatedAt = time.Now()
}

func resolveSource(input task.AddTaskInput) (uri, path string, err error) {
	for _, candidate := range append([]string{input.URI}, input.URIs...) {
		candidate = strings.TrimSpace(candidate)
		if !strings.HasPrefix(strings.ToLower(candidate), "file://") {
			continue
		}
		path, err = parseFileURI(candidate)
		if err != nil {
			return "", "", err
		}
		return candidate, path, nil
	}
	return "", "", fmt.Errorf("file uri is required")
}

func parseFileURI(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if strings.ToLower(u.Scheme) != "file" {
		return "", fmt.Errorf("not a file URI")
	}
	path := u.Path
	if path == "" {
		path = u.Opaque
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && len(path) >= 3 && path[2] == ':' {
		path = path[1:]
	}
	path = filepath.FromSlash(path)
	if path == "" {
		return "", fmt.Errorf("empty file path")
	}
	return path, nil
}

func deriveName(sourcePath, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return filepath.Base(requested)
	}
	return filepath.Base(sourcePath)
}

func shouldAutoRename(opts map[string]string, outputPath string) bool {
	if !common.ResolveBoolOption(opts, "auto-file-renaming", false) {
		return false
	}
	_, err := os.Stat(outputPath)
	return err == nil
}

func nextAvailablePath(original string) (string, string, error) {
	if _, err := os.Stat(original); os.IsNotExist(err) {
		return original, filepath.Base(original), nil
	}
	dir := filepath.Dir(original)
	base := filepath.Base(original)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, filepath.Base(candidate), nil
		}
	}
	return "", "", fmt.Errorf("unable to find available path for %s", original)
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

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("file-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
