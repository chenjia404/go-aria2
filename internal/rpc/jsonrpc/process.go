package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
)

// ProcessBody 解析单条或批量 JSON-RPC 请求并返回应写入传输层的 JSON 字节。
// 若客户端发送的是 Notification（无 id），或批量全部为 Notification，返回 ok=false；
// HTTP 应响应 204 且无正文；WebSocket 不发送帧。
func ProcessBody(ctx context.Context, h Handler, body []byte) (out []byte, ok bool) {
	var batch []json.RawMessage
	if err := json.Unmarshal(body, &batch); err == nil {
		return processBatch(ctx, h, batch)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		b, _ := json.Marshal(response{JSONRPC: "2.0", Error: NewError(CodeParseError, "parse error")})
		return b, true
	}
	return processSingle(ctx, h, raw)
}

func processSingle(ctx context.Context, h Handler, raw json.RawMessage) ([]byte, bool) {
	keys := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &keys); err != nil {
		b, _ := json.Marshal(response{JSONRPC: "2.0", Error: NewError(CodeParseError, "parse error")})
		return b, true
	}
	_, hasID := keys["id"]
	if !hasID && jsonRPCVersionOK(keys) && stringValue(keys["method"]) != "" {
		return nil, false
	}

	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		b, _ := json.Marshal(response{JSONRPC: "2.0", Error: NewError(CodeParseError, "parse error")})
		return b, true
	}
	resp := handleRequest(ctx, h, req)
	b, _ := json.Marshal(resp)
	return b, true
}

func processBatch(ctx context.Context, h Handler, parts []json.RawMessage) ([]byte, bool) {
	if len(parts) == 0 {
		b, _ := json.Marshal(response{JSONRPC: "2.0", Error: NewError(CodeInvalidRequest, "invalid request")})
		return b, true
	}

	out := make([]any, 0, len(parts))
	anyResponse := false
	for _, raw := range parts {
		keys := make(map[string]json.RawMessage)
		if err := json.Unmarshal(raw, &keys); err != nil {
			out = append(out, response{JSONRPC: "2.0", Error: NewError(CodeParseError, "parse error")})
			anyResponse = true
			continue
		}
		_, hasID := keys["id"]
		if !hasID && jsonRPCVersionOK(keys) && stringValue(keys["method"]) != "" {
			out = append(out, nil)
			continue
		}
		anyResponse = true

		var req request
		if err := json.Unmarshal(raw, &req); err != nil {
			out = append(out, response{JSONRPC: "2.0", Error: NewError(CodeParseError, "parse error")})
			continue
		}
		out = append(out, handleRequest(ctx, h, req))
	}

	if !anyResponse {
		return nil, false
	}
	b, err := json.Marshal(out)
	if err != nil {
		b, _ := json.Marshal(response{JSONRPC: "2.0", Error: NewError(CodeInternalError, internalErrorMessage)})
		return b, true
	}
	return b, true
}

func jsonRPCVersionOK(keys map[string]json.RawMessage) bool {
	v := stringValue(keys["jsonrpc"])
	return v == "" || v == "2.0"
}

func stringValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// handleRequest 处理单条请求（与 HTTP Server 共用）。
func handleRequest(ctx context.Context, h Handler, req request) response {
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

	result, err := h.Invoke(ctx, req.Method, params)
	if err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			return response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return response{JSONRPC: "2.0", ID: req.ID, Error: NewError(CodeInternalError, internalErrorMessage)}
	}

	return response{JSONRPC: "2.0", ID: req.ID, Result: result}
}
