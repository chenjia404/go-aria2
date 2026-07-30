package manager

import (
	"context"
	"testing"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

type limitRecordingDriver struct {
	stubDriver
	downloadLimit int64
	uploadLimit   int64
}

func (d *limitRecordingDriver) SetDownloadLimit(bytesPerSec int64) {
	d.downloadLimit = bytesPerSec
}

func (d *limitRecordingDriver) SetUploadLimit(bytesPerSec int64) {
	d.uploadLimit = bytesPerSec
}

func TestChangeGlobalOption_PropagatesSpeedLimitsToDrivers(t *testing.T) {
	t.Parallel()

	driver := &limitRecordingDriver{stubDriver: *newStubDriver()}
	mgr := New(Options{MaxConcurrent: 1})
	mgr.RegisterDriver(driver)

	updated := mgr.ChangeGlobalOption(map[string]string{
		"max-overall-download-limit": "4096",
		"max-overall-upload-limit":   "2048",
	})
	if updated["max-overall-download-limit"] != "4096" {
		t.Fatalf("download limit: %#v", updated["max-overall-download-limit"])
	}
	if updated["max-overall-upload-limit"] != "2048" {
		t.Fatalf("upload limit: %#v", updated["max-overall-upload-limit"])
	}
	if driver.downloadLimit != 4096 {
		t.Fatalf("driver download limit: got %d want 4096", driver.downloadLimit)
	}
	if driver.uploadLimit != 2048 {
		t.Fatalf("driver upload limit: got %d want 2048", driver.uploadLimit)
	}

	mgr.ChangeGlobalOption(map[string]string{
		"max-overall-download-limit": "0",
		"max-overall-upload-limit":   "0",
	})
	if driver.downloadLimit != 0 || driver.uploadLimit != 0 {
		t.Fatalf("clearing limits: download=%d upload=%d", driver.downloadLimit, driver.uploadLimit)
	}

	_, err := mgr.Add(context.Background(), task.AddTaskInput{
		URI: "http://example.com/limit-probe.bin",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
}
