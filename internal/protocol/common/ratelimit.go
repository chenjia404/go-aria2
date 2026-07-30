package common

import (
	"context"
	"sync"
	"time"
)

// ByteLimiter 按字节速率限制下载（令牌桶，aria2 max-download-limit 语义）。
type ByteLimiter struct {
	mu       sync.Mutex
	rate     int64
	tokens   float64
	lastFill time.Time
}

// NewByteLimiter 创建字节/秒限速器；rate<=0 返回 nil（不限速）。
func NewByteLimiter(rate int64) *ByteLimiter {
	if rate <= 0 {
		return nil
	}
	return &ByteLimiter{
		rate:     rate,
		tokens:   float64(rate),
		lastFill: time.Now(),
	}
}

// SetRate 运行期调整限速（字节/秒）。
func (l *ByteLimiter) SetRate(rate int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rate = rate
	if rate > 0 && l.tokens > float64(rate) {
		l.tokens = float64(rate)
	}
}

// Wait 阻塞直到允许消费 n 字节。
func (l *ByteLimiter) Wait(ctx context.Context, n int64) error {
	if l == nil || n <= 0 {
		return nil
	}

	remaining := float64(n)
	for remaining > 0 {
		var wait time.Duration
		l.mu.Lock()
		now := time.Now()
		if elapsed := now.Sub(l.lastFill).Seconds(); elapsed > 0 {
			l.tokens += elapsed * float64(l.rate)
			if l.tokens > float64(l.rate) {
				l.tokens = float64(l.rate)
			}
			l.lastFill = now
		}
		if l.tokens >= remaining {
			l.tokens -= remaining
			l.mu.Unlock()
			return nil
		}
		if l.tokens > 0 {
			remaining -= l.tokens
			l.tokens = 0
		}
		chunk := remaining
		if chunk > float64(l.rate) {
			chunk = float64(l.rate)
		}
		wait = time.Duration(chunk/float64(l.rate)*float64(time.Second))
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		l.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}
