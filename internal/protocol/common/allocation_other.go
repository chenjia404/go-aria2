//go:build !unix

package common

import "os"

func allocateFile(file *os.File, mode FileAllocationMode, total int64) error {
	if total <= 0 {
		return nil
	}
	return file.Truncate(total)
}
