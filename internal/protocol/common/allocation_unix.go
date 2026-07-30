//go:build unix

package common

import (
	"os"
	"syscall"
)

func allocateFile(file *os.File, mode FileAllocationMode, total int64) error {
	if total <= 0 {
		return nil
	}
	if mode == FileAllocationFalloc {
		if err := syscall.Fallocate(int(file.Fd()), 0, 0, total); err == nil {
			return nil
		}
	}
	return file.Truncate(total)
}
