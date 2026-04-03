package config

import "time"

// Config �?aria2 风格配置文件在内部的统一结构�?
type Config struct {
	EnableRPC               bool
	RPCListenPort           int
	RPCListenAll            bool
	RPCAllowOriginAll       bool
	RPCMaxRequestSize       int64
	RPCSecret               string
	EnableWebSocket         bool
	Dir                     string
	DataDir                 string
	MaxConcurrentDownloads  int
	MaxOverallDownloadLimit int64
	MaxOverallUploadLimit   int64
	Pause                   bool
	ContinueDownloads       bool
	Daemon                  bool
	LogPath                 string
	LogLevel                string
	ListenPort              int
	EnableDHT               bool
	BTMaxPeers              int
	SeedRatio               float64
	HTTPUserAgent           string
	HTTPReferer             string
	HTTPProxy               string
	HTTPSProxy              string
	AllProxy                string
	MaxConnectionPerServer  int
	Split                   int
	CheckCertificate        bool
	SaveSession             string
	SaveSessionInterval     time.Duration
	ED2KEnable              bool
	ED2KListenPort          int
	ED2KServerPort          int
	ED2KMaxSources          int
	ED2KKadEnable           bool
	ED2KServerEnable        bool
	ED2KAICHEnable          bool
	ED2KSourceExchange      bool
	ED2KUploadSlots         int
	Warnings                []string
}

// Default 返回适合第一阶段骨架的默认配置�?
func Default() *Config {
	return &Config{
		EnableRPC:              true,
		RPCListenPort:          6800,
		RPCListenAll:           false,
		RPCAllowOriginAll:      false,
		RPCMaxRequestSize:      10 << 20,
		EnableWebSocket:        true,
		Dir:                    ".",
		DataDir:                "./data",
		MaxConcurrentDownloads: 1,
		Pause:                  false,
		ContinueDownloads:      true,
		Daemon:                 false,
		LogLevel:               "info",
		ListenPort:             6881,
		EnableDHT:              true,
		BTMaxPeers:             50,
		SeedRatio:              1.0,
		HTTPUserAgent:          "github.com/chenjia404/go-aria2/0.1",
		CheckCertificate:       true,
		SaveSession:            "./data/session.json",
		SaveSessionInterval:    30 * time.Second,
		ED2KEnable:             true,
		ED2KListenPort:         4662,
		ED2KServerPort:         4661,
		ED2KMaxSources:         200,
		ED2KKadEnable:          true,
		ED2KServerEnable:       true,
		ED2KAICHEnable:         true,
		ED2KSourceExchange:     true,
		ED2KUploadSlots:        3,
	}
}
