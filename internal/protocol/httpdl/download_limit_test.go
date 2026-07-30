package httpdl

import "testing"

func TestSetDownloadLimitUpdatesLimiter(t *testing.T) {
	t.Parallel()

	driver := New(Options{})
	driver.SetDownloadLimit(1024)
	if driver.limiter == nil {
		t.Fatal("expected limiter after SetDownloadLimit")
	}
	driver.SetDownloadLimit(0)
	if driver.limiter != nil {
		t.Fatalf("expected nil limiter after clearing limit")
	}
}
