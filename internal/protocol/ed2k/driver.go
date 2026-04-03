package ed2k

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	goed2k "github.com/monkeyWie/goed2k"
	"github.com/monkeyWie/goed2k/protocol"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// Options 控制 ED2K 驱动底层 goed2k Client 的初始化�?
type Options struct {
	ListenPort   int
	UDPPort      int
	EnableDHT    bool
	EnableServer bool
	UploadSlots  int
	MaxSources   int
	StatePath    string
}

type state struct {
	hash    protocol.Hash
	started bool
	paused  bool
	removed bool
	uri     string
}

// Driver 使用 goed2k 作为 ED2K 协议实现�?
type Driver struct {
	mu        sync.RWMutex
	client    *goed2k.Client
	tasks     map[string]*state
	statePath string
}

// New 创建 ED2K 驱动�?
func New(opts Options) (*Driver, error) {
	settings := goed2k.NewSettings()
	if opts.ListenPort > 0 {
		settings.ListenPort = opts.ListenPort
	}
	if opts.UDPPort > 0 {
		settings.UDPPort = opts.UDPPort
	}
	settings.EnableDHT = opts.EnableDHT
	settings.ReconnectToServer = opts.EnableServer
	if opts.UploadSlots > 0 {
		settings.UploadSlots = opts.UploadSlots
	}
	if opts.MaxSources > 0 {
		settings.MaxPeerListSize = opts.MaxSources
		settings.SessionConnectionsLimit = opts.MaxSources
	}

	client := goed2k.NewClient(settings)
	if opts.StatePath != "" {
		client.SetStatePath(opts.StatePath)
	}
	if err := client.Start(); err != nil {
		return nil, err
	}

	return &Driver{
		client:    client,
		tasks:     make(map[string]*state),
		statePath: opts.StatePath,
	}, nil
}

// Close 关闭底层 goed2k client�?
func (d *Driver) Close() error {
	if d == nil || d.client == nil {
		return nil
	}
	d.client.Close()
	return nil
}

// Name 返回驱动名�?
func (d *Driver) Name() string {
	return "ed2k"
}

// CanHandle 识别 ed2k:// 链接�?
func (d *Driver) CanHandle(input task.AddTaskInput) bool {
	for _, uri := range append([]string{input.URI}, input.URIs...) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "ed2k://") {
			return true
		}
	}
	return false
}

// Add 使用 goed2k 添加任务，并先置�?waiting 状态，�?manager 决定何时 Start�?
func (d *Driver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	_ = ctx

	link, err := firstLink(input)
	if err != nil {
		return nil, err
	}
	handle, targetPath, err := d.client.AddLink(link.SourceURI, input.SaveDir)
	if err != nil {
		return nil, err
	}
	if handle.IsValid() {
		if err := d.client.PauseTransfer(handle.GetHash()); err != nil {
			return nil, err
		}
	}

	name := link.Name
	if input.Name != "" {
		name = input.Name
	}
	item := &task.Task{
		ID:             newID(),
		Protocol:       task.ProtocolED2K,
		Name:           name,
		Status:         task.StatusWaiting,
		SaveDir:        input.SaveDir,
		TotalLength:    link.Size,
		VerifiedLength: 0,
		Files: []task.File{{
			Index:           0,
			Path:            targetPath,
			Length:          link.Size,
			CompletedLength: 0,
			Selected:        true,
			URIs:            []string{link.SourceURI},
		}},
		Options: cloneMap(input.Options),
		Meta:    cloneED2KMeta(input.Meta, link),
	}

	d.mu.Lock()
	d.tasks[item.ID] = &state{
		hash:    handle.GetHash(),
		started: false,
		paused:  false,
		removed: false,
		uri:     link.SourceURI,
	}
	d.mu.Unlock()
	return d.snapshot(task.StatusWaiting, item.ID)
}

// Start 恢复 ED2K 传输�?
func (d *Driver) Start(ctx context.Context, taskID string) error {
	_ = ctx
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.tasks[taskID]
	if state == nil || state.removed {
		return manager.ErrTaskNotFound
	}
	state.started = true
	state.paused = false
	return d.client.ResumeTransfer(state.hash)
}

// Pause 暂停 ED2K 传输�?
func (d *Driver) Pause(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	_ = force
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.tasks[taskID]
	if state == nil || state.removed {
		return manager.ErrTaskNotFound
	}
	state.paused = true
	return d.client.PauseTransfer(state.hash)
}

// Remove 删除 ED2K 传输�?
func (d *Driver) Remove(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	item, _ := d.snapshot("", taskID)
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.tasks[taskID]
	if state == nil {
		return manager.ErrTaskNotFound
	}
	if state.removed {
		return nil
	}
	state.removed = true
	state.started = false
	state.paused = false
	err := d.client.RemoveTransfer(state.hash, force)
	if force && item != nil {
		removePaths(item.Files)
	}
	return err
}

// TellStatus 返回基于 goed2k 快照映射的统一任务状态�?
func (d *Driver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	return d.snapshot("", taskID)
}

// GetFiles 返回 ED2K 文件列表�?
func (d *Driver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	_ = ctx
	item, err := d.snapshot("", taskID)
	if err != nil {
		return nil, err
	}
	return task.CloneFiles(item.Files), nil
}

// GetPeers 返回 ED2K 当前连接到该任务�?peer 视图�?
func (d *Driver) GetPeers(ctx context.Context, taskID string) ([]manager.PeerInfo, error) {
	_ = ctx

	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}

	statuses := d.client.PeerStatuses()
	out := make([]manager.PeerInfo, 0)
	for _, peer := range statuses {
		if peer.TransferHash != st.hash {
			continue
		}
		out = append(out, manager.PeerInfo{
			IP:            peer.Peer.Endpoint.String(),
			Port:          peer.Peer.Endpoint.Port(),
			Bitfield:      bitfieldHex(peer.Peer.RemotePieces),
			AmChoking:     false,
			PeerChoking:   false,
			DownloadSpeed: int64(peer.Peer.DownloadSpeed),
			UploadSpeed:   int64(peer.Peer.UploadSpeed),
		})
	}
	return out, nil
}

// ChangeOption 支持 pause 选项的动态切换�?
func (d *Driver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	if shouldPause, ok := opts["pause"]; ok {
		if strings.EqualFold(shouldPause, "true") || strings.EqualFold(shouldPause, "yes") || shouldPause == "1" {
			return d.Pause(ctx, taskID, false)
		}
		return d.Start(ctx, taskID)
	}
	return nil
}

// LoadSessionTasks 将统一 session �?goed2k 的状态恢复能力重新对齐�?
func (d *Driver) LoadSessionTasks(ctx context.Context, tasks []*task.Task, globalOptions map[string]string) error {
	_ = ctx
	_ = globalOptions
	if d.statePath != "" {
		if err := d.client.LoadState(""); err != nil {
			return err
		}
	}

	snapshots := d.client.TransferSnapshots()
	snapshotByHash := make(map[string]goed2k.TransferSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotByHash[snapshot.Hash.String()] = snapshot
	}

	for _, saved := range tasks {
		if saved == nil || saved.Status == task.StatusRemoved {
			continue
		}

		hashText := saved.Meta["ed2k.hash"]
		if hashText == "" {
			continue
		}

		hash, err := protocol.HashFromString(hashText)
		if err != nil {
			return err
		}
		if _, ok := snapshotByHash[hashText]; !ok {
			if saved.Meta["ed2k.sourceURI"] == "" {
				continue
			}
			handle, _, err := d.client.AddLink(saved.Meta["ed2k.sourceURI"], saved.SaveDir)
			if err != nil {
				return err
			}
			hash = handle.GetHash()
			if err := d.client.PauseTransfer(hash); err != nil {
				return err
			}
		}

		st := &state{
			hash:    hash,
			started: saved.Status == task.StatusActive,
			paused:  saved.Status == task.StatusPaused,
			uri:     saved.Meta["ed2k.sourceURI"],
		}
		d.mu.Lock()
		d.tasks[saved.ID] = st
		d.mu.Unlock()

		if st.started {
			if err := d.client.ResumeTransfer(hash); err != nil {
				return err
			}
		} else {
			if err := d.client.PauseTransfer(hash); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Driver) snapshot(forcedStatus task.Status, taskID string) (*task.Task, error) {
	d.mu.RLock()
	state := d.tasks[taskID]
	d.mu.RUnlock()
	if state == nil {
		return nil, manager.ErrTaskNotFound
	}

	item := &task.Task{
		ID:       taskID,
		Protocol: task.ProtocolED2K,
	}

	if state.removed {
		item.Status = task.StatusRemoved
		item.Meta = map[string]string{"ed2k.sourceURI": state.uri}
		return item, nil
	}

	handle := d.client.FindTransfer(state.hash)
	if !handle.IsValid() {
		return nil, manager.ErrTaskNotFound
	}

	snapshot := buildSnapshot(handle)
	item.Name = snapshot.FileName
	item.SaveDir = filepath.Dir(snapshot.FilePath)
	item.TotalLength = snapshot.Size
	item.CompletedLength = snapshot.Status.TotalDone
	item.VerifiedLength = snapshot.Status.TotalDone
	item.UploadedLength = snapshot.Status.Upload
	item.DownloadSpeed = int64(snapshot.Status.DownloadRate)
	item.UploadSpeed = int64(snapshot.Status.UploadRate)
	item.Connections = snapshot.ActivePeers
	item.Files = []task.File{{
		Index:           0,
		Path:            snapshot.FilePath,
		Length:          snapshot.Size,
		CompletedLength: snapshot.Status.TotalDone,
		Selected:        true,
		URIs:            []string{state.uri},
	}}
	item.Meta = map[string]string{
		"ed2k.hash":      snapshot.Hash.String(),
		"ed2k.sourceURI": state.uri,
	}

	switch {
	case forcedStatus != "":
		item.Status = forcedStatus
	case snapshot.Status.State == goed2k.Finished:
		item.Status = task.StatusComplete
	case state.paused:
		item.Status = task.StatusPaused
	case !state.started:
		item.Status = task.StatusWaiting
	default:
		item.Status = task.StatusActive
	}
	return item, nil
}

func buildSnapshot(handle goed2k.TransferHandle) goed2k.TransferSnapshot {
	status := handle.GetStatus()
	filePath := handle.GetFilePath()
	fileName := filepath.Base(filePath)
	if fileName == "." || fileName == "" {
		fileName = handle.GetHash().String()
	}
	return goed2k.TransferSnapshot{
		Hash:        handle.GetHash(),
		FileName:    fileName,
		FilePath:    filePath,
		CreateTime:  handle.GetCreateTime(),
		Size:        handle.GetSize(),
		ActivePeers: handle.ActiveConnections(),
		Status:      status,
		Peers:       handle.GetPeersInfo(),
		Pieces:      handle.PieceSnapshots(),
	}
}

func newID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(raw)
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

func removePaths(files []task.File) {
	for _, file := range files {
		if file.Path == "" {
			continue
		}
		_ = os.Remove(file.Path)
	}
}

// bitfieldHex �?ED2K �?BitField 编码�?aria2 风格的十六进制字符串�?
func bitfieldHex(bits protocol.BitField) string {
	if bits.Len() == 0 {
		return ""
	}
	raw := make([]byte, (bits.Len()+7)/8)
	for i := 0; i < bits.Len(); i++ {
		if bits.GetBit(i) {
			raw[i/8] |= 1 << (7 - uint(i%8))
		}
	}
	return hex.EncodeToString(raw)
}
