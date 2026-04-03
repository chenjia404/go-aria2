package bt

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	torrentlib "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

type addSource struct {
	Kind          string
	URI           string
	DisplayName   string
	TotalLength   int64
	TorrentBase64 string
	Trackers      []string
}

type addResult struct {
	Spec   *torrentlib.TorrentSpec
	Source addSource
}

// torrentSpecFromSource 从已保存的 addSource 重建仅含内建 tracker 的 Spec（与 magnet/种子一致）。
func torrentSpecFromSource(src addSource) (*torrentlib.TorrentSpec, error) {
	switch src.Kind {
	case "magnet":
		return torrentlib.TorrentSpecFromMagnetUri(src.URI)
	case "torrent-bytes", "torrent-url":
		if src.TorrentBase64 == "" {
			return nil, fmt.Errorf("missing torrent payload")
		}
		payload, err := base64.StdEncoding.DecodeString(src.TorrentBase64)
		if err != nil {
			return nil, err
		}
		mi, _, err := loadMetaInfo(payload)
		if err != nil {
			return nil, err
		}
		return torrentlib.TorrentSpecFromMetaInfoErr(mi)
	default:
		return nil, fmt.Errorf("unsupported bt source kind %q", src.Kind)
	}
}

func parseAddInput(ctx context.Context, input task.AddTaskInput) (*addResult, error) {
	if len(input.Torrent) > 0 {
		mi, raw, err := loadMetaInfo(input.Torrent)
		if err != nil {
			return nil, err
		}
		spec, err := torrentlib.TorrentSpecFromMetaInfoErr(mi)
		if err != nil {
			return nil, err
		}
		return &addResult{
			Spec: spec,
			Source: addSource{
				Kind:          "torrent-bytes",
				DisplayName:   spec.DisplayName,
				TorrentBase64: base64.StdEncoding.EncodeToString(raw),
				Trackers:      flattenTrackers(spec.Trackers),
			},
		}, nil
	}

	for _, rawURI := range append([]string{input.URI}, input.URIs...) {
		uri := strings.TrimSpace(rawURI)
		if uri == "" {
			continue
		}

		lower := strings.ToLower(uri)
		switch {
		case strings.HasPrefix(lower, "magnet:"):
			spec, err := torrentlib.TorrentSpecFromMagnetUri(uri)
			if err != nil {
				return nil, err
			}
			return &addResult{
				Spec: spec,
				Source: addSource{
					Kind:        "magnet",
					URI:         uri,
					DisplayName: spec.DisplayName,
					TotalLength: magnetLength(uri),
					Trackers:    flattenTrackers(spec.Trackers),
				},
			}, nil
		case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
			if !strings.HasSuffix(lower, ".torrent") {
				continue
			}
			payload, err := fetchTorrent(ctx, uri)
			if err != nil {
				return nil, err
			}
			mi, raw, err := loadMetaInfo(payload)
			if err != nil {
				return nil, err
			}
			spec, err := torrentlib.TorrentSpecFromMetaInfoErr(mi)
			if err != nil {
				return nil, err
			}
			return &addResult{
				Spec: spec,
				Source: addSource{
					Kind:          "torrent-url",
					URI:           uri,
					DisplayName:   spec.DisplayName,
					TorrentBase64: base64.StdEncoding.EncodeToString(raw),
					Trackers:      flattenTrackers(spec.Trackers),
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("unsupported bt input")
}

func restoreSource(meta map[string]string) (*addResult, error) {
	switch meta["bt.source.kind"] {
	case "magnet":
		spec, err := torrentlib.TorrentSpecFromMagnetUri(meta["bt.source.uri"])
		if err != nil {
			return nil, err
		}
		return &addResult{
			Spec: spec,
			Source: addSource{
				Kind:        "magnet",
				URI:         meta["bt.source.uri"],
				DisplayName: spec.DisplayName,
				TotalLength: magnetLength(meta["bt.source.uri"]),
				Trackers:    flattenTrackers(spec.Trackers),
			},
		}, nil
	case "torrent-bytes", "torrent-url":
		payload, err := base64.StdEncoding.DecodeString(meta["bt.source.torrentBase64"])
		if err != nil {
			return nil, err
		}
		mi, raw, err := loadMetaInfo(payload)
		if err != nil {
			return nil, err
		}
		spec, err := torrentlib.TorrentSpecFromMetaInfoErr(mi)
		if err != nil {
			return nil, err
		}
		return &addResult{
			Spec: spec,
			Source: addSource{
				Kind:          meta["bt.source.kind"],
				URI:           meta["bt.source.uri"],
				DisplayName:   spec.DisplayName,
				TorrentBase64: base64.StdEncoding.EncodeToString(raw),
				Trackers:      flattenTrackers(spec.Trackers),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported bt session source")
	}
}

func fetchTorrent(ctx context.Context, rawURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch torrent URL failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch torrent URL returned status %s", response.Status)
	}
	return io.ReadAll(response.Body)
}

func loadMetaInfo(payload []byte) (*metainfo.MetaInfo, []byte, error) {
	raw := append([]byte(nil), payload...)
	mi, err := metainfo.Load(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	return mi, raw, nil
}

func magnetLength(raw string) int64 {
	parsed, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	size, _ := strconv.ParseInt(parsed.Query().Get("xl"), 10, 64)
	return size
}

func flattenTrackers(trackers [][]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, tier := range trackers {
		for _, tracker := range tier {
			if tracker == "" {
				continue
			}
			if _, ok := seen[tracker]; ok {
				continue
			}
			seen[tracker] = struct{}{}
			out = append(out, tracker)
		}
	}
	return out
}

func placeholderFiles(source addSource, saveDir, name string) []task.File {
	resolved := name
	if resolved == "" {
		resolved = filepath.Base(source.URI)
	}
	if resolved == "" || resolved == "." || resolved == "/" {
		resolved = "magnet-task"
	}
	if saveDir != "" {
		resolved = filepath.Join(saveDir, resolved)
	}
	uris := []string{}
	if source.URI != "" {
		uris = append(uris, source.URI)
	}
	return []task.File{{
		Index:           0,
		Path:            resolved,
		Length:          source.TotalLength,
		CompletedLength: 0,
		Selected:        true,
		URIs:            uris,
	}}
}
