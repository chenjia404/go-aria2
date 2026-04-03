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
	"syscall"
	"time"

	"github.com/chenjia404/go-aria2/internal/compat/aria2"
	"github.com/chenjia404/go-aria2/internal/config"
	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/protocol/bt"
	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
	"github.com/chenjia404/go-aria2/internal/protocol/httpdl"
	rpcserver "github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
	wserver "github.com/chenjia404/go-aria2/internal/rpc/ws"
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
		logger.Fatalf("bootstrap startup jobs failed: %v", err)
	}

	service := aria2.NewService(mgr, cfg.RPCSecret)
	mux := http.NewServeMux()
	mux.Handle("/jsonrpc", rpcserver.NewServer(service, rpcserver.Options{
		MaxRequestSize: cfg.RPCMaxRequestSize,
		AllowOriginAll: cfg.RPCAllowOriginAll,
	}))
	if cfg.EnableWebSocket {
		mux.Handle("/ws", wserver.NewServer(mgr, cfg.RPCSecret, cfg.RPCAllowOriginAll))
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    listenAddr(cfg.RPCListenPort, cfg.RPCListenAll),
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
			logger.Printf("json-rpc listening on %s", server.Addr)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatalf("rpc server failed: %v", err)
			}
		}()
	} else {
		logger.Printf("rpc disabled by config")
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
		"pause":                      strconv.FormatBool(cfg.Pause),
		"max-concurrent-downloads":   strconv.Itoa(cfg.MaxConcurrentDownloads),
		"max-overall-download-limit": strconv.FormatInt(cfg.MaxOverallDownloadLimit, 10),
		"max-overall-upload-limit":   strconv.FormatInt(cfg.MaxOverallUploadLimit, 10),
		"seed-ratio":                 strconv.FormatFloat(cfg.SeedRatio, 'f', -1, 64),
		"http-user-agent":            cfg.HTTPUserAgent,
		"http-referer":               cfg.HTTPReferer,
		"http-proxy":                 cfg.HTTPProxy,
		"https-proxy":                cfg.HTTPSProxy,
		"all-proxy":                  cfg.AllProxy,
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
