package ed2k

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	goed2k "github.com/monkeyWie/goed2k"

	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
)

// HTTPGateway 将 goed2k.Client 以 goed2kd 风格暴露给 REST（仅 ED2K，不经 manager）。
type HTTPGateway struct {
	client     *goed2k.Client
	statePath  string
	defaultDir string
	startedAt  time.Time
	rpcListen  string
}

// NewHTTPGateway 从已启动的 Driver 构造网关（client 已 Start）。
func NewHTTPGateway(d *Driver, defaultDir, rpcListen string, startedAt time.Time) *HTTPGateway {
	if d == nil || d.client == nil {
		return nil
	}
	return &HTTPGateway{
		client:     d.client,
		statePath:  d.statePath,
		defaultDir: defaultDir,
		startedAt:  startedAt,
		rpcListen:  rpcListen,
	}
}

func (g *HTTPGateway) requireStatePath() error {
	if g == nil {
		return ed2kmodel.NewAppError(ed2kmodel.CodeStateStoreError, "ed2k gateway unavailable", nil)
	}
	if strings.TrimSpace(g.statePath) == "" {
		return ed2kmodel.NewAppError(ed2kmodel.CodeStateStoreError, "ed2k state path not configured", nil)
	}
	return nil
}

// Health 健康检查。
func (g *HTTPGateway) Health(ctx context.Context) (*ed2kmodel.HealthStatus, error) {
	_ = ctx
	ok := true
	if strings.TrimSpace(g.statePath) != "" {
		dir := filepath.Dir(g.statePath)
		if _, err := filepath.Abs(dir); err != nil {
			ok = false
		}
	}
	return &ed2kmodel.HealthStatus{
		DaemonRunning: true,
		EngineRunning: true,
		StateStoreOK:  ok,
		RPCAvailable:  true,
	}, nil
}

// Info 系统信息。
func (g *HTTPGateway) Info(ctx context.Context) *ed2kmodel.SystemInfo {
	_ = ctx
	return &ed2kmodel.SystemInfo{
		DaemonVersion:      "go-aria2",
		EngineRunning:      true,
		UptimeSeconds:      int64(time.Since(g.startedAt).Seconds()),
		RPCListen:          g.rpcListen,
		StatePath:          g.statePath,
		DefaultDownloadDir: g.defaultDir,
	}
}

// SaveState 保存 goed2k 状态。
func (g *HTTPGateway) SaveState(ctx context.Context) error {
	_ = ctx
	if err := g.requireStatePath(); err != nil {
		return err
	}
	if err := g.client.SaveState(""); err != nil {
		return ed2kmodel.NewAppError(ed2kmodel.CodeStateStoreError, "save state failed", err)
	}
	return nil
}

// LoadState 加载 goed2k 状态。
func (g *HTTPGateway) LoadState(ctx context.Context) error {
	_ = ctx
	if err := g.requireStatePath(); err != nil {
		return err
	}
	if err := g.client.LoadState(""); err != nil {
		return ed2kmodel.NewAppError(ed2kmodel.CodeStateStoreError, "load state failed", err)
	}
	return nil
}

// ClientStatus 引擎快照。
func (g *HTTPGateway) ClientStatus(ctx context.Context) ed2kmodel.ClientStatusDTO {
	_ = ctx
	ev := g.client.Status()
	return mapClientStatus(true, ev, g.client.DHTStatus())
}

func mapClientStatus(engineRunning bool, st goed2k.ClientStatus, dht goed2k.DHTStatus) ed2kmodel.ClientStatusDTO {
	servers := make([]ed2kmodel.ServerDTO, 0, len(st.Servers))
	for _, s := range st.Servers {
		servers = append(servers, mapServer(s))
	}
	transfers := make([]ed2kmodel.TransferDTO, 0, len(st.Transfers))
	for _, t := range st.Transfers {
		transfers = append(transfers, mapTransfer(t))
	}
	return ed2kmodel.ClientStatusDTO{
		EngineRunning: engineRunning,
		Servers:       servers,
		Transfers:     transfers,
		DHT:           mapDHT(dht),
		Totals: map[string]any{
			"total_done":     st.TotalDone,
			"total_received": st.TotalReceived,
			"total_wanted":   st.TotalWanted,
			"upload":         st.Upload,
			"download_rate":  st.DownloadRate,
			"upload_rate":    st.UploadRate,
		},
	}
}

// Servers 列表。
func (g *HTTPGateway) Servers(ctx context.Context) ([]ed2kmodel.ServerDTO, error) {
	_ = ctx
	ss := g.client.ServerStatuses()
	out := make([]ed2kmodel.ServerDTO, 0, len(ss))
	for _, s := range ss {
		out = append(out, mapServer(s))
	}
	return out, nil
}

// DHTStatus 当前 DHT。
func (g *HTTPGateway) DHTStatus(ctx context.Context) (*ed2kmodel.DHTStatusDTO, error) {
	_ = ctx
	d := g.client.DHTStatus()
	dd := mapDHT(d)
	return &dd, nil
}

// ConnectServer 连接单个服务器。
func (g *HTTPGateway) ConnectServer(ctx context.Context, addr string) error {
	_ = ctx
	if strings.TrimSpace(addr) == "" {
		return ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "address required", nil)
	}
	return g.client.Connect(addr)
}

// ConnectServers 批量连接。
func (g *HTTPGateway) ConnectServers(ctx context.Context, addrs []string) error {
	_ = ctx
	if len(addrs) == 0 {
		return ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "addresses required", nil)
	}
	return g.client.ConnectServers(addrs...)
}

// LoadServerMetSources 加载 server.met。
func (g *HTTPGateway) LoadServerMetSources(ctx context.Context, sources []string) error {
	_ = ctx
	for _, s := range sources {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if err := g.client.ConnectServerMet(s); err != nil {
			return err
		}
	}
	return nil
}

// EnableDHT 运行时启用 DHT。
func (g *HTTPGateway) EnableDHT(ctx context.Context) error {
	_ = ctx
	tr := g.client.EnableDHT()
	if tr != nil {
		if err := tr.Start(); err != nil {
			return ed2kmodel.NewAppError(ed2kmodel.CodeInternalError, "dht start failed", err)
		}
		g.client.Session().SyncDHTListenPort()
	}
	return nil
}

// LoadDHTNodesSources 加载 nodes.dat。
func (g *HTTPGateway) LoadDHTNodesSources(ctx context.Context, sources []string) error {
	_ = ctx
	if len(sources) == 0 {
		return ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "sources required", nil)
	}
	return g.client.LoadDHTNodesDat(sources...)
}

// AddDHTBootstrapNodes 添加引导节点。
func (g *HTTPGateway) AddDHTBootstrapNodes(ctx context.Context, nodes []string) error {
	_ = ctx
	if len(nodes) == 0 {
		return ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "nodes required", nil)
	}
	return g.client.AddDHTBootstrapNodes(nodes...)
}

// AddTransferParams 添加任务参数。
type AddTransferParams struct {
	ED2KLink   string
	TargetDir  string
	TargetName string
	Paused     bool
}

// AddTransferByED2K 解析 ED2K 并添加任务。
func (g *HTTPGateway) AddTransferByED2K(ctx context.Context, p AddTransferParams) (*ed2kmodel.TransferDTO, error) {
	_ = ctx
	link, err := goed2k.ParseEMuleLink(strings.TrimSpace(p.ED2KLink))
	if err != nil {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeInvalidED2KLink, "invalid ed2k link", err)
	}
	if link.Type != goed2k.LinkFile {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeInvalidED2KLink, "not a file link", nil)
	}
	name := link.StringValue
	if strings.TrimSpace(p.TargetName) != "" {
		name = p.TargetName
	}
	dir := strings.TrimSpace(p.TargetDir)
	if dir == "" {
		dir = g.defaultDir
	}
	synthetic := goed2k.FormatLink(name, link.NumberValue, link.Hash)
	_, targetPath, err := g.client.AddLink(synthetic, dir)
	if err != nil {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "add transfer failed", err)
	}
	if p.Paused {
		_ = g.client.PauseTransfer(link.Hash)
	}
	for _, ts := range g.client.TransferSnapshots() {
		if ts.Hash.Compare(link.Hash) == 0 {
			t := mapTransfer(ts)
			t.FilePath = targetPath
			return &t, nil
		}
	}
	t := mapTransfer(goed2k.TransferSnapshot{
		Hash: link.Hash, FileName: name, FilePath: targetPath, Size: link.NumberValue,
		Status: goed2k.TransferStatus{Paused: p.Paused, State: goed2k.Downloading},
	})
	return &t, nil
}

// ListTransfers 全量任务列表。
func (g *HTTPGateway) ListTransfers(ctx context.Context) ([]ed2kmodel.TransferDTO, error) {
	_ = ctx
	snaps := g.client.TransferSnapshots()
	out := make([]ed2kmodel.TransferDTO, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, mapTransfer(s))
	}
	return out, nil
}

// GetTransfer 单任务详情。
func (g *HTTPGateway) GetTransfer(ctx context.Context, hashHex string) (*ed2kmodel.TransferDetailDTO, error) {
	_ = ctx
	h, herr := parseHashParam(hashHex)
	if herr != nil {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeInvalidHash, "invalid hash", herr)
	}
	for _, s := range g.client.TransferSnapshots() {
		if s.Hash.Compare(h) == 0 {
			base := mapTransfer(s)
			peers := make([]ed2kmodel.PeerDTO, 0, len(s.Peers))
			for _, p := range s.Peers {
				peers = append(peers, mapPeer(p))
			}
			pieces := make([]ed2kmodel.PieceDTO, 0, len(s.Pieces))
			for _, pc := range s.Pieces {
				pieces = append(pieces, mapPiece(pc))
			}
			return &ed2kmodel.TransferDetailDTO{TransferDTO: base, Peers: peers, Pieces: pieces}, nil
		}
	}
	return nil, ed2kmodel.NewAppError(ed2kmodel.CodeTransferNotFound, "transfer not found", nil)
}

// PauseTransfer 暂停。
func (g *HTTPGateway) PauseTransfer(ctx context.Context, hashHex string) error {
	_ = ctx
	h, herr := parseHashParam(hashHex)
	if herr != nil {
		return ed2kmodel.NewAppError(ed2kmodel.CodeInvalidHash, "invalid hash", herr)
	}
	if err := g.client.PauseTransfer(h); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ed2kmodel.NewAppError(ed2kmodel.CodeTransferNotFound, "transfer not found", err)
		}
		return err
	}
	return nil
}

// ResumeTransfer 恢复。
func (g *HTTPGateway) ResumeTransfer(ctx context.Context, hashHex string) error {
	_ = ctx
	h, herr := parseHashParam(hashHex)
	if herr != nil {
		return ed2kmodel.NewAppError(ed2kmodel.CodeInvalidHash, "invalid hash", herr)
	}
	if err := g.client.ResumeTransfer(h); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ed2kmodel.NewAppError(ed2kmodel.CodeTransferNotFound, "transfer not found", err)
		}
		return err
	}
	return nil
}

// DeleteTransfer 删除任务。
func (g *HTTPGateway) DeleteTransfer(ctx context.Context, hashHex string, deleteFiles bool) error {
	_ = ctx
	h, herr := parseHashParam(hashHex)
	if herr != nil {
		return ed2kmodel.NewAppError(ed2kmodel.CodeInvalidHash, "invalid hash", herr)
	}
	if err := g.client.RemoveTransfer(h, deleteFiles); err != nil {
		return err
	}
	return nil
}

// ListTransferPeers peers 列表。
func (g *HTTPGateway) ListTransferPeers(ctx context.Context, hashHex string) ([]ed2kmodel.PeerDTO, error) {
	d, err := g.GetTransfer(ctx, hashHex)
	if err != nil {
		return nil, err
	}
	return d.Peers, nil
}

// ListTransferPieces pieces 列表。
func (g *HTTPGateway) ListTransferPieces(ctx context.Context, hashHex string) ([]ed2kmodel.PieceDTO, error) {
	d, err := g.GetTransfer(ctx, hashHex)
	if err != nil {
		return nil, err
	}
	return d.Pieces, nil
}

// StartSearch 发起搜索。
func (g *HTTPGateway) StartSearch(ctx context.Context, p ed2kmodel.SearchParamsDTO) (*ed2kmodel.SearchDTO, error) {
	_ = ctx
	params := goed2k.SearchParams{
		Query:              strings.TrimSpace(p.Query),
		Scope:              ParseSearchScope(p.Scope),
		MinSize:            p.MinSize,
		MaxSize:            p.MaxSize,
		MinSources:         p.MinSources,
		MinCompleteSources: p.MinCompleteSources,
		FileType:           p.FileType,
		Extension:          p.Extension,
	}
	_, err := g.client.StartSearch(params)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already") {
			return nil, ed2kmodel.NewAppError(ed2kmodel.CodeSearchAlreadyRunning, "search already running", err)
		}
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "start search failed", err)
	}
	snap := mapSearchSnapshot(g.client.SearchSnapshot())
	return &snap, nil
}

// CurrentSearch 当前搜索。
func (g *HTTPGateway) CurrentSearch(ctx context.Context) (*ed2kmodel.SearchDTO, error) {
	_ = ctx
	snap := mapSearchSnapshot(g.client.SearchSnapshot())
	return &snap, nil
}

// StopSearch 停止搜索。
func (g *HTTPGateway) StopSearch(ctx context.Context) error {
	_ = ctx
	return g.client.StopSearch()
}

// AddTransferFromSearchResult 从搜索结果添加下载。
func (g *HTTPGateway) AddTransferFromSearchResult(ctx context.Context, hashHex string, p AddTransferParams) (*ed2kmodel.TransferDTO, error) {
	_ = ctx
	want, herr := parseHashParam(hashHex)
	if herr != nil {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeInvalidHash, "invalid hash", herr)
	}
	snap := g.client.SearchSnapshot()
	var linkStr string
	for _, r := range snap.Results {
		if r.Hash.Compare(want) == 0 {
			linkStr = r.ED2KLink()
			break
		}
	}
	if linkStr == "" {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeNotFound, "result not in current search", nil)
	}
	np := p
	np.ED2KLink = linkStr
	return g.AddTransferByED2K(ctx, np)
}
