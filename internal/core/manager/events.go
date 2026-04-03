package manager

import (
	"time"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

// EventType 描述管理器对外发布的任务事件类型�?
type EventType string

const (
	EventTaskAdded    EventType = "task.added"
	EventTaskUpdated  EventType = "task.updated"
	EventTaskRemoved  EventType = "task.removed"
	EventTaskRestored EventType = "task.restored"
)

// Event 是管理器发布给上�?RPC/通知层的事件快照�?
type Event struct {
	Type       EventType  `json:"type"`
	Task       *task.Task `json:"task,omitempty"`
	GlobalStat GlobalStat `json:"globalStat"`
	Time       time.Time  `json:"time"`
}
