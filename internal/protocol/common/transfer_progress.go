package common

import (
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// ApplyTransferProgress 更新任务进度与下载速度（供 FTP/SFTP 等单连接传输驱动使用）。
func ApplyTransferProgress(item *task.Task, completed, total int64, lastBytes *int64, lastTick *time.Time) {
	now := time.Now()
	if lastTick != nil && lastBytes != nil && !lastTick.IsZero() {
		elapsed := now.Sub(*lastTick).Seconds()
		if elapsed > 0 {
			item.DownloadSpeed = int64(float64(completed-*lastBytes) / elapsed)
		}
	}
	item.CompletedLength = completed
	if total > 0 {
		item.TotalLength = total
		if len(item.Files) > 0 {
			item.Files[0].Length = total
			item.Files[0].CompletedLength = completed
		}
	}
	item.Status = task.StatusActive
	item.Connections = 1
	item.UpdatedAt = now
	if lastTick != nil {
		*lastTick = now
	}
	if lastBytes != nil {
		*lastBytes = completed
	}
}
