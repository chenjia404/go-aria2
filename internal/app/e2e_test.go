//go:build integration

package app_test

import (
	"bytes"
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
			t.Fatal("go.mod not found from cwd")
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

func TestIntegration_DaemonHealthAndRPC(t *testing.T) {
	root := findModuleRoot(t)
	cmdDir := filepath.Join(root, "cmd", "go-aria2")

	work := t.TempDir()
	dl := filepath.Join(work, "downloads")
	data := filepath.Join(work, "data")
	if err := os.MkdirAll(dl, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}

	port := freeListenPort(t)
	secret := "integration-secret"
	session := filepath.Join(data, "session.json")
	conf := filepath.Join(work, "aria2.conf")
	confBody := fmt.Sprintf(`enable-rpc=true
rpc-listen-port=%d
rpc-listen-all=false
rpc-secret=%s
enable-websocket=false
dir=%s
data-dir=%s
max-concurrent-downloads=2
save-session=%s
save-session-interval=0
ed2k-enable=false
listen-port=0
enable-dht=false
`, port, secret, filepath.ToSlash(dl), filepath.ToSlash(data), filepath.ToSlash(session))
	if err := os.WriteFile(conf, []byte(confBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// go run：单进程，避免部分 Windows 环境对临时 .exe 的 fork/exec 问题；代价是首次较慢。
	cmd := exec.Command("go", "run", ".", "daemon", "-conf", conf)
	cmd.Dir = cmdDir
	cmd.Env = os.Environ()
	var logBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &logBuf)
	cmd.Stdout = &logBuf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process == nil {
			return
		}
		pid := cmd.Process.Pid
		if runtime.GOOS == "windows" {
			_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
		} else {
			_ = cmd.Process.Kill()
		}
		waitDone := make(chan struct{})
		go func() {
			_, _ = cmd.Process.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(15 * time.Second):
		}
		time.Sleep(1500 * time.Millisecond)
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	var healthOK bool
	for start := time.Now(); time.Since(start) < 30*time.Second; time.Sleep(50 * time.Millisecond) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && string(b) == "ok" {
				healthOK = true
				break
			}
		}
	}
	if !healthOK {
		t.Fatalf("healthz timeout; log:\n%s", logBuf.String())
	}

	client := &http.Client{Timeout: 10 * time.Second}

	t.Run("unauthenticated", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":1,"method":"system.listMethods","params":[]}`
		resp, err := client.Post(base+"/jsonrpc", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out["error"] == nil {
			t.Fatalf("expected error, got %#v", out)
		}
	})

	t.Run("listMethods", func(t *testing.T) {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"system.listMethods","params":["token:%s"]}`, secret)
		resp, err := client.Post(base+"/jsonrpc", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out["result"] == nil {
			t.Fatalf("expected result, got %#v", out)
		}
	})

	t.Run("addUriLocalHTTP", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello integration"))
		}))
		defer ts.Close()

		addBody := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":3,"method":"aria2.addUri","params":["token:%s",["%s"],{"dir":"%s"}]}`,
			secret, ts.URL+"/file.bin", filepath.ToSlash(dl),
		)
		resp, err := client.Post(base+"/jsonrpc", "application/json", strings.NewReader(addBody))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != nil {
			t.Fatalf("addUri: %#v", out)
		}
		gid, _ := out["result"].(string)
		if gid == "" {
			t.Fatalf("missing gid: %#v", out)
		}

		stBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"aria2.tellStatus","params":["token:%s","%s"]}`, secret, gid)
		resp2, err := client.Post(base+"/jsonrpc", "application/json", strings.NewReader(stBody))
		if err != nil {
			t.Fatal(err)
		}
		defer resp2.Body.Close()
		var st map[string]any
		if err := json.NewDecoder(resp2.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		if st["result"] == nil {
			t.Fatalf("tellStatus: %#v", st)
		}

		rmBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"aria2.remove","params":["token:%s","%s"]}`, secret, gid)
		resp3, err := client.Post(base+"/jsonrpc", "application/json", strings.NewReader(rmBody))
		if err != nil {
			t.Fatal(err)
		}
		defer resp3.Body.Close()
		var rm map[string]any
		if err := json.NewDecoder(resp3.Body).Decode(&rm); err != nil {
			t.Fatal(err)
		}
		if rm["result"] == nil {
			t.Fatalf("remove: %#v", rm)
		}
	})
}
