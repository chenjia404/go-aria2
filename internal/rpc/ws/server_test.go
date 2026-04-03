package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizeRequest(t *testing.T) {
	t.Parallel()

	srv := &Server{rpcSecret: "secret"}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws?token=secret", nil)
	if !srv.authorizeRequest(req) {
		t.Fatalf("expected query token to authorize")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Authorization", "token:secret")
	if !srv.authorizeRequest(req) {
		t.Fatalf("expected authorization header token to authorize")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("X-Auth-Token", "secret")
	if !srv.authorizeRequest(req) {
		t.Fatalf("expected x-auth-token header to authorize")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws?token=wrong", nil)
	if srv.authorizeRequest(req) {
		t.Fatalf("expected wrong token to be rejected")
	}
}

func TestServeHTTPRejectsMissingTokenBeforeUpgrade(t *testing.T) {
	t.Parallel()

	srv := &Server{rpcSecret: "secret"}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rr.Code)
	}
}
