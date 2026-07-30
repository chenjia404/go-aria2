package common

import (
	"context"
	"testing"
	"time"
)

func TestByteLimiter_WaitsWhenExceedingRate(t *testing.T) {
	t.Parallel()

	limiter := NewByteLimiter(1024) // 1 KiB/s
	start := time.Now()
	if err := limiter.Wait(context.Background(), 2048); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Fatalf("expected rate limiting delay, got %v", elapsed)
	}
}

func TestByteLimiter_LargeReadWaitsIncrementally(t *testing.T) {
	t.Parallel()

	limiter := NewByteLimiter(8192)
	start := time.Now()
	if err := limiter.Wait(context.Background(), 32*1024); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 3*time.Second {
		t.Fatalf("expected incremental waits for large read, got %v", elapsed)
	}
}

func TestNewTaskDownloadLimiter(t *testing.T) {
	t.Parallel()

	base := NewByteLimiter(9999)
	if got := NewTaskDownloadLimiter(map[string]string{"max-download-limit": "1024"}, base); got == nil {
		t.Fatal("expected task limiter")
	}
	if got := NewTaskDownloadLimiter(map[string]string{}, base); got != base {
		t.Fatalf("expected base fallback")
	}
	if got := NewTaskDownloadLimiter(map[string]string{"max-download-limit": "0"}, base); got != nil {
		t.Fatalf("zero limit should disable")
	}
	if got := NewTaskDownloadLimiter(map[string]string{"max-overall-download-limit": "2048"}, base); got != base {
		t.Fatalf("max-overall should reuse base limiter, got %p want %p", got, base)
	}
	if got := NewTaskDownloadLimiter(map[string]string{"max-overall-download-limit": "2048"}, nil); got == nil {
		t.Fatal("expected standalone limiter without base")
	}
}
