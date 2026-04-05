package jsonrpc

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/gorilla/websocket"
)

// WebSocketOptions 配置与 aria2 一致的 JSON-RPC over WebSocket（路径 /jsonrpc）。
type WebSocketOptions struct {
	Manager *manager.Manager
	// Secret 与 rpc-secret 一致；空字符串表示不校验。
	Secret string
}

func (s *Server) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	cfg := s.options.WebSocket
	if cfg == nil || cfg.Manager == nil {
		http.Error(w, "websocket unavailable", http.StatusServiceUnavailable)
		return
	}
	if !authorizeWebSocket(r, cfg.Secret) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	up := websocket.Upgrader{
		ReadBufferSize:  4098,
		WriteBufferSize: 4098,
		CheckOrigin: func(r *http.Request) bool {
			if s.options.AllowOriginAll {
				return true
			}
			return r.Header.Get("Origin") == ""
		},
	}

	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	max := int64(1 << 20)
	if s.options.MaxRequestSize > 0 {
		max = s.options.MaxRequestSize
	}
	conn.SetReadLimit(max)

	var writeMu sync.Mutex
	writeText := func(b []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, b)
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	prev := make(map[string]taskSnap)
	evCh, unsub := cfg.Manager.Subscribe(32)
	defer unsub()
	defer conn.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				for _, n := range aria2NotificationsForEvent(ev, prev) {
					payload, err := json.Marshal(n)
					if err != nil {
						continue
					}
					if err := writeText(payload); err != nil {
						cancel()
						return
					}
				}
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			cancel()
			break
		}
		out, ok := ProcessBody(ctx, s.handler, msg)
		if !ok {
			continue
		}
		if err := writeText(out); err != nil {
			cancel()
			break
		}
	}
	wg.Wait()
}

func authorizeWebSocket(r *http.Request, secret string) bool {
	if strings.TrimSpace(secret) == "" {
		return true
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token == secret
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		const p = "token:"
		if len(auth) > len(p) && strings.EqualFold(auth[:len(p)], p) {
			return strings.TrimSpace(auth[len(p):]) == secret
		}
	}
	if token := strings.TrimSpace(r.Header.Get("X-Auth-Token")); token != "" {
		return token == secret
	}
	return false
}

// WebSocketUpgrade 返回是否应把请求当作 WebSocket 升级处理。
func WebSocketUpgrade(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}
