package bt

import (
	"strings"

	torrentlib "github.com/anacrolix/torrent"
)

// applyBTTrackerOpts 按 aria2 语义合并 bt-tracker / bt-exclude-tracker：
// - bt-exclude-tracker：逗号分隔的前缀列表，announce URL 以任一前缀开头则丢弃；
// - bt-tracker：逗号分隔的额外 tracker，在过滤后以独立 tier 追加（去重）。
func applyBTTrackerOpts(spec *torrentlib.TorrentSpec, source *addSource, opts map[string]string) {
	if spec == nil {
		return
	}
	excludes := splitCommaList(opts["bt-exclude-tracker"])
	extras := splitCommaList(opts["bt-tracker"])

	if len(excludes) > 0 {
		spec.Trackers = filterTrackersByExclude(spec.Trackers, excludes)
	}

	seen := map[string]struct{}{}
	for _, tier := range spec.Trackers {
		for _, u := range tier {
			if u != "" {
				seen[u] = struct{}{}
			}
		}
	}
	for _, u := range extras {
		if u == "" || trackerExcludedByPrefixes(u, excludes) {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		spec.Trackers = append(spec.Trackers, []string{u})
	}

	if source != nil {
		source.Trackers = flattenTrackers(spec.Trackers)
	}
}

func splitCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func trackerExcludedByPrefixes(announceURL string, excludePrefixes []string) bool {
	for _, prefix := range excludePrefixes {
		if prefix != "" && strings.HasPrefix(announceURL, prefix) {
			return true
		}
	}
	return false
}

func filterTrackersByExclude(trackers [][]string, excludePrefixes []string) [][]string {
	if len(excludePrefixes) == 0 {
		return trackers
	}
	if len(trackers) == 0 {
		return trackers
	}
	out := make([][]string, 0, len(trackers))
	for _, tier := range trackers {
		filtered := make([]string, 0, len(tier))
		for _, u := range tier {
			if u == "" || trackerExcludedByPrefixes(u, excludePrefixes) {
				continue
			}
			filtered = append(filtered, u)
		}
		if len(filtered) > 0 {
			out = append(out, filtered)
		}
	}
	return out
}
