package ftp

import (
	"bytes"
	"context"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// resumeFTPMock 为 SIZE/REST/RETR 续传测试提供最小 FTP 服务端模拟。
type resumeFTPMock struct {
	t        *testing.T
	listener *net.TCPListener
	payload  []byte
	remote   string

	mu        sync.Mutex
	restSeen  int64
	retrCount int
}

func newResumeFTPMock(t *testing.T, payload []byte) (*resumeFTPMock, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tcpListener, ok := ln.(*net.TCPListener)
	if !ok {
		t.Fatalf("listener is not TCP")
	}

	mock := &resumeFTPMock{
		t:        t,
		listener: tcpListener,
		payload:  append([]byte(nil), payload...),
		remote:   "file.bin",
	}
	go mock.serve()
	return mock, ln.Addr().String()
}

func (m *resumeFTPMock) close() {
	_ = m.listener.Close()
}

func (m *resumeFTPMock) serve() {
	conn, err := m.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	proto := textproto.NewConn(conn)
	_ = proto.PrintfLine("220 FTP ready.")

	var (
		dataListener net.Listener
		restOffset   int64
	)

	for {
		line, err := proto.ReadLine()
		if err != nil {
			return
		}
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		arg := ""
		if len(parts) == 2 {
			arg = parts[1]
		}

		switch cmd {
		case "USER":
			_ = proto.PrintfLine("331 password required")
		case "PASS":
			_ = proto.PrintfLine("230 logged in")
		case "TYPE":
			_ = proto.PrintfLine("200 type set")
		case "SIZE":
			if arg != m.remote {
				_ = proto.PrintfLine("550 file unavailable")
				continue
			}
			_ = proto.PrintfLine("213 %d", len(m.payload))
		case "REST":
			offset, parseErr := strconv.ParseInt(arg, 10, 64)
			if parseErr != nil {
				_ = proto.PrintfLine("500 bad REST")
				continue
			}
			restOffset = offset
			m.mu.Lock()
			m.restSeen = offset
			m.mu.Unlock()
			_ = proto.PrintfLine("350 restart at %d", offset)
		case "PASV", "EPSV":
			if dataListener != nil {
				_ = dataListener.Close()
			}
			dataListener, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				_ = proto.PrintfLine("451 cannot open data connection")
				continue
			}
			tcpAddr, ok := dataListener.Addr().(*net.TCPAddr)
			if !ok {
				_ = proto.PrintfLine("451 bad data address")
				continue
			}
			port := tcpAddr.Port
			if cmd == "EPSV" {
				_ = proto.PrintfLine("229 Entering Extended Passive Mode (|||%d|)", port)
			} else {
				p1 := port / 256
				p2 := port % 256
				_ = proto.PrintfLine("227 Entering Passive Mode (127,0,0,1,%d,%d).", p1, p2)
			}
		case "RETR":
			if arg != m.remote {
				_ = proto.PrintfLine("550 file unavailable")
				continue
			}
			if dataListener == nil {
				_ = proto.PrintfLine("425 use PASV first")
				continue
			}
			m.mu.Lock()
			m.retrCount++
			m.mu.Unlock()
			_ = proto.PrintfLine("150 sending")
			dataConn, err := dataListener.Accept()
			if err != nil {
				_ = proto.PrintfLine("426 transfer aborted")
				continue
			}
			start := restOffset
			if start < 0 || start > int64(len(m.payload)) {
				start = 0
			}
			_, _ = dataConn.Write(m.payload[start:])
			_ = dataConn.Close()
			_ = dataListener.Close()
			dataListener = nil
			restOffset = 0
			_ = proto.PrintfLine("226 done")
		case "QUIT":
			_ = proto.PrintfLine("221 bye")
			return
		default:
			_ = proto.PrintfLine("500 unknown command")
		}
	}
}

func (m *resumeFTPMock) lastREST() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restSeen
}

func waitFTPComplete(t *testing.T, driver *Driver, taskID string, payloadLen int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := driver.TellStatus(context.Background(), taskID)
		if err != nil {
			t.Fatalf("TellStatus: %v", err)
		}
		switch status.Status {
		case task.StatusComplete:
			if status.CompletedLength != int64(payloadLen) {
				t.Fatalf("completed length: got %d want %d", status.CompletedLength, payloadLen)
			}
			return
		case task.StatusError:
			t.Fatalf("download failed: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for FTP download")
}

func TestDriverResumesPartialDownload_FTP(t *testing.T) {
	t.Parallel()

	payload := []byte("hello world")
	mock, addr := newResumeFTPMock(t, payload)
	defer mock.close()

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	uri := "ftp://" + host + ":" + port + "/" + mock.remote

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.bin")
	if err := os.WriteFile(outputPath, payload[:5], 0o644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     uri,
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"continue": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFTPComplete(t, driver, created.ID, len(payload))

	if rest := mock.lastREST(); rest != 5 {
		t.Fatalf("expected REST offset 5, got %d", rest)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("content: got %q want %q", data, payload)
	}
}

func TestDriverRestartsWhenContinueFalse_FTP(t *testing.T) {
	t.Parallel()

	payload := []byte("hello world")
	mock, addr := newResumeFTPMock(t, payload)
	defer mock.close()

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	uri := "ftp://" + host + ":" + port + "/" + mock.remote

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.bin")
	if err := os.WriteFile(outputPath, payload[:5], 0o644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     uri,
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"continue": "false", "allow-overwrite": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFTPComplete(t, driver, created.ID, len(payload))

	if rest := mock.lastREST(); rest != 0 {
		t.Fatalf("expected no REST offset, got %d", rest)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("content: got %q want %q", data, payload)
	}
}

func TestDriverResumeWhenFileSizeUnavailable_FTP(t *testing.T) {
	t.Parallel()

	payload := []byte("partial resume test data")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tcpListener, ok := ln.(*net.TCPListener)
	if !ok {
		t.Fatalf("not tcp listener")
	}

	remote := "/nosize.bin"
	go func() {
		conn, acceptErr := tcpListener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		proto := textproto.NewConn(conn)
		_ = proto.PrintfLine("220 FTP ready.")
		var dataListener net.Listener
		var restOffset int64
		for {
			line, readErr := proto.ReadLine()
			if readErr != nil {
				return
			}
			parts := strings.SplitN(line, " ", 2)
			cmd := strings.ToUpper(parts[0])
			arg := ""
			if len(parts) == 2 {
				arg = parts[1]
			}
			switch cmd {
			case "USER", "PASS":
				_ = proto.PrintfLine("230 ok")
			case "TYPE":
				_ = proto.PrintfLine("200 ok")
			case "SIZE":
				_ = proto.PrintfLine("550 unavailable")
			case "REST":
				offset, parseErr := strconv.ParseInt(arg, 10, 64)
				if parseErr != nil {
					_ = proto.PrintfLine("500 bad REST")
					continue
				}
				restOffset = offset
				_ = proto.PrintfLine("350 ok")
			case "PASV":
				if dataListener != nil {
					_ = dataListener.Close()
				}
				dataListener, _ = net.Listen("tcp", "127.0.0.1:0")
				tcpAddr := dataListener.Addr().(*net.TCPAddr)
				port := tcpAddr.Port
				_ = proto.PrintfLine("227 Entering Passive Mode (127,0,0,1,%d,%d).", port/256, port%256)
			case "RETR":
				_ = proto.PrintfLine("150 ok")
				dataConn, acceptErr := dataListener.Accept()
				if acceptErr != nil {
					_ = proto.PrintfLine("426 aborted")
					continue
				}
				start := restOffset
				if start < 0 {
					start = 0
				}
				_, _ = dataConn.Write(payload[start:])
				_ = dataConn.Close()
				_ = dataListener.Close()
				dataListener = nil
				_ = proto.PrintfLine("226 done")
			case "QUIT":
				_ = proto.PrintfLine("221 bye")
				return
			default:
				_ = proto.PrintfLine("500 unknown")
			}
		}
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	uri := "ftp://" + host + ":" + port + remote

	saveDir := t.TempDir()
	outputPath := filepath.Join(saveDir, "file.bin")
	partial := payload[:10]
	if err := os.WriteFile(outputPath, partial, 0o644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	driver := New(Options{})
	created, err := driver.Add(context.Background(), task.AddTaskInput{
		URI:     uri,
		SaveDir: saveDir,
		Name:    "file.bin",
		Options: map[string]string{"continue": "true"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := driver.Start(context.Background(), created.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFTPComplete(t, driver, created.ID, len(payload))

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("content: got %q want %q", data, payload)
	}
	_ = tcpListener.Close()
}
