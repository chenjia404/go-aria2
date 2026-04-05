package app

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chenjia404/go-aria2/internal/compat/aria2"
	"github.com/chenjia404/go-aria2/internal/config"
	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/protocol/bt"
	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
	"github.com/chenjia404/go-aria2/internal/protocol/httpdl"
	"github.com/chenjia404/go-aria2/internal/rpc/httpapi"
	rpcserver "github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// runDaemon 启动守护进程。
func runDaemon(args []string) error {
	daemonOpts, err := parseDaemonArgs(args)
	if err != nil {
		return err
	}

	cfg, err := loadConfig(daemonOpts.configPath)
	if err != nil {
		return err
	}
	if err := applyDaemonCLIOptions(cfg, daemonOpts); err != nil {
		return err
	}
	normalizeDaemonInputFile(cfg, &daemonOpts)
	runtimePaths := resolveRuntimePaths(cfg)

	logger, closer, err := newLogger(cfg)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	for _, warning := range cfg.Warnings {
		logger.Printf("config warning: %s", warning)
	}
	if cfg.BTForceEncryption {
		logger.Printf("config warning: bt-force-encryption is accepted for aria2 compatibility, but strict BT encryption is not implemented yet")
	}
	if cfg.BTRequireCrypto {
		logger.Printf("config warning: bt-require-crypto is accepted for aria2 compatibility, but strict BT crypto policy is not implemented yet")
	}
	if cfg.BTMinCryptoLevel != "" && cfg.BTMinCryptoLevel != "plain" {
		logger.Printf("config warning: bt-min-crypto-level=%s is accepted for aria2 compatibility, but crypto level enforcement is not implemented yet", cfg.BTMinCryptoLevel)
	}
	if !cfg.FollowTorrent {
		logger.Printf("config warning: follow-torrent=false is accepted for aria2 compatibility, but downloading .torrent files without following is not implemented yet")
	}
	if cfg.SeedTime > 0 {
		logger.Printf("config warning: seed-time is accepted for aria2 compatibility, but automatic stop-seeding by time is not implemented yet")
	}
	if !cfg.BTLoadSavedMetadata {
		logger.Printf("config warning: bt-load-saved-metadata=false is accepted for aria2 compatibility, but saved metadata loading policy is not implemented yet")
	}
	if !cfg.BTSaveMetadata {
		logger.Printf("config warning: bt-save-metadata=false is accepted for aria2 compatibility, but metadata persistence policy is not implemented yet")
	}
	if cfg.DHTFilePath != "" || cfg.DHTFilePath6 != "" || cfg.DHTListenPort != 0 || !cfg.EnableDHT6 {
		logger.Printf("config warning: dht-file-path, dht-file-path6, dht-listen-port, and enable-dht6 are accepted for aria2 compatibility, but custom DHT state/listen behavior is not implemented yet")
	}
	if !cfg.FollowMetalink {
		logger.Printf("config warning: follow-metalink=false is accepted for aria2 compatibility, but metalink handling controls are not implemented yet")
	}
	if cfg.PauseMetadata {
		logger.Printf("config warning: pause-metadata is accepted for aria2 compatibility, but metadata pause workflow is not implemented yet")
	}
	if cfg.NoProxy != "" {
		logger.Printf("config warning: no-proxy is accepted for aria2 compatibility, but proxy bypass rules are not implemented yet")
	}

	store := session.NewFileStore(runtimePaths.sessionPath)
	mgr := manager.New(manager.Options{
		DefaultDir:    cfg.Dir,
		MaxConcurrent: cfg.MaxConcurrentDownloads,
		StartPaused:   cfg.Pause,
		GlobalOptions: buildGlobalOptions(cfg),
		Store:         store,
	})

	btDriver, err := bt.New(bt.Options{
		DataDir:    runtimePaths.btDataDir,
		ListenPort: cfg.ListenPort,
		EnableDHT:  cfg.EnableDHT,
		MaxPeers:   cfg.BTMaxPeers,
	})
	if err != nil {
		logger.Fatalf("init bt driver failed: %v", err)
	}
	defer btDriver.Close()
	mgr.RegisterDriver(btDriver)

	var ed2kDriver *ed2k.Driver
	if cfg.ED2KEnable {
		ed2kDriver, err = ed2k.New(ed2k.Options{
			ListenPort:   cfg.ED2KListenPort,
			UDPPort:      cfg.ED2KServerPort,
			EnableDHT:    cfg.ED2KKadEnable,
			EnableServer: cfg.ED2KServerEnable,
			UploadSlots:  cfg.ED2KUploadSlots,
			MaxSources:   cfg.ED2KMaxSources,
			StatePath:    runtimePaths.ed2kStatePath,
		})
		if err != nil {
			logger.Fatalf("init ed2k driver failed: %v", err)
		}
		defer ed2kDriver.Close()
		mgr.RegisterDriver(ed2kDriver)
	}

	httpDriver := httpdl.New(httpdl.Options{
		UserAgent:               cfg.HTTPUserAgent,
		Referer:                 cfg.HTTPReferer,
		HTTPProxy:               cfg.HTTPProxy,
		HTTPSProxy:              cfg.HTTPSProxy,
		AllProxy:                cfg.AllProxy,
		CheckCertificate:        cfg.CheckCertificate,
		Split:                   cfg.Split,
		MaxConnectionPerServer:  cfg.MaxConnectionPerServer,
		MaxOverallDownloadLimit: cfg.MaxOverallDownloadLimit,
	})
	mgr.RegisterDriver(httpDriver)

	ctx := context.Background()
	if err := mgr.LoadSession(ctx); err != nil {
		logger.Fatalf("restore session failed: %v", err)
	}
	if err := bootstrapStartupJobs(ctx, mgr, cfg, daemonOpts, logger); err != nil {
		// 某些 aria2 前端会传入非 aria2 input-file 格式的临时文件。
		// 这里降级为 warning，避免守护进程因启动任务注入失败而整体退出。
		logger.Printf("bootstrap startup jobs skipped: %v", err)
	}

	service := aria2.NewService(mgr, cfg.RPCSecret)
	mux := http.NewServeMux()
	daemonStarted := time.Now()
	rpcListenAddr := listenAddr(cfg.RPCListenPort, cfg.RPCListenAll)
	if cfg.ED2KEnable && ed2kDriver != nil {
		gw := ed2k.NewHTTPGateway(ed2kDriver, cfg.Dir, rpcListenAddr, daemonStarted)
		mux.Handle("/api/", httpapi.NewRouter(&httpapi.Server{
			Log:                logger,
			Gateway:            gw,
			CFG:                cfg,
			ED2KStatePath:      runtimePaths.ed2kStatePath,
			RPCListenAddr:      rpcListenAddr,
			RPCSecret:          cfg.RPCSecret,
			ReadTimeoutSeconds: 60,
		}))
	}
	rpcOpts := rpcserver.Options{
		MaxRequestSize: cfg.RPCMaxRequestSize,
		AllowOriginAll: cfg.RPCAllowOriginAll,
	}
	if cfg.EnableWebSocket {
		rpcOpts.WebSocket = &rpcserver.WebSocketOptions{
			Manager: mgr,
			Secret:  cfg.RPCSecret,
		}
	}
	rpcHandler := rpcserver.NewServer(service, rpcOpts)
	mux.Handle("/jsonrpc", rpcHandler)
	if cfg.EnableWebSocket {
		mux.Handle("/ws", rpcHandler)
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    rpcListenAddr,
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	if cfg.SaveSessionInterval > 0 {
		go persistLoop(logger, mgr, cfg.SaveSessionInterval)
	}
	go syncLoop(logger, mgr, time.Second)

	if cfg.EnableRPC {
		go func() {
			rpcURL := rpcEndpointURL(cfg.RPCListenPort, cfg.RPCListenAll)
			logger.Printf("json-rpc listening on %s (POST: %s)", server.Addr, rpcURL)
			if cfg.EnableWebSocket {
				wsURL := rpcWebSocketExampleURL(cfg.RPCListenPort, cfg.RPCListenAll)
				logger.Printf("JSON-RPC over WebSocket（aria2 兼容）: %s；/ws 为同服务别名", wsURL)
			}
			if cfg.ED2KEnable && ed2kDriver != nil {
				logger.Printf("ED2K REST API (goed2kd-compatible paths): http://%s/api/v1/system/health — transfers use {hash}, auth: rpc-secret (Bearer / X-Auth-Token)", server.Addr)
			}
			if !cfg.RPCListenAll {
				logger.Printf("rpc 仅绑定本机回环；ERR_CONNECTION_REFUSED 常见于用局域网 IP/容器 IP 访问——请改用 127.0.0.1，或设置 rpc-listen-all=true 并放行防火墙")
			}
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatalf("rpc server failed: %v", err)
			}
		}()
	} else {
		logger.Printf("rpc disabled by config (enable-rpc=false)，无法提供 JSON-RPC")
	}

	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if cfg.EnableRPC {
		_ = server.Shutdown(shutdownCtx)
	}
	if err := mgr.Close(shutdownCtx); err != nil {
		logger.Printf("save session on shutdown failed: %v", err)
	}
	return nil
}

func normalizeDaemonInputFile(cfg *config.Config, opts *daemonCLIOptions) {
	if cfg == nil || opts == nil {
		return
	}
	inputPath := strings.TrimSpace(opts.inputFile)
	if inputPath == "" || inputPath == "-" {
		return
	}
	if !strings.EqualFold(filepath.Ext(inputPath), ".json") {
		return
	}

	// 前端若把 go-aria2 自己的 session.json 误传给 -i，
	// 则把它视为 session 存储路径，而不是 aria2 input-file。
	cfg.SaveSession = inputPath
	opts.inputFile = ""
}

func loadConfig(path string) (*config.Config, error) {
	_, err := os.Stat(path)
	if err == nil {
		return config.LoadFile(path)
	}
	if os.IsNotExist(err) && path == "aria2.conf" {
		return config.Default(), nil
	}
	return nil, err
}

type runtimePaths struct {
	sessionPath   string
	btDataDir     string
	ed2kStatePath string
}

func resolveRuntimePaths(cfg *config.Config) runtimePaths {
	dataDir := filepath.Clean(cfg.DataDir)
	if dataDir == "." || dataDir == "" {
		dataDir = filepath.Dir(cfg.SaveSession)
		if dataDir == "." || dataDir == "" {
			dataDir = "./data"
		}
	}

	sessionPath := cfg.SaveSession
	if sessionPath == "" || sessionPath == "./data/session.json" {
		sessionPath = filepath.Join(dataDir, "session.json")
	}

	_ = os.MkdirAll(dataDir, 0o755)
	return runtimePaths{
		sessionPath:   sessionPath,
		btDataDir:     filepath.Join(dataDir, "bt"),
		ed2kStatePath: filepath.Join(dataDir, "ed2k-state.json"),
	}
}

func buildGlobalOptions(cfg *config.Config) map[string]string {
	return map[string]string{
		"dir":                        cfg.Dir,
		"allow-overwrite":            strconv.FormatBool(cfg.AllowOverwrite),
		"auto-file-renaming":         strconv.FormatBool(cfg.AutoFileRenaming),
		"pause":                      strconv.FormatBool(cfg.Pause),
		"continue":                   strconv.FormatBool(cfg.ContinueDownloads),
		"max-concurrent-downloads":   strconv.Itoa(cfg.MaxConcurrentDownloads),
		"max-download-limit":         strconv.FormatInt(cfg.MaxDownloadLimit, 10),
		"max-overall-download-limit": strconv.FormatInt(cfg.MaxOverallDownloadLimit, 10),
		"max-overall-upload-limit":   strconv.FormatInt(cfg.MaxOverallUploadLimit, 10),
		"seed-ratio":                 strconv.FormatFloat(cfg.SeedRatio, 'f', -1, 64),
		"seed-time":                  strconv.FormatInt(int64(cfg.SeedTime/time.Minute), 10),
		"dht-file-path":              cfg.DHTFilePath,
		"dht-file-path6":             cfg.DHTFilePath6,
		"dht-listen-port":            strconv.Itoa(cfg.DHTListenPort),
		"enable-dht6":                strconv.FormatBool(cfg.EnableDHT6),
		"bt-force-encryption":        strconv.FormatBool(cfg.BTForceEncryption),
		"bt-require-crypto":          strconv.FormatBool(cfg.BTRequireCrypto),
		"bt-min-crypto-level":        cfg.BTMinCryptoLevel,
		"bt-tracker":                 cfg.BTTracker,
		"bt-exclude-tracker":         cfg.BTExcludeTracker,
		"bt-load-saved-metadata":     strconv.FormatBool(cfg.BTLoadSavedMetadata),
		"bt-save-metadata":           strconv.FormatBool(cfg.BTSaveMetadata),
		"follow-torrent":             strconv.FormatBool(cfg.FollowTorrent),
		"follow-metalink":            strconv.FormatBool(cfg.FollowMetalink),
		"pause-metadata":             strconv.FormatBool(cfg.PauseMetadata),
		"http-user-agent":            cfg.HTTPUserAgent,
		"user-agent":                 cfg.HTTPUserAgent,
		"http-referer":               cfg.HTTPReferer,
		"http-proxy":                 cfg.HTTPProxy,
		"https-proxy":                cfg.HTTPSProxy,
		"all-proxy":                  cfg.AllProxy,
		"no-proxy":                   cfg.NoProxy,
		"max-connection-per-server":  strconv.Itoa(cfg.MaxConnectionPerServer),
		"split":                      strconv.Itoa(cfg.Split),
		"check-certificate":          strconv.FormatBool(cfg.CheckCertificate),
	}
}

func persistLoop(logger *log.Logger, mgr *manager.Manager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := mgr.SaveSession(context.Background()); err != nil {
			logger.Printf("periodic save session failed: %v", err)
		}
	}
}

func syncLoop(logger *log.Logger, mgr *manager.Manager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := mgr.SyncActive(context.Background()); err != nil {
			logger.Printf("sync active tasks failed: %v", err)
		}
	}
}

func listenAddr(port int, listenAll bool) string {
	if port <= 0 {
		port = 6800
	}
	if listenAll {
		return ":" + strconv.Itoa(port)
	}
	return "127.0.0.1:" + strconv.Itoa(port)
}

// rpcEndpointURL 用于日志示例；rpc-listen-all=true 时局域网需把主机名换成本机 IP。
func rpcEndpointURL(port int, listenAll bool) string {
	if port <= 0 {
		port = 6800
	}
	u := "http://127.0.0.1:" + strconv.Itoa(port) + "/jsonrpc"
	if listenAll {
		return u + "（监听所有网卡；外机访问请用本机局域网 IP 替代 127.0.0.1）"
	}
	return u
}

func rpcWebSocketExampleURL(port int, listenAll bool) string {
	if port <= 0 {
		port = 6800
	}
	u := "ws://127.0.0.1:" + strconv.Itoa(port) + "/jsonrpc"
	if listenAll {
		return u + "（外机同上，替换主机名）"
	}
	return u
}

func newLogger(cfg *config.Config) (*log.Logger, io.Closer, error) {
	if cfg.LogPath == "" {
		return log.New(os.Stdout, "[go-aria2] ", log.LstdFlags|log.Lmicroseconds), nil, nil
	}

	file, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	writer := io.MultiWriter(os.Stdout, file)
	return log.New(writer, "[go-aria2] ", log.LstdFlags|log.Lmicroseconds), file, nil
}
