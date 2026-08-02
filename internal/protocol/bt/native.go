package bt

import (
	"context"

	"github.com/chenjia404/go-aria2/internal/core/manager"
)

// GetBTTrackers 返回 BT 任务当前 tracker 列表（native.getBtTrackers）。
func (d *Driver) GetBTTrackers(ctx context.Context, taskID string) (map[string]any, error) {
	_ = ctx
	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return nil, manager.ErrTaskNotFound
	}

	mi := st.torrent.Metainfo()
	announceList := (&mi).UpvertedAnnounceList()
	trackers := flattenTrackers(announceList)
	if len(trackers) == 0 && len(st.source.Trackers) > 0 {
		trackers = append([]string(nil), st.source.Trackers...)
	}

	return map[string]any{
		"infoHash":     st.torrent.InfoHash().HexString(),
		"trackers":     trackers,
		"announceList": announceList,
	}, nil
}

// ReannounceTorrent 向 tracker 与 DHT 重新宣告（native.reannounceTorrent）。
func (d *Driver) ReannounceTorrent(ctx context.Context, taskID string) error {
	_ = ctx
	d.mu.RLock()
	st := d.tasks[taskID]
	client := d.client
	d.mu.RUnlock()
	if st == nil || st.removed {
		return manager.ErrTaskNotFound
	}

	mi := st.torrent.Metainfo()
	announceList := (&mi).UpvertedAnnounceList()
	if len(announceList) > 0 {
		st.torrent.ModifyTrackers(announceList)
	}
	if client != nil {
		announceTorrentToDHT(client, st.torrent)
	}
	return nil
}

// ForcePieceCheck 在后台触发 piece 校验（native.forcePieceCheck）。
func (d *Driver) ForcePieceCheck(ctx context.Context, taskID string) error {
	d.mu.RLock()
	st := d.tasks[taskID]
	d.mu.RUnlock()
	if st == nil || st.removed {
		return manager.ErrTaskNotFound
	}
	if st.torrent.Info() == nil {
		return manager.ErrTaskNotFound
	}

	d.startIntegrityCheck(taskID, st)
	return nil
}
