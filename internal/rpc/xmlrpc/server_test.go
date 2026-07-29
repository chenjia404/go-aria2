package xmlrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubHandler struct {
	lastMethod string
	lastParams []any
	result     any
	err        error
}

func (h *stubHandler) Invoke(ctx context.Context, method string, params []any) (any, error) {
	_ = ctx
	h.lastMethod = method
	h.lastParams = append([]any(nil), params...)
	if h.err != nil {
		return nil, h.err
	}
	return h.result, nil
}

func TestDecodeAndInvokeGetVersion(t *testing.T) {
	t.Parallel()

	body := `<?xml version="1.0"?><methodCall>
<methodName>aria2.getVersion</methodName>
<params><param><value><string>token:secret</string></value></param></params>
</methodCall>`

	handler := &stubHandler{result: map[string]string{"version": "1.36.0"}}
	out, err := InvokeMethod(context.Background(), handler, []byte(body))
	if err != nil {
		t.Fatalf("InvokeMethod: %v", err)
	}
	if handler.lastMethod != "aria2.getVersion" {
		t.Fatalf("unexpected method: %s", handler.lastMethod)
	}
	if len(handler.lastParams) != 1 || handler.lastParams[0] != "token:secret" {
		t.Fatalf("unexpected params: %#v", handler.lastParams)
	}
	text := string(out)
	if !strings.Contains(text, "<string>1.36.0</string>") {
		t.Fatalf("unexpected response: %s", text)
	}
}

func TestDecodeArrayParams(t *testing.T) {
	t.Parallel()

	body := `<?xml version="1.0"?><methodCall>
<methodName>aria2.addUri</methodName>
<params>
<param><value><array><data>
<value><string>token:secret</string></value>
<value><array><data><value><string>http://example.com/a</string></value></data></array></value>
</data></array></value></param>
</params></methodCall>`

	method, params, err := decodeMethodCall([]byte(body))
	if err != nil {
		t.Fatalf("decodeMethodCall: %v", err)
	}
	if method != "aria2.addUri" {
		t.Fatalf("unexpected method: %s", method)
	}
	if len(params) != 1 {
		t.Fatalf("expected wrapped array param, got %#v", params)
	}
	top, ok := params[0].([]any)
	if !ok || len(top) != 2 {
		t.Fatalf("unexpected top params: %#v", params[0])
	}
}

func TestServerHTTPGetVersion(t *testing.T) {
	t.Parallel()

	handler := &stubHandler{result: "pong"}
	server := NewServer(handler, Options{})
	req := httptest.NewRequest(http.MethodPost, "/xmlrpc", strings.NewReader(`<?xml version="1.0"?><methodCall><methodName>aria2.ping</methodName><params/></methodCall>`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "pong") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
