package bt

import (
	"strings"

	torrentlib "github.com/anacrolix/torrent"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

func mergeStringStringMaps(base, over map[string]string) map[string]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// effectiveOptsForSessionRestore 会话恢复：有 LocalOptions 时用「当前全局 + 任务级」合并；否则用落盘的 Options（旧会话兼容）。
func effectiveOptsForSessionRestore(t *task.Task, global map[string]string) map[string]string {
	if t == nil {
		return nil
	}
	if t.LocalOptions != nil {
		return mergeStringStringMaps(global, t.LocalOptions)
	}
	return t.Options
}

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
