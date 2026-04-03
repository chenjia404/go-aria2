package bt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring"
	torrentlib "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// Options 控制 BT 驱动底层 anacrolix/torrent Client 的初始化�?
type Options struct {
	DataDir    string
	ListenPort int
	EnableDHT  bool
	MaxPeers   int
}

type state struct {
	torrent        *torrentlib.Torrent
	source         addSource
	started        bool
	paused         bool
	removed        bool
	lastReadBytes  int64
	lastWriteBytes int64
	lastSampleAt   time.Time
}

// Driver 使用 anacrolix/torrent 作为 BT 协议实现�?
type Driver struct {
	mu     sync.RWMutex
	client *torrentlib.Client
	tasks  map[string]*state
}

// New 创建 BT 驱动�?
func New(opts Options) (*Driver, error) {
	cfg := torrentlib.NewDefaultClientConfig()
	cfg.DataDir = opts.DataDir
	cfg.ListenPort = opts.ListenPort
	cfg.NoDHT = !opts.EnableDHT
	if opts.MaxPeers > 0 {
		cfg.EstablishedConnsPerTorrent = opts.MaxPeers
		cfg.TorrentPeersHighWater = opts.MaxPeers * 4
		cfg.TorrentPeersLowWater = max(20, opts.MaxPeers/2)
	}

	client, err := torrentlib.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Driver{
		client: client,
		tasks:  make(map[string]*state),
	}, nil
}

// Close 关闭底层 torrent client�?
func (d *Driver) Close() error {
	if d == nil || d.client == nil {
		return nil
	}
	errs := d.client.Close()
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("close bt client: %v", errs[0])
}

// Name 返回驱动名�?
func (d *Driver) Name() string {
	return "bt"
}

// CanHandle 识别 magnet�?torrent URL 和原�?torrent 数据�?
func (d *Driver) CanHandle(input task.AddTaskInput) bool {
	if len(input.Torrent) > 0 {
		return true
	}
	for _, uri := range append([]string{input.URI}, input.URIs...) {
		normalized := strings.ToLower(strings.TrimSpace(uri))
		if strings.HasPrefix(normalized, "magnet:") {
			return true
		}
		if (strings.HasPrefix(normalized, "http://") || strings.HasPrefix(normalized, "https://")) && strings.HasSuffix(normalized, ".torrent") {
			return true
		}
	}
	return false
}

// Add 添加 torrent �?anacrolix client，但默认保持等待状态，�?manager 决定何时 Start�?
func (d *Driver) Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error) {
	result, err := parseAddInput(ctx, input)
	if err != nil {
		return nil, err
	}

	result.Spec.AddTorrentOpts.Storage = storage.NewFile(input.SaveDir)
	result.Spec.AddTorrentOpts.DisallowDataDownload = true
	result.Spec.AddTorrentOpts.DisallowDataUpload = true

	tor, _, err := d.client.AddTorrentSpec(result.Spec)
	if err != nil {
		return nil, err
	}
	if input.Name != "" {
		tor.SetDisplayName(input.Name)
	}

	item := &task.Task{
		ID:       newID(),
		Protocol: task.ProtocolBT,
		Name:     chooseName(input.Name, result.Source.DisplayName, tor.Name()),
		Status:   task.StatusWaiting,
		SaveDir:  input.SaveDir,
		Files:    placeholderFiles(result.Source, input.SaveDir, chooseName(input.Name, result.Source.DisplayName, tor.Name())),
		Options:  cloneMap(input.Options),
		Meta:     buildSourceMeta(input.Meta, result.Source),
	}

	d.mu.Lock()
	d.tasks[item.ID] = &state{
		torrent: tor,
		source:  result.Source,
		paused:  false,
		started: false,
	}
	d.mu.Unlock()

	if strings.EqualFold(input.Meta["aria2.import"], "true") {
		mode := strings.ToLower(strings.TrimSpace(input.Options["bt.resume.mode"]))
		if mode != "strict" {
			FastResume(item)
		}
		VerifyInBackground(item)
		go scheduleImportedProgress(item, tor, mode == "strict")
	}
	return d.snapshot(task.StatusWaiting, item.ID)
}

// Start 开始下载或恢复下载�?
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
	state.torrent.AllowDataUpload()
	state.torrent.AllowDataDownload()
	go startTorrentDownload(state.torrent)
	return nil
}

// Pause 暂停下载与上传�?
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
	state.torrent.DisallowDataDownload()
	state.torrent.DisallowDataUpload()
	return nil
}

// Remove �?client 中移�?torrent�?
func (d *Driver) Remove(ctx context.Context, taskID string, force bool) error {
	_ = ctx
	item, _ := d.snapshot("", taskID)

	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.tasks[taskID]
	if state == nil {
		return manager.ErrTaskNotFound
	}
	if !state.removed {
		state.torrent.Drop()
		state.removed = true
		state.started = false
		state.paused = false
	}

	if force && item != nil {
		removePaths(item.Files)
	}
	return nil
}

// TellStatus 返回基于 anacrolix/torrent 实时状态构造的统一任务模型�?
func (d *Driver) TellStatus(ctx context.Context, taskID string) (*task.Task, error) {
	_ = ctx
	return d.snapshot("", taskID)
}

// GetFiles 返回真实 torrent 文件列表�?
func (d *Driver) GetFiles(ctx context.Context, taskID string) ([]task.File, error) {
	_ = ctx
	item, err := d.snapshot("", taskID)
	if err != nil {
		return nil, err
	}
	return task.CloneFiles(item.Files), nil
}

// GetPeers 返回当前 torrent �?peer 视图，供 aria2.getPeers 使用�?
func (d *Driver) GetPeers(ctx context.Context, taskID string) ([]manager.PeerInfo, error) {
	_ = ctx

	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}

	peerConns := st.torrent.PeerConns()
	totalPieces := 0
	if info := st.torrent.Info(); info != nil {
		totalPieces = info.NumPieces()
	}

	out := make([]manager.PeerInfo, 0, len(peerConns))
	for _, pc := range peerConns {
		if pc == nil {
			continue
		}
		ip, port := splitRemoteAddr(pc.RemoteAddr.String())
		stats := pc.Stats()
		out = append(out, manager.PeerInfo{
			PeerID:        url.QueryEscape(string(pc.PeerID[:])),
			IP:            ip,
			Port:          port,
			Bitfield:      peerBitfieldHex(pc.PeerPieces(), totalPieces),
			AmChoking:     false,
			PeerChoking:   false,
			DownloadSpeed: int64(stats.DownloadRate),
			UploadSpeed:   int64(stats.LastWriteUploadRate),
			Seeder:        totalPieces > 0 && stats.RemotePieceCount >= totalPieces,
		})
	}
	return out, nil
}

// ChangeOption 支持 pause 选项的动态切换，其余选项�?core 层保存在 Task.Options�?
func (d *Driver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	if shouldPause, ok := opts["pause"]; ok {
		if strings.EqualFold(shouldPause, "true") || strings.EqualFold(shouldPause, "yes") || shouldPause == "1" {
			return d.Pause(ctx, taskID, false)
		}
		return d.Start(ctx, taskID)
	}
	return nil
}

// LoadSessionTasks 根据统一 session 重新�?torrent 任务注入到底�?client�?
func (d *Driver) LoadSessionTasks(ctx context.Context, tasks []*task.Task) error {
	for _, saved := range tasks {
		if saved == nil || saved.Status == task.StatusRemoved {
			continue
		}

		result, err := restoreSource(saved.Meta)
		if err != nil {
			return err
		}
		result.Spec.AddTorrentOpts.Storage = storage.NewFile(saved.SaveDir)
		result.Spec.AddTorrentOpts.DisallowDataDownload = true
		result.Spec.AddTorrentOpts.DisallowDataUpload = true

		tor, _, err := d.client.AddTorrentSpec(result.Spec)
		if err != nil {
			return err
		}
		if saved.Name != "" {
			tor.SetDisplayName(saved.Name)
		}

		st := &state{
			torrent: tor,
			source:  result.Source,
			started: saved.Status == task.StatusActive,
			paused:  saved.Status == task.StatusPaused,
		}

		if st.started {
			tor.AllowDataUpload()
			tor.AllowDataDownload()
			go startTorrentDownload(tor)
		}
		if strings.EqualFold(saved.Meta["aria2.import"], "true") {
			mode := strings.ToLower(strings.TrimSpace(saved.Options["bt.resume.mode"]))
			if mode != "strict" {
				FastResume(saved)
			}
			VerifyInBackground(saved)
			go scheduleImportedProgress(saved, tor, mode == "strict")
		}

		d.mu.Lock()
		d.tasks[saved.ID] = st
		d.mu.Unlock()
	}
	return nil
}

func (d *Driver) snapshot(forcedStatus task.Status, taskID string) (*task.Task, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.tasks[taskID]
	if state == nil {
		return nil, manager.ErrTaskNotFound
	}

	item := &task.Task{
		ID:       taskID,
		Protocol: task.ProtocolBT,
		Name:     state.torrent.Name(),
		SaveDir:  "",
		Meta:     buildSourceMeta(nil, state.source),
	}

	if state.removed {
		item.Status = task.StatusRemoved
		item.Name = chooseName("", state.source.DisplayName, item.Name)
		item.Files = placeholderFiles(state.source, "", item.Name)
		return item, nil
	}

	if state.torrent.Info() != nil {
		item.TotalLength = state.torrent.Length()
		item.PieceLength = int64(state.torrent.Info().PieceLength)
		item.Files = torrentFiles(state.torrent)
		item.Name = chooseName("", state.source.DisplayName, state.torrent.Name())
		item.InfoHash = state.torrent.InfoHash().HexString()
		item.Meta = enrichMetaFromTorrent(item.Meta, state.torrent)
	} else {
		item.Name = chooseName("", state.source.DisplayName, state.torrent.Name())
		item.TotalLength = state.source.TotalLength
		item.Files = placeholderFiles(state.source, "", item.Name)
	}

	item.CompletedLength = state.torrent.BytesCompleted()
	item.VerifiedLength = item.CompletedLength
	item.Seeder = state.torrent.Seeding()

	stats := state.torrent.Stats()
	item.Connections = stats.ActivePeers
	item.NumSeeders = stats.ConnectedSeeders

	readBytes := stats.ConnStats.BytesReadUsefulData.Int64()
	writeBytes := stats.ConnStats.BytesWrittenData.Int64()
	now := time.Now()
	if !state.lastSampleAt.IsZero() {
		elapsed := now.Sub(state.lastSampleAt).Seconds()
		if elapsed > 0 {
			item.DownloadSpeed = int64(float64(readBytes-state.lastReadBytes) / elapsed)
			item.UploadSpeed = int64(float64(writeBytes-state.lastWriteBytes) / elapsed)
			if item.DownloadSpeed < 0 {
				item.DownloadSpeed = 0
			}
			if item.UploadSpeed < 0 {
				item.UploadSpeed = 0
			}
		}
	}
	state.lastSampleAt = now
	state.lastReadBytes = readBytes
	state.lastWriteBytes = writeBytes
	item.UploadedLength = writeBytes

	switch {
	case forcedStatus != "":
		item.Status = forcedStatus
	case state.paused:
		item.Status = task.StatusPaused
	case !state.started:
		item.Status = task.StatusWaiting
	case state.torrent.Complete().Bool():
		item.Status = task.StatusComplete
	default:
		item.Status = task.StatusActive
	}
	return item, nil
}

func torrentFiles(tor *torrentlib.Torrent) []task.File {
	files := tor.Files()
	if len(files) == 0 {
		return []task.File{{
			Index:           0,
			Path:            tor.Name(),
			Length:          tor.Length(),
			CompletedLength: tor.BytesCompleted(),
			Selected:        true,
		}}
	}

	out := make([]task.File, 0, len(files))
	for index, file := range files {
		out = append(out, task.File{
			Index:           index,
			Path:            filepath.Clean(file.Path()),
			Length:          file.Length(),
			CompletedLength: file.BytesCompleted(),
			Selected:        true,
		})
	}
	return out
}

func buildSourceMeta(base map[string]string, source addSource) map[string]string {
	out := cloneMap(base)
	out["bt.source.kind"] = source.Kind
	out["bt.source.uri"] = source.URI
	out["bt.source.torrentBase64"] = source.TorrentBase64
	out["bt.trackers"] = strings.Join(source.Trackers, "\n")
	return out
}

func enrichMetaFromTorrent(meta map[string]string, tor *torrentlib.Torrent) map[string]string {
	out := cloneMap(meta)
	mi := tor.Metainfo()
	out["bt.mode"] = "single"
	if info, err := mi.UnmarshalInfo(); err == nil {
		if len(info.UpvertedFiles()) > 1 {
			out["bt.mode"] = "multi"
		}
	}
	out["bt.trackers"] = strings.Join(flattenTrackers(mi.UpvertedAnnounceList()), "\n")
	out["bt.comment"] = mi.Comment
	out["bt.createdBy"] = mi.CreatedBy
	if mi.CreationDate != 0 {
		out["bt.creationDate"] = strconv.FormatInt(mi.CreationDate, 10)
	}
	return out
}

func startTorrentDownload(tor *torrentlib.Torrent) {
	select {
	case <-tor.GotInfo():
		tor.DownloadAll()
	default:
		go func() {
			<-tor.GotInfo()
			tor.DownloadAll()
		}()
	}
}

func chooseName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "bt-task"
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func removePaths(files []task.File) {
	for _, file := range files {
		if file.Path == "" {
			continue
		}
		_ = os.Remove(file.Path)
	}
}

func scheduleImportedProgress(item *task.Task, tor *torrentlib.Torrent, strict bool) {
	if item == nil || tor == nil {
		return
	}
	_ = strict
	if tor.Info() != nil {
		if err := RebuildBTProgress(item, tor); err != nil {
			log.Printf("[WARN] rebuild BT progress failed: %v", err)
		}
		return
	}
	<-tor.GotInfo()
	if err := RebuildBTProgress(item, tor); err != nil {
		log.Printf("[WARN] rebuild BT progress failed: %v", err)
	}
}

// splitRemoteAddr 尝试�?torrent peer 的远端地址中拆�?IP 和端口�?
func splitRemoteAddr(raw string) (string, int) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		return raw, 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return host, 0
	}
	return host, port
}

// peerBitfieldHex �?aria2 的约定把 piece 集合编码�?bitfield 十六进制字符串�?
func peerBitfieldHex(bits *roaring.Bitmap, totalPieces int) string {
	if bits == nil {
		return ""
	}
	if totalPieces <= 0 {
		maxPiece := -1
		bits.Iterate(func(piece uint32) bool {
			if int(piece) > maxPiece {
				maxPiece = int(piece)
			}
			return true
		})
		if maxPiece < 0 {
			return ""
		}
		totalPieces = maxPiece + 1
	}

	raw := make([]byte, (totalPieces+7)/8)
	bits.Iterate(func(piece uint32) bool {
		index := int(piece)
		if index < 0 || index >= totalPieces {
			return true
		}
		raw[index/8] |= 1 << (7 - uint(index%8))
		return true
	})
	return hex.EncodeToString(raw)
}
