package model

// SystemInfo 守护进程与 ED2K 引擎概要（goed2kd 风格）。
type SystemInfo struct {
	DaemonVersion      string `json:"daemon_version"`
	EngineRunning      bool   `json:"engine_running"`
	UptimeSeconds      int64  `json:"uptime_seconds"`
	RPCListen          string `json:"rpc_listen"`
	StatePath          string `json:"state_path"`
	DefaultDownloadDir string `json:"default_download_dir"`
}

// HealthStatus 健康检查。
type HealthStatus struct {
	DaemonRunning bool `json:"daemon_running"`
	EngineRunning bool `json:"engine_running"`
	StateStoreOK  bool `json:"state_store_ok"`
	RPCAvailable  bool `json:"rpc_available"`
}

// ConfigSummary 对外可见的配置摘要。
type ConfigSummary struct {
	RPCListen              string `json:"rpc_listen"`
	EngineListenPort       int    `json:"engine_listen_port"`
	EngineUDPPort          int    `json:"engine_udp_port"`
	EnableDHT              bool   `json:"enable_dht"`
	DefaultDownloadDir     string `json:"default_download_dir"`
	StateEnabled           bool   `json:"state_enabled"`
	StatePath              string `json:"state_path"`
	AutoSaveIntervalSec    int    `json:"auto_save_interval_seconds"`
	BootstrapServerCount   int    `json:"bootstrap_server_count"`
	BootstrapServerMetURLs int    `json:"bootstrap_server_met_url_count"`
	BootstrapNodesDatURLs  int    `json:"bootstrap_nodes_dat_url_count"`
}

// TransferDTO 任务列表项。
type TransferDTO struct {
	Hash              string  `json:"hash"`
	FileName          string  `json:"file_name"`
	FilePath          string  `json:"file_path"`
	Size              int64   `json:"size"`
	CreateTime        int64   `json:"create_time"`
	State             string  `json:"state"`
	Paused            bool    `json:"paused"`
	DownloadRate      int     `json:"download_rate"`
	UploadRate        int     `json:"upload_rate"`
	TotalDone         int64   `json:"total_done"`
	TotalReceived     int64   `json:"total_received"`
	TotalWanted       int64   `json:"total_wanted"`
	ETA               int64   `json:"eta"`
	NumPeers          int     `json:"num_peers"`
	ActivePeers       int     `json:"active_peers"`
	DownloadingPieces int     `json:"downloading_pieces"`
	Progress          float64 `json:"progress"`
	ED2KLink          string  `json:"ed2k_link"`
}

// TransferDetailDTO 任务详情。
type TransferDetailDTO struct {
	TransferDTO
	Peers  []PeerDTO  `json:"peers"`
	Pieces []PieceDTO `json:"pieces"`
}

// PeerDTO 对端信息。
type PeerDTO struct {
	Endpoint             string `json:"endpoint"`
	DownloadSpeed        int    `json:"download_speed"`
	PayloadDownloadSpeed int    `json:"payload_download_speed"`
	UploadSpeed          int    `json:"upload_speed"`
	Source               string `json:"source"`
	ModName              string `json:"mod_name"`
	FailCount            int    `json:"fail_count"`
}

// PieceDTO 分片快照。
type PieceDTO struct {
	Index         int    `json:"index"`
	State         string `json:"state"`
	TotalBytes    int64  `json:"total_bytes"`
	DoneBytes     int64  `json:"done_bytes"`
	ReceivedBytes int64  `json:"received_bytes"`
	BlocksTotal   int    `json:"blocks_total"`
	BlocksDone    int    `json:"blocks_done"`
	BlocksWriting int    `json:"blocks_writing"`
	BlocksPending int    `json:"blocks_pending"`
}

// ServerDTO 服务器连接状态。
type ServerDTO struct {
	Identifier                   string `json:"identifier"`
	Address                      string `json:"address"`
	Configured                   bool   `json:"configured"`
	Connected                    bool   `json:"connected"`
	HandshakeCompleted           bool   `json:"handshake_completed"`
	Primary                      bool   `json:"primary"`
	Disconnecting                bool   `json:"disconnecting"`
	ClientID                     int32  `json:"client_id"`
	IDClass                      string `json:"id_class"`
	DownloadRate                 int    `json:"download_rate"`
	UploadRate                   int    `json:"upload_rate"`
	MillisecondsSinceLastReceive int64  `json:"milliseconds_since_last_receive"`
}

// ClientStatusDTO 引擎整体状态快照。
type ClientStatusDTO struct {
	EngineRunning bool           `json:"engine_running"`
	Servers       []ServerDTO    `json:"servers"`
	Transfers     []TransferDTO  `json:"transfers"`
	DHT           DHTStatusDTO   `json:"dht"`
	Totals        map[string]any `json:"totals"`
}

// DHTStatusDTO DHT/KAD 状态。
type DHTStatusDTO struct {
	Bootstrapped      bool   `json:"bootstrapped"`
	Firewalled        bool   `json:"firewalled"`
	LiveNodes         int    `json:"live_nodes"`
	ReplacementNodes  int    `json:"replacement_nodes"`
	RouterNodes       int    `json:"router_nodes"`
	RunningTraversals int    `json:"running_traversals"`
	KnownNodes        int    `json:"known_nodes"`
	InitialBootstrap  bool   `json:"initial_bootstrap"`
	ListenPort        int    `json:"listen_port"`
	StoragePoint      string `json:"storage_point"`
}

// SearchParamsDTO 搜索参数。
type SearchParamsDTO struct {
	Query              string `json:"query"`
	Scope              string `json:"scope"`
	MinSize            int64  `json:"min_size"`
	MaxSize            int64  `json:"max_size"`
	MinSources         int    `json:"min_sources"`
	MinCompleteSources int    `json:"min_complete_sources"`
	FileType           string `json:"file_type"`
	Extension          string `json:"extension"`
}

// SearchResultDTO 单条搜索结果。
type SearchResultDTO struct {
	Hash              string `json:"hash"`
	FileName          string `json:"file_name"`
	FileSize          int64  `json:"file_size"`
	Sources           int    `json:"sources"`
	CompleteSources   int    `json:"complete_sources"`
	MediaBitrate      int    `json:"media_bitrate"`
	MediaLength       int    `json:"media_length"`
	MediaCodec        string `json:"media_codec"`
	Extension         string `json:"extension"`
	FileType          string `json:"file_type"`
	Source            string `json:"source"`
	ED2KLink          string `json:"ed2k_link"`
}

// SearchDTO 当前搜索快照。
type SearchDTO struct {
	ID         uint32            `json:"id"`
	State      string            `json:"state"`
	Params     SearchParamsDTO   `json:"params"`
	Results    []SearchResultDTO `json:"results"`
	UpdatedAt  int64             `json:"updated_at"`
	StartedAt  int64             `json:"started_at"`
	ServerBusy bool              `json:"server_busy"`
	DHTBusy    bool              `json:"dht_busy"`
	KadKeyword string            `json:"kad_keyword"`
	Error      string            `json:"error"`
}
