package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const (
	// 标准 JSON-RPC 错误码。
	CodeParseError       = -32700
	CodeInvalidRequest   = -32600
	CodeMethodNotFound   = -32601
	CodeInvalidParams    = -32602
	CodeInternalError    = -32603
	internalErrorMessage = "internal error"
)

// Handler 抽象 RPC 方法处理器。
type Handler interface {
	Invoke(ctx context.Context, method string, params []any) (any, error)
}

// RPCError 允许业务层返回更准确的 JSON-RPC 错误码。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

// NewError 创建 JSON-RPC 错误。
func NewError(code int, message string) *RPCError {
	return &RPCError{Code: code, Message: message}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// Options 控制 JSON-RPC Server 的 HTTP 层行为。
type Options struct {
	MaxRequestSize int64
	AllowOriginAll bool
	// WebSocket 非 nil 时，GET + Upgrade 走 aria2 兼容的 JSON-RPC WebSocket（与 POST /jsonrpc 相同语义）。
	WebSocket *WebSocketOptions
}

// Server 是 HTTP JSON-RPC 2.0 服务。
type Server struct {
	handler Handler
	options Options
}

// NewServer 创建 JSON-RPC Server。
func NewServer(handler Handler, options Options) *Server {
	return &Server{handler: handler, options: options}
}

// ServeHTTP 支持单请求和批量请求。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.options.AllowOriginAll {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		methods := "POST, OPTIONS"
		if s.options.WebSocket != nil {
			methods = "POST, GET, OPTIONS"
		}
		w.Header().Set("Access-Control-Allow-Methods", methods)
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.options.WebSocket != nil && WebSocketUpgrade(r) && r.Method == http.MethodGet {
		s.serveWebSocket(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", strings.Join([]string{http.MethodPost, http.MethodGet}, ", "))
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	bodyReader := io.Reader(r.Body)
	if s.options.MaxRequestSize > 0 {
		bodyReader = http.MaxBytesReader(w, r.Body, s.options.MaxRequestSize)
	}

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		s.writeSingle(w, response{JSONRPC: "2.0", Error: NewError(CodeInvalidRequest, err.Error())})
		return
	}

	out, ok := ProcessBody(r.Context(), s.handler, body)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.writeBytes(w, out)
}

func (s *Server) writeBytes(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) writeSingle(w http.ResponseWriter, resp response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
