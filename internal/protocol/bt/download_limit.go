package bt

import "golang.org/x/time/rate"

// SetDownloadLimit 运行时调整全局下载限速（字节/秒，0 表示不限速）。
func (d *Driver) SetDownloadLimit(bytesPerSec int64) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if bytesPerSec <= 0 {
		if d.downloadLimiter != nil {
			d.downloadLimiter.SetLimit(rate.Inf)
			d.downloadLimiter.SetBurst(1 << 20)
		}
		return
	}
	burst := max(16*1024, int(bytesPerSec))
	if d.downloadLimiter == nil {
		d.downloadLimiter = rate.NewLimiter(rate.Limit(bytesPerSec), burst)
		return
	}
	d.downloadLimiter.SetLimit(rate.Limit(bytesPerSec))
	d.downloadLimiter.SetBurst(burst)
}
