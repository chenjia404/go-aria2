//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type daemonHandle struct {
	name    string
	baseURL string
	secret  string
	cmd     *exec.Cmd
	log     bytes.Buffer
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func freeListenPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func findAria2cBinary(t *testing.T) string {
	t.Helper()
	if p := strings.TrimSpace(os.Getenv("ARIA2C_BIN")); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("ARIA2C_BIN=%s: %v", p, err)
		}
		return p
	}
	p, err := exec.LookPath("aria2c")
	if err != nil {
		t.Skip("未找到 aria2c，请安装 aria2 或设置环境变量 ARIA2C_BIN 指向 aria2c 可执行文件")
	}
	return p
}

func writeAria2DaemonConf(t *testing.T, path string, port int, secret, downloadDir, sessionPath string) {
	t.Helper()
	// 仅使用 aria2 原生支持的配置项；go-aria2 扩展项（enable-websocket、listen-port=0 等）会导致 aria2c 启动失败。
	body := fmt.Sprintf(`enable-rpc=true
rpc-listen-port=%d
rpc-listen-all=false
rpc-secret=%s
dir=%s
max-concurrent-downloads=3
save-session=%s
save-session-interval=0
enable-dht=false
bt-enable-lpd=false
listen-port=16881
`, port, secret, filepath.ToSlash(downloadDir), filepath.ToSlash(sessionPath))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGoAria2DaemonConf(t *testing.T, path string, port int, secret, downloadDir, dataDir, sessionPath string, extraLines ...string) {
	t.Helper()
	body := fmt.Sprintf(`enable-rpc=true
rpc-listen-port=%d
rpc-listen-all=false
rpc-secret=%s
enable-websocket=false
dir=%s
data-dir=%s
max-concurrent-downloads=3
save-session=%s
save-session-interval=0
listen-port=0
enable-dht=false
ed2k-enable=false
`, port, secret, filepath.ToSlash(downloadDir), filepath.ToSlash(dataDir), filepath.ToSlash(sessionPath))
	if len(extraLines) > 0 {
		body += strings.Join(extraLines, "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func startAria2Daemon(t *testing.T, workDir string, port int, secret string) *daemonHandle {
	t.Helper()
	aria2c := findAria2cBinary(t)
	downloadDir := filepath.Join(workDir, "downloads")
	dataDir := filepath.Join(workDir, "aria2-data")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(workDir, "aria2.conf")
	session := filepath.Join(dataDir, "session.txt")
	writeAria2DaemonConf(t, conf, port, secret, downloadDir, session)

	cmd := exec.Command(aria2c, "--conf-path="+conf, "--daemon=false", "--quiet=true")
	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start aria2c: %v", err)
	}

	handle := &daemonHandle{
		name:    "aria2",
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		secret:  secret,
		cmd:     cmd,
		log:     logBuf,
	}
	waitRPCReady(t, handle)
	t.Cleanup(func() { stopDaemon(handle) })
	return handle
}

func startGoAria2Daemon(t *testing.T, workDir string, port int, secret string) *daemonHandle {
	return startGoAria2DaemonWithExtra(t, workDir, port, secret)
}

func startGoAria2DaemonWithExtra(t *testing.T, workDir string, port int, secret string, extraLines ...string) *daemonHandle {
	t.Helper()
	root := findModuleRoot(t)
	downloadDir := filepath.Join(workDir, "downloads")
	dataDir := filepath.Join(workDir, "go-aria2-data")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(workDir, "go-aria2.conf")
	session := filepath.Join(dataDir, "session.json")
	writeGoAria2DaemonConf(t, conf, port, secret, downloadDir, dataDir, session, extraLines...)

	cmd := exec.Command("go", "run", ".", "daemon", "-conf", conf)
	cmd.Dir = filepath.Join(root, "cmd", "go-aria2")
	cmd.Env = os.Environ()
	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start go-aria2: %v", err)
	}

	handle := &daemonHandle{
		name:    "go-aria2",
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		secret:  secret,
		cmd:     cmd,
		log:     logBuf,
	}
	waitHealthReady(t, handle)
	t.Cleanup(func() { stopDaemon(handle) })
	return handle
}

func waitRPCReady(t *testing.T, d *daemonHandle) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, err := d.call(context.Background(), "aria2.getVersion", nil)
		if err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s RPC not ready; log:\n%s", d.name, d.log.String())
}

func waitHealthReady(t *testing.T, d *daemonHandle) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(d.baseURL + "/healthz")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && string(body) == "ok" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s healthz not ready; log:\n%s", d.name, d.log.String())
}

func stopDaemon(d *daemonHandle) {
	if d == nil || d.cmd == nil || d.cmd.Process == nil {
		return
	}
	pid := d.cmd.Process.Pid
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
	} else {
		_ = d.cmd.Process.Kill()
	}
	done := make(chan struct{})
	go func() {
		_, _ = d.cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
	time.Sleep(300 * time.Millisecond)
}

func (d *daemonHandle) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	return d.callRPC(ctx, method, params, true)
}

func (d *daemonHandle) callUnauthenticated(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	return d.callRPC(ctx, method, params, false)
}

func (d *daemonHandle) callRPC(ctx context.Context, method string, params []any, injectToken bool) (json.RawMessage, error) {
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      method,
		"method":  method,
	}
	if params == nil {
		params = []any{}
	}
	if injectToken && d.secret != "" && method != "system.multicall" {
		params = append([]any{"token:" + d.secret}, params...)
	}
	reqBody["params"] = params

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/jsonrpc", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s: %s", d.name, out.Error.Message)
	}
	return out.Result, nil
}

func decodeJSON[T any](t *testing.T, raw json.RawMessage, label string) T {
	t.Helper()
	var out T
	if len(raw) == 0 {
		t.Fatalf("%s: empty result", label)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s decode: %v raw=%s", label, err, string(raw))
	}
	return out
}

func mustString(t *testing.T, raw json.RawMessage, label string) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("%s: expected string, got %s (%v)", label, string(raw), err)
	}
	return s
}

// startSlowDownloadServer 提供长时间保持连接的 HTTP 端点，便于在集成测试中构造 active 下载。
func startSlowDownloadServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		buf := make([]byte, 1024)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if _, err := w.Write(buf); err != nil {
					return
				}
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
	}))
	t.Cleanup(func() {
		ts.CloseClientConnections()
	})
	return ts
}

func forceRemoveDownloads(t *testing.T, ctx context.Context, d *daemonHandle, gids ...string) {
	t.Helper()
	for _, gid := range gids {
		_, _ = d.call(ctx, "aria2.pause", []any{gid})
		if _, err := d.call(ctx, "aria2.forceRemove", []any{gid}); err != nil {
			t.Logf("%s forceRemove %s: %v", d.name, gid, err)
		}
	}
}

func waitUntilTaskStatus(t *testing.T, ctx context.Context, d *daemonHandle, gid, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" tellStatus")
		if status["status"] == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	status := decodeJSON[map[string]any](t, mustCall(t, ctx, d, "aria2.tellStatus", gid), d.name+" tellStatus final")
	t.Fatalf("%s task %s status want %q got %#v", d.name, gid, want, status["status"])
}
