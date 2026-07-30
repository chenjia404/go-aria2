package aria2

import "github.com/chenjia404/go-aria2/internal/protocol/common"

// ParseIndexOut 解析 aria2 index-out 选项（见 protocol/common）。
func ParseIndexOut(raw string) (map[int]string, error) {
	return common.ParseIndexOut(raw)
}
