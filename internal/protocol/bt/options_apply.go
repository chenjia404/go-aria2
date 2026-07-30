package bt

import (
	"strconv"
	"strings"

	torrentlib "github.com/anacrolix/torrent"
)

// applyBTMaxPeers 按任务选项或驱动默认值设置单 torrent 最大连接数。
func applyBTMaxPeers(tor *torrentlib.Torrent, opts map[string]string, defaultPeers int) {
	if tor == nil {
		return
	}
	if opts != nil {
		if raw, ok := opts["bt-max-peers"]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
				tor.SetMaxEstablishedConns(n)
				return
			}
		}
	}
	if defaultPeers > 0 {
		tor.SetMaxEstablishedConns(defaultPeers)
	}
}
