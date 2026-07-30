package common

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// ChecksumRaw 返回任务选项或 meta 中的 checksum 原始字符串。
func ChecksumRaw(item *task.Task) string {
	return checksumRaw(item)
}

// VerifyTaskChecksum 校验单文件任务 checksum；checked=false 表示跳过。
func VerifyTaskChecksum(item *task.Task) (checked bool, matched bool, actual string, err error) {
	if item == nil {
		return false, false, "", fmt.Errorf("task is nil")
	}

	raw := checksumRaw(item)
	if raw == "" {
		return false, false, "", nil
	}

	spec, err := parseChecksumSpec(raw)
	if err != nil {
		return false, false, "", err
	}

	path, ok := checksumPath(item)
	if !ok {
		return false, false, "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, "", nil
		}
		return false, false, "", err
	}
	if info.IsDir() {
		return false, false, "", nil
	}

	h, err := newChecksumHash(spec.Algorithm)
	if err != nil {
		return false, false, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return false, false, "", err
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return false, false, "", err
	}
	actual = hex.EncodeToString(h.Sum(nil))
	return true, strings.EqualFold(actual, spec.Value), actual, nil
}

type checksumSpec struct {
	Algorithm string
	Value     string
}

func parseChecksumSpec(raw string) (checksumSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return checksumSpec{}, fmt.Errorf("checksum is empty")
	}

	algo, value, ok := strings.Cut(raw, "=")
	if !ok {
		return checksumSpec{}, fmt.Errorf("checksum must be in algo=value format")
	}
	algo = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(algo, "-", "")))
	value = strings.ToLower(strings.TrimSpace(value))
	if algo == "" || value == "" {
		return checksumSpec{}, fmt.Errorf("checksum is incomplete")
	}
	return checksumSpec{Algorithm: algo, Value: value}, nil
}

// ValidateChecksumOption 校验 aria2 checksum 选项格式（algo=digest）。
func ValidateChecksumOption(raw string) error {
	spec, err := parseChecksumSpec(raw)
	if err != nil {
		return err
	}
	_, err = newChecksumHash(spec.Algorithm)
	return err
}

func checksumRaw(item *task.Task) string {
	if raw := strings.TrimSpace(item.Meta["aria2.checksum"]); raw != "" {
		return raw
	}
	return strings.TrimSpace(item.Options["checksum"])
}

func checksumPath(item *task.Task) (string, bool) {
	if len(item.Files) == 1 && strings.TrimSpace(item.Files[0].Path) != "" {
		return item.Files[0].Path, true
	}
	if strings.TrimSpace(item.SaveDir) != "" && strings.TrimSpace(item.Name) != "" {
		return filepath.Join(item.SaveDir, item.Name), true
	}
	return "", false
}

func newChecksumHash(algo string) (hash.Hash, error) {
	switch algo {
	case "sha1":
		return sha1.New(), nil
	case "sha256":
		return sha256.New(), nil
	case "md5":
		return md5.New(), nil
	default:
		return nil, fmt.Errorf("unsupported checksum algorithm: %s", algo)
	}
}
