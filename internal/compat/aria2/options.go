package aria2

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// hiddenOptions 与 aria2 一致：getOption/getGlobalOption 不返回这些内部选项。
var hiddenOptions = map[string]struct{}{
	"startup-idle-time": {},
	// aria2 getOption 不返回 pause（仅 add* 时作为输入选项）。
	"pause": {},
}

// taskDisallowedOptions 仅允许 changeGlobalOption 修改；changeOption 传入时静默忽略。
var taskDisallowedOptions = map[string]struct{}{
	"max-overall-download-limit": {},
	"max-overall-upload-limit":   {},
}

// globalDisallowedOptions 不允许通过 changeGlobalOption 在运行时修改（静默忽略）。
var globalDisallowedOptions = map[string]struct{}{
	"enable-rpc":           {},
	"rpc-listen-port":      {},
	"rpc-listen-all":       {},
	"rpc-secret":           {},
	"rpc-allow-origin-all": {},
	"rpc-max-request-size": {},
	"enable-websocket":     {},
	"daemon":               {},
	"log":                  {},
	"log-level":            {},
	"save-session":         {},
	"input-file":           {},
}

var fileAllocationValues = map[string]struct{}{
	"none":    {},
	"prealloc": {},
	"trunc":   {},
	"falloc":  {},
}

// validateAddOptions 校验 addUri/addTorrent/addMetalink 中的已知选项值。
func validateAddOptions(opts map[string]string) error {
	for key, value := range opts {
		if err := validateKnownOption(key, value); err != nil {
			return err
		}
		opts[key] = normalizeOptionValue(key, value)
	}
	return nil
}

// prepareChangeTaskOptions 过滤全局专属选项并校验任务级选项值。
func prepareChangeTaskOptions(opts map[string]string) (map[string]string, error) {
	filtered := make(map[string]string, len(opts))
	for key, value := range opts {
		if _, disallowed := taskDisallowedOptions[key]; disallowed {
			continue
		}
		if err := validateKnownOption(key, value); err != nil {
			return nil, err
		}
		filtered[key] = normalizeOptionValue(key, value)
	}
	return filtered, nil
}

// prepareChangeGlobalOptions 过滤不可运行时修改的选项并校验全局选项值。
func prepareChangeGlobalOptions(opts map[string]string) (map[string]string, error) {
	filtered := make(map[string]string, len(opts))
	for key, value := range opts {
		if _, disallowed := globalDisallowedOptions[key]; disallowed {
			continue
		}
		if isTaskOnlyOption(key) {
			continue
		}
		if err := validateKnownOption(key, value); err != nil {
			return nil, err
		}
		filtered[key] = normalizeOptionValue(key, value)
	}
	return filtered, nil
}

// isTaskOnlyOption 标识仅适用于单个下载任务的选项（不应通过 changeGlobalOption 修改）。
func isTaskOnlyOption(key string) bool {
	switch key {
	case "out", "pause", "gid", "index-out", "select-file", "split", "min-split-size",
		"max-download-limit", "max-upload-limit", "header", "referer", "user-agent",
		"http-user-agent", "http-referer", "piece-length", "follow-torrent":
		return true
	default:
		return false
	}
}

func validateKnownOption(key, value string) error {
	switch key {
	case "file-allocation":
		if _, ok := fileAllocationValues[strings.ToLower(strings.TrimSpace(value))]; !ok {
			return optionError(key, value)
		}
	case "max-download-limit", "max-upload-limit", "max-overall-download-limit", "max-overall-upload-limit",
		"bt-request-peer-speed-limit":
		if _, err := parseSpeedLimit(value); err != nil {
			return optionError(key, value)
		}
	case "pause", "allow-overwrite", "auto-file-renaming", "continue", "check-certificate",
		"enable-dht", "enable-dht6", "bt-enable-lpd", "bt-remove-unselected-file",
		"bt-detach-seed-only", "bt-save-metadata", "rpc-save-upload-metadata":
		if !isBoolOption(value) {
			return optionError(key, value)
		}
	case "split", "max-connection-per-server", "min-split-size", "max-concurrent-downloads",
		"listen-port", "dht-listen-port", "bt-max-peers":
		if _, err := parsePositiveInt(value); err != nil {
			return optionError(key, value)
		}
	case "seed-ratio":
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return optionError(key, value)
		}
	case "seed-time", "connect-timeout", "timeout", "retry-wait":
		if _, err := parsePositiveInt(value); err != nil {
			return optionError(key, value)
		}
	}
	return nil
}

func normalizeOptionValue(key, value string) string {
	switch key {
	case "max-download-limit", "max-upload-limit", "max-overall-download-limit", "max-overall-upload-limit",
		"bt-request-peer-speed-limit":
		if parsed, err := parseSpeedLimit(value); err == nil {
			return strconv.FormatInt(parsed, 10)
		}
	}
	return value
}

func optionError(key, value string) error {
	return jsonrpc.NewError(jsonrpc.CodeInvalidParams,
		fmt.Sprintf("Invalid option value: %s=%s", key, value))
}

func isBoolOption(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no":
		return true
	default:
		return false
	}
}

func parsePositiveInt(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return parsed, nil
}

// parseSpeedLimit 解析 aria2 风格的速度限制（支持 K/M/G 后缀，基数 1024）。
func parseSpeedLimit(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, fmt.Errorf("empty speed limit")
	}
	multiplier := int64(1)
	if len(text) >= 2 {
		suffix := strings.ToUpper(text[len(text)-1:])
		switch suffix {
		case "K":
			multiplier = 1024
			text = text[:len(text)-1]
		case "M":
			multiplier = 1024 * 1024
			text = text[:len(text)-1]
		case "G":
			multiplier = 1024 * 1024 * 1024
			text = text[:len(text)-1]
		}
	}
	base, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0, err
	}
	return base * multiplier, nil
}

func filterHiddenOptions(opts map[string]string) map[string]string {
	if len(opts) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(opts))
	for key, value := range opts {
		if _, hidden := hiddenOptions[key]; hidden {
			continue
		}
		out[key] = value
	}
	return out
}

// isValidURI 检查 aria2 addUri 接受的 URI 格式（需含 scheme 或为 magnet 链接）。
func isValidURI(uri string) bool {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(uri), "magnet:?") {
		return true
	}
	return strings.Contains(uri, "://")
}
