package httpdl

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHostBypassesProxy(t *testing.T) {
	t.Parallel()

	patterns := parseNoProxyList("localhost,127.0.0.1,.example.com")
	if !hostBypassesProxy("localhost", patterns) {
		t.Fatal("expected localhost bypass")
	}
	if !hostBypassesProxy("cdn.example.com", patterns) {
		t.Fatal("expected suffix bypass")
	}
	if hostBypassesProxy("evil.com", patterns) {
		t.Fatal("unexpected bypass")
	}
}

func TestProxyBypassesNoProxyHosts(t *testing.T) {
	t.Parallel()

	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}

	fn := proxyFunc(Options{
		HTTPProxy: proxyServer.URL,
		NoProxy:   "localhost",
	})

	req, err := http.NewRequest(http.MethodGet, "http://localhost/file.bin", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	got, err := fn(req)
	if err != nil {
		t.Fatalf("proxy func returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected direct connection for localhost, got %#v", got)
	}

	req, err = http.NewRequest(http.MethodGet, "http://example.com/file.bin", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	got, err = fn(req)
	if err != nil {
		t.Fatalf("proxy func returned error: %v", err)
	}
	if got == nil || got.String() != proxyURL.String() {
		t.Fatalf("expected proxy %#v, got %#v", proxyURL, got)
	}
}
