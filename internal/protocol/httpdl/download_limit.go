package httpdl

// SetDownloadLimit 运行时调整全局下载限速（字节/秒，0 表示不限速）。
func (d *Driver) SetDownloadLimit(bytesPerSec int64) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if bytesPerSec <= 0 {
		d.limiter = nil
		return
	}
	if d.limiter == nil {
		d.limiter = newByteLimiter(bytesPerSec)
		return
	}
	d.limiter.setRate(bytesPerSec)
}
