package bt

import (
	"time"

	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

func applyBTRateLimiters(st *state, opts map[string]string) {
	if st == nil {
		return
	}
	st.downloadLimiter = common.NewTaskDownloadLimiter(opts, nil)
	st.uploadLimiter = common.NewTaskUploadLimiter(opts, nil)
	if st.downloadLimiter == nil {
		st.rateLimitPausedDL = false
	}
	if st.uploadLimiter == nil {
		st.rateLimitPausedUL = false
	}
}

func (d *Driver) runRateLimitLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.enforceTaskRateLimits()
		}
	}
}

func (d *Driver) enforceTaskRateLimits() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client == nil {
		return
	}

	now := time.Now()
	for taskID, st := range d.tasks {
		if st == nil || st.removed || !st.started || st.paused || st.torrent == nil {
			continue
		}

		stats := st.torrent.Stats()
		readBytes := stats.ConnStats.BytesReadUsefulData.Int64()
		writeBytes := stats.ConnStats.BytesWrittenData.Int64()

		if st.lastRateSampleAt.IsZero() {
			st.lastRateSampleAt = now
			st.lastRateBytesRead = readBytes
			st.lastRateBytesWrite = writeBytes
			continue
		}

		deltaRead := readBytes - st.lastRateBytesRead
		deltaWrite := writeBytes - st.lastRateBytesWrite
		if deltaRead < 0 {
			deltaRead = 0
		}
		if deltaWrite < 0 {
			deltaWrite = 0
		}
		var downSpeed, upSpeed int64
		if elapsed := now.Sub(st.lastRateSampleAt).Seconds(); elapsed > 0 && !st.lastRateSampleAt.IsZero() {
			downSpeed = int64(float64(deltaRead) / elapsed)
			upSpeed = int64(float64(deltaWrite) / elapsed)
		}
		st.lastRateSampleAt = now
		st.lastRateBytesRead = readBytes
		st.lastRateBytesWrite = writeBytes

		d.enforceDownloadRateLimit(st, deltaRead)
		d.enforceUploadRateLimit(st, deltaWrite)
		d.enforcePeerBoost(st, downSpeed, upSpeed)
		d.handleBTCompletionLocked(st, taskID)
	}
}

func (d *Driver) enforceDownloadRateLimit(st *state, deltaRead int64) {
	if st == nil || st.torrent == nil {
		return
	}
	if st.downloadLimiter == nil {
		if st.rateLimitPausedDL {
			st.rateLimitPausedDL = false
			st.torrent.AllowDataDownload()
		}
		return
	}
	if deltaRead > 0 {
		if !st.downloadLimiter.TryConsume(deltaRead) {
			if !st.rateLimitPausedDL {
				st.rateLimitPausedDL = true
				st.torrent.DisallowDataDownload()
			}
			return
		}
	}
	if st.rateLimitPausedDL && st.downloadLimiter.CanAfford(1) {
		st.rateLimitPausedDL = false
		st.torrent.AllowDataDownload()
	}
}

func (d *Driver) enforceUploadRateLimit(st *state, deltaWrite int64) {
	if st == nil || st.torrent == nil {
		return
	}
	if st.uploadLimiter == nil {
		if st.rateLimitPausedUL {
			st.rateLimitPausedUL = false
			st.torrent.AllowDataUpload()
		}
		return
	}
	if deltaWrite > 0 {
		if !st.uploadLimiter.TryConsume(deltaWrite) {
			if !st.rateLimitPausedUL {
				st.rateLimitPausedUL = true
				st.torrent.DisallowDataUpload()
			}
			return
		}
	}
	if st.rateLimitPausedUL && st.uploadLimiter.CanAfford(1) {
		st.rateLimitPausedUL = false
		st.torrent.AllowDataUpload()
	}
}
