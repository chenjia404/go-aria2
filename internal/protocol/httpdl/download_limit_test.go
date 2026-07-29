package httpdl

import "testing"

func TestSetDownloadLimitUpdatesLimiter(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	driver.SetDownloadLimit(1024)
	if driver.limiter == nil || driver.limiter.rate != 1024 {
		t.Fatalf("expected limiter rate 1024, got %#v", driver.limiter)
	}
	driver.SetDownloadLimit(0)
	if driver.limiter != nil {
		t.Fatalf("expected nil limiter after clearing limit")
	}
}
