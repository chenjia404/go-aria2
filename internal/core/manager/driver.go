package manager

import (
	"context"
	"errors"

	"github.com/chenjia404/go-aria2/internal/core/task"
)

var (
	// ErrTaskNotFound 表示根据任务 ID 或 GID 找不到任务。
	ErrTaskNotFound = errors.New("task not found")
	// ErrDriverNotFound 表示没有驱动可以处理当前输入。
	ErrDriverNotFound = errors.New("no protocol driver can handle input")
)

// Driver 定义统一协议驱动接口�?
type Driver interface {
	Name() string
	CanHandle(input task.AddTaskInput) bool
	Add(ctx context.Context, input task.AddTaskInput) (*task.Task, error)
	Start(ctx context.Context, taskID string) error
	Pause(ctx context.Context, taskID string, force bool) error
	Remove(ctx context.Context, taskID string, force bool) error
	TellStatus(ctx context.Context, taskID string) (*task.Task, error)
	GetFiles(ctx context.Context, taskID string) ([]task.File, error)
	ChangeOption(ctx context.Context, taskID string, opts map[string]string) error
}

// LocalStatePurger 可选实现：Remove 成功后由管理器调用，释放驱动内该 taskID 的状态（如内部 map 条目）。
type LocalStatePurger interface {
	PurgeLocalState(taskID string)
}

// PeerInfo 描述 aria2 getPeers 需要的统一 peer 视图�?
type PeerInfo struct {
	PeerID        string
	IP            string
	Port          int
	Bitfield      string
	AmChoking     bool
	PeerChoking   bool
	DownloadSpeed int64
	UploadSpeed   int64
	Seeder        bool
}

// ServerEntry 描述 aria2 getServers 中的单个 server 记录�?
type ServerEntry struct {
	URI           string
	CurrentURI    string
	DownloadSpeed int64
}

// FileServerInfo 描述 aria2 getServers 的单个文件条目�?
type FileServerInfo struct {
	Index   int
	Servers []ServerEntry
}

// SessionAwareDriver 允许驱动在恢�?session 时重建内部状态�?
type SessionAwareDriver interface {
	Driver
	LoadSessionTasks(ctx context.Context, tasks []*task.Task, globalOptions map[string]string) error
}

// PeerLister 允许驱动暴露连接中的 peer 列表�?
type PeerLister interface {
	Driver
	GetPeers(ctx context.Context, taskID string) ([]PeerInfo, error)
}

// ServerLister 允许驱动暴露当前 file 关联的服务器列表�?
type ServerLister interface {
	Driver
	GetServers(ctx context.Context, taskID string) ([]FileServerInfo, error)
}

// GlobalStat 是面向管理器和兼容层的统一全局统计模型�?
type GlobalStat struct {
	NumActive     int
	NumWaiting    int
	NumStopped    int
	DownloadSpeed int64
	UploadSpeed   int64
}
