package ws

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/chenjia404/go-aria2/internal/compat/aria2"
	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type snapshotMessage struct {
	Type       string           `json:"type"`
	Tasks      []map[string]any `json:"tasks"`
	GlobalStat map[string]any   `json:"globalStat"`
}

type client struct {
	conn    net.Conn
	writeMu sync.Mutex
}

// Server 将管理器事件桥接�?WebSocket 文本消息�?
type Server struct {
	manager        *manager.Manager
	rpcSecret      string
	allowOriginAll bool
}

// NewServer 创建 WebSocket 通知服务�?
func NewServer(mgr *manager.Manager, rpcSecret string, allowOriginAll bool) *Server {
	return &Server{
		manager:        mgr,
		rpcSecret:      rpcSecret,
		allowOriginAll: allowOriginAll,
	}
}

// ServeHTTP 处理 WebSocket 握手并持续推送任务事件�?
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRequest(r) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if !headerContainsToken(r.Header, "Connection", "Upgrade") || !headerContainsToken(r.Header, "Upgrade", "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("websocket hijack failed: %v", err), http.StatusInternalServerError)
		return
	}

	accept := computeAcceptKey(key)
	if _, err := fmt.Fprintf(buf, "HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		_ = conn.Close()
		return
	}
	if _, err := fmt.Fprintf(buf, "Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept); err != nil {
		_ = conn.Close()
		return
	}
	if err := buf.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	wsClient := &client{conn: conn}
	events, unsubscribe := s.manager.Subscribe(32)
	defer unsubscribe()
	defer conn.Close()

	initial := snapshotMessage{
		Type:       "snapshot",
		Tasks:      snapshotTasksAria2(s.manager.SnapshotTasks()),
		GlobalStat: aria2.GlobalStatToAria2JSON(s.manager.GetGlobalStat()),
	}
	if err := wsClient.sendJSON(initial); err != nil {
		return
	}

	for event := range events {
		if err := wsClient.sendJSON(wsEventToJSON(event)); err != nil {
			return
		}
	}
}

func snapshotTasksAria2(tasks []*task.Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		if t == nil {
			continue
		}
		out = append(out, aria2.TaskToAria2StatusJSON(t))
	}
	return out
}

// wsEventToJSON 将内部事件转为与 JSON-RPC tellStatus / getGlobalStat 相同字段类型，便于 aria2 系前端解析。
func wsEventToJSON(ev manager.Event) map[string]any {
	m := map[string]any{
		"type":       string(ev.Type),
		"globalStat": aria2.GlobalStatToAria2JSON(ev.GlobalStat),
		"time":       ev.Time,
	}
	if ev.Task != nil {
		m["task"] = aria2.TaskToAria2StatusJSON(ev.Task)
	}
	return m
}

func (c *client) sendJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeTextFrame(c.conn, payload)
}

func writeTextFrame(conn net.Conn, payload []byte) error {
	header := []byte{0x81}
	length := len(payload)

	switch {
	case length <= 125:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127,
			byte(length>>56), byte(length>>48), byte(length>>40), byte(length>>32),
			byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}

	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func computeAcceptKey(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerContainsToken(header http.Header, key, expected string) bool {
	for _, value := range header.Values(key) {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), expected) {
				return true
			}
		}
	}
	return false
}

func (s *Server) authorizeRequest(r *http.Request) bool {
	if s == nil || strings.TrimSpace(s.rpcSecret) == "" {
		return true
	}

	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token == s.rpcSecret
	}

	if token := strings.TrimSpace(r.Header.Get("Authorization")); token != "" {
		if strings.HasPrefix(token, "token:") {
			return strings.TrimPrefix(token, "token:") == s.rpcSecret
		}
	}

	if token := strings.TrimSpace(r.Header.Get("X-Auth-Token")); token != "" {
		return token == s.rpcSecret
	}

	return false
}
