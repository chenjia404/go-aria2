package bt

import (
	torrentlib "github.com/anacrolix/torrent"
)

// announceTorrentToDHT 向所有 DHT 服务器发起一次 announce，用于低速时尝试发现更多 peer。
func announceTorrentToDHT(client *torrentlib.Client, tor *torrentlib.Torrent) {
	if client == nil || tor == nil {
		return
	}
	for _, ds := range client.DhtServers() {
		done, stop, err := tor.AnnounceToDht(ds)
		if err != nil {
			continue
		}
		go func() {
			<-done
			if stop != nil {
				stop()
			}
		}()
	}
}
