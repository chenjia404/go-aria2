package aria2

import (
	"fmt"
	"strings"

	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// supportedAddURISchemes 与 getVersion.supportedProtocols 对齐的 addUri scheme。
var supportedAddURISchemes = map[string]struct{}{
	"ftp":   {},
	"http":  {},
	"https": {},
	"ed2k":  {},
	"sftp":  {},
}

func validateAddURIScheme(uri string) error {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return jsonrpc.NewError(jsonrpc.CodeInvalidParams, "uri must be a non-empty string")
	}
	if strings.HasPrefix(strings.ToLower(uri), "magnet:?") {
		return nil
	}
	idx := strings.Index(uri, "://")
	if idx < 0 {
		return jsonrpc.NewError(jsonrpc.CodeInvalidParams, "uri must be a valid URI")
	}
	scheme := strings.ToLower(uri[:idx])
	if _, ok := supportedAddURISchemes[scheme]; ok {
		return nil
	}
	return jsonrpc.NewError(jsonrpc.CodeInvalidParams,
		fmt.Sprintf("Unsupported URI scheme: %s", scheme))
}

// isValidURI 检查 aria2 addUri 接受的 URI 格式（含 scheme 校验）。
func isValidURI(uri string) bool {
	return validateAddURIScheme(uri) == nil
}
