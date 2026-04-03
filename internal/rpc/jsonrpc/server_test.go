package jsonrpc

import (
	"context"
	"errors"
	"testing"
)

type testHandler struct {
	invoke func(ctx context.Context, method string, params []any) (any, error)
}

func (h testHandler) Invoke(ctx context.Context, method string, params []any) (any, error) {
	return h.invoke(ctx, method, params)
}

func TestHandleRequestHidesInternalErrorDetails(t *testing.T) {
	t.Parallel()

	srv := NewServer(testHandler{
		invoke: func(context.Context, string, []any) (any, error) {
			return nil, errors.New("open /tmp/private/file: permission denied")
		},
	}, Options{})

	resp := srv.handleRequest(context.Background(), request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "aria2.tellStatus",
	})

	if resp.Error == nil {
		t.Fatalf("expected error response")
	}
	if resp.Error.Message != internalErrorMessage {
		t.Fatalf("expected generic internal error message, got %q", resp.Error.Message)
	}
}
