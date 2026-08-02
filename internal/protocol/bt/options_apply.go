package bt

import (
	torrentlib "github.com/anacrolix/torrent"
)

// applyBTMaxPeers 按任务选项或驱动默认值设置单 torrent 最大连接数，并返回生效值。
func applyBTMaxPeers(tor *torrentlib.Torrent, opts map[string]string, defaultPeers int) int {
	max := resolveBTMaxPeers(opts, defaultPeers)
	if tor != nil && max > 0 {
		tor.SetMaxEstablishedConns(max)
	}
	return max
}
