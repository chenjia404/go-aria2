package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
)

// APIResponse 统一 HTTP JSON 响应（对齐 goed2kd）。
type APIResponse struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// WriteSuccess 写入成功响应。
func WriteSuccess(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(APIResponse{Code: ed2kmodel.CodeOK, Data: data})
}

// WriteError 写入错误响应。
func WriteError(w http.ResponseWriter, lg *log.Logger, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var ae *ed2kmodel.AppError
	if errors.As(err, &ae) {
		status := codeToHTTPStatus(ae.Code)
		w.WriteHeader(status)
		msg := ae.Message
		if msg == "" && ae.Err != nil {
			msg = ae.Err.Error()
		}
		if lg != nil {
			lg.Printf("httpapi: api error code=%s msg=%s err=%v", ae.Code, msg, ae.Err)
		}
		_ = json.NewEncoder(w).Encode(APIResponse{Code: ae.Code, Message: msg})
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	if lg != nil {
		lg.Printf("httpapi: internal error: %v", err)
	}
	_ = json.NewEncoder(w).Encode(APIResponse{
		Code:    ed2kmodel.CodeInternalError,
		Message: "internal error",
	})
}

func codeToHTTPStatus(code string) int {
	switch code {
	case ed2kmodel.CodeBadRequest, ed2kmodel.CodeInvalidHash, ed2kmodel.CodeInvalidED2KLink, ed2kmodel.CodeConfigInvalid:
		return http.StatusBadRequest
	case ed2kmodel.CodeUnauthorized:
		return http.StatusUnauthorized
	case ed2kmodel.CodeForbidden:
		return http.StatusForbidden
	case ed2kmodel.CodeNotFound, ed2kmodel.CodeTransferNotFound, ed2kmodel.CodeSearchNotRunning:
		return http.StatusNotFound
	case ed2kmodel.CodeEngineNotRunning:
		return http.StatusServiceUnavailable
	case ed2kmodel.CodeEngineAlreadyRunning, ed2kmodel.CodeSearchAlreadyRunning:
		return http.StatusConflict
	case ed2kmodel.CodeStateStoreError:
		return http.StatusBadRequest
	case ed2kmodel.CodeInternalError:
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}
