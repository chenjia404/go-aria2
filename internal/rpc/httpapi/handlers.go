package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	gw := s.Gateway
	if gw == nil {
		WriteSuccess(w, map[string]any{
			"daemon_running": true, "engine_running": false, "state_store_ok": true, "rpc_available": true,
		})
		return
	}
	h, err := gw.Health(r.Context())
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, h)
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, gw.Info(r.Context()))
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	_, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: system.start (no-op, ed2k always active in go-aria2)")
	}
	WriteSuccess(w, map[string]any{"started": true, "note": "go-aria2: ED2K client is always active"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest,
		"stopping embedded ED2K via API is not supported; stop the go-aria2 daemon instead", nil))
}

func (s *Server) handleSaveState(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if err := gw.SaveState(r.Context()); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: system.save-state")
	}
	WriteSuccess(w, map[string]any{"saved": true})
}

func (s *Server) handleLoadState(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if err := gw.LoadState(r.Context()); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: system.load-state")
	}
	WriteSuccess(w, map[string]any{"loaded": true})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	WriteSuccess(w, buildConfigSummary(s.CFG, s.ED2KStatePath, s.RPCListenAddr))
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	WriteError(w, RequestLogger(r), ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest,
		"config hot-reload is not implemented for go-aria2; edit aria2.conf and restart", nil))
}

func (s *Server) handleNetworkServers(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	list, err := gw.Servers(r.Context())
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, list)
}

type connectOne struct {
	Address string `json:"address"`
}

func (s *Server) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body connectOne
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	if err := gw.ConnectServer(r.Context(), body.Address); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: network.connect address=%s", body.Address)
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

type connectBatch struct {
	Addresses []string `json:"addresses"`
}

func (s *Server) handleNetworkConnectBatch(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body connectBatch
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	if err := gw.ConnectServers(r.Context(), body.Addresses); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: network.connect_batch count=%d", len(body.Addresses))
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

type sourcesBody struct {
	Sources []string `json:"sources"`
}

func (s *Server) handleNetworkLoadMet(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body sourcesBody
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	if err := gw.LoadServerMetSources(r.Context(), body.Sources); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: network.load_server_met count=%d", len(body.Sources))
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

func (s *Server) handleNetworkDHT(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	st, err := gw.DHTStatus(r.Context())
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, st)
}

func (s *Server) handleNetworkDHTEnable(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if err := gw.EnableDHT(r.Context()); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: network.dht_enable")
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

func (s *Server) handleNetworkDHTLoadNodes(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body sourcesBody
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	if err := gw.LoadDHTNodesSources(r.Context(), body.Sources); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: network.load_nodes_dat count=%d", len(body.Sources))
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

type nodesBody struct {
	Nodes []string `json:"nodes"`
}

func (s *Server) handleNetworkDHTBootstrap(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body nodesBody
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	if err := gw.AddDHTBootstrapNodes(r.Context(), body.Nodes); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: network.dht_bootstrap count=%d", len(body.Nodes))
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

type listQuery struct {
	State  string
	Sort   string
	Limit  int
	Offset int
	Paused *bool
}

func (s *Server) handleTransfersList(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	q := listQuery{State: r.URL.Query().Get("state"), Sort: r.URL.Query().Get("sort")}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			q.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			q.Offset = n
		}
	}
	if v := r.URL.Query().Get("paused"); v != "" {
		b := v == "true" || v == "1"
		q.Paused = &b
	}
	all, err := gw.ListTransfers(r.Context())
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	out := make([]ed2kmodel.TransferDTO, 0, len(all))
	for _, t := range all {
		if q.State != "" && !strings.EqualFold(t.State, q.State) {
			continue
		}
		if q.Paused != nil && t.Paused != *q.Paused {
			continue
		}
		out = append(out, t)
	}
	if q.Offset > 0 && q.Offset < len(out) {
		out = out[q.Offset:]
	} else if q.Offset >= len(out) {
		out = nil
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	_ = q.Sort
	WriteSuccess(w, out)
}

type addTransferBody struct {
	ED2KLink   string `json:"ed2k_link"`
	TargetDir  string `json:"target_dir"`
	TargetName string `json:"target_name"`
	Paused     bool   `json:"paused"`
}

func (s *Server) handleTransfersAdd(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body addTransferBody
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	t, err := gw.AddTransferByED2K(r.Context(), ed2k.AddTransferParams{
		ED2KLink: body.ED2KLink, TargetDir: body.TargetDir, TargetName: body.TargetName, Paused: body.Paused,
	})
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: transfer.add hash=%s", t.Hash)
	}
	WriteSuccess(w, t)
}

func (s *Server) handleTransfersDetail(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	hash := chi.URLParam(r, "hash")
	d, err := gw.GetTransfer(r.Context(), hash)
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, d)
}

func (s *Server) handleTransfersPause(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	hash := chi.URLParam(r, "hash")
	if err := gw.PauseTransfer(r.Context(), hash); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: transfer.pause hash=%s", hash)
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

func (s *Server) handleTransfersResume(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	hash := chi.URLParam(r, "hash")
	if err := gw.ResumeTransfer(r.Context(), hash); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: transfer.resume hash=%s", hash)
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

func (s *Server) handleTransfersDelete(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	hash := chi.URLParam(r, "hash")
	delFiles := r.URL.Query().Get("delete_files") == "true" || r.URL.Query().Get("delete_files") == "1"
	if err := gw.DeleteTransfer(r.Context(), hash, delFiles); err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: transfer.delete hash=%s delete_files=%v", hash, delFiles)
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

func (s *Server) handleTransfersPeers(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	hash := chi.URLParam(r, "hash")
	p, err := gw.ListTransferPeers(r.Context(), hash)
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, p)
}

func (s *Server) handleTransfersPieces(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	hash := chi.URLParam(r, "hash")
	p, err := gw.ListTransferPieces(r.Context(), hash)
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, p)
}

func (s *Server) handleSearchesCreate(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	var body ed2kmodel.SearchParamsDTO
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	sch, err := gw.StartSearch(r.Context(), body)
	if err != nil {
		WriteError(w, log, err)
		return
	}
	WriteSuccess(w, sch)
}

func (s *Server) handleSearchesCurrent(w http.ResponseWriter, r *http.Request) {
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	sch, err := gw.CurrentSearch(r.Context())
	if err != nil {
		WriteError(w, RequestLogger(r), err)
		return
	}
	WriteSuccess(w, sch)
}

func (s *Server) handleSearchesStop(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if err := gw.StopSearch(r.Context()); err != nil {
		WriteError(w, log, err)
		return
	}
	WriteSuccess(w, map[string]any{"ok": true})
}

type resultDLBody struct {
	TargetDir  string `json:"target_dir"`
	TargetName string `json:"target_name"`
	Paused     bool   `json:"paused"`
}

func (s *Server) handleSearchesResultDownload(w http.ResponseWriter, r *http.Request) {
	log := RequestLogger(r)
	gw, err := s.mustGateway()
	if err != nil {
		WriteError(w, log, err)
		return
	}
	hash := chi.URLParam(r, "hash")
	var body resultDLBody
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, log, ed2kmodel.NewAppError(ed2kmodel.CodeBadRequest, "invalid json", err))
		return
	}
	t, err := gw.AddTransferFromSearchResult(r.Context(), hash, ed2k.AddTransferParams{
		TargetDir: body.TargetDir, TargetName: body.TargetName, Paused: body.Paused,
	})
	if err != nil {
		WriteError(w, log, err)
		return
	}
	if log != nil {
		log.Printf("httpapi audit: search.result_download hash=%s", hash)
	}
	WriteSuccess(w, t)
}
