package aria2

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/migrate/aria2session"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// ED2KNativeAPI 为 native.* ED2K 扩展提供可选后端（通常由 HTTPGateway 适配）。
type ED2KNativeAPI interface {
	Servers(ctx context.Context) ([]map[string]any, error)
	ConnectServer(ctx context.Context, addr string) error
	KadStatus(ctx context.Context) (map[string]any, error)
	ConnectKad(ctx context.Context, nodes []string) error
}

var nativeMethodNames = []string{
	"native.getVersion",
	"native.getTask",
	"native.getTaskMeta",
	"native.exportSession",
	"native.importSession",
	"native.importFromAria2RPC",
	"native.getProtocolStats",
	"native.getBtTrackers",
	"native.getBtPeers",
	"native.reannounceTorrent",
	"native.forcePieceCheck",
	"native.addEd2k",
	"native.getEd2kSources",
	"native.getEd2kServers",
	"native.connectEd2kServer",
	"native.getKadStatus",
	"native.connectKad",
	"native.recheckEd2kFile",
}

func (s *Service) invokeNative(ctx context.Context, method string, params []any) (any, error) {
	switch method {
	case "native.getVersion":
		return s.nativeGetVersion(), nil
	case "native.getTask":
		return s.nativeGetTask(ctx, params)
	case "native.getTaskMeta":
		return s.nativeGetTaskMeta(ctx, params)
	case "native.exportSession":
		return s.saveSession(ctx, params)
	case "native.importSession":
		return s.nativeImportSession(ctx, params)
	case "native.importFromAria2RPC":
		return s.nativeImportFromAria2RPC(ctx, params)
	case "native.getProtocolStats":
		return s.nativeGetProtocolStats(), nil
	case "native.getBtTrackers":
		return s.nativeGetBtTrackers(ctx, params)
	case "native.getBtPeers":
		return s.getPeers(ctx, params)
	case "native.reannounceTorrent":
		return s.nativeReannounceTorrent(ctx, params)
	case "native.forcePieceCheck":
		return s.nativeForcePieceCheck(ctx, params)
	case "native.addEd2k":
		return s.nativeAddEd2k(ctx, params)
	case "native.getEd2kSources":
		return s.nativeGetEd2kSources(ctx, params)
	case "native.getEd2kServers":
		return s.nativeGetEd2kServers(ctx)
	case "native.connectEd2kServer":
		return s.nativeConnectEd2kServer(ctx, params)
	case "native.getKadStatus":
		return s.nativeGetKadStatus(ctx)
	case "native.connectKad":
		return s.nativeConnectKad(ctx, params)
	case "native.recheckEd2kFile":
		return s.nativeRecheckEd2kFile(ctx, params)
	default:
		return nil, jsonrpc.NewError(jsonrpc.CodeMethodNotFound, "method not found")
	}
}

func (s *Service) nativeGetVersion() map[string]any {
	base := s.getVersion()
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	out["implementation"] = "go-aria2"
	out["nativeRpc"] = true
	return out
}

func (s *Service) nativeGetTask(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	item, err := s.manager.GetNativeTask(ctx, gid)
	if err != nil {
		return nil, err
	}
	return taskToNativeMap(item), nil
}

func (s *Service) nativeGetTaskMeta(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	return s.manager.GetNativeTaskMeta(ctx, gid)
}

func (s *Service) nativeImportSession(ctx context.Context, params []any) (any, error) {
	path := s.sessionPath
	if len(params) > 0 {
		if value, ok := params[0].(string); ok && strings.TrimSpace(value) != "" {
			path = value
		}
	}
	if strings.TrimSpace(path) == "" {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "session path is required")
	}
	count, err := s.manager.ImportSessionFrom(ctx, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"imported": count}, nil
}

func (s *Service) nativeImportFromAria2RPC(ctx context.Context, params []any) (any, error) {
	endpoint, err := stringParam(params, 0, "endpoint")
	if err != nil {
		return nil, err
	}
	secret := s.rpcSecret
	if len(params) > 1 {
		if value, ok := params[1].(string); ok {
			secret = value
		}
	}
	importer := &aria2session.Importer{Manager: s.manager}
	imported, err := aria2session.ImportFromAria2RPC(ctx, importer, endpoint, secret)
	if err != nil {
		return nil, err
	}
	return map[string]any{"imported": len(imported)}, nil
}

func (s *Service) nativeGetProtocolStats() map[string]any {
	stats := s.manager.GetProtocolStats()
	return map[string]any{"protocols": stats.ByProtocol}
}

func (s *Service) nativeGetBtTrackers(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	return s.manager.GetBTTrackers(ctx, gid)
}

func (s *Service) nativeReannounceTorrent(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	if err := s.manager.ReannounceTorrent(ctx, gid); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) nativeForcePieceCheck(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	if err := s.manager.ForcePieceCheck(ctx, gid); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) nativeAddEd2k(ctx context.Context, params []any) (any, error) {
	if len(params) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "ed2k link is required")
	}
	link, ok := params[0].(string)
	if !ok || strings.TrimSpace(link) == "" {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "ed2k link must be a non-empty string")
	}
	options := map[string]string{}
	if len(params) >= 2 {
		options = parseOptions(params[1])
	}
	created, err := s.manager.Add(ctx, task.AddTaskInput{
		URI:     link,
		Options: options,
	})
	if err != nil {
		return nil, err
	}
	return created.GID, nil
}

func (s *Service) nativeGetEd2kSources(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	return s.manager.GetED2KSources(ctx, gid)
}

func (s *Service) nativeGetEd2kServers(ctx context.Context) (any, error) {
	if s.ed2kNative == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "ed2k native API is not available")
	}
	return s.ed2kNative.Servers(ctx)
}

func (s *Service) nativeConnectEd2kServer(ctx context.Context, params []any) (any, error) {
	if s.ed2kNative == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "ed2k native API is not available")
	}
	addr, err := stringParam(params, 0, "address")
	if err != nil {
		return nil, err
	}
	if err := s.ed2kNative.ConnectServer(ctx, addr); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) nativeGetKadStatus(ctx context.Context) (any, error) {
	if s.ed2kNative == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "ed2k native API is not available")
	}
	return s.ed2kNative.KadStatus(ctx)
}

func (s *Service) nativeConnectKad(ctx context.Context, params []any) (any, error) {
	if s.ed2kNative == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "ed2k native API is not available")
	}
	var nodes []string
	if len(params) > 0 {
		nodes = parseStringList(params[0])
	}
	if len(nodes) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "kad nodes are required")
	}
	if err := s.ed2kNative.ConnectKad(ctx, nodes); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) nativeRecheckEd2kFile(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	if err := s.manager.RecheckEd2kFile(ctx, gid); err != nil {
		return nil, err
	}
	return "OK", nil
}

func taskToNativeMap(item *task.Task) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	data, err := json.Marshal(item)
	if err != nil {
		return map[string]any{"gid": item.GID}
	}
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}
