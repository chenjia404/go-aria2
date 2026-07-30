package ed2k

import (
	"fmt"
	"net/url"
	"strings"

	goed2k "github.com/goed2k/core"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

type link struct {
	Name      string
	Size      int64
	Hash      string
	AICH      string
	Sources   []string
	SourceURI string
}

func parseLink(raw string) (*link, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("invalid ed2k URI")
	}

	baseURI := baseEMuleLink(raw)
	parsed, err := goed2k.ParseEMuleLink(baseURI)
	if err != nil {
		return nil, fmt.Errorf("invalid ed2k URI: %w", err)
	}
	if parsed.Type != goed2k.LinkFile {
		return nil, fmt.Errorf("unsupported ed2k entity %q", parsed.Type)
	}

	aich, sources := parseLinkExtras(raw)
	return &link{
		Name:      parsed.StringValue,
		Size:      parsed.NumberValue,
		Hash:      strings.ToLower(parsed.Hash.String()),
		AICH:      aich,
		Sources:   sources,
		SourceURI: raw,
	}, nil
}

// baseEMuleLink 剥离 h=/s= 等扩展段，供 goed2k/core ParseEMuleLink 解析标准五段文件链接。
func baseEMuleLink(raw string) string {
	parts := strings.Split(raw, "|")
	if len(parts) < 5 {
		return raw
	}
	return strings.Join(append(parts[:5], "/"), "|")
}

// parseLinkExtras 从 ed2k 文件链接尾部提取 goed2k.ParseEMuleLink 不解析的扩展段（h=、s=）。
// 在原始 URI 上按 | 切分，仅对扩展值做解码，避免整段 QueryUnescape 破坏段内转义。
func parseLinkExtras(raw string) (aich string, sources []string) {
	parts := strings.Split(raw, "|")
	for _, part := range parts[5:] {
		switch {
		case strings.HasPrefix(part, "h="):
			aich = decodeLinkExtraValue(strings.TrimPrefix(part, "h="))
		case strings.HasPrefix(part, "s="):
			sources = append(sources, decodeLinkExtraValue(strings.TrimPrefix(part, "s=")))
		}
	}
	return aich, sources
}

func decodeLinkExtraValue(v string) string {
	decoded, err := url.QueryUnescape(v)
	if err != nil {
		return v
	}
	return decoded
}

func toTaskFile(item *link) task.File {
	return task.File{
		Index:           1,
		Path:            item.Name,
		Length:          item.Size,
		CompletedLength: 0,
		Selected:        true,
		URIs:            []string{item.SourceURI},
	}
}

func firstLink(input task.AddTaskInput) (*link, error) {
	links, err := collectLinks(input)
	if err != nil {
		return nil, err
	}
	return links[0], nil
}

func collectLinks(input task.AddTaskInput) ([]*link, error) {
	seen := map[string]struct{}{}
	out := make([]*link, 0)
	for _, raw := range append([]string{input.URI}, input.URIs...) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		link, err := parseLink(raw)
		if err != nil {
			return nil, err
		}
		seen[raw] = struct{}{}
		out = append(out, link)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("missing ed2k URI")
	}
	return out, nil
}

func linkURIs(links []*link) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		if l != nil && l.SourceURI != "" {
			out = append(out, l.SourceURI)
		}
	}
	return out
}

func primaryURI(uris []string) string {
	if len(uris) == 0 {
		return ""
	}
	return uris[0]
}

func urisFromSession(saved *task.Task) []string {
	if saved == nil {
		return nil
	}
	if len(saved.Files) > 0 && len(saved.Files[0].URIs) > 0 {
		return append([]string(nil), saved.Files[0].URIs...)
	}
	if saved.Meta != nil && saved.Meta["ed2k.sourceURI"] != "" {
		return []string{saved.Meta["ed2k.sourceURI"]}
	}
	return nil
}

func cloneED2KMeta(base map[string]string, link *link) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	out["ed2k.hash"] = link.Hash
	out["ed2k.aich"] = link.AICH
	out["ed2k.sources"] = strings.Join(link.Sources, "\n")
	out["ed2k.sourceURI"] = link.SourceURI
	return out
}
