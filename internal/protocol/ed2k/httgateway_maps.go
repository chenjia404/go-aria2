package ed2k

import (
	"strings"

	goed2k "github.com/monkeyWie/goed2k"
	"github.com/monkeyWie/goed2k/protocol"

	ed2kmodel "github.com/chenjia404/go-aria2/internal/rpc/ed2kapi/model"
)

func mapServer(s goed2k.ServerSnapshot) ed2kmodel.ServerDTO {
	return ed2kmodel.ServerDTO{
		Identifier:                   s.Identifier,
		Address:                      s.Address,
		Configured:                   s.Configured,
		Connected:                    s.Connected,
		HandshakeCompleted:           s.HandshakeCompleted,
		Primary:                      s.Primary,
		Disconnecting:                s.Disconnecting,
		ClientID:                     s.ClientID,
		IDClass:                      s.IDClass(),
		DownloadRate:                 s.DownloadRate,
		UploadRate:                   s.UploadRate,
		MillisecondsSinceLastReceive: s.MillisecondsSinceLastReceive,
	}
}

func mapDHT(d goed2k.DHTStatus) ed2kmodel.DHTStatusDTO {
	return ed2kmodel.DHTStatusDTO{
		Bootstrapped:      d.Bootstrapped,
		Firewalled:        d.Firewalled,
		LiveNodes:         d.LiveNodes,
		ReplacementNodes:  d.ReplacementNodes,
		RouterNodes:       d.RouterNodes,
		RunningTraversals: d.RunningTraversals,
		KnownNodes:        d.KnownNodes,
		InitialBootstrap:  d.InitialBootstrap,
		ListenPort:        d.ListenPort,
		StoragePoint:      d.StoragePoint,
	}
}

func mapTransfer(s goed2k.TransferSnapshot) ed2kmodel.TransferDTO {
	st := s.Status
	prog := float64(0)
	if st.TotalWanted > 0 {
		prog = float64(st.TotalDone) / float64(st.TotalWanted)
		if prog > 1 {
			prog = 1
		}
	}
	return ed2kmodel.TransferDTO{
		Hash:              s.Hash.String(),
		FileName:          s.FileName,
		FilePath:          s.FilePath,
		Size:              s.Size,
		CreateTime:        s.CreateTime,
		State:             string(st.State),
		Paused:            st.Paused,
		DownloadRate:      st.DownloadRate,
		UploadRate:        st.UploadRate,
		TotalDone:         st.TotalDone,
		TotalReceived:     st.TotalReceived,
		TotalWanted:       st.TotalWanted,
		ETA:               st.ETA,
		NumPeers:          st.NumPeers,
		ActivePeers:       s.ActivePeers,
		DownloadingPieces: st.DownloadingPieces,
		Progress:          prog,
		ED2KLink:          s.ED2KLink(),
	}
}

func mapPeer(p goed2k.PeerInfo) ed2kmodel.PeerDTO {
	return ed2kmodel.PeerDTO{
		Endpoint:             p.Endpoint.String(),
		DownloadSpeed:        p.DownloadSpeed,
		PayloadDownloadSpeed: p.PayloadDownloadSpeed,
		UploadSpeed:          p.UploadSpeed,
		Source:               p.SourceString(),
		ModName:              p.ModName,
		FailCount:            p.FailCount,
	}
}

func mapPiece(p goed2k.PieceSnapshot) ed2kmodel.PieceDTO {
	return ed2kmodel.PieceDTO{
		Index:         p.Index,
		State:         string(p.State),
		TotalBytes:    p.TotalBytes,
		DoneBytes:     p.DoneBytes,
		ReceivedBytes: p.ReceivedBytes,
		BlocksTotal:   p.BlocksTotal,
		BlocksDone:    p.BlocksDone,
		BlocksWriting: p.BlocksWriting,
		BlocksPending: p.BlocksPending,
	}
}

func parseHashParam(hexHash string) (protocol.Hash, error) {
	h, err := protocol.HashFromString(strings.TrimSpace(hexHash))
	if err != nil {
		return protocol.Invalid, err
	}
	return h, nil
}

// ParseSearchScope 将 API 字符串解析为 goed2k 搜索范围。
func ParseSearchScope(s string) goed2k.SearchScope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "server":
		return goed2k.SearchScopeServer
	case "dht", "kad":
		return goed2k.SearchScopeDHT
	case "all", "":
		return goed2k.SearchScopeAll
	default:
		return goed2k.SearchScopeAll
	}
}

func searchScopeToString(s goed2k.SearchScope) string {
	if s == goed2k.SearchScopeAll {
		return "all"
	}
	if s == goed2k.SearchScopeServer {
		return "server"
	}
	if s == goed2k.SearchScopeDHT {
		return "dht"
	}
	return "all"
}

func mapSearchSnapshot(snap goed2k.SearchSnapshot) ed2kmodel.SearchDTO {
	p := snap.Params
	dto := ed2kmodel.SearchDTO{
		ID:         snap.ID,
		State:      string(snap.State),
		UpdatedAt:  snap.UpdatedAt,
		StartedAt:  snap.StartedAt,
		ServerBusy: snap.ServerBusy,
		DHTBusy:    snap.DHTBusy,
		KadKeyword: snap.KadKeyword,
		Error:      snap.Error,
		Params: ed2kmodel.SearchParamsDTO{
			Query:              p.Query,
			Scope:              searchScopeToString(p.Scope),
			MinSize:            p.MinSize,
			MaxSize:            p.MaxSize,
			MinSources:         p.MinSources,
			MinCompleteSources: p.MinCompleteSources,
			FileType:           p.FileType,
			Extension:          p.Extension,
		},
	}
	dto.Results = make([]ed2kmodel.SearchResultDTO, 0, len(snap.Results))
	for _, r := range snap.Results {
		dto.Results = append(dto.Results, mapSearchResult(r))
	}
	if snap.ID == 0 && snap.State == "" {
		dto.State = "IDLE"
	}
	return dto
}

func mapSearchResult(r goed2k.SearchResult) ed2kmodel.SearchResultDTO {
	src := ""
	switch {
	case r.Source&(goed2k.SearchResultServer|goed2k.SearchResultKAD) == (goed2k.SearchResultServer | goed2k.SearchResultKAD):
		src = "server|kad"
	case r.Source&goed2k.SearchResultServer != 0:
		src = "server"
	case r.Source&goed2k.SearchResultKAD != 0:
		src = "kad"
	default:
		src = "unknown"
	}
	return ed2kmodel.SearchResultDTO{
		Hash:            r.Hash.String(),
		FileName:        r.FileName,
		FileSize:        r.FileSize,
		Sources:         r.Sources,
		CompleteSources: r.CompleteSources,
		MediaBitrate:    r.MediaBitrate,
		MediaLength:     r.MediaLength,
		MediaCodec:      r.MediaCodec,
		Extension:       r.Extension,
		FileType:        r.FileType,
		Source:          src,
		ED2KLink:        r.ED2KLink(),
	}
}
