package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
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

	var batch []request
	if err := json.Unmarshal(body, &batch); err == nil {
		s.handleBatch(w, r.Context(), batch)
		return
	}

	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeSingle(w, response{JSONRPC: "2.0", Error: NewError(CodeParseError, "parse error")})
		return
	}
	s.writeSingle(w, s.handleRequest(r.Context(), req))
}

func (s *Server) handleBatch(w http.ResponseWriter, ctx context.Context, batch []request) {
	if len(batch) == 0 {
		s.writeSingle(w, response{JSONRPC: "2.0", Error: NewError(CodeInvalidRequest, "invalid request")})
		return
	}

	out := make([]response, 0, len(batch))
	for _, req := range batch {
		out = append(out, s.handleRequest(ctx, req))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleRequest(ctx context.Context, req request) response {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return response{JSONRPC: "2.0", ID: req.ID, Error: NewError(CodeInvalidRequest, "invalid request")}
	}
	if req.Method == "" {
		return response{JSONRPC: "2.0", ID: req.ID, Error: NewError(CodeInvalidRequest, "method is required")}
	}

	var params []any
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return response{JSONRPC: "2.0", ID: req.ID, Error: NewError(CodeInvalidParams, "params must be an array")}
		}
	}

	result, err := s.handler.Invoke(ctx, req.Method, params)
	if err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			return response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Error: NewError(CodeInternalError, internalErrorMessage)}
	}

	return response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) writeSingle(w http.ResponseWriter, resp response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
