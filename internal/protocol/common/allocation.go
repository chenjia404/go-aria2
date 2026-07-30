package common

import (
	"os"
	"strings"
)

// FileAllocationMode 表示 aria2 file-allocation 选项的分配策略。
type FileAllocationMode string

const (
	FileAllocationNone    FileAllocationMode = "none"
	FileAllocationTrunc   FileAllocationMode = "trunc"
	FileAllocationPrealloc FileAllocationMode = "prealloc"
	FileAllocationFalloc  FileAllocationMode = "falloc"
)

// ParseFileAllocation 解析 file-allocation 选项，未知或空值返回 none。
func ParseFileAllocation(opts map[string]string) FileAllocationMode {
	if opts == nil {
		return FileAllocationNone
	}
	value := strings.ToLower(strings.TrimSpace(opts["file-allocation"]))
	switch FileAllocationMode(value) {
	case FileAllocationTrunc, FileAllocationPrealloc, FileAllocationFalloc:
		return FileAllocationMode(value)
	default:
		return FileAllocationNone
	}
}

// PrepareDownloadFile 按 file-allocation 策略创建或打开目标文件。
// existingSize 为本地已存在字节数；total 为已知总长度（<=0 表示未知）。
func PrepareDownloadFile(path string, mode FileAllocationMode, existingSize, total int64, resumePartial bool) (*os.File, int64, error) {
	switch mode {
	case FileAllocationTrunc, FileAllocationPrealloc, FileAllocationFalloc:
		if resumePartial && existingSize > 0 {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
			if err != nil {
				return nil, 0, err
			}
			return file, existingSize, nil
		}
		if total > 0 {
			return openPreallocated(path, mode, total)
		}
		return openGrowing(path)
	case FileAllocationNone:
		fallthrough
	default:
		if resumePartial && existingSize > 0 {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
			if err != nil {
				return nil, 0, err
			}
			return file, existingSize, nil
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, 0, err
		}
		return file, 0, nil
	}
}

func openPreallocated(path string, mode FileAllocationMode, total int64) (*os.File, int64, error) {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, 0, err
	}
	if err := allocateFile(file, mode, total); err != nil {
		file.Close()
		return nil, 0, err
	}
	return file, 0, nil
}

func openGrowing(path string) (*os.File, int64, error) {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, 0, err
	}
	return file, 0, nil
}

func dirOf(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[:i]
	}
	return "."
}
