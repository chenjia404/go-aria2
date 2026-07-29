package aria2

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// decodeTorrentPayload 解析 aria2.addTorrent 首参：Base64 字符串、Node Buffer JSON 或字节数组。
func decodeTorrentPayload(value any) ([]byte, error) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "torrent payload must not be empty")
		}
		payload, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, fmt.Sprintf("invalid torrent payload: %v", err))
		}
		return payload, nil
	case map[string]any:
		typeName, _ := v["type"].(string)
		if !strings.EqualFold(typeName, "Buffer") {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "torrent payload must be base64 string, Buffer object, or byte array")
		}
		rawData, ok := v["data"].([]any)
		if !ok {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "invalid Buffer data")
		}
		return bytesFromAnySlice(rawData)
	case []any:
		return bytesFromAnySlice(v)
	default:
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "torrent payload must be base64 string, Buffer object, or byte array")
	}
}

func bytesFromAnySlice(values []any) ([]byte, error) {
	out := make([]byte, 0, len(values))
	for i, item := range values {
		switch n := item.(type) {
		case float64:
			if n < 0 || n > 255 || n != float64(byte(n)) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, fmt.Sprintf("invalid byte at index %d", i))
			}
			out = append(out, byte(n))
		case int:
			if n < 0 || n > 255 {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, fmt.Sprintf("invalid byte at index %d", i))
			}
			out = append(out, byte(n))
		default:
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, fmt.Sprintf("invalid byte at index %d", i))
		}
	}
	if len(out) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "torrent payload must not be empty")
	}
	return out, nil
}

// parseAddTorrentParams 解析 aria2.addTorrent 参数，兼容两参数与三参数调用。
func parseAddTorrentParams(params []any) (payload []byte, uris []string, options map[string]string, err error) {
	if len(params) == 0 {
		return nil, nil, nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "torrent is required")
	}

	payload, err = decodeTorrentPayload(params[0])
	if err != nil {
		return nil, nil, nil, err
	}

	options = map[string]string{}
	if len(params) == 1 {
		return payload, nil, options, nil
	}

	switch second := params[1].(type) {
	case nil:
		if len(params) >= 3 {
			options = parseOptions(params[2])
		}
	case map[string]any:
		options = parseOptions(second)
	case []any:
		uris = parseStringList(second)
		if len(params) >= 3 {
			options = parseOptions(params[2])
		}
	default:
		// JSON 解码后 null 为 nil；部分客户端传空字符串等非法类型
		if params[1] == nil {
			if len(params) >= 3 {
				options = parseOptions(params[2])
			}
			break
		}
		return nil, nil, nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "second param must be options object, uri array, or null")
	}

	return payload, uris, options, nil
}
