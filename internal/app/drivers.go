package app

import (
	"fmt"

	"github.com/chenjia404/go-aria2/internal/config"
	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/protocol/bt"
	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
	"github.com/chenjia404/go-aria2/internal/protocol/httpdl"
)

// registeredDrivers 保存已注册到 manager 的协议驱动实例，供 daemon 后续挂载 ED2K 网关等。
type registeredDrivers struct {
	BT   *bt.Driver
	ED2K *ed2k.Driver
}

func registerProtocolDrivers(mgr *manager.Manager, cfg *config.Config, paths runtimePaths) (*registeredDrivers, error) {
	if mgr == nil {
		return nil, fmt.Errorf("manager is required")
	}

	btDriver, err := bt.New(bt.Options{
		DataDir:    paths.btDataDir,
		ListenPort: cfg.ListenPort,
		EnableDHT:  cfg.EnableDHT,
		MaxPeers:   cfg.BTMaxPeers,
		Crypto: bt.CryptoOptions{
			ForceEncryption: cfg.BTForceEncryption,
			RequireCrypto:   cfg.BTRequireCrypto,
			MinCryptoLevel:  cfg.BTMinCryptoLevel,
		},
		DHTFilePath:           cfg.DHTFilePath,
		DHTFilePath6:          cfg.DHTFilePath6,
		DHTListenPort:         cfg.DHTListenPort,
		EnableDHT6:            cfg.EnableDHT6,
		MaxOverallUploadLimit:   cfg.MaxOverallUploadLimit,
		MaxOverallDownloadLimit: cfg.MaxOverallDownloadLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("init bt driver: %w", err)
	}
	mgr.RegisterDriver(btDriver)

	out := &registeredDrivers{BT: btDriver}
	if cfg.ED2KEnable {
		ed2kDriver, err := ed2k.New(ed2k.Options{
			ListenPort:           cfg.ED2KListenPort,
			UDPPort:              cfg.ED2KServerPort,
			EnableDHT:            cfg.ED2KKadEnable,
			EnableServer:         cfg.ED2KServerEnable,
			UploadSlots:          cfg.ED2KUploadSlots,
			MaxSources:           cfg.ED2KMaxSources,
			StatePath:            paths.ed2kStatePath,
			AICHEnable:           cfg.ED2KAICHEnable,
			SourceExchangeEnable: cfg.ED2KSourceExchange,
		})
		if err != nil {
			btDriver.Close()
			return nil, fmt.Errorf("init ed2k driver: %w", err)
		}
		mgr.RegisterDriver(ed2kDriver)
		out.ED2K = ed2kDriver
	}

	mgr.RegisterDriver(httpdl.New(httpdl.Options{
		UserAgent:               cfg.HTTPUserAgent,
		Referer:                 cfg.HTTPReferer,
		HTTPProxy:               cfg.HTTPProxy,
		HTTPSProxy:              cfg.HTTPSProxy,
		AllProxy:                cfg.AllProxy,
		NoProxy:                 cfg.NoProxy,
		CheckCertificate:        cfg.CheckCertificate,
		Split:                   cfg.Split,
		MaxConnectionPerServer:  cfg.MaxConnectionPerServer,
		MaxOverallDownloadLimit: cfg.MaxOverallDownloadLimit,
	}))
	return out, nil
}
