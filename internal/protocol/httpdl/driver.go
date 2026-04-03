package httpdl

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// Options 控制 HTTP/HTTPS 驱动的基础行为�?
type Options struct {
	UserAgent               string
	Referer                 string
	HTTPProxy               string
	HTTPSProxy              string
	AllProxy                string
	CheckCertificate        bool
	Split                   int
	MaxConnectionPerServer  int
	MaxOverallDownloadLimit int64
}

type state struct {
	task         *task.Task
	sourceURL    string
	outputPath   string
	client       *http.Client
	userAgent    string
	referer      string
	limiter      *byteLimiter
	progressBase int64
	progressDone int64
	cancel       context.CancelFunc
	running      bool
	paused       bool
	removed      bool
	lastTick     time.Time
	lastBytes    int64
}

// Driver 使用标准库完�?HTTP/HTTPS 下载�?
type Driver struct {
	mu       sync.RWMutex
	defaults Options
	tasks    map[string]*state
	limiter  *byteLimiter
}

// New 创建 HTTP/HTTPS 驱动�?
func New(opts Options) *Driver {
	return &Driver{
		defaults: opts,
		tasks:    make(map[string]*state),
		limiter:  newByteLimiter(opts.MaxOverallDownloadLimit),
	}
}

// Name 返回驱动名�?
func (d *Driver) Name() string {
	return "http"
}

// CanHandle 识别普�?HTTP/HTTPS 下载，但排除 .torrent URL�?
func (d *Driver) CanHandle(input task.AddTaskInput) bool {
	for _, uri := range append([]string{input.URI}, input.URIs...) {
		lower := strings.ToLower(strings.TrimSpace(uri))
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			if strings.HasSuffix(lower, ".torrent") {
				continue
			}
			return true
		}
	}
	return false
}

// Add 仅注册任务，不立即开始下载�?
func (d *Driver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx

	sourceURL, err := firstHTTPURL(input)
	if err != nil {
		return nil, err
	}
	name := deriveName(sourceURL, input.Name)
	outputPath := outputPathFor(input.SaveDir, name)
	if shouldAutoRenameOnAdd(input.Options, outputPath) {
		renamedPath, renamedName, err := nextAvailablePath(outputPath)
		if err != nil {
			return nil, err
		}
		outputPath = renamedPath
		name = renamedName
	}

	item := &task.Task{
		ID:       newID(),
		Protocol: task.Protocol("http"),
		Name:     name,
		Status:   task.StatusWaiting,
		SaveDir:  input.SaveDir,
		Files: []task.File{{
			Index:           0,
			Path:            outputPath,
			Length:          0,
			CompletedLength: 0,
			Selected:        true,
			URIs:            []string{sourceURL},
		}},
		Options: cloneMap(input.Options),
		Meta:    cloneMeta(input.Meta, sourceURL, outputPath),
	}

	d.mu.Lock()
	d.tasks[item.ID] = &state{
		task:       item.Clone(),
		sourceURL:  sourceURL,
		outputPath: outputPath,
		client:     buildTaskClient(d.defaults, input.Options),
		userAgent:  resolveStringOption(input.Options, "http-user-agent", defaultUserAgent(d.defaults)),
		referer:    resolveStringOption(input.Options, "http-referer", d.defaults.Referer),
		limiter:    buildTaskLimiter(d.limiter, d.defaults, input.Options),
		lastTick:   time.Now(),
	}
	d.mu.Unlock()
	return item.Clone(), nil
}

// Start 开始或恢复下载�?
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

	// 下载生命周期长于单次 JSON-RPC 请求。若使用 r.Context()，响应返回后 context 取消，
	// download 会立刻退出，表现为 aria2.unpause / addUri 后立即停住。
	runCtx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	st.running = true
	st.paused = false
	st.task.Status = task.StatusActive
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()

	go d.download(runCtx, taskID)
	return nil
}

// Pause 中断当前下载，但保留本地文件用于断点续传�?
func (d *Driver) Pause(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force

	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	st.paused = true
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.running = false
	st.task.Status = task.StatusPaused
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()
	return nil
}

// Remove 取消下载并可选删除目标文件�?
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
	st.task.UpdatedAt = time.Now()
	outputPath := st.outputPath
	d.mu.Unlock()

	if force && outputPath != "" {
		_ = os.Remove(outputPath)
	}
	return nil
}

// TellStatus 返回当前统一任务状态�?
func (d *Driver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	d.mu.Lock()
	defer d.mu.Unlock()

	st := d.tasks[taskID]
	if st == nil {
		return nil, manager.ErrTaskNotFound
	}

	if st.task.Status == task.StatusActive {
		now := time.Now()
		if !st.lastTick.IsZero() {
			elapsed := now.Sub(st.lastTick).Seconds()
			if elapsed > 0 {
				advanced := st.task.CompletedLength - st.lastBytes
				if advanced > 0 {
					st.task.DownloadSpeed = int64(float64(advanced) / elapsed)
				}
			}
		}
		st.lastTick = now
		st.lastBytes = st.task.CompletedLength
	}

	return st.task.Clone(), nil
}

// GetFiles 返回单文件任务模型�?
func (d *Driver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	_ = ctx
	item, err := d.TellStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.CloneFiles(item.Files), nil
}

// GetServers 返回当前 HTTP 任务使用的服务器视图，供 aria2.getServers 使用�?
func (d *Driver) GetServers(ctx context.Context, taskID string) ([]manager.FileServerInfo, error) {
	_ = ctx

	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}

	seen := make(map[string]struct{})
	entries := make([]manager.ServerEntry, 0, len(st.task.Files))
	for _, file := range st.task.Files {
		for _, uri := range file.URIs {
			if uri == "" {
				continue
			}
			if _, ok := seen[uri]; ok {
				continue
			}
			seen[uri] = struct{}{}
			currentURI := uri
			if uri == st.sourceURL {
				currentURI = st.sourceURL
			}
			speed := int64(0)
			if uri == st.sourceURL {
				speed = st.task.DownloadSpeed
			}
			entries = append(entries, manager.ServerEntry{
				URI:           uri,
				CurrentURI:    currentURI,
				DownloadSpeed: speed,
			})
		}
	}
	if len(entries) == 0 && st.sourceURL != "" {
		entries = append(entries, manager.ServerEntry{
			URI:           st.sourceURL,
			CurrentURI:    st.sourceURL,
			DownloadSpeed: st.task.DownloadSpeed,
		})
	}
	if len(entries) == 0 {
		return []manager.FileServerInfo{}, nil
	}
	return []manager.FileServerInfo{{
		Index:   1,
		Servers: entries,
	}}, nil
}

// ChangeOption 支持 pause 选项�?
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
	for key, value := range opts {
		st.task.Options[key] = value
	}
	st.client = buildTaskClient(d.defaults, st.task.Options)
	st.userAgent = resolveStringOption(st.task.Options, "http-user-agent", defaultUserAgent(d.defaults))
	st.referer = resolveStringOption(st.task.Options, "http-referer", d.defaults.Referer)
	st.limiter = buildTaskLimiter(d.limiter, d.defaults, st.task.Options)
	shouldPause, hasPause := opts["pause"]
	d.mu.Unlock()

	if hasPause {
		if strings.EqualFold(shouldPause, "true") || strings.EqualFold(shouldPause, "yes") || shouldPause == "1" {
			return d.Pause(ctx, taskID, false)
		}
		return d.Start(ctx, taskID)
	}
	return nil
}

// LoadSessionTasks 恢复会话中的 HTTP 任务�?
func (d *Driver) LoadSessionTasks(ctx context.Context, tasks []*task.Task) error {
	_ = ctx
	for _, saved := range tasks {
		if saved == nil || saved.Protocol != task.Protocol("http") {
			continue
		}

		sourceURL := saved.Meta["http.sourceURL"]
		if sourceURL == "" && len(saved.Files) > 0 && len(saved.Files[0].URIs) > 0 {
			sourceURL = saved.Files[0].URIs[0]
		}
		if sourceURL == "" {
			continue
		}

		outputPath := saved.Meta["http.outputPath"]
		if outputPath == "" && len(saved.Files) > 0 {
			outputPath = saved.Files[0].Path
		}

		st := &state{
			task:       saved.Clone(),
			sourceURL:  sourceURL,
			outputPath: outputPath,
			client:     buildTaskClient(d.defaults, saved.Options),
			userAgent:  resolveStringOption(saved.Options, "http-user-agent", defaultUserAgent(d.defaults)),
			referer:    resolveStringOption(saved.Options, "http-referer", d.defaults.Referer),
			limiter:    buildTaskLimiter(d.limiter, d.defaults, saved.Options),
			paused:     saved.Status == task.StatusPaused,
			running:    saved.Status == task.StatusActive,
			lastTick:   time.Now(),
			lastBytes:  saved.CompletedLength,
		}

		d.mu.Lock()
		d.tasks[saved.ID] = st
		d.mu.Unlock()

		if saved.Status == task.StatusActive {
			if err := d.Start(context.Background(), saved.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Driver) download(ctx context.Context, taskID string) {
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

	existingSize, _ := fileSize(st.outputPath)
	if shouldRejectExistingFile(st.task.Options, existingSize) {
		d.fail(taskID, fmt.Errorf("target file already exists and allow-overwrite=false: %s", st.outputPath))
		return
	}
	if shouldResetExistingFile(st.task.Options, existingSize) {
		existingSize = 0
	}
	total, acceptRanges, err := d.probeResource(ctx, st)
	if err != nil && total <= 0 {
		total = 0
		acceptRanges = false
	}

	if total > 0 && existingSize >= total {
		d.complete(taskID, total)
		return
	}

	segmentCount := d.segmentCount(st)
	if total > 0 && acceptRanges && segmentCount > 1 {
		if err := d.downloadChunked(ctx, taskID, st, existingSize, total, segmentCount); err != nil {
			if ctx.Err() != nil {
				d.pauseAfterCancel(taskID)
				return
			}
			d.fail(taskID, err)
			return
		}
		d.complete(taskID, total)
		return
	}

	if err := d.downloadSingle(ctx, taskID, st, existingSize, total); err != nil {
		if ctx.Err() != nil {
			d.pauseAfterCancel(taskID)
			return
		}
		d.fail(taskID, err)
		return
	}
	if total > 0 {
		d.complete(taskID, total)
	}
}

func (d *Driver) downloadSingle(ctx context.Context, taskID string, st *state, existingSize, total int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, st.sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", st.userAgent)
	if st.referer != "" {
		req.Header.Set("Referer", st.referer)
	}
	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	resp, err := st.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	offset := existingSize
	mode := os.O_CREATE | os.O_RDWR
	if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
	} else {
		mode |= os.O_TRUNC
		offset = 0
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("http status %s", resp.Status)
	}

	file, err := os.OpenFile(st.outputPath, mode, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if total <= 0 {
		total = totalLengthFromResponse(resp, offset)
	}
	if total > 0 {
		if err := file.Truncate(total); err != nil {
			return err
		}
		d.setTaskTotal(taskID, total)
	}
	d.prepareProgress(taskID, offset)

	buf := make([]byte, 32*1024)
	written := offset
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if err := d.waitBytes(ctx, st, int64(n)); err != nil {
				return err
			}
			if _, err := file.WriteAt(buf[:n], written); err != nil {
				return err
			}
			written += int64(n)
			d.advanceProgress(taskID, int64(n))
			continue
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	d.setCompleted(taskID, written)
	return nil
}

func (d *Driver) downloadChunked(ctx context.Context, taskID string, st *state, existingSize, total int64, segments int) error {
	startOffset := existingSize
	if startOffset < 0 {
		startOffset = 0
	}
	if startOffset > total {
		startOffset = 0
	}

	file, err := os.OpenFile(st.outputPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(total); err != nil {
		return err
	}
	d.setTaskTotal(taskID, total)
	d.prepareProgress(taskID, startOffset)
	d.update(taskID, func(item *task.Task) {
		item.Connections = segments
	})

	ranges := splitRanges(startOffset, total, segments)
	if len(ranges) == 0 {
		d.setCompleted(taskID, total)
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg   sync.WaitGroup
		once sync.Once
		errC error
		sem  = make(chan struct{}, len(ranges))
	)

	for _, r := range ranges {
		rangeStart := r[0]
		rangeEnd := r[1]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if err := d.downloadRange(ctx, st, file, rangeStart, rangeEnd, taskID); err != nil {
				once.Do(func() {
					errC = err
					cancel()
				})
			}
		}()
	}
	wg.Wait()
	if errC != nil {
		return errC
	}
	return nil
}

func (d *Driver) downloadRange(ctx context.Context, st *state, file *os.File, start, end int64, taskID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, st.sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", st.userAgent)
	if st.referer != "" {
		req.Header.Set("Referer", st.referer)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := st.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && !(start == 0 && resp.StatusCode == http.StatusOK) {
		return fmt.Errorf("range request rejected: %s", resp.Status)
	}

	buf := make([]byte, 32*1024)
	offset := start
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if err := d.waitBytes(ctx, st, int64(n)); err != nil {
				return err
			}
			if _, err := file.WriteAt(buf[:n], offset); err != nil {
				return err
			}
			offset += int64(n)
			d.advanceProgress(taskID, int64(n))
			continue
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

func (d *Driver) probeResource(ctx context.Context, st *state) (int64, bool, error) {
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, st.sourceURL, nil)
	if err == nil {
		headReq.Header.Set("User-Agent", st.userAgent)
		if st.referer != "" {
			headReq.Header.Set("Referer", st.referer)
		}
		if resp, err := st.client.Do(headReq); err == nil {
			total := totalLengthFromHead(resp)
			acceptRanges := supportsRanges(resp)
			_ = resp.Body.Close()
			if total > 0 {
				return total, acceptRanges, nil
			}
		}
	}

	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, st.sourceURL, nil)
	if err != nil {
		return 0, false, err
	}
	getReq.Header.Set("User-Agent", st.userAgent)
	if st.referer != "" {
		getReq.Header.Set("Referer", st.referer)
	}
	getReq.Header.Set("Range", "bytes=0-0")

	resp, err := st.client.Do(getReq)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent {
		io.Copy(io.Discard, resp.Body)
		return totalLengthFromResponse(resp, 0), true, nil
	}
	if resp.StatusCode == http.StatusOK {
		return totalLengthFromResponse(resp, 0), supportsRanges(resp), nil
	}
	return 0, false, fmt.Errorf("probe status %s", resp.Status)
}

func (d *Driver) advanceProgress(taskID string, delta int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		return
	}
	st.progressDone += delta
	now := time.Now()
	completed := st.progressBase + st.progressDone
	if !st.lastTick.IsZero() {
		elapsed := now.Sub(st.lastTick).Seconds()
		if elapsed > 0 {
			st.task.DownloadSpeed = int64(float64(completed-st.lastBytes) / elapsed)
		}
	}
	st.task.CompletedLength = completed
	st.task.Status = task.StatusActive
	st.task.UploadSpeed = 0
	st.task.Connections = 1
	st.task.UpdatedAt = now
	st.lastTick = now
	st.lastBytes = completed
}

func (d *Driver) prepareProgress(taskID string, base int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		return
	}
	st.progressBase = base
	st.progressDone = 0
	st.lastTick = time.Now()
	st.lastBytes = base
	st.task.CompletedLength = base
	st.task.TotalLength = maxInt64(st.task.TotalLength, base)
	st.task.Status = task.StatusActive
	if st.task.Connections < 1 {
		st.task.Connections = 1
	}
	st.task.UploadSpeed = 0
	st.task.DownloadSpeed = 0
	st.task.UpdatedAt = st.lastTick
}

func (d *Driver) setTaskTotal(taskID string, total int64) {
	d.update(taskID, func(item *task.Task) {
		item.TotalLength = total
	})
}

func (d *Driver) setCompleted(taskID string, completed int64) {
	d.update(taskID, func(item *task.Task) {
		item.CompletedLength = completed
		item.TotalLength = maxInt64(item.TotalLength, completed)
		item.DownloadSpeed = 0
		item.UploadSpeed = 0
		item.Status = task.StatusComplete
		item.Connections = 0
		item.UpdatedAt = time.Now()
	})
}

func (d *Driver) complete(taskID string, total int64) {
	d.setCompleted(taskID, total)
}

func (d *Driver) segmentCount(st *state) int {
	count := optionInt(st.task.Options, "split", d.defaults.Split)
	if count <= 0 {
		count = 1
	}
	if maxConn := optionInt(st.task.Options, "max-connection-per-server", d.defaults.MaxConnectionPerServer); maxConn > 0 && count > maxConn {
		count = maxConn
	}
	if count <= 0 {
		count = 1
	}
	return count
}

func (d *Driver) waitBytes(ctx context.Context, st *state, n int64) error {
	if st == nil || st.limiter == nil {
		return nil
	}
	return st.limiter.Wait(ctx, n)
}

func (d *Driver) update(taskID string, fn func(item *task.Task)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil {
		return
	}
	fn(st.task)
}

func (d *Driver) fail(taskID string, err error) {
	d.update(taskID, func(item *task.Task) {
		item.Status = task.StatusError
		item.ErrorMessage = err.Error()
		item.DownloadSpeed = 0
		item.Connections = 0
		item.UpdatedAt = time.Now()
	})
}

func (d *Driver) pauseAfterCancel(taskID string) {
	d.update(taskID, func(item *task.Task) {
		if item.Status != task.StatusRemoved {
			item.Status = task.StatusPaused
			item.DownloadSpeed = 0
			item.Connections = 0
			item.UpdatedAt = time.Now()
		}
	})
}

func firstHTTPURL(input task.AddTaskInput) (string, error) {
	for _, raw := range append([]string{input.URI}, input.URIs...) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "http://") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "https://") {
			return raw, nil
		}
	}
	return "", fmt.Errorf("missing http/https URL")
}

func deriveName(rawURL, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	parsed, err := url.Parse(rawURL)
	if err == nil {
		base := filepath.Base(parsed.Path)
		if base != "." && base != "/" && base != "" {
			return base
		}
	}
	return "download"
}

func outputPathFor(saveDir, name string) string {
	if saveDir == "" {
		saveDir = "."
	}
	return filepath.Join(saveDir, name)
}

func cloneMeta(base map[string]string, sourceURL, outputPath string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	out["http.sourceURL"] = sourceURL
	out["http.outputPath"] = outputPath
	return out
}

func buildTaskClient(base Options, opts map[string]string) *http.Client {
	resolved := base
	if value := resolveStringOption(opts, "http-user-agent", resolved.UserAgent); value != "" {
		resolved.UserAgent = value
	}
	if value := resolveStringOption(opts, "http-referer", resolved.Referer); value != "" {
		resolved.Referer = value
	}
	if value := resolveStringOption(opts, "http-proxy", resolved.HTTPProxy); value != "" {
		resolved.HTTPProxy = value
	}
	if value := resolveStringOption(opts, "https-proxy", resolved.HTTPSProxy); value != "" {
		resolved.HTTPSProxy = value
	}
	if value := resolveStringOption(opts, "all-proxy", resolved.AllProxy); value != "" {
		resolved.AllProxy = value
	}
	if value, ok := opts["check-certificate"]; ok {
		if parsed, err := parseBoolOption(value); err == nil {
			resolved.CheckCertificate = parsed
		}
	}
	if value, ok := opts["max-connection-per-server"]; ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			resolved.MaxConnectionPerServer = parsed
		}
	}
	return buildClient(resolved)
}

func buildTaskLimiter(base *byteLimiter, defaults Options, opts map[string]string) *byteLimiter {
	if opts == nil {
		return base
	}
	if value, ok := opts["max-download-limit"]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			if parsed <= 0 {
				return nil
			}
			return newByteLimiter(parsed)
		}
	}
	if value, ok := opts["max-overall-download-limit"]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			if parsed <= 0 {
				return nil
			}
			return newByteLimiter(parsed)
		}
	}
	return base
}

func buildClient(opts Options) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !opts.CheckCertificate}, //nolint:gosec // aria2 兼容配置需要支持关闭证书校验�?		Proxy:           proxyFunc(opts),
	}
	if opts.MaxConnectionPerServer > 0 {
		transport.MaxConnsPerHost = opts.MaxConnectionPerServer
	}
	return &http.Client{
		Transport: transport,
	}
}

func defaultUserAgent(opts Options) string {
	if opts.UserAgent != "" {
		return opts.UserAgent
	}
	return "github.com/chenjia404/go-aria2/0.1"
}

func resolveStringOption(opts map[string]string, key, fallback string) string {
	if opts == nil {
		return fallback
	}
	if value, ok := opts[key]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func nextAvailablePath(original string) (string, string, error) {
	dir := filepath.Dir(original)
	base := filepath.Base(original)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 10000; i++ {
		candidateName := fmt.Sprintf("%s.%d%s", stem, i, ext)
		candidatePath := filepath.Join(dir, candidateName)
		_, err := os.Stat(candidatePath)
		if os.IsNotExist(err) {
			return candidatePath, candidateName, nil
		}
		if err != nil {
			return "", "", err
		}
	}
	return "", "", fmt.Errorf("unable to find available file name for %s", original)
}

func resolveBoolOption(opts map[string]string, key string, fallback bool) bool {
	if opts == nil {
		return fallback
	}
	value, ok := opts[key]
	if !ok {
		return fallback
	}
	parsed, err := parseBoolOption(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func shouldAutoRenameOnAdd(opts map[string]string, outputPath string) bool {
	if outputPath == "" {
		return false
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	allowOverwrite := resolveBoolOption(opts, "allow-overwrite", false)
	continueDownloads := resolveBoolOption(opts, "continue", true)
	autoFileRenaming := resolveBoolOption(opts, "auto-file-renaming", false)
	return autoFileRenaming && !allowOverwrite && !continueDownloads
}

func parseBoolOption(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", value)
	}
}

func shouldRejectExistingFile(opts map[string]string, existingSize int64) bool {
	if existingSize <= 0 {
		return false
	}
	allowOverwrite := resolveBoolOption(opts, "allow-overwrite", false)
	continueDownloads := resolveBoolOption(opts, "continue", true)
	return !allowOverwrite && !continueDownloads
}

func shouldResetExistingFile(opts map[string]string, existingSize int64) bool {
	if existingSize <= 0 {
		return false
	}
	allowOverwrite := resolveBoolOption(opts, "allow-overwrite", false)
	continueDownloads := resolveBoolOption(opts, "continue", true)
	return allowOverwrite && !continueDownloads
}

func optionInt(options map[string]string, key string, fallback int) int {
	if options == nil {
		return fallback
	}
	value, ok := options[key]
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func supportsRanges(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") {
		return true
	}
	return resp.StatusCode == http.StatusPartialContent
}

func totalLengthFromHead(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		if total := parseContentRangeTotal(contentRange); total > 0 {
			return total
		}
	}
	return 0
}

func splitRanges(start, total int64, segments int) [][2]int64 {
	if segments <= 1 || total <= 0 || start >= total {
		return nil
	}

	remaining := total - start
	if remaining <= 0 {
		return nil
	}
	if int64(segments) > remaining {
		segments = int(remaining)
	}
	if segments <= 1 {
		return [][2]int64{{start, total - 1}}
	}

	chunkSize := remaining / int64(segments)
	if chunkSize <= 0 {
		chunkSize = 1
	}

	ranges := make([][2]int64, 0, segments)
	current := start
	for i := 0; i < segments; i++ {
		end := current + chunkSize - 1
		if i == segments-1 || end >= total-1 {
			end = total - 1
		}
		ranges = append(ranges, [2]int64{current, end})
		current = end + 1
		if current >= total {
			break
		}
	}
	if len(ranges) > 0 {
		ranges[len(ranges)-1][1] = total - 1
	}
	return ranges
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type byteLimiter struct {
	mu       sync.Mutex
	rate     int64
	tokens   float64
	lastFill time.Time
}

func newByteLimiter(rate int64) *byteLimiter {
	if rate <= 0 {
		return nil
	}
	return &byteLimiter{
		rate:     rate,
		tokens:   float64(rate),
		lastFill: time.Now(),
	}
}

func (l *byteLimiter) Wait(ctx context.Context, n int64) error {
	if l == nil || n <= 0 {
		return nil
	}

	need := float64(n)
	for {
		l.mu.Lock()
		now := time.Now()
		if !l.lastFill.IsZero() {
			elapsed := now.Sub(l.lastFill).Seconds()
			if elapsed > 0 {
				l.tokens += elapsed * float64(l.rate)
				if l.tokens > float64(l.rate) {
					l.tokens = float64(l.rate)
				}
				l.lastFill = now
			}
		}
		if l.tokens >= need {
			l.tokens -= need
			l.mu.Unlock()
			return nil
		}
		deficit := need - l.tokens
		wait := time.Duration(deficit / float64(l.rate) * float64(time.Second))
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		l.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
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

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func totalLengthFromResponse(resp *http.Response, existing int64) int64 {
	if resp.ContentLength > 0 {
		return resp.ContentLength + existing
	}
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		if total := parseContentRangeTotal(contentRange); total > 0 {
			return total
		}
	}
	return 0
}

func parseContentRangeTotal(value string) int64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0
	}
	total, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	return total
}

func newID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(raw)
}

func proxyFunc(opts Options) func(*http.Request) (*url.URL, error) {
	if opts.AllProxy != "" {
		proxyURL, err := url.Parse(opts.AllProxy)
		if err == nil {
			return http.ProxyURL(proxyURL)
		}
	}

	httpProxy, httpErr := parseProxyURL(opts.HTTPProxy)
	httpsProxy, httpsErr := parseProxyURL(opts.HTTPSProxy)
	if httpErr == nil || httpsErr == nil {
		return func(req *http.Request) (*url.URL, error) {
			switch req.URL.Scheme {
			case "https":
				if httpsErr == nil {
					return httpsProxy, nil
				}
			case "http":
				if httpErr == nil {
					return httpProxy, nil
				}
			}
			return http.ProxyFromEnvironment(req)
		}
	}

	return http.ProxyFromEnvironment
}

func parseProxyURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("empty proxy")
	}
	return url.Parse(raw)
}
