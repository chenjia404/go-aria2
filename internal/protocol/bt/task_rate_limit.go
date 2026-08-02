package bt

import (
	"strconv"
	"strings"
	"time"

	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

func applyBTRateLimiters(st *state, opts map[string]string) {
	if st == nil {
		return
	}
	st.downloadLimiter = common.NewTaskDownloadLimiter(opts, nil)
	st.uploadLimiter = common.NewTaskUploadLimiter(opts, nil)
}

func parseRateLimitBytes(opts map[string]string, key string) (int64, bool) {
	if opts == nil {
		return 0, false
	}
	raw, ok := opts[key]
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func (d *Driver) runRateLimitLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		d.enforceTaskRateLimits()
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
		st.lastRateSampleAt = now
		st.lastRateBytesRead = readBytes
		st.lastRateBytesWrite = writeBytes

		if st.downloadLimiter != nil && deltaRead > 0 {
			if !st.downloadLimiter.TryConsume(deltaRead) {
				if !st.rateLimitPausedDL {
					st.rateLimitPausedDL = true
					st.torrent.DisallowDataDownload()
				}
			} else if st.rateLimitPausedDL {
				st.rateLimitPausedDL = false
				st.torrent.AllowDataDownload()
			}
		}

		if st.uploadLimiter != nil && deltaWrite > 0 {
			if !st.uploadLimiter.TryConsume(deltaWrite) {
				if !st.rateLimitPausedUL {
					st.rateLimitPausedUL = true
					st.torrent.DisallowDataUpload()
				}
			} else if st.rateLimitPausedUL {
				st.rateLimitPausedUL = false
				st.torrent.AllowDataUpload()
			}
		}

		d.handleBTCompletionLocked(st, taskID)
	}
}
