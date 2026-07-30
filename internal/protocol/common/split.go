package common

import (
	"strconv"
	"strings"
)

// ParseByteSize 解析 aria2 风格字节大小（支持 K/M/G 后缀，基数 1024）。
func ParseByteSize(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, strconv.ErrSyntax
	}
	multiplier := int64(1)
	if len(text) >= 2 {
		suffix := strings.ToUpper(text[len(text)-1:])
		switch suffix {
		case "K":
			multiplier = 1024
			text = text[:len(text)-1]
		case "M":
			multiplier = 1024 * 1024
			text = text[:len(text)-1]
		case "G":
			multiplier = 1024 * 1024 * 1024
			text = text[:len(text)-1]
		}
	}
	base, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || base < 0 {
		return 0, err
	}
	return base * multiplier, nil
}

// EffectiveSegmentCount 按 aria2 min-split-size 语义限制并发分段数。
// minSplitSize <= 0 表示未设置该选项，不施加限制。
func EffectiveSegmentCount(total, start int64, split, maxConn int, minSplitSize int64) int {
	if split <= 0 {
		split = 1
	}
	if maxConn > 0 && split > maxConn {
		split = maxConn
	}
	if total <= start {
		return 1
	}
	if minSplitSize > 0 {
		remaining := total - start
		maxSeg := int(remaining / minSplitSize)
		if maxSeg < 1 {
			maxSeg = 1
		}
		if split > maxSeg {
			split = maxSeg
		}
	}
	if split <= 0 {
		split = 1
	}
	return split
}

// BuildDownloadRanges 生成分段下载区间；pieceLength > 0 时按 piece-length 对齐边界。
func BuildDownloadRanges(start, total int64, segmentCount int, pieceLength int64) [][2]int64 {
	if total <= 0 || start >= total {
		return nil
	}
	if pieceLength > 0 {
		return buildPieceAlignedRanges(start, total, pieceLength)
	}
	return splitEvenRanges(start, total, segmentCount)
}

func buildPieceAlignedRanges(start, total, pieceLength int64) [][2]int64 {
	if pieceLength <= 0 || start >= total {
		return nil
	}
	ranges := make([][2]int64, 0, (total-start)/pieceLength+1)
	pos := start
	for pos < total {
		end := pos + pieceLength - 1
		if end >= total {
			end = total - 1
		}
		ranges = append(ranges, [2]int64{pos, end})
		pos = end + 1
	}
	if len(ranges) == 0 {
		return [][2]int64{{start, total - 1}}
	}
	return ranges
}

func splitEvenRanges(start, total int64, segments int) [][2]int64 {
	if segments <= 1 || total <= 0 || start >= total {
		if total > start {
			return [][2]int64{{start, total - 1}}
		}
		return nil
	}

	remaining := total - start
	if remaining <= 0 {
		return nil
	}
	if int64(segments) > remaining {
		segments = int(remaining)
	}
	if segments <= 1 {
		return [][2]int64{{start, total - 1}}
	}

	chunkSize := remaining / int64(segments)
	if chunkSize <= 0 {
		chunkSize = 1
	}

	ranges := make([][2]int64, 0, segments)
	current := start
	for i := 0; i < segments; i++ {
		end := current + chunkSize - 1
		if i == segments-1 || end >= total-1 {
			end = total - 1
		}
		ranges = append(ranges, [2]int64{current, end})
		current = end + 1
		if current >= total {
			break
		}
	}
	if len(ranges) > 0 {
		ranges[len(ranges)-1][1] = total - 1
	}
	return ranges
}
