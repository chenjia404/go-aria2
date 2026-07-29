package aria2session

import (
	"github.com/chenjia404/go-aria2/internal/core/task"
	"github.com/chenjia404/go-aria2/internal/protocol/common"
)

// verifyTaskChecksum 尝试对单文件任务进行 checksum 校验。
func verifyTaskChecksum(item *task.Task) (checked bool, matched bool, actual string, err error) {
	return common.VerifyTaskChecksum(item)
}

func checksumRaw(item *task.Task) string {
	return common.ChecksumRaw(item)
}
