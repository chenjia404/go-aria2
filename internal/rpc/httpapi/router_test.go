package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chenjia404/go-aria2/internal/config"
)

func TestHealthWithoutGateway(t *testing.T) {
	h := NewRouter(&Server{RPCSecret: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestInfoFailsWithoutGateway(t *testing.T) {
	h := NewRouter(&Server{RPCSecret: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/info", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without ed2k gateway, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestGetConfigSummaryWithoutAuth(t *testing.T) {
	cfg := config.Default()
	h := NewRouter(&Server{
		CFG:           cfg,
		ED2KStatePath: "/tmp/ed2k.json",
		RPCListenAddr: "127.0.0.1:16800",
		RPCSecret:     "",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status %d %s", rec.Code, rec.Body.String())
	}
}
