package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadFile 从 aria2.conf 风格文件加载配置。
func LoadFile(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return Parse(file)
}

// Parse 解析 key=value 配置，保留未知项 warning 而不是直接退出。
func Parse(r io.Reader) (*Config, error) {
	cfg := Default()
	scanner := bufio.NewScanner(r)
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("line %d ignored: invalid format", lineNo))
			continue
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		if err := apply(cfg, key, value); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func apply(cfg *Config, key, value string) error {
	switch key {
	case "enable-rpc":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.EnableRPC = v
	case "rpc-listen-port":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid rpc-listen-port: %w", err)
		}
		cfg.RPCListenPort = v
	case "rpc-listen-all":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.RPCListenAll = v
	case "rpc-allow-origin-all":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.RPCAllowOriginAll = v
	case "rpc-max-request-size":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid rpc-max-request-size: %w", err)
		}
		cfg.RPCMaxRequestSize = v
	case "rpc-secret":
		cfg.RPCSecret = value
	case "enable-websocket":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.EnableWebSocket = v
	case "dir":
		cfg.Dir = value
	case "data-dir":
		cfg.DataDir = value
	case "max-concurrent-downloads":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid max-concurrent-downloads: %w", err)
		}
		cfg.MaxConcurrentDownloads = v
	case "max-overall-download-limit":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid max-overall-download-limit: %w", err)
		}
		cfg.MaxOverallDownloadLimit = v
	case "max-overall-upload-limit":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid max-overall-upload-limit: %w", err)
		}
		cfg.MaxOverallUploadLimit = v
	case "pause":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Pause = v
	case "continue":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.ContinueDownloads = v
	case "daemon":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Daemon = v
	case "log":
		cfg.LogPath = value
	case "log-level":
		cfg.LogLevel = strings.ToLower(value)
	case "listen-port":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid listen-port: %w", err)
		}
		cfg.ListenPort = v
	case "enable-dht":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.EnableDHT = v
	case "bt-max-peers":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid bt-max-peers: %w", err)
		}
		cfg.BTMaxPeers = v
	case "seed-ratio":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid seed-ratio: %w", err)
		}
		cfg.SeedRatio = v
	case "http-user-agent":
		cfg.HTTPUserAgent = value
	case "http-referer":
		cfg.HTTPReferer = value
	case "http-proxy":
		cfg.HTTPProxy = value
	case "https-proxy":
		cfg.HTTPSProxy = value
	case "all-proxy":
		cfg.AllProxy = value
	case "max-connection-per-server":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid max-connection-per-server: %w", err)
		}
		cfg.MaxConnectionPerServer = v
	case "split":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid split: %w", err)
		}
		cfg.Split = v
	case "check-certificate":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.CheckCertificate = v
	case "save-session":
		cfg.SaveSession = value
	case "save-session-interval":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid save-session-interval: %w", err)
		}
		cfg.SaveSessionInterval = time.Duration(v) * time.Second
	case "ed2k-enable":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.ED2KEnable = v
	case "ed2k-listen-port":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ed2k-listen-port: %w", err)
		}
		cfg.ED2KListenPort = v
	case "ed2k-server-port":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ed2k-server-port: %w", err)
		}
		cfg.ED2KServerPort = v
	case "ed2k-max-sources":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ed2k-max-sources: %w", err)
		}
		cfg.ED2KMaxSources = v
	case "ed2k-kad-enable":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.ED2KKadEnable = v
	case "ed2k-server-enable":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.ED2KServerEnable = v
	case "ed2k-aich-enable":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.ED2KAICHEnable = v
	case "ed2k-source-exchange":
		v, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.ED2KSourceExchange = v
	case "ed2k-upload-slots":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ed2k-upload-slots: %w", err)
		}
		cfg.ED2KUploadSlots = v
	default:
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("unknown option ignored: %s=%s", key, value))
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", value)
	}
}
