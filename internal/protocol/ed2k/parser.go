package ed2k

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "ed2k://") {
		return nil, fmt.Errorf("invalid ed2k URI")
	}

	parts := strings.Split(strings.TrimPrefix(raw, "ed2k://"), "|")
	if len(parts) < 6 {
		return nil, fmt.Errorf("invalid ed2k URI format")
	}
	if parts[1] != "file" {
		return nil, fmt.Errorf("unsupported ed2k entity %q", parts[1])
	}

	name, err := url.PathUnescape(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode ed2k file name failed: %w", err)
	}
	size, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ed2k size: %w", err)
	}

	item := &link{
		Name:      name,
		Size:      size,
		Hash:      strings.ToLower(parts[4]),
		SourceURI: raw,
	}

	for _, part := range parts[5:] {
		switch {
		case strings.HasPrefix(part, "h="):
			item.AICH = strings.TrimPrefix(part, "h=")
		case strings.HasPrefix(part, "s="):
			item.Sources = append(item.Sources, strings.TrimPrefix(part, "s="))
		}
	}
	return item, nil
}

func toTaskFile(item *link) task.File {
	return task.File{
		Index:           0,
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
