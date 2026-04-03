package task

import "time"

// Protocol 是内部统一任务模型使用的协议枚举。
type Protocol string

const (
	ProtocolBT   Protocol = "bt"
	ProtocolED2K Protocol = "ed2k"
	ProtocolHTTP Protocol = "http"
)

// Status 是内部统一任务状态枚举。
type Status string

const (
	StatusWaiting  Status = "waiting"
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusComplete Status = "complete"
	StatusError    Status = "error"
	StatusRemoved  Status = "removed"
)

// File 描述统一任务模型中的单个文件。
type File struct {
	Index           int      `json:"index"`
	Path            string   `json:"path"`
	Length          int64    `json:"length"`
	CompletedLength int64    `json:"completedLength"`
	Selected        bool     `json:"selected"`
	URIs            []string `json:"uris,omitempty"`
}

// Task 是内部核心领域的统一任务模型。
type Task struct {
	ID                     string            `json:"id"`
	GID                    string            `json:"gid"`
	Protocol               Protocol          `json:"protocol"`
	Name                   string            `json:"name"`
	Status                 Status            `json:"status"`
	SaveDir                string            `json:"saveDir"`
	TotalLength            int64             `json:"totalLength"`
	CompletedLength        int64             `json:"completedLength"`
	UploadedLength         int64             `json:"uploadedLength"`
	DownloadSpeed          int64             `json:"downloadSpeed"`
	UploadSpeed            int64             `json:"uploadSpeed"`
	Connections            int               `json:"connections"`
	PieceLength            int64             `json:"pieceLength"`
	VerifiedLength         int64             `json:"verifiedLength"`
	NumSeeders             int               `json:"numSeeders"`
	Seeder                 bool              `json:"seeder"`
	VerifyIntegrityPending bool              `json:"verifyIntegrityPending"`
	InfoHash               string            `json:"infoHash"`
	ErrorCode              string            `json:"errorCode"`
	ErrorMessage           string            `json:"errorMessage"`
	Files                  []File            `json:"files"`
	Options                map[string]string `json:"options"`
	// LocalOptions 为任务级选项（未与全局合并）；与 Options 二选一语义见 manager。
	// 旧 session 无此字段时为 nil，按兼容路径仅使用 Options。
	LocalOptions map[string]string `json:"localOptions,omitempty"`
	Meta         map[string]string `json:"meta"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// AddTaskInput 是协议驱动统一接收的新增任务输入。
type AddTaskInput struct {
	GID     string
	URI     string
	URIs    []string
	Torrent []byte
	SaveDir string
	Name    string
	Options map[string]string
	Meta    map[string]string
}

// Clone 返回任务的深拷贝，避免跨层共享可变状态。
func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}

	cloned := *t
	cloned.Files = CloneFiles(t.Files)
	cloned.Options = cloneMap(t.Options)
	if t.LocalOptions != nil {
		cloned.LocalOptions = cloneMap(t.LocalOptions)
	}
	cloned.Meta = cloneMap(t.Meta)
	return &cloned
}

// CloneFiles 返回文件列表的副本。
func CloneFiles(files []File) []File {
	cloned := make([]File, 0, len(files))
	for _, file := range files {
		fileClone := file
		fileClone.URIs = append([]string(nil), file.URIs...)
		cloned = append(cloned, fileClone)
	}
	return cloned
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
