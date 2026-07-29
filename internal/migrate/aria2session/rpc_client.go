package aria2session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcErrorBody   `json:"error"`
}

type rpcErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RPCClient 调用 aria2 JSON-RPC 接口。
type RPCClient struct {
	Endpoint   string
	Secret     string
	HTTPClient *http.Client
}

// NewRPCClient 创建 aria2 RPC 客户端。
func NewRPCClient(endpoint, secret string) *RPCClient {
	return &RPCClient{
		Endpoint: strings.TrimSpace(endpoint),
		Secret:   strings.TrimSpace(secret),
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *RPCClient) Call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	if c == nil || c.Endpoint == "" {
		return nil, fmt.Errorf("rpc endpoint is required")
	}
	callParams := append([]any(nil), params...)
	if c.Secret != "" {
		callParams = append([]any{"token:" + c.Secret}, callParams...)
	}
	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      "go-aria2-migrate",
		Method:  method,
		Params:  callParams,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc http %s: %s", resp.Status, string(body))
	}

	var out rpcResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}
