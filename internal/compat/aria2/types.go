package aria2

import (
	"strconv"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/manager"
	"github.com/chenjia404/go-aria2/internal/core/task"
)

// toStatusResponse 将内部任务模型映射为 aria2 兼容返回结构�?
func toStatusResponse(item *task.Task, keys []string) map[string]any {
	all := map[string]any{
		"gid":                    item.GID,
		"status":                 string(item.Status),
		"totalLength":            formatInt64(item.TotalLength),
		"completedLength":        formatInt64(item.CompletedLength),
		"uploadLength":           formatInt64(item.UploadedLength),
		"downloadSpeed":          formatInt64(item.DownloadSpeed),
		"uploadSpeed":            formatInt64(item.UploadSpeed),
		"connections":            strconv.Itoa(item.Connections),
		"numSeeders":             strconv.Itoa(item.NumSeeders),
		"pieceLength":            formatInt64(item.PieceLength),
		"verifiedLength":         formatInt64(item.VerifiedLength),
		"verifyIntegrityPending": strconv.FormatBool(item.VerifyIntegrityPending),
		"seeder":                 strconv.FormatBool(item.Seeder),
		"infoHash":               item.InfoHash,
		"dir":                    item.SaveDir,
		"files":                  toFilesResponse(item.Files),
		"errorCode":              item.ErrorCode,
		"errorMessage":           item.ErrorMessage,
	}
	if bittorrent := toBitTorrentResponse(item); bittorrent != nil {
		all["bittorrent"] = bittorrent
	}

	if len(keys) == 0 {
		return all
	}

	filtered := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := all[key]; ok {
			filtered[key] = value
		}
	}
	return filtered
}

// toFilesResponse 将统一文件模型映射�?aria2 文件视图�?
func toFilesResponse(files []task.File) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		out = append(out, map[string]any{
			"index":           strconv.Itoa(file.Index),
			"path":            file.Path,
			"length":          formatInt64(file.Length),
			"completedLength": formatInt64(file.CompletedLength),
			"selected":        strconv.FormatBool(file.Selected),
			"uris":            toURIResponse(file.URIs),
		})
	}
	return out
}

// toPeersResponse 将统一 peer 视图映射�?aria2 getPeers 返回结构�?
func toPeersResponse(peers []manager.PeerInfo) []map[string]any {
	out := make([]map[string]any, 0, len(peers))
	for _, peer := range peers {
		out = append(out, map[string]any{
			"peerId":        peer.PeerID,
			"ip":            peer.IP,
			"port":          strconv.Itoa(peer.Port),
			"bitfield":      peer.Bitfield,
			"amChoking":     strconv.FormatBool(peer.AmChoking),
			"peerChoking":   strconv.FormatBool(peer.PeerChoking),
			"downloadSpeed": formatInt64(peer.DownloadSpeed),
			"uploadSpeed":   formatInt64(peer.UploadSpeed),
			"seeder":        strconv.FormatBool(peer.Seeder),
		})
	}
	return out
}

// toServersResponse 将统一服务器视图映射为 aria2 getServers 返回结构�?
func toServersResponse(files []manager.FileServerInfo) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		servers := make([]map[string]any, 0, len(file.Servers))
		for _, server := range file.Servers {
			servers = append(servers, map[string]any{
				"uri":           server.URI,
				"currentUri":    server.CurrentURI,
				"downloadSpeed": formatInt64(server.DownloadSpeed),
			})
		}
		out = append(out, map[string]any{
			"index":   strconv.Itoa(file.Index),
			"servers": servers,
		})
	}
	return out
}

// toURIsResponse 将统一文件 URI 列表映射�?aria2 URI 视图�?
func toURIsResponse(files []task.File) []map[string]any {
	out := make([]map[string]any, 0)
	seen := map[string]struct{}{}
	for _, file := range files {
		for _, uri := range file.URIs {
			if uri == "" {
				continue
			}
			if _, ok := seen[uri]; ok {
				continue
			}
			seen[uri] = struct{}{}
			out = append(out, map[string]any{
				"uri":    uri,
				"status": "used",
			})
		}
	}
	return out
}

// toGlobalStatResponse 将全局统计映射�?aria2 风格结构�?
func toGlobalStatResponse(stat manager.GlobalStat) map[string]any {
	return map[string]any{
		"numActive":     strconv.Itoa(stat.NumActive),
		"numWaiting":    strconv.Itoa(stat.NumWaiting),
		"numStopped":    strconv.Itoa(stat.NumStopped),
		"downloadSpeed": formatInt64(stat.DownloadSpeed),
		"uploadSpeed":   formatInt64(stat.UploadSpeed),
	}
}

// TaskToAria2StatusJSON 与 aria2.tellStatus 全量字段一致（数值均为字符串），供 WebSocket 等前端消费。
func TaskToAria2StatusJSON(item *task.Task) map[string]any {
	if item == nil {
		return nil
	}
	return toStatusResponse(item, nil)
}

// GlobalStatToAria2JSON 与 aria2.getGlobalStat 字段一致。
func GlobalStatToAria2JSON(stat manager.GlobalStat) map[string]any {
	return toGlobalStatResponse(stat)
}

// toOptionResponse 将统一任务选项映射�?aria2 风格选项�?
func toOptionResponse(item *task.Task) map[string]string {
	options := cloneOptionMap(item.Options)
	if item.SaveDir != "" {
		options["dir"] = item.SaveDir
	}
	if item.Status == task.StatusPaused {
		options["pause"] = "true"
	} else {
		options["pause"] = "false"
	}
	if item.Name != "" {
		options["out"] = item.Name
	}
	return options
}

func cloneOptionMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func toURIResponse(uris []string) []map[string]any {
	out := make([]map[string]any, 0, len(uris))
	for _, uri := range uris {
		out = append(out, map[string]any{
			"uri":    uri,
			"status": "used",
		})
	}
	return out
}

func toBitTorrentResponse(item *task.Task) map[string]any {
	if item.Protocol != task.ProtocolBT {
		return nil
	}

	response := map[string]any{
		"mode": item.Meta["bt.mode"],
		"info": map[string]any{
			"name": item.Name,
		},
	}

	if trackers := splitMetaLines(item.Meta["bt.trackers"]); len(trackers) > 0 {
		response["announceList"] = trackers
	}
	if comment := item.Meta["bt.comment"]; comment != "" {
		response["comment"] = comment
	}
	if createdBy := item.Meta["bt.createdBy"]; createdBy != "" {
		response["createdBy"] = createdBy
	}
	if creationDate := item.Meta["bt.creationDate"]; creationDate != "" {
		response["creationDate"] = creationDate
	}
	return response
}

func splitMetaLines(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}
