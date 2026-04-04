package bt

import (
	"fmt"
	"strconv"
	"strings"

	torrentlib "github.com/anacrolix/torrent"
)

// parseAria2SelectFile 解析 aria2 的 --select-file：逗号分隔，1-based 索引，闭区间范围 "a-b"。
// 空串或仅空白表示「下载全部文件」，返回 all=true。
func parseAria2SelectFile(s string) (all bool, set map[int]struct{}, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return true, nil, nil
	}
	set = make(map[int]struct{})
	for _, raw := range strings.Split(s, ",") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			loStr, hiStr, ok := strings.Cut(part, "-")
			if !ok {
				return false, nil, fmt.Errorf("select-file: 无效片段 %q", part)
			}
			loStr, hiStr = strings.TrimSpace(loStr), strings.TrimSpace(hiStr)
			if loStr == "" || hiStr == "" {
				return false, nil, fmt.Errorf("select-file: 无效范围 %q", part)
			}
			lo, err1 := strconv.Atoi(loStr)
			hi, err2 := strconv.Atoi(hiStr)
			if err1 != nil || err2 != nil {
				return false, nil, fmt.Errorf("select-file: 无效范围 %q", part)
			}
			if lo < 1 || hi < 1 {
				return false, nil, fmt.Errorf("select-file: 索引须 ≥1（%q）", part)
			}
			if lo > hi {
				return false, nil, fmt.Errorf("select-file: 范围上界小于下界 %q", part)
			}
			for i := lo; i <= hi; i++ {
				set[i] = struct{}{}
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return false, nil, fmt.Errorf("select-file: 无效索引 %q", part)
		}
		if n < 1 {
			return false, nil, fmt.Errorf("select-file: 索引须 ≥1（%q）", part)
		}
		set[n] = struct{}{}
	}
	return false, set, nil
}

func applySelectFileToTorrent(tor *torrentlib.Torrent, selectFile string) error {
	if tor.Info() == nil {
		return fmt.Errorf("bt: torrent 元数据尚未就绪")
	}
	files := tor.Files()
	n := len(files)
	all, set, err := parseAria2SelectFile(selectFile)
	if err != nil {
		return err
	}
	if !all {
		for idx := range set {
			if idx < 1 || idx > n {
				return fmt.Errorf("select-file: 索引 %d 超出范围 [1,%d]", idx, n)
			}
		}
	}
	if all {
		tor.DownloadAll()
		return nil
	}
	for i, f := range files {
		idx := i + 1
		if _, ok := set[idx]; ok {
			f.Download()
		} else {
			f.Cancel()
		}
	}
	return nil
}
