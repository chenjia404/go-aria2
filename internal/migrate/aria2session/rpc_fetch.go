package aria2session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FetchAria2SessionTasksFromRPC 从运行中的 aria2 实例拉取任务列表。
func FetchAria2SessionTasksFromRPC(ctx context.Context, endpoint, secret string) ([]Aria2SessionTask, error) {
	client := NewRPCClient(endpoint, secret)
	statuses, err := fetchAllRPCStatuses(ctx, client)
	if err != nil {
		return nil, err
	}

	out := make([]Aria2SessionTask, 0, len(statuses))
	seen := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		gid := strings.ToLower(strings.TrimSpace(mapString(status, "gid")))
		if gid == "" {
			continue
		}
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}

		options, err := fetchRPCOptions(ctx, client, gid)
		if err != nil {
			return nil, fmt.Errorf("getOption %s: %w", gid, err)
		}
		item, err := sessionTaskFromRPC(status, options)
		if err != nil {
			return nil, fmt.Errorf("gid %s: %w", gid, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func fetchAllRPCStatuses(ctx context.Context, client *RPCClient) ([]map[string]any, error) {
	var all []map[string]any

	activeRaw, err := client.Call(ctx, "aria2.tellActive")
	if err != nil {
		return nil, err
	}
	active, err := decodeStatusList(activeRaw)
	if err != nil {
		return nil, err
	}
	all = append(all, active...)

	for _, method := range []string{"aria2.tellWaiting", "aria2.tellStopped"} {
		offset := 0
		const pageSize = 1000
		for {
			pageRaw, err := client.Call(ctx, method, offset, pageSize)
			if err != nil {
				return nil, err
			}
			page, err := decodeStatusList(pageRaw)
			if err != nil {
				return nil, err
			}
			all = append(all, page...)
			if len(page) < pageSize {
				break
			}
			offset += pageSize
		}
	}
	return all, nil
}

func fetchRPCOptions(ctx context.Context, client *RPCClient, gid string) (map[string]string, error) {
	raw, err := client.Call(ctx, "aria2.getOption", gid)
	if err != nil {
		return nil, err
	}
	var options map[string]string
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, err
	}
	if options == nil {
		options = map[string]string{}
	}
	return options, nil
}

func decodeStatusList(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func sessionTaskFromRPC(status map[string]any, options map[string]string) (Aria2SessionTask, error) {
	uri := firstURIFromRPCStatus(status)
	if uri == "" {
		return Aria2SessionTask{}, fmt.Errorf("missing download URI")
	}
	opts := cloneMap(options)
	opts["aria2.import.source"] = "aria2-rpc"

	item := Aria2SessionTask{
		GID:      strings.ToLower(strings.TrimSpace(mapString(status, "gid"))),
		URI:      uri,
		Dir:      firstNonEmpty(options["dir"], mapString(status, "dir")),
		Out:      firstNonEmpty(options["out"], derivePreviewName(uri)),
		Checksum: options["checksum"],
		Metalink: options["metalink"],
		Options:  opts,
	}
	switch strings.ToLower(mapString(status, "status")) {
	case "paused":
		item.Paused = true
	}
	if parseBoolValue(options["pause"]) || parseBoolValue(options["paused"]) {
		item.Paused = true
	}
	return item, nil
}

func firstURIFromRPCStatus(status map[string]any) string {
	files, ok := status["files"].([]any)
	if !ok {
		return ""
	}
	for _, rawFile := range files {
		file, ok := rawFile.(map[string]any)
		if !ok {
			continue
		}
		uris, ok := file["uris"].([]any)
		if !ok {
			continue
		}
		for _, rawURI := range uris {
			switch entry := rawURI.(type) {
			case map[string]any:
				if uri := strings.TrimSpace(mapString(entry, "uri")); uri != "" {
					return uri
				}
			case string:
				if strings.TrimSpace(entry) != "" {
					return entry
				}
			}
		}
	}
	return ""
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	value, ok := m[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func importSource(item Aria2SessionTask) string {
	if item.Options != nil {
		if source := strings.TrimSpace(item.Options["aria2.import.source"]); source != "" {
			return source
		}
	}
	return "save-session"
}
