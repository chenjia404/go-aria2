package app

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/chenjia404/go-aria2/internal/config"
)

// daemonCLIOptions 表示命令行上显式传入的 aria2 风格覆盖项。
type daemonCLIOptions struct {
	configPath      string
	confSeen        bool
	noConf          bool
	values          map[string]any
	inputFile       string
	uris            []string
	startup         map[string]string
	unknownWarnings []string
}

// parseDaemonArgs 解析 aria2 风格守护进程参数。
// 这里支持常见短参数和长参数别名，方便从 aria2 迁移。
func parseDaemonArgs(args []string) (daemonCLIOptions, error) {
	opts := daemonCLIOptions{
		configPath: "aria2.conf",
		values:     map[string]any{},
		startup:    map[string]string{},
	}
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)

	var (
		configPath       string
		dir              string
		dataDir          string
		rpcSecret        string
		logPath          string
		logLevel         string
		inputFile        string
		outName          string
		checksum         string
		gid              string
		saveSess         string
		httpUA           string
		httpRef          string
		httpProxy        string
		httpsProxy       string
		allProxy         string
		noProxy          string
		btMinCryptoLevel string
		btTracker        string
		btExcludeTracker string
		dhtFilePath      string
		dhtFilePath6     string
		listenPortStr    string
		dhtListenPortStr string
		maxDownloadStr   string
		maxOverallDLStr  string
		maxOverallULStr  string
		minSplitSizeStr  string
		lowestSpeedStr   string
		httpUser         string
		httpPasswd       string
		ftpUser          string
		ftpPasswd        string
		fileAllocation   string
	)
	var (
		rpcListenPort          int
		rpcMaxRequestSizeStr   string
		maxConcurrentDownloads int
		btMaxPeers             int
		maxTries               int
		maxConnPerServer       int
		split                  int
		saveSessInterval       int64
		ed2kListenPort         int
		ed2kServerPort         int
		ed2kMaxSources         int
		ed2kUploadSlots        int
		seedRatio              float64
		seedTime               int64
	)
	var (
		enableRPC          bool
		rpcListenAll       bool
		rpcAllowOriginAll  bool
		rpcStrictAuth      bool
		aria2CompatMode    bool
		enableWebSocket    bool
		allowOverwrite     bool
		autoFileRenaming   bool
		followTorrent      bool
		followMetalink     bool
		pause              bool
		pauseMetadata      bool
		continueDownloads  bool
		daemon             bool
		enableDHT          bool
		enableDHT6         bool
		btForceEncryption  bool
		btRequireCrypto    bool
		btLoadSavedMeta    bool
		btSaveMetadata     bool
		checkCertificate   bool
		checkIntegrity     bool
		forceSave          bool
		ed2kEnable         bool
		ed2kKadEnable      bool
		ed2kServerEnable   bool
		ed2kAICHEnable     bool
		ed2kSourceExchange bool
		noConf             bool
		quiet              bool
		btEnableLPD        bool
	)

	fs.StringVar(&configPath, "conf", "aria2.conf", "path to aria2 style config file")
	fs.StringVar(&configPath, "conf-path", "aria2.conf", "path to aria2 style config file")
	fs.BoolVar(&noConf, "no-conf", false, "do not load aria2.conf")
	fs.BoolVar(&quiet, "quiet", false, "suppress console log output")
	fs.BoolVar(&quiet, "q", false, "suppress console log output")
	fs.StringVar(&inputFile, "i", "", "input file")
	fs.StringVar(&inputFile, "input-file", "", "input file")
	fs.StringVar(&dir, "d", "", "download directory")
	fs.StringVar(&dir, "dir", "", "download directory")
	fs.StringVar(&outName, "o", "", "output file name")
	fs.StringVar(&outName, "out", "", "output file name")
	fs.StringVar(&dataDir, "data-dir", "", "data directory")
	fs.IntVar(&maxConcurrentDownloads, "j", 0, "max concurrent downloads")
	fs.IntVar(&maxConcurrentDownloads, "max-concurrent-downloads", 0, "max concurrent downloads")
	fs.BoolVar(&daemon, "D", false, "run as daemon")
	fs.BoolVar(&daemon, "daemon", false, "run as daemon")
	fs.BoolVar(&continueDownloads, "c", false, "continue downloads")
	fs.BoolVar(&continueDownloads, "continue", false, "continue downloads")
	fs.StringVar(&logPath, "l", "", "log file path")
	fs.StringVar(&logPath, "log", "", "log file path")
	fs.StringVar(&logLevel, "log-level", "", "log level")
	fs.BoolVar(&enableRPC, "enable-rpc", false, "enable rpc server")
	fs.BoolVar(&rpcListenAll, "rpc-listen-all", false, "listen on all interfaces")
	fs.BoolVar(&rpcAllowOriginAll, "rpc-allow-origin-all", false, "allow all origins")
	fs.StringVar(&rpcMaxRequestSizeStr, "rpc-max-request-size", "", "max rpc request size")
	fs.IntVar(&rpcListenPort, "rpc-listen-port", 0, "rpc listen port")
	fs.StringVar(&rpcSecret, "rpc-secret", "", "rpc secret")
	fs.BoolVar(&rpcStrictAuth, "rpc-strict-auth", false, "require token for all RPC methods including system.*")
	fs.BoolVar(&aria2CompatMode, "aria2-compat-mode", false, "enable aria2-compatible defaults (strict RPC auth, text save-session export)")
	fs.BoolVar(&enableWebSocket, "enable-websocket", false, "enable websocket notifications")
	fs.BoolVar(&allowOverwrite, "allow-overwrite", false, "overwrite existing files")
	fs.BoolVar(&autoFileRenaming, "auto-file-renaming", false, "auto rename when target exists")
	fs.BoolVar(&pause, "pause", false, "start paused")
	fs.StringVar(&listenPortStr, "listen-port", "", "bt listen port or range")
	fs.BoolVar(&enableDHT, "enable-dht", false, "enable dht")
	fs.BoolVar(&enableDHT6, "enable-dht6", false, "enable ipv6 dht")
	fs.StringVar(&dhtFilePath, "dht-file-path", "", "dht file path")
	fs.StringVar(&dhtFilePath6, "dht-file-path6", "", "ipv6 dht file path")
	fs.StringVar(&dhtListenPortStr, "dht-listen-port", "", "dht listen port or range")
	fs.BoolVar(&btEnableLPD, "bt-enable-lpd", false, "enable bt local peer discovery")
	fs.IntVar(&btMaxPeers, "bt-max-peers", 0, "bt max peers")
	fs.BoolVar(&btForceEncryption, "bt-force-encryption", false, "force bt encryption")
	fs.BoolVar(&btRequireCrypto, "bt-require-crypto", false, "require bt crypto")
	fs.StringVar(&btMinCryptoLevel, "bt-min-crypto-level", "", "minimum bt crypto level")
	fs.StringVar(&btTracker, "bt-tracker", "", "extra bt trackers")
	fs.StringVar(&btExcludeTracker, "bt-exclude-tracker", "", "exclude bt trackers")
	fs.BoolVar(&btLoadSavedMeta, "bt-load-saved-metadata", false, "load saved bt metadata")
	fs.BoolVar(&btSaveMetadata, "bt-save-metadata", false, "save bt metadata")
	fs.BoolVar(&checkIntegrity, "V", false, "check integrity")
	fs.BoolVar(&checkIntegrity, "check-integrity", false, "check integrity")
	fs.BoolVar(&forceSave, "force-save", false, "force save")
	fs.StringVar(&gid, "gid", "", "set gid")
	fs.StringVar(&checksum, "checksum", "", "checksum")
	fs.Float64Var(&seedRatio, "seed-ratio", 0, "seed ratio")
	fs.Int64Var(&seedTime, "seed-time", 0, "seed time in minutes")
	fs.StringVar(&saveSess, "save-session", "", "save session path")
	fs.StringVar(&maxDownloadStr, "max-download-limit", "", "download limit")
	fs.Int64Var(&saveSessInterval, "save-session-interval", 0, "save session interval seconds")
	fs.StringVar(&maxOverallDLStr, "max-overall-download-limit", "", "overall download limit")
	fs.StringVar(&maxOverallULStr, "max-overall-upload-limit", "", "overall upload limit")
	fs.StringVar(&minSplitSizeStr, "min-split-size", "", "min split size")
	fs.IntVar(&maxTries, "max-tries", 0, "max tries")
	fs.StringVar(&lowestSpeedStr, "lowest-speed-limit", "", "lowest speed limit")
	fs.StringVar(&fileAllocation, "file-allocation", "", "file allocation mode")
	fs.StringVar(&httpUser, "http-user", "", "http username")
	fs.StringVar(&httpPasswd, "http-passwd", "", "http password")
	fs.StringVar(&ftpUser, "ftp-user", "", "ftp username")
	fs.StringVar(&ftpPasswd, "ftp-passwd", "", "ftp password")
	fs.StringVar(&httpUA, "http-user-agent", "", "http user agent")
	fs.StringVar(&httpUA, "user-agent", "", "user agent")
	fs.StringVar(&httpRef, "http-referer", "", "http referer")
	fs.StringVar(&httpProxy, "http-proxy", "", "http proxy")
	fs.StringVar(&httpsProxy, "https-proxy", "", "https proxy")
	fs.StringVar(&allProxy, "all-proxy", "", "all proxy")
	fs.StringVar(&noProxy, "no-proxy", "", "no proxy")
	fs.IntVar(&maxConnPerServer, "max-connection-per-server", 0, "max connections per server")
	fs.IntVar(&maxConnPerServer, "x", 0, "max connections per server")
	fs.IntVar(&split, "split", 0, "split count")
	fs.IntVar(&split, "s", 0, "split count")
	fs.BoolVar(&followTorrent, "follow-torrent", false, "follow torrent/metalink downloads")
	fs.BoolVar(&followMetalink, "follow-metalink", false, "follow metalink downloads")
	fs.BoolVar(&pauseMetadata, "pause-metadata", false, "pause metadata downloads")
	fs.BoolVar(&checkCertificate, "check-certificate", false, "check certificate")
	fs.BoolVar(&ed2kEnable, "ed2k-enable", false, "enable ed2k")
	fs.IntVar(&ed2kListenPort, "ed2k-listen-port", 0, "ed2k listen port")
	fs.IntVar(&ed2kServerPort, "ed2k-server-port", 0, "ed2k server port")
	fs.IntVar(&ed2kMaxSources, "ed2k-max-sources", 0, "ed2k max sources")
	fs.BoolVar(&ed2kKadEnable, "ed2k-kad-enable", false, "enable ed2k kad")
	fs.BoolVar(&ed2kServerEnable, "ed2k-server-enable", false, "enable ed2k server")
	fs.BoolVar(&ed2kAICHEnable, "ed2k-aich-enable", false, "enable ed2k aich")
	fs.BoolVar(&ed2kSourceExchange, "ed2k-source-exchange", false, "enable ed2k source exchange")
	fs.IntVar(&ed2kUploadSlots, "ed2k-upload-slots", 0, "ed2k upload slots")

	knownFlags := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		knownFlags[f.Name] = true
	})
	filteredArgs, unknown := filterUnknownDaemonArgs(args, knownFlags)
	opts.unknownWarnings = unknown

	if err := fs.Parse(filteredArgs); err != nil {
		return opts, err
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "conf", "conf-path":
			opts.confSeen = true
			opts.configPath = configPath
		case "no-conf":
			opts.noConf = noConf
		case "quiet", "q":
			opts.values["quiet"] = quiet
		case "d", "dir":
			opts.values["dir"] = dir
		case "data-dir":
			opts.values["data-dir"] = dataDir
		case "i", "input-file":
			opts.inputFile = inputFile
		case "j", "max-concurrent-downloads":
			opts.values["max-concurrent-downloads"] = maxConcurrentDownloads
		case "max-download-limit":
			if parsed, err := config.ParseSpeedBytes(maxDownloadStr); err == nil {
				opts.values["max-download-limit"] = parsed
			}
		case "D", "daemon":
			opts.values["daemon"] = daemon
		case "c", "continue":
			opts.values["continue"] = continueDownloads
		case "l", "log":
			opts.values["log"] = logPath
		case "log-level":
			opts.values["log-level"] = logLevel
		case "enable-rpc":
			opts.values["enable-rpc"] = enableRPC
		case "rpc-listen-all":
			opts.values["rpc-listen-all"] = rpcListenAll
		case "rpc-allow-origin-all":
			opts.values["rpc-allow-origin-all"] = rpcAllowOriginAll
		case "rpc-max-request-size":
			if parsed, err := config.ParseSpeedBytes(rpcMaxRequestSizeStr); err == nil {
				opts.values["rpc-max-request-size"] = parsed
			}
		case "rpc-listen-port":
			opts.values["rpc-listen-port"] = rpcListenPort
		case "rpc-secret":
			opts.values["rpc-secret"] = rpcSecret
		case "rpc-strict-auth":
			opts.values["rpc-strict-auth"] = rpcStrictAuth
		case "aria2-compat-mode":
			opts.values["aria2-compat-mode"] = aria2CompatMode
		case "enable-websocket":
			opts.values["enable-websocket"] = enableWebSocket
		case "allow-overwrite":
			opts.values["allow-overwrite"] = allowOverwrite
		case "auto-file-renaming":
			opts.values["auto-file-renaming"] = autoFileRenaming
		case "pause":
			opts.values["pause"] = pause
		case "o", "out":
			opts.startup["out"] = outName
		case "listen-port":
			if parsed, err := config.ParsePortSpec(listenPortStr); err == nil {
				opts.values["listen-port"] = parsed
			}
		case "enable-dht":
			opts.values["enable-dht"] = enableDHT
		case "enable-dht6":
			opts.values["enable-dht6"] = enableDHT6
		case "dht-file-path":
			opts.values["dht-file-path"] = dhtFilePath
		case "dht-file-path6":
			opts.values["dht-file-path6"] = dhtFilePath6
		case "dht-listen-port":
			if parsed, err := config.ParsePortSpec(dhtListenPortStr); err == nil {
				opts.values["dht-listen-port"] = parsed
			}
		case "bt-enable-lpd":
			opts.values["bt-enable-lpd"] = btEnableLPD
		case "bt-max-peers":
			opts.values["bt-max-peers"] = btMaxPeers
		case "bt-force-encryption":
			opts.values["bt-force-encryption"] = btForceEncryption
		case "bt-require-crypto":
			opts.values["bt-require-crypto"] = btRequireCrypto
		case "bt-min-crypto-level":
			opts.values["bt-min-crypto-level"] = btMinCryptoLevel
		case "bt-tracker":
			opts.values["bt-tracker"] = btTracker
		case "bt-exclude-tracker":
			opts.values["bt-exclude-tracker"] = btExcludeTracker
		case "bt-load-saved-metadata":
			opts.values["bt-load-saved-metadata"] = btLoadSavedMeta
		case "bt-save-metadata":
			opts.values["bt-save-metadata"] = btSaveMetadata
		case "V", "check-integrity":
			opts.startup["check-integrity"] = fmt.Sprintf("%t", checkIntegrity)
		case "force-save":
			opts.startup["force-save"] = fmt.Sprintf("%t", forceSave)
		case "gid":
			opts.startup["gid"] = gid
		case "checksum":
			opts.startup["checksum"] = checksum
		case "seed-ratio":
			opts.values["seed-ratio"] = seedRatio
		case "seed-time":
			opts.values["seed-time"] = seedTime
		case "save-session":
			opts.values["save-session"] = saveSess
		case "save-session-interval":
			opts.values["save-session-interval"] = saveSessInterval
		case "max-overall-download-limit":
			if parsed, err := config.ParseSpeedBytes(maxOverallDLStr); err == nil {
				opts.values["max-overall-download-limit"] = parsed
			}
		case "max-overall-upload-limit":
			if parsed, err := config.ParseSpeedBytes(maxOverallULStr); err == nil {
				opts.values["max-overall-upload-limit"] = parsed
			}
		case "min-split-size":
			if parsed, err := config.ParseSpeedBytes(minSplitSizeStr); err == nil {
				opts.values["min-split-size"] = parsed
			}
		case "max-tries":
			opts.values["max-tries"] = maxTries
		case "lowest-speed-limit":
			if parsed, err := config.ParseSpeedBytes(lowestSpeedStr); err == nil {
				opts.values["lowest-speed-limit"] = parsed
			}
		case "file-allocation":
			opts.values["file-allocation"] = fileAllocation
		case "http-user":
			opts.values["http-user"] = httpUser
		case "http-passwd":
			opts.values["http-passwd"] = httpPasswd
		case "ftp-user":
			opts.values["ftp-user"] = ftpUser
		case "ftp-passwd":
			opts.values["ftp-passwd"] = ftpPasswd
		case "http-user-agent", "user-agent":
			opts.values["http-user-agent"] = httpUA
			opts.values["user-agent"] = httpUA
		case "http-referer":
			opts.values["http-referer"] = httpRef
		case "http-proxy":
			opts.values["http-proxy"] = httpProxy
		case "https-proxy":
			opts.values["https-proxy"] = httpsProxy
		case "all-proxy":
			opts.values["all-proxy"] = allProxy
		case "no-proxy":
			opts.values["no-proxy"] = noProxy
		case "max-connection-per-server":
			opts.values["max-connection-per-server"] = maxConnPerServer
			opts.startup["max-connection-per-server"] = fmt.Sprintf("%d", maxConnPerServer)
		case "x":
			opts.values["max-connection-per-server"] = maxConnPerServer
			opts.startup["max-connection-per-server"] = fmt.Sprintf("%d", maxConnPerServer)
		case "split":
			opts.values["split"] = split
			opts.startup["split"] = fmt.Sprintf("%d", split)
		case "s":
			opts.values["split"] = split
			opts.startup["split"] = fmt.Sprintf("%d", split)
		case "follow-torrent":
			opts.values["follow-torrent"] = followTorrent
		case "follow-metalink":
			opts.values["follow-metalink"] = followMetalink
		case "pause-metadata":
			opts.values["pause-metadata"] = pauseMetadata
		case "check-certificate":
			opts.values["check-certificate"] = checkCertificate
		case "ed2k-enable":
			opts.values["ed2k-enable"] = ed2kEnable
		case "ed2k-listen-port":
			opts.values["ed2k-listen-port"] = ed2kListenPort
		case "ed2k-server-port":
			opts.values["ed2k-server-port"] = ed2kServerPort
		case "ed2k-max-sources":
			opts.values["ed2k-max-sources"] = ed2kMaxSources
		case "ed2k-kad-enable":
			opts.values["ed2k-kad-enable"] = ed2kKadEnable
		case "ed2k-server-enable":
			opts.values["ed2k-server-enable"] = ed2kServerEnable
		case "ed2k-aich-enable":
			opts.values["ed2k-aich-enable"] = ed2kAICHEnable
		case "ed2k-source-exchange":
			opts.values["ed2k-source-exchange"] = ed2kSourceExchange
		case "ed2k-upload-slots":
			opts.values["ed2k-upload-slots"] = ed2kUploadSlots
		}
	})

	opts.uris = append(opts.uris, fs.Args()...)
	for _, uri := range opts.uris {
		if uri != "" {
			break
		}
	}

	if outName != "" {
		opts.startup["out"] = outName
	}
	if gid != "" {
		opts.startup["gid"] = gid
	}
	if checksum != "" {
		opts.startup["checksum"] = checksum
	}
	if checkIntegrity {
		opts.startup["check-integrity"] = "true"
	}
	if forceSave {
		opts.startup["force-save"] = "true"
	}

	if !opts.confSeen {
		opts.configPath = configPath
	}
	return opts, nil
}

func filterUnknownDaemonArgs(args []string, known map[string]bool) (filtered []string, warnings []string) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			filtered = append(filtered, args[i:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			filtered = append(filtered, arg)
			i++
			continue
		}
		name, hasValue := splitDaemonFlag(arg)
		if known[name] {
			filtered = append(filtered, arg)
			i++
			continue
		}
		if !config.IsKnownIgnoredOption(name) {
			warnings = append(warnings, fmt.Sprintf("unknown option ignored: %s", arg))
		}
		i++
		if !hasValue && i < len(args) && looksLikeUnknownFlagValue(args[i]) {
			i++
		}
	}
	return filtered, warnings
}

func splitDaemonFlag(arg string) (name string, hasValue bool) {
	trimmed := strings.TrimLeft(arg, "-")
	if idx := strings.IndexByte(trimmed, '='); idx >= 0 {
		return trimmed[:idx], true
	}
	return trimmed, false
}

func looksLikeUnknownFlagValue(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	lower := strings.ToLower(value)
	if strings.Contains(value, "://") || strings.HasPrefix(lower, "magnet:") {
		return false
	}
	if strings.HasSuffix(lower, ".torrent") {
		return false
	}
	return true
}

// applyDaemonCLIOptions 将命令行覆盖应用到配置对象。
func applyDaemonCLIOptions(cfg *config.Config, opts daemonCLIOptions) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	for key, value := range opts.values {
		switch key {
		case "dir":
			cfg.Dir = value.(string)
		case "data-dir":
			cfg.DataDir = value.(string)
		case "max-concurrent-downloads":
			cfg.MaxConcurrentDownloads = value.(int)
		case "max-download-limit":
			cfg.MaxDownloadLimit = value.(int64)
		case "daemon":
			cfg.Daemon = value.(bool)
		case "continue":
			cfg.ContinueDownloads = value.(bool)
		case "log":
			cfg.LogPath = value.(string)
		case "log-level":
			cfg.LogLevel = value.(string)
		case "enable-rpc":
			cfg.EnableRPC = value.(bool)
		case "rpc-listen-all":
			cfg.RPCListenAll = value.(bool)
		case "rpc-allow-origin-all":
			cfg.RPCAllowOriginAll = value.(bool)
		case "rpc-max-request-size":
			cfg.RPCMaxRequestSize = value.(int64)
		case "rpc-listen-port":
			cfg.RPCListenPort = value.(int)
		case "rpc-secret":
			cfg.RPCSecret = value.(string)
		case "rpc-strict-auth":
			cfg.RPCStrictAuth = value.(bool)
		case "aria2-compat-mode":
			cfg.Aria2CompatMode = value.(bool)
		case "enable-websocket":
			cfg.EnableWebSocket = value.(bool)
		case "allow-overwrite":
			cfg.AllowOverwrite = value.(bool)
		case "auto-file-renaming":
			cfg.AutoFileRenaming = value.(bool)
		case "pause":
			cfg.Pause = value.(bool)
		case "listen-port":
			cfg.ListenPort = value.(int)
		case "enable-dht":
			cfg.EnableDHT = value.(bool)
		case "enable-dht6":
			cfg.EnableDHT6 = value.(bool)
		case "dht-file-path":
			cfg.DHTFilePath = value.(string)
		case "dht-file-path6":
			cfg.DHTFilePath6 = value.(string)
		case "dht-listen-port":
			cfg.DHTListenPort = value.(int)
		case "bt-max-peers":
			cfg.BTMaxPeers = value.(int)
		case "bt-force-encryption":
			cfg.BTForceEncryption = value.(bool)
		case "bt-require-crypto":
			cfg.BTRequireCrypto = value.(bool)
		case "bt-min-crypto-level":
			cfg.BTMinCryptoLevel = value.(string)
		case "bt-tracker":
			cfg.BTTracker = value.(string)
		case "bt-exclude-tracker":
			cfg.BTExcludeTracker = value.(string)
		case "bt-load-saved-metadata":
			cfg.BTLoadSavedMetadata = value.(bool)
		case "bt-save-metadata":
			cfg.BTSaveMetadata = value.(bool)
		case "seed-ratio":
			cfg.SeedRatio = value.(float64)
		case "seed-time":
			cfg.SeedTime = time.Duration(value.(int64)) * time.Minute
		case "save-session":
			cfg.SaveSession = value.(string)
		case "save-session-interval":
			cfg.SaveSessionInterval = time.Duration(value.(int64)) * time.Second
		case "max-overall-download-limit":
			cfg.MaxOverallDownloadLimit = value.(int64)
		case "max-overall-upload-limit":
			cfg.MaxOverallUploadLimit = value.(int64)
		case "http-user-agent":
			cfg.HTTPUserAgent = value.(string)
		case "user-agent":
			cfg.HTTPUserAgent = value.(string)
		case "http-referer":
			cfg.HTTPReferer = value.(string)
		case "http-proxy":
			cfg.HTTPProxy = value.(string)
		case "https-proxy":
			cfg.HTTPSProxy = value.(string)
		case "all-proxy":
			cfg.AllProxy = value.(string)
		case "no-proxy":
			cfg.NoProxy = value.(string)
		case "max-connection-per-server":
			cfg.MaxConnectionPerServer = value.(int)
		case "split":
			cfg.Split = value.(int)
		case "follow-torrent":
			cfg.FollowTorrent = value.(bool)
		case "follow-metalink":
			cfg.FollowMetalink = value.(bool)
		case "pause-metadata":
			cfg.PauseMetadata = value.(bool)
		case "check-certificate":
			cfg.CheckCertificate = value.(bool)
		case "ed2k-enable":
			cfg.ED2KEnable = value.(bool)
		case "ed2k-listen-port":
			cfg.ED2KListenPort = value.(int)
		case "ed2k-server-port":
			cfg.ED2KServerPort = value.(int)
		case "ed2k-max-sources":
			cfg.ED2KMaxSources = value.(int)
		case "ed2k-kad-enable":
			cfg.ED2KKadEnable = value.(bool)
		case "ed2k-server-enable":
			cfg.ED2KServerEnable = value.(bool)
		case "ed2k-aich-enable":
			cfg.ED2KAICHEnable = value.(bool)
		case "ed2k-source-exchange":
			cfg.ED2KSourceExchange = value.(bool)
		case "ed2k-upload-slots":
			cfg.ED2KUploadSlots = value.(int)
		case "quiet":
			cfg.Quiet = value.(bool)
		case "bt-enable-lpd":
			cfg.BTEnableLPD = value.(bool)
		case "min-split-size":
			cfg.MinSplitSize = value.(int64)
		case "max-tries":
			cfg.MaxTries = value.(int)
		case "lowest-speed-limit":
			cfg.LowestSpeedLimit = value.(int64)
		case "file-allocation":
			cfg.FileAllocation = value.(string)
		case "http-user":
			cfg.HTTPUser = value.(string)
		case "http-passwd":
			cfg.HTTPPasswd = value.(string)
		case "ftp-user":
			cfg.FTPUser = value.(string)
		case "ftp-passwd":
			cfg.FTPPasswd = value.(string)
		}
	}
	return nil
}
