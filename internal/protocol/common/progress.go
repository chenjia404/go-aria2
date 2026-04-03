package common

import (
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// AdvanceActiveTask 根据经过时间推进 active 任务的下载和上传统计�?
func AdvanceActiveTask(item *task.Task, now time.Time) bool {
	if item == nil || item.Status != task.StatusActive {
		return false
	}

	changed := false
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
		return false
	}

	elapsed := now.Sub(item.UpdatedAt)
	if elapsed <= 0 {
		return false
	}

	if item.DownloadSpeed > 0 && item.TotalLength > 0 && item.CompletedLength < item.TotalLength {
		advanced := int64(elapsed.Seconds() * float64(item.DownloadSpeed))
		if advanced <= 0 {
			advanced = item.DownloadSpeed
		}

		item.CompletedLength += advanced
		if item.CompletedLength > item.TotalLength {
			item.CompletedLength = item.TotalLength
		}
		if item.VerifiedLength < item.CompletedLength {
			item.VerifiedLength = item.CompletedLength
		}
		rebalanceFileProgress(item)
		changed = true
	}

	if item.UploadSpeed > 0 {
		uploaded := int64(elapsed.Seconds() * float64(item.UploadSpeed))
		if uploaded > 0 {
			item.UploadedLength += uploaded
			changed = true
		}
	}

	if item.TotalLength > 0 && item.CompletedLength >= item.TotalLength {
		item.Status = task.StatusComplete
		item.DownloadSpeed = 0
		item.UploadSpeed = 0
		item.Connections = 0
		item.VerifyIntegrityPending = false
		changed = true
	}

	item.UpdatedAt = now
	return changed
}

func rebalanceFileProgress(item *task.Task) {
	remaining := item.CompletedLength
	for i := range item.Files {
		file := &item.Files[i]
		file.CompletedLength = 0
		if remaining <= 0 {
			continue
		}
		if remaining >= file.Length {
			file.CompletedLength = file.Length
			remaining -= file.Length
			continue
		}
		file.CompletedLength = remaining
		remaining = 0
	}
}
