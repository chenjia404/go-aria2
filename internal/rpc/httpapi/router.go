package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/chenjia404/go-aria2/internal/config"
	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
	"github.com/go-chi/chi/v5"
)

// Server 仅服务 ED2K HTTP API（路径与 goed2kd 对齐，不经 manager）。
type Server struct {
	Log                *log.Logger
	Gateway            *ed2k.HTTPGateway
	CFG                *config.Config
	ED2KStatePath      string
	RPCListenAddr      string
	RPCSecret          string
	ReadTimeoutSeconds int
}

// NewRouter 构建 chi 路由树，前缀为 /api/v1。
func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	readTO := time.Duration(s.ReadTimeoutSeconds) * time.Second
	if readTO <= 0 {
		readTO = 60 * time.Second
	}

	r.Use(RequestID())
	r.Use(WithRequestLogger(s.Log))
	r.Use(RecoverJSON(s.Log))
	r.Use(RealIP())
	r.Use(CORS())
	if s.Log != nil {
		r.Use(accessLog(s.Log))
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/system/health", s.handleHealth)

		r.Group(func(r chi.Router) {
			r.Use(Timeout(readTO + 30*time.Second))
			r.Use(AuthRPCSecret(s.RPCSecret))

			r.Get("/system/info", s.handleInfo)
			r.Post("/system/start", s.handleStart)
			r.Post("/system/stop", s.handleStop)
			r.Post("/system/save-state", s.handleSaveState)
			r.Post("/system/load-state", s.handleLoadState)
			r.Get("/system/config", s.handleGetConfig)
			r.Put("/system/config", s.handlePutConfig)

			r.Get("/network/servers", s.handleNetworkServers)
			r.Post("/network/servers/connect", s.handleNetworkConnect)
			r.Post("/network/servers/connect-batch", s.handleNetworkConnectBatch)
			r.Post("/network/servers/load-met", s.handleNetworkLoadMet)
			r.Get("/network/dht", s.handleNetworkDHT)
			r.Post("/network/dht/enable", s.handleNetworkDHTEnable)
			r.Post("/network/dht/load-nodes", s.handleNetworkDHTLoadNodes)
			r.Post("/network/dht/bootstrap-nodes", s.handleNetworkDHTBootstrap)

			r.Get("/transfers", s.handleTransfersList)
			r.Post("/transfers", s.handleTransfersAdd)
			r.Get("/transfers/{hash}", s.handleTransfersDetail)
			r.Post("/transfers/{hash}/pause", s.handleTransfersPause)
			r.Post("/transfers/{hash}/resume", s.handleTransfersResume)
			r.Delete("/transfers/{hash}", s.handleTransfersDelete)
			r.Get("/transfers/{hash}/peers", s.handleTransfersPeers)
			r.Get("/transfers/{hash}/pieces", s.handleTransfersPieces)

			r.Post("/searches", s.handleSearchesCreate)
			r.Get("/searches/current", s.handleSearchesCurrent)
			r.Post("/searches/current/stop", s.handleSearchesStop)
			r.Post("/searches/current/results/{hash}/download", s.handleSearchesResultDownload)
		})
	})

	return r
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) mustGateway() (*ed2k.HTTPGateway, error) {
	if s.Gateway == nil {
		return nil, ed2kmodel.NewAppError(ed2kmodel.CodeEngineNotRunning, "ed2k gateway unavailable", nil)
	}
	return s.Gateway, nil
}
