package sftpproto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

type endpoint struct {
	rawURL   string
	user     string
	password string
	host     string
	port     string
	remote   string
}

type state struct {
	task       *task.Task
	endpoints  []endpoint
	active     int
	outputPath string
	cancel     context.CancelFunc
	running    bool
	paused     bool
	removed    bool
	fileAlloc  common.FileAllocationMode
	limiter    *common.ByteLimiter
	lastTick   time.Time
	lastBytes  int64
}

// Options 配置 SFTP 驱动。
type Options struct {
	MaxOverallDownloadLimit int64
}

// Driver 实现 aria2 兼容的 SFTP 下载。
type Driver struct {
	mu      sync.RWMutex
	tasks   map[string]*state
	limiter *common.ByteLimiter
}

// New 创建 SFTP 驱动。
func New(opts Options) *Driver {
	return &Driver{
		tasks:   make(map[string]*state),
		limiter: common.NewByteLimiter(opts.MaxOverallDownloadLimit),
	}
}

func (d *Driver) Name() string { return "sftp" }

func (d *Driver) CanHandle(input task.AddTaskInput) bool {
	for _, uri := range append([]string{input.URI}, input.URIs...) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "sftp://") {
			return true
		}
	}
	return false
}

func (d *Driver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx
	eps, err := collectEndpoints(input)
	if err != nil {
		return nil, err
	}
	name := deriveName(eps[0], input.Name)
	name = common.ResolveIndexOutName(input.Options, 1, name)
	outputPath := filepath.Join(input.SaveDir, name)

	item := &task.Task{
		ID:       newID(),
		Protocol: task.Protocol("sftp"),
		Name:     name,
		Status:   task.StatusWaiting,
		SaveDir:  input.SaveDir,
		Files: []task.File{{
			Index:    1,
			Path:     outputPath,
			Selected: true,
			URIs:     endpointURLs(eps),
		}},
		Options: cloneMap(input.Options),
		Meta: map[string]string{
			"sftp.outputPath": outputPath,
		},
	}

	d.mu.Lock()
	d.tasks[item.ID] = &state{
		task:       item.Clone(),
		endpoints:  eps,
		outputPath: outputPath,
		fileAlloc:  common.ParseFileAllocation(input.Options),
		limiter:    common.NewTaskDownloadLimiter(input.Options, d.limiter),
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
	st.task.UpdatedAt = time.Now()
	d.mu.Unlock()
	go d.download(runCtx, taskID)
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
	st.paused = true
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.running = false
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
		applySFTPIndexOut(st)
	}
	if _, ok := opts["max-download-limit"]; ok {
		st.limiter = common.NewTaskDownloadLimiter(st.task.Options, d.limiter)
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
		if saved == nil || saved.Protocol != task.Protocol("sftp") {
			continue
		}
		eps := endpointsFromFiles(saved.Files)
		if len(eps) == 0 {
			continue
		}
		outputPath := saved.Meta["sftp.outputPath"]
		if outputPath == "" && len(saved.Files) > 0 {
			outputPath = saved.Files[0].Path
		}
		d.mu.Lock()
		d.tasks[saved.ID] = &state{
			task:       saved.Clone(),
			endpoints:  eps,
			outputPath: outputPath,
			paused:     saved.Status == task.StatusPaused,
			running:    saved.Status == task.StatusActive,
			fileAlloc:  common.ParseFileAllocation(saved.Options),
			limiter:    common.NewTaskDownloadLimiter(saved.Options, nil),
		}
		d.mu.Unlock()
		if saved.Status == task.StatusActive {
			_ = d.Start(context.Background(), saved.ID)
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

	localSize := int64(0)
	if info, err := os.Stat(st.outputPath); err == nil {
		localSize = info.Size()
	}
	if common.ShouldRejectExistingFile(st.task.Options, localSize) {
		d.fail(taskID, fmt.Errorf("target file already exists and allow-overwrite=false: %s", st.outputPath))
		return
	}
	if common.ShouldResetExistingFile(st.task.Options, localSize) {
		_ = os.Remove(st.outputPath)
	}

	start := st.active
	if start < 0 || start >= len(st.endpoints) {
		start = 0
	}
	var lastErr error
	for i := start; i < len(st.endpoints); i++ {
		d.mu.Lock()
		if cur := d.tasks[taskID]; cur != nil {
			cur.active = i
		}
		d.mu.Unlock()
		if err := d.downloadEndpoint(ctx, taskID, st, st.endpoints[i]); err == nil {
			d.complete(taskID)
			return
		} else {
			lastErr = err
			if ctx.Err() != nil {
				d.pauseAfterCancel(taskID)
				return
			}
			if i+1 < len(st.endpoints) {
				if waitErr := common.SleepBetweenMirrors(ctx, st.task.Options); waitErr != nil {
					d.pauseAfterCancel(taskID)
					return
				}
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all sftp mirrors failed")
	}
	d.fail(taskID, lastErr)
}

func (d *Driver) downloadEndpoint(ctx context.Context, taskID string, st *state, ep endpoint) error {
	user := ep.user
	if user == "" {
		user = "root"
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(ep.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // aria2 兼容：默认不校验主机密钥
		Timeout:         sftpConnectTimeout(st.task.Options),
	}
	addr := net.JoinHostPort(ep.host, ep.port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(ep.remote)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	info, err := remoteFile.Stat()
	if err != nil {
		return err
	}
	total := info.Size()
	localSize := int64(0)
	if localInfo, statErr := os.Stat(st.outputPath); statErr == nil {
		localSize = localInfo.Size()
	}
	resumePartial := common.ShouldResumePartial(st.task.Options, localSize, total)
	file, offset, err := common.PrepareDownloadFile(st.outputPath, st.fileAlloc, localSize, total, resumePartial)
	if err != nil {
		return err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := remoteFile.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}

	buf := make([]byte, 32*1024)
	written := offset
	d.mu.Lock()
	if cur := d.tasks[taskID]; cur != nil {
		cur.lastTick = time.Now()
		cur.lastBytes = written
	}
	d.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, readErr := remoteFile.Read(buf)
		if n > 0 {
			if st.limiter != nil {
				if err := st.limiter.Wait(ctx, int64(n)); err != nil {
					return err
				}
			}
			if _, err := file.WriteAt(buf[:n], written); err != nil {
				return err
			}
			written += int64(n)
			d.advance(taskID, written, total)
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

func (d *Driver) advance(taskID string, completed, total int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		return
	}
	common.ApplyTransferProgress(st.task, completed, total, &st.lastBytes, &st.lastTick)
}

func (d *Driver) complete(taskID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil {
		return
	}
	st.task.Status = task.StatusComplete
	st.task.DownloadSpeed = 0
	st.task.Connections = 0
	st.task.UpdatedAt = time.Now()
}

func (d *Driver) fail(taskID string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.tasks[taskID]
	if st == nil {
		return
	}
	st.task.Status = task.StatusError
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
	st.task.Status = task.StatusPaused
	st.task.DownloadSpeed = 0
	st.task.Connections = 0
	st.task.UpdatedAt = time.Now()
}

func collectEndpoints(input task.AddTaskInput) ([]endpoint, error) {
	seen := map[string]struct{}{}
	out := make([]endpoint, 0)
	for _, raw := range append([]string{input.URI}, input.URIs...) {
		ep, err := parseEndpoint(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[ep.rawURL]; ok {
			continue
		}
		seen[ep.rawURL] = struct{}{}
		out = append(out, ep)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("missing sftp URL")
	}
	return out, nil
}

func parseEndpoint(raw string) (endpoint, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return endpoint{}, err
	}
	if !strings.EqualFold(parsed.Scheme, "sftp") {
		return endpoint{}, fmt.Errorf("not sftp")
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = "22"
	}
	remote := parsed.Path
	if remote == "" {
		remote = "."
	}
	user := parsed.User.Username()
	password, _ := parsed.User.Password()
	return endpoint{
		rawURL:   raw,
		user:     user,
		password: password,
		host:     host,
		port:     port,
		remote:   remote,
	}, nil
}

func deriveName(ep endpoint, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	base := filepath.Base(ep.remote)
	if base != "" && base != "." && base != "/" {
		return base
	}
	return "download"
}

func endpointURLs(eps []endpoint) []string {
	out := make([]string, 0, len(eps))
	for _, ep := range eps {
		out = append(out, ep.rawURL)
	}
	return out
}

func endpointsFromFiles(files []task.File) []endpoint {
	if len(files) == 0 {
		return nil
	}
	out := make([]endpoint, 0, len(files[0].URIs))
	for _, raw := range files[0].URIs {
		if ep, err := parseEndpoint(raw); err == nil {
			out = append(out, ep)
		}
	}
	return out
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
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(raw)
}

// GetServers 供 aria2.getServers 使用。
func (d *Driver) GetServers(ctx context.Context, taskID string) ([]manager.FileServerInfo, error) {
	_ = ctx
	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}
	entries := make([]manager.ServerEntry, 0, len(st.endpoints))
	for i, ep := range st.endpoints {
		speed := int64(0)
		if i == st.active {
			speed = st.task.DownloadSpeed
		}
		entries = append(entries, manager.ServerEntry{
			URI:           ep.rawURL,
			CurrentURI:    ep.rawURL,
			DownloadSpeed: speed,
		})
	}
	return []manager.FileServerInfo{{Index: 1, Servers: entries}}, nil
}

func sftpConnectTimeout(opts map[string]string) time.Duration {
	if d := common.ParseTimeoutSeconds(opts, "connect-timeout"); d > 0 {
		return d
	}
	return 30 * time.Second
}

func applySFTPIndexOut(st *state) {
	if st == nil || st.task == nil {
		return
	}
	name := common.ResolveIndexOutName(st.task.Options, 1, st.task.Name)
	newPath := filepath.Join(st.task.SaveDir, name)
	st.outputPath = newPath
	st.task.Name = name
	if len(st.task.Files) > 0 {
		st.task.Files[0].Path = newPath
	}
	if st.task.Meta == nil {
		st.task.Meta = map[string]string{}
	}
	st.task.Meta["sftp.outputPath"] = newPath
}
