package httpapi

import (
	"github.com/chenjia404/go-aria2/internal/config"
	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
)

func buildConfigSummary(cfg *config.Config, ed2kStatePath, rpcListen string) *ed2kmodel.ConfigSummary {
	if cfg == nil {
		return &ed2kmodel.ConfigSummary{RPCListen: rpcListen}
	}
	return &ed2kmodel.ConfigSummary{
		RPCListen:              rpcListen,
		EngineListenPort:       cfg.ED2KListenPort,
		EngineUDPPort:          cfg.ED2KServerPort,
		EnableDHT:              cfg.ED2KKadEnable,
		DefaultDownloadDir:     cfg.Dir,
		StateEnabled:           ed2kStatePath != "",
		StatePath:              ed2kStatePath,
		AutoSaveIntervalSec:    0,
		BootstrapServerCount:   0,
		BootstrapServerMetURLs: 0,
		BootstrapNodesDatURLs:  0,
	}
}
