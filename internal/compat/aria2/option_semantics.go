package aria2

import (
	"fmt"

	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// unimplementedOptions 已通过格式校验但尚未在驱动层实现语义的选项。
var unimplementedOptions = map[string]struct{}{
	"min-split-size": {},
	"index-out":      {},
	"piece-length":   {},
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
