package session

import (
	"context"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// Store 抽象 session 持久化能力，便于后续替换为数据库或更复杂格式�?
type Store interface {
	Load(ctx context.Context) ([]*task.Task, error)
	Save(ctx context.Context, tasks []*task.Task) error
}
