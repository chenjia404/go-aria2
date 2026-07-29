package ed2k

import (
	"fmt"
	"strings"

	goed2k "github.com/monkeyWie/goed2k"

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

	parsed, err := goed2k.ParseEMuleLink(raw)
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

// parseLinkExtras 从 ed2k 文件链接尾部提取 goed2k.ParseEMuleLink 不解析的扩展段（h=、s=）。
func parseLinkExtras(raw string) (aich string, sources []string) {
	parts := strings.Split(raw, "|")
	for _, part := range parts[5:] {
		switch {
		case strings.HasPrefix(part, "h="):
			aich = strings.TrimPrefix(part, "h=")
		case strings.HasPrefix(part, "s="):
			sources = append(sources, strings.TrimPrefix(part, "s="))
		}
	}
	return aich, sources
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
	for _, raw := range append([]string{input.URI}, input.URIs...) {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		return parseLink(raw)
	}
	return nil, fmt.Errorf("missing ed2k URI")
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
