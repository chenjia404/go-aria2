package httpapi

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
	"github.com/go-chi/chi/v5/middleware"
)

type ctxKey int

const ctxKeyLogger ctxKey = iota

// WithRequestLogger 将带 request_id 前缀的 logger 放入 context。
func WithRequestLogger(base *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := middleware.GetReqID(r.Context())
			var l *log.Logger
			if base != nil && rid != "" {
				l = log.New(base.Writer(), base.Prefix()+"["+rid+"] ", base.Flags())
			} else {
				l = base
			}
			ctx := context.WithValue(r.Context(), ctxKeyLogger, l)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestLogger 从 context 取 logger（可能为 nil）。
func RequestLogger(r *http.Request) *log.Logger {
	if v := r.Context().Value(ctxKeyLogger); v != nil {
		if l, ok := v.(*log.Logger); ok {
			return l
		}
	}
	return nil
}

// RecoverJSON panic 时返回统一 JSON。
func RecoverJSON(base *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					l := RequestLogger(r)
					if l != nil {
						l.Printf("httpapi panic: %v", rec)
					} else if base != nil {
						base.Printf("httpapi panic: %v", rec)
					}
					WriteError(w, l, ed2kmodel.NewAppError(ed2kmodel.CodeInternalError, "internal error", nil))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS 为浏览器跨域补充响应头（对齐 goed2kd）。
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Auth-Token")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthRPCSecret 校验与 aria2 rpc-secret 一致的令牌（Bearer / X-Auth-Token / ?token=）。
// secret 为空时不校验（与 JSON-RPC 无 secret 时行为一致）。
func AuthRPCSecret(secret string) func(http.Handler) http.Handler {
	sec := strings.TrimSpace(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sec == "" {
				next.ServeHTTP(w, r)
				return
			}
			got := extractToken(r)
			if got == "" || got != sec {
				WriteError(w, RequestLogger(r), ed2kmodel.NewAppError(ed2kmodel.CodeUnauthorized, "unauthorized", nil))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if v := r.Header.Get("X-Auth-Token"); v != "" {
		return strings.TrimSpace(v)
	}
	return r.URL.Query().Get("token")
}

// Timeout 包装请求超时。
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return middleware.Timeout(d)
}

// RequestID chi 中间件别名。
func RequestID() func(http.Handler) http.Handler {
	return middleware.RequestID
}

// RealIP chi 中间件别名。
func RealIP() func(http.Handler) http.Handler {
	return middleware.RealIP
}

func accessLog(lg *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			if lg != nil {
				lg.Printf("http %s %s %dms", r.Method, r.URL.Path, time.Since(start).Milliseconds())
			}
		})
	}
}
