package metalink

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// File 表示 metalink 文档中的单个文件条目。
type File struct {
	Name     string
	Size     int64
	URLs     []string
	Checksum string
}

// Parse 解析 Metalink 3/4 XML，返回其中声明的文件列表。
func Parse(data []byte) ([]File, error) {
	data = trimBOM(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("empty metalink payload")
	}

	var doc struct {
		Files []xmlFile `xml:"file"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse metalink xml: %w", err)
	}
	if len(doc.Files) == 0 {
		return nil, fmt.Errorf("metalink contains no files")
	}

	out := make([]File, 0, len(doc.Files))
	for _, raw := range doc.Files {
		item := File{Name: strings.TrimSpace(raw.Name)}
		if raw.Size != "" {
			size, err := strconv.ParseInt(strings.TrimSpace(raw.Size), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid metalink size for %q: %w", item.Name, err)
			}
			item.Size = size
		}

		seen := map[string]struct{}{}
		appendURL := func(uri string) {
			uri = strings.TrimSpace(uri)
			if uri == "" {
				return
			}
			if _, ok := seen[uri]; ok {
				return
			}
			seen[uri] = struct{}{}
			item.URLs = append(item.URLs, uri)
		}

		for _, u := range raw.URLs {
			appendURL(u.Value)
		}
		for _, u := range raw.Resources.URLs {
			appendURL(u.Value)
		}

		if hash := firstHash(raw); hash != "" {
			item.Checksum = hash
		}

		if item.Name == "" && len(item.URLs) > 0 {
			item.Name = deriveName(item.URLs[0])
		}
		if len(item.URLs) == 0 {
			return nil, fmt.Errorf("metalink file %q has no download URL", item.Name)
		}
		out = append(out, item)
	}
	return out, nil
}

type xmlFile struct {
	Name      string   `xml:"name,attr"`
	Size      string   `xml:"size"`
	URLs      []xmlURL `xml:"url"`
	Resources struct {
		URLs []xmlURL `xml:"url"`
	} `xml:"resources"`
	Verification struct {
		Hashes []xmlHash `xml:"hash"`
	} `xml:"verification"`
	Hashes []xmlHash `xml:"hash"`
}

type xmlURL struct {
	Value string `xml:",chardata"`
}

type xmlHash struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

func firstHash(raw xmlFile) string {
	candidates := append([]xmlHash{}, raw.Hashes...)
	candidates = append(candidates, raw.Verification.Hashes...)
	for _, hash := range candidates {
		value := strings.TrimSpace(hash.Value)
		if value == "" {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(hash.Type))
		if typ == "" {
			typ = "sha-1"
		}
		return typ + "=" + value
	}
	return ""
}

func deriveName(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "download"
	}
	if idx := strings.LastIndex(rawURL, "/"); idx >= 0 && idx+1 < len(rawURL) {
		name := rawURL[idx+1:]
		if q := strings.Index(name, "?"); q >= 0 {
			name = name[:q]
		}
		if name != "" {
			return name
		}
	}
	return "download"
}

func trimBOM(data []byte) []byte {
	return []byte(strings.TrimPrefix(string(data), "\ufeff"))
}

// ParseReader 从 reader 解析 metalink 文档。
func ParseReader(r io.Reader) ([]File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}
