package aria2

import (
	"fmt"

	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// unimplementedOptions 已通过格式校验但尚未在驱动层实现语义的选项。
var unimplementedOptions = map[string]struct{}{}

// storeOnlyOptions 记录已接受并存储、但运行时语义未完整实现的选项（供审计与文档引用，不触发拒绝）。
var storeOnlyOptions = map[string]string{
	"bt-request-peer-speed-limit": "BT peer 请求限速（仅存储）",
	"bt-enable-lpd":               "BT LPD（仅存储）",
	"bt-remove-unselected-file":   "BT 删除未选文件（仅存储）",
	"bt-detach-seed-only":         "BT 仅分离做种（仅存储）",
	"max-upload-limit":            "任务级上传限速（BT 等协议仅存储；全局限速用 max-overall-upload-limit）",
	"max-download-limit":          "任务级下载限速（BT 仅存储；HTTP/FTP/SFTP 已实现）",
}

// startupOnlyOptions 仅 daemon 启动配置生效，changeGlobalOption 写入后静默忽略（见 globalDisallowedOptions）。
var startupOnlyOptions = map[string]string{
	"listen-port":     "BT 监听端口（仅启动配置）",
	"dht-listen-port": "DHT 监听端口（仅启动配置）",
	"bt-max-peers":    "BT 最大 peer 数（仅启动配置）",
	"enable-dht":      "DHT 开关（仅启动配置）",
	"enable-dht6":     "DHT IPv6 开关（仅启动配置）",
}

// optionAliases 将 aria2 别名选项规范为 go-aria2 内部使用的键名。
var optionAliases = map[string]string{
	"referer":    "http-referer",
	"user-agent": "http-user-agent",
}

func normalizeOptionKeys(opts map[string]string) {
	for alias, target := range optionAliases {
		value, ok := opts[alias]
		if !ok {
			continue
		}
		if _, exists := opts[target]; !exists {
			opts[target] = value
		}
		delete(opts, alias)
	}
}

func rejectUnimplementedOptions(opts map[string]string) error {
	for key := range opts {
		if _, unsupported := unimplementedOptions[key]; unsupported {
			return unimplementedOptionError(key)
		}
	}
	return nil
}

func unimplementedOptionError(key string) error {
	return jsonrpc.NewError(jsonrpc.CodeInvalidParams,
		fmt.Sprintf("Option not implemented: %s", key))
}
