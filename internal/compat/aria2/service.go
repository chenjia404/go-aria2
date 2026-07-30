package aria2

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/bt"
	"github.com/chenjia404/go-aria2/internal/protocol/common"
	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// Service ??? aria2 ???? JSON-RPC ????????????
type Service struct {
	manager     *manager.Manager
	rpcSecret   string
	strictAuth  bool
	methods     []string
	startedAt   time.Time
	sessionID   string
	sessionPath string
	ed2kNative  ED2KNativeAPI
	onShutdown  func(force bool)
}

// ServiceConfig 配置 aria2 兼容 RPC 服务。
type ServiceConfig struct {
	RPCSecret  string
	StrictAuth bool
}

// NewService 创建 aria2 兼容 JSON-RPC 服务。
func NewService(mgr *manager.Manager, rpcSecret string) *Service {
	return NewServiceWithConfig(mgr, ServiceConfig{RPCSecret: rpcSecret})
}

// NewServiceWithConfig 使用完整配置创建服务。
func NewServiceWithConfig(mgr *manager.Manager, cfg ServiceConfig) *Service {
	methods := []string{
		"aria2.addUri",
		"aria2.addTorrent",
		"aria2.addMetalink",
		"aria2.remove",
		"aria2.forceRemove",
		"aria2.pause",
		"aria2.forcePause",
		"aria2.pauseAll",
		"aria2.forcePauseAll",
		"aria2.unpauseAll",
		"aria2.unpause",
		"aria2.removeDownloadResult",
		"aria2.purgeDownloadResult",
		"aria2.shutdown",
		"aria2.forceShutdown",
		"aria2.tellStatus",
		"aria2.tellActive",
		"aria2.tellWaiting",
		"aria2.tellStopped",
		"aria2.getFiles",
		"aria2.getPeers",
		"aria2.getServers",
		"aria2.getOption",
		"aria2.changeOption",
		"aria2.changePosition",
		"aria2.changeUri",
		"aria2.getUris",
		"aria2.getGlobalOption",
		"aria2.changeGlobalOption",
		"aria2.getGlobalStat",
		"aria2.getVersion",
		"aria2.getSessionInfo",
		"aria2.ping",
		"aria2.saveSession",
		"system.listMethods",
		"system.listNotifications",
		"system.multicall",
	}

	return &Service{
		manager:    mgr,
		rpcSecret:  cfg.RPCSecret,
		strictAuth: cfg.StrictAuth,
		methods:    append(append([]string(nil), methods...), nativeMethodNames...),
		startedAt:  time.Now(),
		sessionID:  newSessionID(),
	}
}

// SetShutdownHook 注册 aria2.shutdown 触发时的回调（由 daemon 注入优雅退出逻辑）。
func (s *Service) SetShutdownHook(fn func(force bool)) {
	s.onShutdown = fn
}

// SetSessionPath 设置 aria2.saveSession 未指定路径时的默认落盘位置。
func (s *Service) SetSessionPath(path string) {
	s.sessionPath = path
}

// SetED2KNativeAPI 注入 ED2K 扩展 RPC 后端（ed2k 未启用时可省略）。
func (s *Service) SetED2KNativeAPI(api ED2KNativeAPI) {
	s.ed2kNative = api
}

// VersionInfo 返回与 aria2.getVersion 一致的结构，供 REST 等适配层使用。
func (s *Service) VersionInfo() map[string]any {
	return s.getVersion()
}

// SessionInfo 返回与 aria2.getSessionInfo 一致的结构，供 REST 等适配层使用。
func (s *Service) SessionInfo() map[string]any {
	return s.getSessionInfo()
}

// Invoke 分发 aria2 兼容 JSON-RPC 方法；若配置了 rpc-secret 则校验 token。
func (s *Service) Invoke(ctx context.Context, method string, params []any) (any, error) {
	if s.requiresAuth(method) {
		authorizedParams, err := s.authorize(params)
		if err != nil {
			return nil, err
		}
		return s.invokeWithoutAuth(ctx, method, authorizedParams)
	}
	return s.invokeWithoutAuth(ctx, method, params)
}

func (s *Service) requiresAuth(method string) bool {
	if s.strictAuth {
		return true
	}
	return method != "system.listMethods" && method != "system.listNotifications" && method != "system.multicall"
}

func (s *Service) invokeWithoutAuth(ctx context.Context, method string, params []any) (any, error) {
	switch method {
	case "aria2.addUri":
		return s.addURI(ctx, params)
	case "aria2.addTorrent":
		return s.addTorrent(ctx, params)
	case "aria2.addMetalink":
		return s.addMetalink(ctx, params)
	case "aria2.remove":
		return s.remove(ctx, params, false)
	case "aria2.forceRemove":
		return s.remove(ctx, params, true)
	case "aria2.pause":
		return s.pause(ctx, params, false)
	case "aria2.forcePause":
		return s.pause(ctx, params, true)
	case "aria2.pauseAll":
		return s.pauseAll(ctx)
	case "aria2.forcePauseAll":
		return s.forcePauseAll(ctx)
	case "aria2.unpause":
		return s.unpause(ctx, params)
	case "aria2.unpauseAll":
		return s.unpauseAll(ctx)
	case "aria2.removeDownloadResult":
		return s.removeDownloadResult(ctx, params)
	case "aria2.purgeDownloadResult":
		return s.purgeDownloadResult(ctx)
	case "aria2.shutdown", "aria2.forceShutdown":
		return s.shutdown(ctx, params, method == "aria2.forceShutdown")
	case "aria2.tellStatus":
		return s.tellStatus(ctx, params)
	case "aria2.tellActive":
		return s.tellActive(ctx, params)
	case "aria2.tellWaiting":
		return s.tellWaiting(ctx, params)
	case "aria2.tellStopped":
		return s.tellStopped(ctx, params)
	case "aria2.getFiles":
		return s.getFiles(ctx, params)
	case "aria2.getPeers":
		return s.getPeers(ctx, params)
	case "aria2.getServers":
		return s.getServers(ctx, params)
	case "aria2.getUris":
		return s.getUris(ctx, params)
	case "aria2.getOption":
		return s.getOption(ctx, params)
	case "aria2.changeOption":
		return s.changeOption(ctx, params)
	case "aria2.changePosition":
		return s.changePosition(ctx, params)
	case "aria2.changeUri":
		return s.changeUri(ctx, params)
	case "aria2.getGlobalOption":
		return s.getGlobalOption(), nil
	case "aria2.changeGlobalOption":
		return s.changeGlobalOption(ctx, params)
	case "aria2.getGlobalStat":
		return toGlobalStatResponse(s.manager.GetGlobalStat()), nil
	case "aria2.getVersion":
		return s.getVersion(), nil
	case "aria2.getSessionInfo":
		return s.getSessionInfo(), nil
	case "aria2.ping":
		return s.ping(ctx, params)
	case "aria2.saveSession":
		return s.saveSession(ctx, params)
	case "system.listMethods":
		return append([]string(nil), s.methods...), nil
	case "system.listNotifications":
		return []string{
			"aria2.onDownloadStart",
			"aria2.onDownloadPause",
			"aria2.onDownloadStop",
			"aria2.onDownloadComplete",
			"aria2.onDownloadError",
			"aria2.onBtDownloadComplete",
		}, nil
	case "system.multicall":
		return s.multicall(ctx, params)
	default:
		if strings.HasPrefix(method, "native.") {
			return s.invokeNative(ctx, method, params)
		}
		return nil, jsonrpc.NewError(jsonrpc.CodeMethodNotFound, "method not found")
	}
}

func (s *Service) addURI(ctx context.Context, params []any) (any, error) {
	if len(params) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "uris are required")
	}

	rawURIs, ok := params[0].([]any)
	if !ok {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "first param must be uri array")
	}

	uris := make([]string, 0, len(rawURIs))
	for _, value := range rawURIs {
		uri, ok := value.(string)
		if !ok || strings.TrimSpace(uri) == "" {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "uri must be a non-empty string")
		}
		if err := validateAddURIScheme(uri); err != nil {
			return nil, err
		}
		uris = append(uris, uri)
	}

	position := -1
	rest := params[1:]
	if pos, ok, trimmed := parseOptionalTrailingPosition(rest); ok {
		if pos < 0 {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "position must be non-negative")
		}
		position = pos
		rest = trimmed
	}

	options := map[string]string{}
	if len(rest) >= 1 {
		options = parseOptions(rest[0])
	}
	if err := validateAddOptions(options); err != nil {
		return nil, err
	}

	input := task.AddTaskInput{
		URIs:          uris,
		SaveDir:       options["dir"],
		Name:          options["out"],
		Options:       options,
		QueuePosition: position,
	}
	created, err := s.manager.Add(ctx, input)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return created.GID, nil
}

func (s *Service) addTorrent(ctx context.Context, params []any) (any, error) {
	payload, uris, options, position, err := parseAddTorrentParams(params)
	if err != nil {
		return nil, err
	}
	if err := validateAddOptions(options); err != nil {
		return nil, err
	}

	created, err := s.manager.Add(ctx, task.AddTaskInput{
		Torrent:       payload,
		URIs:          uris,
		SaveDir:       options["dir"],
		Name:          options["out"],
		Options:       options,
		QueuePosition: position,
	})
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	if common.OptionBool(options, "rpc-save-upload-metadata", false) {
		saveDir := created.SaveDir
		if saveDir == "" {
			saveDir = options["dir"]
		}
		if saveDir != "" {
			_ = bt.SaveUploadedTorrentMetadata(saveDir, payload)
		}
	}
	return created.GID, nil
}

func (s *Service) remove(ctx context.Context, params []any, force bool) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	removed, err := s.manager.Remove(ctx, gid, force)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return removed.GID, nil
}

func (s *Service) pause(ctx context.Context, params []any, force bool) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	if !taskCanBePaused(item) {
		return nil, errCannotPauseNow(gid)
	}
	paused, err := s.manager.Pause(ctx, gid, force)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return paused.GID, nil
}

func (s *Service) unpause(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	if !taskCanBeUnpaused(item) {
		return nil, errCannotUnpauseNow(gid)
	}
	updated, err := s.manager.Unpause(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return updated.GID, nil
}

func (s *Service) pauseAll(ctx context.Context) (any, error) {
	if err := s.manager.PauseAll(ctx); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) forcePauseAll(ctx context.Context) (any, error) {
	if err := s.manager.ForcePauseAll(ctx); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) unpauseAll(ctx context.Context) (any, error) {
	if err := s.manager.UnpauseAll(ctx); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) removeDownloadResult(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	if err := s.manager.RemoveDownloadResult(ctx, gid); err != nil {
		return nil, mapManagerRPCError(err)
	}
	return "OK", nil
}

func (s *Service) purgeDownloadResult(ctx context.Context) (any, error) {
	if err := s.manager.PurgeDownloadResult(ctx); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) ping(ctx context.Context, params []any) (any, error) {
	_ = ctx
	_ = params
	return "pong", nil
}

func (s *Service) saveSession(ctx context.Context, params []any) (any, error) {
	path := s.sessionPath
	if len(params) > 0 {
		if value, ok := params[0].(string); ok && strings.TrimSpace(value) != "" {
			path = value
		}
	}
	if strings.TrimSpace(path) == "" {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "save-session path is not configured")
	}
	if err := s.manager.SaveSessionTo(ctx, path); err != nil {
		return nil, err
	}
	return "OK", nil
}

func (s *Service) shutdown(ctx context.Context, params []any, force bool) (any, error) {
	_ = ctx
	if !force && len(params) > 0 {
		switch value := params[0].(type) {
		case bool:
			force = value
		case string:
			force = strings.EqualFold(value, "true") || value == "1"
		case float64:
			force = value != 0
		}
	}
	if s.onShutdown != nil {
		go s.onShutdown(force)
	}
	return "OK", nil
}

func (s *Service) tellStatus(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}

	keys := []string{}
	if len(params) >= 2 {
		keys = parseStringList(params[1])
	}

	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return toStatusResponse(item, keys), nil
}

func (s *Service) tellActive(ctx context.Context, params []any) (any, error) {
	keys := []string{}
	if len(params) >= 1 {
		keys = parseStringList(params[0])
	}

	items, err := s.manager.TellActive(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toStatusResponse(item, keys))
	}
	return out, nil
}

func (s *Service) tellWaiting(ctx context.Context, params []any) (any, error) {
	offset, err := intParam(params, 0, "offset")
	if err != nil {
		return nil, err
	}
	limit, err := intParam(params, 1, "num")
	if err != nil {
		return nil, err
	}

	keys := []string{}
	if len(params) >= 3 {
		keys = parseStringList(params[2])
	}

	items, err := s.manager.TellWaiting(ctx, offset, limit)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toStatusResponse(item, keys))
	}
	return out, nil
}

func (s *Service) tellStopped(ctx context.Context, params []any) (any, error) {
	offset, err := intParam(params, 0, "offset")
	if err != nil {
		return nil, err
	}
	limit, err := intParam(params, 1, "num")
	if err != nil {
		return nil, err
	}

	keys := []string{}
	if len(params) >= 3 {
		keys = parseStringList(params[2])
	}

	items, err := s.manager.TellStopped(ctx, offset, limit)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toStatusResponse(item, keys))
	}
	return out, nil
}

func (s *Service) getFiles(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}

	files, err := s.manager.GetFiles(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return toFilesResponse(files, item.Status), nil
}

func (s *Service) getPeers(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}

	peers, err := s.manager.GetPeers(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return toPeersResponse(peers), nil
}

func (s *Service) getServers(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}

	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	if item.Status != task.StatusActive {
		return nil, errNoActiveDownload(gid)
	}

	servers, err := s.manager.GetServers(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return toServersResponse(servers), nil
}

func (s *Service) getUris(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}

	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return toURIsResponse(item.Files, item.Status), nil
}

func (s *Service) getOption(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}

	item, err := s.manager.TellStatus(ctx, gid)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return toOptionResponse(item), nil
}

func (s *Service) changeOption(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	if len(params) < 2 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "options are required")
	}

	options := parseOptions(params[1])
	if len(options) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "options must be an object")
	}

	filtered, err := prepareChangeTaskOptions(options)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return "OK", nil
	}

	updated, err := s.manager.ChangeOption(ctx, gid, filtered)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	_ = updated
	return "OK", nil
}

func (s *Service) getGlobalOption() map[string]string {
	return filterHiddenOptions(s.manager.GetGlobalOption())
}

func (s *Service) changeGlobalOption(ctx context.Context, params []any) (any, error) {
	_ = ctx
	if len(params) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "options are required")
	}

	options := parseOptions(params[0])
	if len(options) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "options must be an object")
	}

	filtered, err := prepareChangeGlobalOptions(options)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return s.getGlobalOption(), nil
	}

	return filterHiddenOptions(s.manager.ChangeGlobalOption(filtered)), nil
}

func (s *Service) multicall(ctx context.Context, params []any) (any, error) {
	if len(params) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "multicall payload is required")
	}

	rawCalls, ok := params[0].([]any)
	if !ok {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "multicall payload must be an array")
	}

	out := make([]any, 0, len(rawCalls))
	for _, rawCall := range rawCalls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			out = append(out, multicallError(jsonrpc.CodeInvalidParams, "invalid multicall item"))
			continue
		}

		method, ok := call["methodName"].(string)
		if !ok || method == "" {
			out = append(out, multicallError(jsonrpc.CodeInvalidParams, "methodName is required"))
			continue
		}

		callParams, _ := call["params"].([]any)
		authorizedParams, err := s.authorize(callParams)
		if err != nil {
			out = append(out, multicallErrorFromErr(err))
			continue
		}
		result, err := s.invokeWithoutAuth(ctx, method, authorizedParams)
		if err != nil {
			out = append(out, multicallErrorFromErr(err))
			continue
		}
		out = append(out, []any{result})
	}
	return out, nil
}

func multicallError(code int, message string) map[string]any {
	return map[string]any{
		"code":    code,
		"message": message,
	}
}

func multicallErrorFromErr(err error) map[string]any {
	var rpcErr *jsonrpc.RPCError
	if errors.As(err, &rpcErr) {
		return multicallError(rpcErr.Code, rpcErr.Message)
	}
	return multicallError(jsonrpc.CodeInternalError, err.Error())
}

// releaseDate 与构建版本对齐，避免每次 RPC 调用返回值变化（aria2 为固定发布日期）。
const releaseDate = "2026-07-29"

func (s *Service) getVersion() map[string]any {
	return map[string]any{
		"version":            "0.1.1",
		"enabledFeatures":    []string{"BitTorrent", "ED2K", "HTTP", "JSON-RPC", "XML-RPC", "WebSocket"},
		"fullVersion":        "github.com/chenjia404/go-aria2/0.1.1",
		"releaseDate":        releaseDate,
		"organization":       "github.com/chenjia404/go-aria2",
		"copyright":          "github.com/chenjia404/go-aria2 contributors",
		"enabledProtocols":   []string{"bt", "ed2k", "ftp", "http", "https", "sftp"},
		"supportedProtocols": []string{"bt", "ed2k", "ftp", "http", "https", "sftp"},
	}
}

func (s *Service) getSessionInfo() map[string]any {
	return map[string]any{
		"sessionId":   s.sessionID,
		"startTime":   s.startedAt.Format(time.RFC3339),
		"uptimeSecs":  int(time.Since(s.startedAt).Seconds()),
		"aria2Style":  true,
		"server":      "github.com/chenjia404/go-aria2",
		"generatedAt": time.Now().Format(time.RFC3339),
	}
}

func (s *Service) authorize(params []any) ([]any, error) {
	if s.rpcSecret == "" {
		if len(params) > 0 {
			if token, ok := params[0].(string); ok && strings.HasPrefix(token, "token:") {
				return params[1:], nil
			}
		}
		return params, nil
	}

	if len(params) == 0 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "missing token")
	}
	token, ok := params[0].(string)
	if !ok || !strings.HasPrefix(token, "token:") {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "missing token")
	}
	if strings.TrimPrefix(token, "token:") != s.rpcSecret {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "invalid token")
	}
	return params[1:], nil
}

func newSessionID() string {
	return fmt.Sprintf("%d-%d", time.Now().Unix(), time.Now().UnixNano())
}

func parseOptions(value any) map[string]string {
	options := map[string]string{}
	raw, ok := value.(map[string]any)
	if !ok {
		return options
	}

	for key, item := range raw {
		options[key] = fmt.Sprint(item)
	}
	return options
}

func parseStringList(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func stringParam(params []any, index int, name string) (string, error) {
	if len(params) <= index {
		return "", jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" is required")
	}
	switch v := params[index].(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "", jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" must be a non-empty string")
		}
		return v, nil
	case float64:
		// JSON 数字在解码为 []any 时为 float64；部分客户端把 gid 当成数字传参
		if v < 0 || v != float64(int64(v)) {
			return "", jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" must be a string")
		}
		return strconv.FormatInt(int64(v), 10), nil
	default:
		return "", jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" must be a string")
	}
}

func intParam(params []any, index int, name string) (int, error) {
	if len(params) <= index {
		return 0, jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" is required")
	}
	switch value := params[index].(type) {
	case float64:
		return int(value), nil
	case int:
		return value, nil
	case string:
		var parsed int
		_, err := fmt.Sscanf(value, "%d", &parsed)
		if err != nil {
			return 0, jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" must be an integer")
		}
		return parsed, nil
	default:
		return 0, jsonrpc.NewError(jsonrpc.CodeInvalidParams, name+" must be an integer")
	}
}

func (s *Service) changePosition(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	pos, err := intParam(params, 1, "pos")
	if err != nil {
		return nil, err
	}
	how := "POS_SET"
	if len(params) > 2 {
		if value, ok := params[2].(string); ok && strings.TrimSpace(value) != "" {
			how = value
		}
	}
	newPos, err := s.manager.ChangePosition(ctx, gid, pos, how)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return newPos, nil
}

func (s *Service) changeUri(ctx context.Context, params []any) (any, error) {
	gid, err := stringParam(params, 0, "gid")
	if err != nil {
		return nil, err
	}
	fileIndex, err := intParam(params, 1, "fileIndex")
	if err != nil {
		return nil, err
	}
	if fileIndex < 1 {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "fileIndex must be >= 1")
	}
	delURIs := []string{}
	if len(params) > 2 && params[2] != nil {
		if _, ok := params[2].([]any); !ok {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "delURIs must be an array")
		}
		delURIs = parseStringList(params[2])
	}
	addURIs := []string{}
	if len(params) > 3 && params[3] != nil {
		if _, ok := params[3].([]any); !ok {
			return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "addURIs must be an array")
		}
		addURIs = parseStringList(params[3])
	}
	position := -1
	if len(params) > 4 {
		position, err = intParam(params, 4, "position")
		if err != nil {
			return nil, err
		}
	}
	delCount, addCount, err := s.manager.ChangeURI(ctx, gid, fileIndex, delURIs, addURIs, position)
	if err != nil {
		return nil, mapManagerRPCError(err)
	}
	return []any{delCount, addCount}, nil
}
