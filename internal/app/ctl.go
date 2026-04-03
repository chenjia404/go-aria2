package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// runCtl 是统一入口下的调试 JSON-RPC 命令。
func runCtl(args []string) error {
	var endpoint string
	var secret string
	var method string
	var paramsJSON string

	fs := flag.NewFlagSet("ctl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&endpoint, "endpoint", "http://127.0.0.1:6800/jsonrpc", "json-rpc endpoint")
	fs.StringVar(&secret, "secret", "", "rpc secret")
	fs.StringVar(&secret, "rpc-secret", "", "rpc secret")
	fs.StringVar(&method, "method", "system.listMethods", "json-rpc method")
	fs.StringVar(&paramsJSON, "params", "[]", "json array params")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var params []any
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return fmt.Errorf("invalid params json: %w", err)
	}
	if secret != "" {
		params = append([]any{"token:" + secret}, params...)
	}

	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      "go-aria2ctl",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("marshal request failed: %w", err)
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("request failed: %w\n提示: 确认进程已启动且 enable-rpc=true；端口与 -endpoint 一致（默认 6800）；若从本机以外访问需设置 rpc-listen-all=true 并放行防火墙；本仓库 RPC 路径为 /jsonrpc", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %s: %s", resp.Status, string(body))
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err == nil {
		fmt.Println(pretty.String())
		return nil
	}
	fmt.Println(string(body))
	return nil
}
