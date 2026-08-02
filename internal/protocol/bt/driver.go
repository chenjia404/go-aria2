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
	"golang.org/x/time/rate"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

// Options 控制 BT 驱动底层 anacrolix/torrent Client 的初始化�?
type Options struct {
	DataDir               string
	ListenPort            int
	EnableDHT             bool
	MaxPeers              int
	Crypto                CryptoOptions
	DHTFilePath           string
	DHTFilePath6          string
	DHTListenPort         int
	EnableDHT6            bool
	MaxOverallUploadLimit   int64
	MaxOverallDownloadLimit int64
	EnableLPD               bool
	CheckIntegrity          bool
}

type state struct {
	torrent        *torrentlib.Torrent
	source         addSource
	saveDir        string
	webSeeds       []string
	started        bool
	paused         bool
	removed        bool
	completed      int64
	verified       int64
	lastReadBytes  int64
	lastWriteBytes int64
	lastSampleAt   time.Time
	seedStartedAt  time.Time
	seedStopped    bool
	// selectFile 为 aria2 的 select-file 原始串；空表示下载全部文件（与未指定一致）。
	selectFile string
	options    map[string]string
	// 任务级限速（aria2 max-download-limit / max-upload-limit）。
	downloadLimiter   *common.ByteLimiter
	uploadLimiter     *common.ByteLimiter
	rateLimitPausedDL bool
	rateLimitPausedUL bool
	lastRateBytesRead  int64
	lastRateBytesWrite int64
	lastRateSampleAt   time.Time
	// bt-detach-seed-only：做种任务不再写入 session。
	sessionDetached bool
	// 完成回调（删除未选文件等）仅执行一次。
	completionHandled bool
}

// Driver 使用 anacrolix/torrent 作为 BT 协议实现�?
type Driver struct {
	mu              sync.RWMutex
	client          *torrentlib.Client
	tasks           map[string]*state
	rebuildProgress func(*task.Task, *torrentlib.Torrent) error
	closeOnce       sync.Once
	closeErr        error
	opts            Options
	uploadLimiter   *rate.Limiter
	downloadLimiter *rate.Limiter
	dhtIPv4Path     string
	dhtIPv6Path     string
}

func buildTorrentConfig(opts Options, listenPort int, uploadLimiter **rate.Limiter, downloadLimiter **rate.Limiter) *torrentlib.ClientConfig {
	cfg := torrentlib.NewDefaultClientConfig()
	cfg.DataDir = opts.DataDir
	cfg.ListenPort = listenPort
	cfg.NoDHT = !opts.EnableDHT
	if !opts.EnableDHT6 {
		cfg.DisableIPv6 = true
	}
	if opts.MaxPeers > 0 {
		cfg.EstablishedConnsPerTorrent = opts.MaxPeers
		cfg.TorrentPeersHighWater = opts.MaxPeers * 4
		cfg.TorrentPeersLowWater = max(20, opts.MaxPeers/2)
	}
	if *uploadLimiter == nil {
		if opts.MaxOverallUploadLimit > 0 {
			burst := max(16*1024, int(opts.MaxOverallUploadLimit))
			*uploadLimiter = rate.NewLimiter(rate.Limit(opts.MaxOverallUploadLimit), burst)
		} else {
			*uploadLimiter = rate.NewLimiter(rate.Inf, 64*1024)
		}
	}
	cfg.UploadRateLimiter = *uploadLimiter
	if *downloadLimiter == nil {
		if opts.MaxOverallDownloadLimit > 0 {
			burst := max(16*1024, int(opts.MaxOverallDownloadLimit))
			*downloadLimiter = rate.NewLimiter(rate.Limit(opts.MaxOverallDownloadLimit), burst)
		} else {
			*downloadLimiter = rate.NewLimiter(rate.Inf, 1<<20)
		}
	}
	cfg.DownloadRateLimiter = *downloadLimiter
	applyBTCryptoOptions(cfg, opts.Crypto)
	applySeparateDHTConfig(cfg, opts)
	return cfg
}

func effectiveListenPort(opts Options) int {
	if opts.ListenPort > 0 {
		return opts.ListenPort
	}
	if opts.DHTListenPort > 0 {
		return opts.DHTListenPort
	}
	return 0
}

func newTorrentClient(opts Options, uploadLimiter **rate.Limiter, downloadLimiter **rate.Limiter) (*torrentlib.Client, error) {
	listenPort := effectiveListenPort(opts)
	client, err := torrentlib.NewClient(buildTorrentConfig(opts, listenPort, uploadLimiter, downloadLimiter))
	if err == nil {
		return client, nil
	}
	// 固定端口：多协议绑定失败时换动态端口
	if opts.ListenPort != 0 {
		log.Printf("[bt] listen on port %d failed: %v; retrying with listen-port=0", opts.ListenPort, err)
		client, err = torrentlib.NewClient(buildTorrentConfig(opts, 0, uploadLimiter, downloadLimiter))
		if err == nil {
			return client, nil
		}
	}
	// Windows 常见：udp4 报 WSAEACCES（权限/防火墙/Hyper-V 保留端口等），或 IPv6 UDP 异常
	log.Printf("[bt] listen failed: %v; retrying with DisableIPv6", err)
	cfg := buildTorrentConfig(opts, 0, uploadLimiter, downloadLimiter)
	cfg.DisableIPv6 = true
	client, err = torrentlib.NewClient(cfg)
	if err == nil {
		return client, nil
	}
	// 不再监听 UDP：仅 TCP BT（无 uTP、无 DHT），仍可与多数 peer 通信
	log.Printf("[bt] listen failed: %v; retrying TCP-only (DisableUTP, NoDHT)", err)
	cfg2 := buildTorrentConfig(opts, 0, uploadLimiter, downloadLimiter)
	cfg2.DisableIPv6 = true
	cfg2.DisableUTP = true
	cfg2.NoDHT = true
	client, err = torrentlib.NewClient(cfg2)
	return client, err
}

// New 创建 BT 驱动�?
func New(opts Options) (*Driver, error) {
	var uploadLimiter *rate.Limiter
	var downloadLimiter *rate.Limiter
	client, err := newTorrentClient(opts, &uploadLimiter, &downloadLimiter)
	if err != nil {
		return nil, err
	}
	dhtIPv4 := resolveDHTNodePath(opts.DHTFilePath, filepath.Join(opts.DataDir, "dht.dat"))
	dhtIPv6 := resolveDHTNodePath(opts.DHTFilePath6, filepath.Join(opts.DataDir, "dht6.dat"))
	loadDHTNodes(client, dhtIPv4, dhtIPv6)
	attachSeparateDHTServers(client, opts)
	drv := &Driver{
		client:          client,
		tasks:           make(map[string]*state),
		rebuildProgress: RebuildBTProgress,
		opts:            opts,
		uploadLimiter:   uploadLimiter,
		downloadLimiter: downloadLimiter,
		dhtIPv4Path:     dhtIPv4,
		dhtIPv6Path:     dhtIPv6,
	}
	go drv.runRateLimitLoop()
	drv.startLPDIfEnabled()
	return drv, nil
}

// SetUploadLimit 运行时调整全局限速（字节/秒，0 表示不限速）。
func (d *Driver) SetUploadLimit(bytesPerSec int64) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if bytesPerSec <= 0 {
		if d.uploadLimiter != nil {
			d.uploadLimiter.SetLimit(rate.Inf)
			d.uploadLimiter.SetBurst(64 * 1024)
		}
		return
	}
	burst := max(16*1024, int(bytesPerSec))
	if d.uploadLimiter == nil {
		d.uploadLimiter = rate.NewLimiter(rate.Limit(bytesPerSec), burst)
		return
	}
	d.uploadLimiter.SetLimit(rate.Limit(bytesPerSec))
	d.uploadLimiter.SetBurst(burst)
}

// Close 关闭底层 torrent client�?
func (d *Driver) Close() error {
	if d == nil {
		return nil
	}
	// anacrolix/torrent 关闭时会级联释放底层持久化资源，重复关闭会放大底层锁释放异常。
	d.closeOnce.Do(func() {
		d.mu.Lock()
		client := d.client
		d.client = nil
		d.mu.Unlock()

		if client == nil {
			return
		}

		saveDHTNodes(client, d.dhtIPv4Path, d.dhtIPv6Path)
		errs := client.Close()
		if len(errs) == 0 {
			return
		}
		d.closeErr = fmt.Errorf("close bt client: %v", errs[0])
	})
	return d.closeErr
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
			return common.ShouldFollowTorrentURL(input.Options)
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
	applyBTTrackerOpts(result.Spec, &result.Source, input.Options)

	result.Spec.AddTorrentOpts.Storage = torrentStorageForAdd(input.SaveDir, input.Options)
	result.Spec.AddTorrentOpts.DisallowDataDownload = true
	result.Spec.AddTorrentOpts.DisallowDataUpload = true

	tor, _, err := d.client.AddTorrentSpec(result.Spec)
	if err != nil {
		return nil, err
	}
	applyBTMaxPeers(tor, input.Options, d.opts.MaxPeers)
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

	st := &state{
		torrent:    tor,
		source:     result.Source,
		saveDir:    input.SaveDir,
		webSeeds:   append([]string(nil), result.Spec.Webseeds...),
		paused:     false,
		started:    false,
		selectFile: strings.TrimSpace(input.Options["select-file"]),
		options:    cloneMap(input.Options),
	}
	applyBTRateLimiters(st, input.Options)
	if len(item.Files) > 0 {
		item.Files[0].URIs = d.btURIsForFile(st, 1)
	}
	d.mu.Lock()
	d.tasks[item.ID] = st
	d.mu.Unlock()
	// 与 aria2 一致：元数据就绪后即按 select-file 标记文件选中状态；实际拉取仍由 DisallowDataDownload 门禁。
	go d.scheduleBTFileSelection(st)

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
	d.runCheckIntegrityIfNeeded(state)
	go d.scheduleBTFileSelection(state)
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
	if force {
		for _, pc := range state.torrent.PeerConns() {
			if pc != nil {
				pc.Close()
			}
		}
	}
	return nil
}

// Remove �?client 中移�?torrent�?
// EnforceSeedPolicy 按 aria2 seed-ratio / seed-time 停止做种（ratio < 0 表示不限制比例）。
func (d *Driver) EnforceSeedPolicy(ctx context.Context, taskID string, ratio float64, seedTime time.Duration) error {
	_ = ctx
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.tasks[taskID]
	if state == nil || state.removed || state.seedStopped {
		return nil
	}
	if state.torrent.Info() == nil || !state.torrent.Complete().Bool() {
		return nil
	}
	if state.seedStartedAt.IsZero() {
		state.seedStartedAt = time.Now()
	}

	stats := state.torrent.Stats()
	uploaded := stats.ConnStats.BytesWrittenData.Int64()
	downloaded := state.torrent.Length()
	if downloaded <= 0 {
		downloaded = state.torrent.BytesCompleted()
	}

	stop := false
	if ratio >= 0 {
		switch {
		case ratio == 0:
			stop = true
		case downloaded > 0 && float64(uploaded) >= ratio*float64(downloaded):
			stop = true
		}
	}
	if seedTime > 0 && time.Since(state.seedStartedAt) >= seedTime {
		stop = true
	}
	if !stop {
		return nil
	}
	d.stopSeedingLocked(state)
	return nil
}

func (d *Driver) stopSeedingLocked(state *state) {
	state.seedStopped = true
	state.torrent.DisallowDataUpload()
	for _, pc := range state.torrent.PeerConns() {
		if pc != nil {
			pc.Close()
		}
	}
}

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

// PurgeLocalState 在管理器摘除任务后删除内部状态。
func (d *Driver) PurgeLocalState(taskID string) {
	d.mu.Lock()
	delete(d.tasks, taskID)
	d.mu.Unlock()
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

// ChangeOption 支持 pause、select-file 等运行期切换；其余选项由 core 层保存在 Task.Options。
func (d *Driver) ChangeOption(ctx context.Context, taskID string, opts map[string]string) error {
	if shouldPause, ok := common.PauseRequestedFromChangeOption(opts); ok {
		if shouldPause {
			if err := d.Pause(ctx, taskID, false); err != nil {
				return err
			}
		} else {
			if err := d.Start(ctx, taskID); err != nil {
				return err
			}
		}
	}
	if _, ok := opts["select-file"]; ok {
		if err := d.setSelectFile(taskID, opts["select-file"]); err != nil {
			return err
		}
	}
	if value, ok := opts["bt-max-peers"]; ok {
		d.mu.Lock()
		st := d.tasks[taskID]
		if st == nil || st.removed {
			d.mu.Unlock()
			return manager.ErrTaskNotFound
		}
		if st.options == nil {
			st.options = map[string]string{}
		}
		st.options["bt-max-peers"] = value
		tor := st.torrent
		merged := cloneMap(st.options)
		d.mu.Unlock()
		applyBTMaxPeers(tor, merged, d.opts.MaxPeers)
	}
	if _, hasDL := opts["max-download-limit"]; hasDL || opts["max-upload-limit"] != "" {
		d.mu.Lock()
		st := d.tasks[taskID]
		if st == nil || st.removed {
			d.mu.Unlock()
			return manager.ErrTaskNotFound
		}
		if st.options == nil {
			st.options = map[string]string{}
		}
		if v, ok := opts["max-download-limit"]; ok {
			st.options["max-download-limit"] = v
		}
		if v, ok := opts["max-upload-limit"]; ok {
			st.options["max-upload-limit"] = v
		}
		applyBTRateLimiters(st, st.options)
		st.rateLimitPausedDL = false
		st.rateLimitPausedUL = false
		d.mu.Unlock()
	}
	return nil
}

// SyncBTTrackerOptions 运行期按当前选项重建 tracker 列表并写入 anacrolix（ModifyTrackers）。
func (d *Driver) SyncBTTrackerOptions(ctx context.Context, taskID string, opts map[string]string) error {
	_ = ctx
	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	src := st.source
	tor := st.torrent
	d.mu.Unlock()

	spec, err := torrentSpecFromSource(src)
	if err != nil {
		return err
	}
	applyBTTrackerOpts(spec, &src, opts)
	tor.ModifyTrackers(spec.Trackers)

	d.mu.Lock()
	if st2 := d.tasks[taskID]; st2 != nil && st2.torrent == tor && !st2.removed {
		st2.source = src
	}
	d.mu.Unlock()
	return nil
}

// LoadSessionTasks 根据统一 session 将 torrent 任务注入到底层 client。
func (d *Driver) LoadSessionTasks(ctx context.Context, tasks []*task.Task, globalOptions map[string]string) error {
	for _, saved := range tasks {
		if saved == nil || saved.Status == task.StatusRemoved {
			continue
		}

		result, err := restoreSource(saved.Meta)
		if err != nil {
			return err
		}
		effOpts := effectiveOptsForSessionRestore(saved, globalOptions)
		applyBTTrackerOpts(result.Spec, &result.Source, effOpts)
		result.Spec.AddTorrentOpts.Storage = torrentStorageForAdd(saved.SaveDir, effOpts)
		result.Spec.AddTorrentOpts.DisallowDataDownload = true
		result.Spec.AddTorrentOpts.DisallowDataUpload = true

		tor, _, err := d.client.AddTorrentSpec(result.Spec)
		if err != nil {
			return err
		}
		if saved.Name != "" {
			tor.SetDisplayName(saved.Name)
		}
		applyBTMaxPeers(tor, effOpts, d.opts.MaxPeers)

		resumeAfterRestore := saved.Status == task.StatusActive
		st := &state{
			torrent:    tor,
			source:     result.Source,
			saveDir:    saved.SaveDir,
			webSeeds:   webSeedsFromSession(saved),
			started:    false,
			paused:     saved.Status == task.StatusPaused,
			completed:  saved.CompletedLength,
			verified:   saved.VerifiedLength,
			selectFile: strings.TrimSpace(effOpts["select-file"]),
			options:    cloneMap(effOpts),
		}
		applyBTRateLimiters(st, effOpts)
		if strings.EqualFold(saved.Meta["bt.sessionDetached"], "true") {
			st.sessionDetached = true
		}
		if len(st.webSeeds) > 0 {
			tor.AddWebSeeds(st.webSeeds)
		}

		d.mu.Lock()
		d.tasks[saved.ID] = st
		d.mu.Unlock()

		go d.scheduleBTFileSelection(st)
		if strings.EqualFold(saved.Meta["aria2.import"], "true") {
			mode := strings.ToLower(strings.TrimSpace(effOpts["bt.resume.mode"]))
			if mode != "strict" {
				FastResume(saved)
			}
			VerifyInBackground(saved)
			go scheduleImportedProgress(saved, tor, mode == "strict")
		} else if shouldRestoreBTProgress(saved, result.Source) {
			go d.restoreProgressAfterLoad(saved.Clone(), st, resumeAfterRestore)
		} else if resumeAfterRestore {
			st.started = true
			tor.AllowDataUpload()
			tor.AllowDataDownload()
		}
	}
	return nil
}

func shouldRestoreBTProgress(saved *task.Task, source addSource) bool {
	if saved == nil {
		return false
	}
	if strings.TrimSpace(saved.SaveDir) == "" {
		return false
	}
	switch source.Kind {
	case "torrent-bytes", "torrent-url":
		return true
	default:
		return false
	}
}

func (d *Driver) restoreProgressAfterLoad(saved *task.Task, st *state, resume bool) {
	if saved == nil || st == nil || st.torrent == nil {
		return
	}
	if st.torrent.Info() == nil {
		<-st.torrent.GotInfo()
	}
	rebuild := d.rebuildProgress
	if rebuild == nil {
		rebuild = RebuildBTProgress
	}
	if err := rebuild(saved, st.torrent); err != nil {
		log.Printf("[WARN] rebuild BT progress failed: %v", err)
	}

	d.mu.Lock()
	for _, current := range d.tasks {
		if current != st {
			continue
		}
		current.completed = saved.CompletedLength
		current.verified = saved.VerifiedLength
		break
	}
	d.mu.Unlock()

	if !resume {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, current := range d.tasks {
		if current != st {
			continue
		}
		if current.removed || current.paused {
			return
		}
		current.started = true
		current.torrent.AllowDataUpload()
		current.torrent.AllowDataDownload()
		return
	}
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
		SaveDir:  state.saveDir,
		Meta:     buildSourceMeta(nil, state.source),
	}

	if state.removed {
		item.Status = task.StatusRemoved
		item.Name = chooseName("", state.source.DisplayName, item.Name)
		item.Files = placeholderFiles(state.source, "", item.Name)
		return item, nil
	}

	if state.torrent.Info() != nil {
		info := state.torrent.Info()
		item.TotalLength = state.torrent.Length()
		item.PieceLength = int64(info.PieceLength)
		item.Files = torrentFiles(state.torrent)
		applyIndexOutPaths(item.Files, state.options, state.saveDir)
		item.Name = chooseName("", state.source.DisplayName, state.torrent.Name())
		item.InfoHash = state.torrent.InfoHash().HexString()
		item.Meta = enrichMetaFromTorrent(item.Meta, state.torrent)
		totalPieces := info.NumPieces()
		if totalPieces > 0 {
			if item.Meta == nil {
				item.Meta = map[string]string{}
			}
			item.Meta["bt.totalPieces"] = strconv.Itoa(totalPieces)
			item.Meta["bitfield"] = torrentCompletedBitfieldHex(state.torrent, totalPieces)
		}
	} else {
		item.Name = chooseName("", state.source.DisplayName, state.torrent.Name())
		item.TotalLength = state.source.TotalLength
		item.Files = placeholderFiles(state.source, "", item.Name)
	}

	item.CompletedLength = state.torrent.BytesCompleted()
	item.VerifiedLength = item.CompletedLength
	if state.completed > item.CompletedLength {
		item.CompletedLength = state.completed
	}
	if state.verified > item.VerifiedLength {
		item.VerifiedLength = state.verified
	}
	item.Seeder = state.torrent.Seeding()

	stats := state.torrent.Stats()
	item.Connections = stats.ActivePeers
	item.NumSeeders = stats.ConnectedSeeders

	readBytes := stats.ConnStats.BytesReadUsefulData.Int64()
	writeBytes := stats.ConnStats.BytesWrittenData.Int64()
	now := time.Now()

	// 优先使用各 Peer 的瞬时速率之和（与 getPeers 一致），避免首次采样前 tellStatus 速度恒为 0。
	var peerDown, peerUp int64
	for _, pc := range state.torrent.PeerConns() {
		if pc == nil {
			continue
		}
		ps := pc.Stats()
		peerDown += int64(ps.DownloadRate)
		peerUp += int64(ps.LastWriteUploadRate)
	}
	if peerDown > 0 || peerUp > 0 {
		item.DownloadSpeed = peerDown
		item.UploadSpeed = peerUp
	} else if !state.lastSampleAt.IsZero() {
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
	applyCompletedToFiles(item.Files, item.CompletedLength)
	d.attachBTFileURIs(state, item.Files)

	if state.sessionDetached {
		if item.Meta == nil {
			item.Meta = map[string]string{}
		}
		item.Meta["bt.sessionDetached"] = "true"
	}
	if len(state.options) > 0 {
		if item.Options == nil {
			item.Options = map[string]string{}
		}
		for k, v := range state.options {
			item.Options[k] = v
		}
	}

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

func applyCompletedToFiles(files []task.File, total int64) {
	remaining := total
	for i := range files {
		files[i].CompletedLength = 0
		if remaining <= 0 {
			continue
		}
		if remaining >= files[i].Length {
			files[i].CompletedLength = files[i].Length
			remaining -= files[i].Length
			continue
		}
		files[i].CompletedLength = remaining
		remaining = 0
	}
}

func (d *Driver) attachBTFileURIs(st *state, files []task.File) {
	if st == nil || len(files) == 0 {
		return
	}
	uris := d.btURIsForFile(st, 1)
	if len(uris) == 0 {
		return
	}
	files[0].URIs = append([]string(nil), uris...)
}

func (d *Driver) btURIsForFile(st *state, fileIndex int) []string {
	if st == nil || fileIndex != 1 {
		return nil
	}
	uris := make([]string, 0, 1+len(st.webSeeds))
	if st.source.URI != "" {
		uris = append(uris, st.source.URI)
	}
	uris = append(uris, st.webSeeds...)
	return uris
}

func webSeedsFromSession(saved *task.Task) []string {
	if saved == nil || len(saved.Files) == 0 {
		return nil
	}
	sourceURI := ""
	if saved.Meta != nil {
		sourceURI = saved.Meta["bt.source.uri"]
	}
	out := make([]string, 0)
	for _, uri := range saved.Files[0].URIs {
		if uri == "" || uri == sourceURI {
			continue
		}
		if isWebSeedURL(uri) {
			out = append(out, uri)
		}
	}
	return dedupeStrings(out)
}

func torrentFiles(tor *torrentlib.Torrent) []task.File {
	files := tor.Files()
	if len(files) == 0 {
		return []task.File{{
			Index:           1,
			Path:            tor.Name(),
			Length:          tor.Length(),
			CompletedLength: tor.BytesCompleted(),
			Selected:        true,
		}}
	}

	out := make([]task.File, 0, len(files))
	for index, file := range files {
		out = append(out, task.File{
			Index:           index + 1,
			Path:            filepath.Clean(file.Path()),
			Length:          file.Length(),
			CompletedLength: file.BytesCompleted(),
			Selected:        file.Priority() != torrentlib.PiecePriorityNone,
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

func (d *Driver) scheduleBTFileSelection(st *state) {
	tor := st.torrent
	run := func() {
		d.mu.RLock()
		sel := st.selectFile
		opts := cloneMap(st.options)
		source := st.source
		saveDir := st.saveDir
		d.mu.RUnlock()
		if err := applySelectFileToTorrent(tor, sel); err != nil {
			log.Printf("[bt] apply select-file: %v", err)
		}
		if shouldSaveTorrentMetadata(opts, source.Kind) {
			if err := saveTorrentMetadata(tor, saveDir); err != nil {
				log.Printf("[bt] save metadata: %v", err)
			}
		}
		if shouldPauseAfterMetadata(opts, source.Kind) {
			d.mu.Lock()
			if st != nil && !st.removed {
				st.paused = true
				st.started = false
				st.torrent.DisallowDataDownload()
				st.torrent.DisallowDataUpload()
			}
			d.mu.Unlock()
		}
	}
	select {
	case <-tor.GotInfo():
		run()
	default:
		go func() {
			<-tor.GotInfo()
			run()
		}()
	}
}

func (d *Driver) setSelectFile(taskID, value string) error {
	sel := strings.TrimSpace(value)
	if _, _, err := parseAria2SelectFile(sel); err != nil {
		return err
	}
	d.mu.Lock()
	st := d.tasks[taskID]
	if st == nil || st.removed {
		d.mu.Unlock()
		return manager.ErrTaskNotFound
	}
	st.selectFile = sel
	tor := st.torrent
	d.mu.Unlock()
	if tor.Info() == nil {
		return nil
	}
	return applySelectFileToTorrent(tor, sel)
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

// torrentCompletedBitfieldHex 将本地已完成 piece 编码为 aria2 tellStatus 的 bitfield 十六进制。
func torrentCompletedBitfieldHex(tor *torrentlib.Torrent, totalPieces int) string {
	if tor == nil || totalPieces <= 0 {
		return ""
	}
	raw := make([]byte, (totalPieces+7)/8)
	for pieceIndex := 0; pieceIndex < totalPieces; pieceIndex++ {
		if !tor.Piece(pieceIndex).State().Complete {
			continue
		}
		raw[pieceIndex/8] |= 1 << (7 - uint(pieceIndex%8))
	}
	return hex.EncodeToString(raw)
}
