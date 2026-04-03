package app

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/session"
	"github.com/chenjia404/go-aria2/internal/migrate/aria2session"
	"github.com/chenjia404/go-aria2/internal/protocol/bt"
	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
	"github.com/chenjia404/go-aria2/internal/protocol/httpdl"
)

// runMigrate 导入 aria2 save-session 文件。
func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate-from-aria2", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		configPath  string
		sessionPath string
		strict      bool
	)
	fs.StringVar(&configPath, "conf", "aria2.conf", "path to aria2 style config file")
	fs.StringVar(&sessionPath, "session", "", "aria2 save-session file path")
	fs.BoolVar(&strict, "strict", false, "verify BT pieces strictly before returning")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if sessionPath == "" {
		return fmt.Errorf("session file is required")
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
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

	if _, err := os.Stat(sessionPath); err != nil {
		return err
	}
	logger.Printf("[INFO] Reading session file: %s", sessionPath)
	parsed, err := aria2session.ParseAria2Session(sessionPath)
	if err != nil {
		return err
	}
	logger.Printf("[INFO] Parsed %d session tasks", len(parsed))

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
		return err
	}
	defer btDriver.Close()
	mgr.RegisterDriver(btDriver)

	if cfg.ED2KEnable {
		ed2kDriver, err := ed2k.New(ed2k.Options{
			ListenPort:   cfg.ED2KListenPort,
			UDPPort:      cfg.ED2KServerPort,
			EnableDHT:    cfg.ED2KKadEnable,
			EnableServer: cfg.ED2KServerEnable,
			UploadSlots:  cfg.ED2KUploadSlots,
			MaxSources:   cfg.ED2KMaxSources,
			StatePath:    runtimePaths.ed2kStatePath,
		})
		if err != nil {
			return err
		}
		defer ed2kDriver.Close()
		mgr.RegisterDriver(ed2kDriver)
	}

	mgr.RegisterDriver(httpdl.New(httpdl.Options{
		UserAgent:               cfg.HTTPUserAgent,
		Referer:                 cfg.HTTPReferer,
		HTTPProxy:               cfg.HTTPProxy,
		HTTPSProxy:              cfg.HTTPSProxy,
		AllProxy:                cfg.AllProxy,
		CheckCertificate:        cfg.CheckCertificate,
		Split:                   cfg.Split,
		MaxConnectionPerServer:  cfg.MaxConnectionPerServer,
		MaxOverallDownloadLimit: cfg.MaxOverallDownloadLimit,
	}))

	ctx := context.Background()
	if err := mgr.LoadSession(ctx); err != nil {
		return err
	}

	importer := aria2session.Importer{
		Manager: mgr,
		Logger:  logger,
		Strict:  strict,
	}
	imported, importErr := importer.ImportAria2Tasks(ctx, normalizeImportedSessionTasks(parsed))
	if importErr != nil {
		logger.Printf("[WARN] migration completed with errors: %v", importErr)
	}
	logger.Printf("[INFO] Imported %d tasks", len(imported))
	if err := mgr.SaveSession(ctx); err != nil {
		return err
	}
	logger.Printf("[INFO] Session saved to %s", runtimePaths.sessionPath)
	if importErr != nil && len(imported) == 0 {
		return importErr
	}
	return nil
}

// normalizeImportedSessionTasks keeps a dedicated hook for future pre-processing.
func normalizeImportedSessionTasks(tasks []aria2session.Aria2SessionTask) []aria2session.Aria2SessionTask {
	out := make([]aria2session.Aria2SessionTask, 0, len(tasks))
	for _, item := range tasks {
		if item.Options == nil {
			item.Options = map[string]string{}
		}
		out = append(out, item)
	}
	return out
}
