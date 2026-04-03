
# 一、项目定位

## 1.1 项目目标

开发一个使用 **Go** 编写的后台下载内核程序，具备以下能力：

* 原生支持 **BitTorrent**
* 原生支持 **ED2K / eMule**
* 对外提供 **aria2 常用 JSON-RPC 兼容接口**
* 支持读取 **aria2 风格配置文件**
* 支持守护进程运行
* 支持任务持久化、断点续传、重启恢复
* 后续可扩展 HTTP/FTP/磁力元数据代理等协议

---

## 1.2 项目原则

### 核心原则

1. **内部模型独立**

   * 内部不能直接照着 aria2 的数据模型写
   * aria2 只是兼容层，不是核心领域模型

2. **协议驱动化**

   * BT 和 ED2K 都是协议驱动
   * 统一任务调度层管理不同协议任务

3. **兼容优先级明确**

   * 优先兼容 aria2 常用 RPC
   * 不追求一开始 100% 全兼容 aria2 所有边角能力

4. **可渐进扩展**

   * 第一阶段先做 BT + ED2K 基础下载
   * 第二阶段补 WebSocket 通知、更多配置项、更多状态字段
   * 第三阶段补 Kad、AICH、更多原生扩展 RPC

---

## 1.3 非目标

第一阶段不做这些：

* 不要求完整兼容 aria2 所有协议
* 不要求支持 Metalink
* 不要求支持 XML-RPC
* 不要求支持 aria2 所有配置项
* 不要求完整实现 eMule 的所有积分与上传队列细节
* 不要求第一版就支持复杂 GUI

---

# 二、总体架构

---

## 2.1 总体结构

整体分为六层：

### 1. Core Core

统一任务调度和生命周期管理

### 2. Protocol Drivers

* BT Driver
* ED2K Driver

### 3. Storage / Persistence

* 文件落盘
* part 文件
* session 保存
* 配置持久化

### 4. Compatibility Layer

* aria2 JSON-RPC 兼容层
* aria2 配置文件兼容层

### 5. Native Extension Layer

* 自定义扩展 RPC
* 暴露 aria2 无法表达的 BT / ED2K 能力

### 6. Runtime Layer

* 守护进程
* 日志
* 配置加载
* 优雅退出
* 监听端口

---

## 2.2 架构图

```text
+---------------------------------------------------+
|                   Client / UI                     |
| aria2 GUI / Web Panel / Custom UI / CLI / Script |
+-------------------------+-------------------------+
                          |
         +----------------+----------------+
         |                                 |
+--------v---------+             +---------v---------+
| aria2 Compat RPC |             | Native Extension  |
| JSON-RPC         |             | JSON-RPC          |
+--------+---------+             +---------+---------+
         |                                 |
         +----------------+----------------+
                          |
                 +--------v--------+
                 | Task Manager    |
                 | Queue/Scheduler |
                 +--------+--------+
                          |
          +---------------+----------------+
          |                                |
+---------v---------+             +--------v---------+
| BT Driver         |             | ED2K Driver      |
| torrent/magnet    |             | ed2k/server/kad  |
+---------+---------+             +--------+---------+
          |                                |
          +---------------+----------------+
                          |
                 +--------v--------+
                 | Storage Layer   |
                 | Files/Parts     |
                 | Session         |
                 +--------+--------+
                          |
                 +--------v--------+
                 | OS / Network    |
                 +-----------------+
```

---

# 三、模块划分

---

## 3.1 推荐目录结构

```text
/cmd/downloadd
/cmd/downloadctl

/internal/app
/internal/config
/internal/logging

/internal/core/task
/internal/core/manager
/internal/core/queue
/internal/core/scheduler
/internal/core/session
/internal/core/storage
/internal/core/types

/internal/protocol/common
/internal/protocol/bt
/internal/protocol/ed2k

/internal/compat/aria2
/internal/rpc/jsonrpc
/internal/rpc/native
/internal/rpc/ws

/internal/persist
/internal/util
```

---

## 3.2 模块职责

### `/cmd/downloadd`

守护进程入口

负责：

* 读取配置
* 初始化日志
* 初始化任务管理器
* 初始化协议驱动
* 启动 RPC 服务
* 监听退出信号
* 优雅关闭

### `/cmd/downloadctl`

CLI 工具

负责：

* 调用 aria2 兼容 RPC
* 测试添加任务
* 查询状态
* 调试用

### `/internal/core/manager`

任务管理核心

负责：

* 创建任务
* 恢复任务
* 启动/暂停/删除任务
* 维护 active/waiting/stopped 队列
* 管理并发下载数
* 汇总全局统计

### `/internal/protocol/bt`

BitTorrent 驱动

负责：

* magnet 解析
* torrent 元数据加载
* tracker
* DHT
* peer 连接
* piece 管理
* 文件映射
* seeding

### `/internal/protocol/ed2k`

ED2K 驱动

负责：

* ed2k link 解析
* server 协议
* source 管理
* 分块下载
* part 文件
* hash 校验
* Kad 预留

### `/internal/compat/aria2`

aria2 兼容层

负责：

* 方法名映射
* `gid` 管理
* 任务状态映射
* 返回字段转换
* 兼容 aria2 的 token 认证方式
* 字符串数字序列化

### `/internal/config`

配置解析

负责：

* 解析 aria2.conf 风格配置
* 解析自定义 `ed2k-*` 配置
* 生成内部配置结构
* 对不支持配置项输出 warning

---

# 四、统一领域模型设计

这是最关键的一部分。

---

## 4.1 统一任务模型

```go
type TaskProtocol string

const (
    TaskProtocolBT   TaskProtocol = "bt"
    TaskProtocolED2K TaskProtocol = "ed2k"
)

type TaskStatus string

const (
    TaskStatusWaiting  TaskStatus = "waiting"
    TaskStatusActive   TaskStatus = "active"
    TaskStatusPaused   TaskStatus = "paused"
    TaskStatusComplete TaskStatus = "complete"
    TaskStatusError    TaskStatus = "error"
    TaskStatusRemoved  TaskStatus = "removed"
)

type Task struct {
    ID               string
    GID              string
    Protocol         TaskProtocol
    Name             string
    Status           TaskStatus
    SaveDir          string
    TotalLength      int64
    CompletedLength  int64
    UploadedLength   int64
    DownloadSpeed    int64
    UploadSpeed      int64
    Connections      int
    ErrorCode        string
    ErrorMessage     string
    CreatedAt        time.Time
    UpdatedAt        time.Time
    CompletedAt      *time.Time
    MetadataReady    bool
    Files            []TaskFile
    Options          map[string]string
    Meta             map[string]any
}
```

---

## 4.2 文件模型

```go
type TaskFile struct {
    Index           int
    Path            string
    Length          int64
    CompletedLength int64
    Selected        bool
    Hash            string
}
```

---

## 4.3 BT 专属 Meta

建议放 `Task.Meta`：

```go
{
  "infoHash": "...",
  "trackers": ["udp://...", "http://..."],
  "numSeeders": 12,
  "seeder": false,
  "pieceLength": 4194304,
  "numPieces": 256,
  "bitfield": "..."
}
```

---

## 4.4 ED2K 专属 Meta

```go
{
  "ed2kHash": "...",
  "aichHash": "...",
  "partCount": 47,
  "sources": 126,
  "activeSources": 18,
  "serverConnected": true,
  "serverName": "ExampleServer",
  "kadConnected": false,
  "lowID": false
}
```

---

# 五、协议驱动抽象

---

## 5.1 Driver 接口

```go
type Driver interface {
    Name() string
    CanHandle(input AddTaskInput) bool
    Add(ctx context.Context, input AddTaskInput) (*Task, error)
    Start(ctx context.Context, taskID string) error
    Pause(ctx context.Context, taskID string, force bool) error
    Remove(ctx context.Context, taskID string, force bool) error
    TellStatus(ctx context.Context, taskID string) (*Task, error)
    GetFiles(ctx context.Context, taskID string) ([]TaskFile, error)
    ChangeOption(ctx context.Context, taskID string, opts map[string]string) error
}
```

---

## 5.2 AddTaskInput

```go
type AddTaskInput struct {
    URIs      []string
    Torrent   []byte
    Options   map[string]string
    Source    string
}
```

说明：

* `URIs` 用于 `aria2.addUri`
* `Torrent` 用于 `aria2.addTorrent`
* `Options` 用于每任务参数
* `Source` 记录来源，如 `rpc`, `restore`, `cli`

---

# 六、aria2 兼容层设计

---

## 6.1 兼容目标

第一阶段兼容以下方法：

* `aria2.addUri`
* `aria2.addTorrent`
* `aria2.remove`
* `aria2.forceRemove`
* `aria2.pause`
* `aria2.forcePause`
* `aria2.unpause`
* `aria2.tellStatus`
* `aria2.tellActive`
* `aria2.tellWaiting`
* `aria2.tellStopped`
* `aria2.getFiles`
* `aria2.getPeers`
* `aria2.changeOption`
* `aria2.getOption`
* `aria2.getGlobalStat`
* `aria2.getGlobalOption`
* `aria2.changeGlobalOption`
* `system.listMethods`
* `system.multicall`
* `system.listNotifications`

---

## 6.2 token 验证规则

兼容 aria2 风格：

RPC 参数第一个值可能是：

```text
token:your-secret
```

例如：

```json
{
  "jsonrpc": "2.0",
  "id": "qwer",
  "method": "aria2.tellActive",
  "params": ["token:abc123"]
}
```

要求：

* 若配置了 `rpc-secret`，则必须校验
* 支持无 token 模式
* 校验失败返回标准 JSON-RPC 错误

---

## 6.3 `gid` 规则

要求：

* 对外始终暴露 aria2 风格 `gid`
* 建议 16 位十六进制字符串
* `gid` 与内部 `Task.ID` 分离
* session 恢复后 `gid` 必须稳定不变

例如：

```go
type GIDManager interface {
    NewGID() string
    Bind(taskID string, gid string) error
    GetTaskID(gid string) (string, bool)
}
```

---

## 6.4 字段映射规则

### 内部 Task -> aria2 返回结构

| 内部字段            | aria2 字段        |
| --------------- | --------------- |
| GID             | gid             |
| Status          | status          |
| TotalLength     | totalLength     |
| CompletedLength | completedLength |
| UploadedLength  | uploadLength    |
| DownloadSpeed   | downloadSpeed   |
| UploadSpeed     | uploadSpeed     |
| Connections     | connections     |
| SaveDir         | dir             |
| ErrorCode       | errorCode       |
| ErrorMessage    | errorMessage    |
| Files           | files           |

注意：

**aria2 常把数值序列化成字符串。**

所以这些字段都按字符串输出：

* totalLength
* completedLength
* uploadLength
* downloadSpeed
* uploadSpeed
* connections

---

## 6.5 `aria2.addUri` 兼容策略

支持：

* `magnet:?xt=urn:btih:...`
* `http://.../file.torrent`
* `https://.../file.torrent`
* `ed2k://|file|...|/`

处理逻辑：

1. 校验 token
2. 读取 URI 列表
3. 自动识别协议
4. 交给对应 Driver
5. 返回 `gid`

---

## 6.6 `aria2.addTorrent`

支持：

* 传入 base64 编码的 torrent 内容
* 可选传 trackers
* 可选任务级 options

若内部驱动成功创建任务，返回 `gid`

---

## 6.7 `aria2.tellStatus`

至少返回这些字段：

```json
{
  "gid": "2089b05ecca3d829",
  "status": "active",
  "totalLength": "104857600",
  "completedLength": "52428800",
  "uploadLength": "123456",
  "downloadSpeed": "1048576",
  "uploadSpeed": "65536",
  "connections": "42",
  "dir": "/downloads",
  "files": [],
  "errorCode": "0",
  "errorMessage": ""
}
```

BT 场景尽量补：

* `infoHash`
* `numSeeders`
* `seeder`
* `pieceLength`
* `numPieces`
* `bitfield`

ED2K 场景可补：

* `infoHash` 留空
* `connections` 表示活跃来源数
* 其他 BT 专属字段置空或省略

---

## 6.8 `aria2.getPeers`

对于 BT：

* 返回 peer 列表

对于 ED2K：

* 可以先返回 source 的简化结构
* 或第一版返回空数组

建议第一版：

* BT 正常返回
* ED2K 返回空数组，避免错误映射

---

## 6.9 `system.multicall`

必须支持。

因为很多 aria2 面板会使用这个一次查多个方法。

实现方式：

* 顺序执行每个子调用
* 逐个返回结果或错误
* 与 aria2 接近的格式即可

---

# 七、原生扩展 RPC 设计

aria2 表达不了 ED2K 和一部分 BT 特性，所以必须提供扩展接口。

命名空间建议：`native.*`

---

## 7.1 BT 扩展

* `native.getBtTrackers`
* `native.getBtPeers`
* `native.reannounceTorrent`
* `native.forcePieceCheck`

---

## 7.2 ED2K 扩展

* `native.addEd2k`
* `native.getEd2kSources`
* `native.getEd2kServers`
* `native.connectEd2kServer`
* `native.getKadStatus`
* `native.connectKad`
* `native.recheckEd2kFile`

---

## 7.3 全局扩展

* `native.getTask`
* `native.getTaskMeta`
* `native.exportSession`
* `native.importSession`
* `native.getVersion`
* `native.getProtocolStats`

---

# 八、配置文件设计

---

## 8.1 配置兼容目标

支持读取 aria2 风格配置文件：

```ini
dir=/data/downloads
enable-rpc=true
rpc-listen-port=6800
rpc-secret=abc123
max-concurrent-downloads=5
max-overall-download-limit=0
max-overall-upload-limit=0
listen-port=6881
enable-dht=true
bt-max-peers=80
seed-ratio=1.0
save-session=session.json
save-session-interval=60
```

同时支持扩展的 ED2K 配置：

```ini
ed2k-enable=true
ed2k-listen-port=4662
ed2k-server-port=4672
ed2k-max-sources=500
ed2k-kad-enable=true
ed2k-server-enable=true
ed2k-aich-enable=true
ed2k-source-exchange=true
ed2k-upload-slots=8
```

---

## 8.2 解析规则

### 文件规则

* 按行解析
* 忽略空行
* 忽略 `#` 注释
* `key=value`
* key 统一转小写
* 保留 value 原始字符串
* 支持同 key 多次出现

### 处理规则

* 通用配置转内部 Config
* 不支持项记录 warning
* 严重非法配置返回错误

---

## 8.3 内部配置结构

```go
type Config struct {
    Runtime RuntimeConfig
    RPC     RPCConfig
    Global  GlobalConfig
    BT      BTConfig
    ED2K    ED2KConfig
    Session SessionConfig
}

type RPCConfig struct {
    Enable         bool
    ListenHost     string
    ListenPort     int
    Secret         string
    AllowOriginAll bool
}

type GlobalConfig struct {
    Dir                    string
    MaxConcurrentDownloads int
    MaxOverallDownloadRate int64
    MaxOverallUploadRate   int64
    PauseOnStart           bool
    LogPath                string
    LogLevel               string
}

type BTConfig struct {
    Enable         bool
    ListenPort     int
    EnableDHT      bool
    EnableDHT6     bool
    MaxPeers       int
    SeedRatio      float64
    SeedTime       int
    StopTimeoutSec int
}

type ED2KConfig struct {
    Enable         bool
    ListenPort     int
    ServerPort     int
    MaxSources     int
    KadEnable      bool
    ServerEnable   bool
    AICHEnable     bool
    SourceExchange bool
    UploadSlots    int
}

type SessionConfig struct {
    SavePath       string
    SaveIntervalSec int
    AutoRestore    bool
}
```

---

# 九、任务状态机

---

## 9.1 状态定义

统一状态：

* `waiting`
* `active`
* `paused`
* `complete`
* `error`
* `removed`

---

## 9.2 状态流转

```text
new -> waiting
waiting -> active
active -> paused
paused -> active
active -> complete
active -> error
waiting/paused/active -> removed
error -> removed
complete -> removed
```

---

## 9.3 任务队列规则

* 有 `max-concurrent-downloads`
* 超出的任务进入 waiting
* active 任务结束后，waiting 自动提升
* 手动 unpause 时若 active 已满，则进入 waiting
* forcePause 立即暂停，不等待内部缓冲

---

# 十、BT 设计要求

这里只写到可落地的第一版边界。

---

## 10.1 输入支持

* magnet URI
* `.torrent` 文件内容
* `.torrent` 远程 URL

---

## 10.2 第一版能力

* 元数据获取
* tracker announce
* 基础 peer 管理
* piece 下载
* piece 校验
* 断点续传
* seeding
* 速度统计
* 文件选择下载可预留

---

## 10.3 BT 状态字段

任务 Meta 维护：

* `infoHash`
* `trackers`
* `numSeeders`
* `seeder`
* `pieceLength`
* `numPieces`
* `bitfield`

---

# 十一、ED2K 设计要求

---

## 11.1 输入支持

* `ed2k://|file|...|/`

例如：

```text
ed2k://|file|example.iso|734003200|0123456789ABCDEF0123456789ABCDEF|/
```

后续可扩展：

* 带 AICH
* 带来源
* 带目录信息

---

## 11.2 第一版能力

建议第一阶段做到：

* ed2k link 解析
* 任务创建
* server 连接
* source 获取
* 分块下载
* part 文件保存
* 完成后 hash 校验
* 断点恢复

第二阶段再做：

* Kad
* AICH
* source exchange
* 上传队列
* credit

---

## 11.3 ED2K 数据模型建议

### ED2K 文件元信息

```go
type ED2KMeta struct {
    FileName   string
    FileSize   int64
    ED2KHash   string
    AICHHash   string
    PartCount  int
}
```

### Source 模型

```go
type ED2KSource struct {
    ID          string
    IP          string
    Port        int
    ClientName  string
    LowID       bool
    Availability int
    Downloading bool
}
```

---

# 十二、持久化设计

---

## 12.1 session 持久化目标

程序重启后能够恢复：

* 未完成任务
* 已暂停任务
* waiting 队列
* 任务 `gid`
* 任务 options
* BT / ED2K 基本元信息
* 文件完成度

---

## 12.2 存储建议

第一版建议：

* 元数据与 session：JSON 文件
* 下载数据：真实文件 + part 文件
* 任务状态：内存 + 周期性刷盘

后续如任务量大再换 BoltDB / SQLite。

---

## 12.3 session 文件结构

```go
type PersistedTask struct {
    ID              string
    GID             string
    Protocol        string
    Name            string
    Status          string
    SaveDir         string
    TotalLength     int64
    CompletedLength int64
    UploadedLength  int64
    Options         map[string]string
    Meta            map[string]any
    Files           []TaskFile
}
```

---

# 十三、RPC 错误处理规范

---

## 13.1 错误分类

* 鉴权失败
* 参数错误
* 任务不存在
* 协议不支持
* 配置非法
* 驱动内部错误

---

## 13.2 返回规范

对外走 JSON-RPC 错误对象：

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "error": {
    "code": -32602,
    "message": "Invalid params"
  }
}
```

aria2 风格上可以尽量 message 简洁直白。

---

# 十四、日志设计

---

## 14.1 日志等级

* debug
* info
* warn
* error

---

## 14.2 关键日志点

必须记录：

* 配置加载结果
* RPC 启动信息
* 任务创建/恢复/删除
* BT 元数据获取
* ED2K server 连接状态
* session 保存
* 错误栈

---

# 十五、第一阶段 MVP 范围

---

## 15.1 必做

### 框架

* 守护进程主程序
* 配置加载
* session 恢复
* 任务管理器
* 驱动接口
* aria2 JSON-RPC 服务

### aria2 方法

* `aria2.addUri`
* `aria2.addTorrent`
* `aria2.remove`
* `aria2.pause`
* `aria2.unpause`
* `aria2.tellStatus`
* `aria2.tellActive`
* `aria2.tellWaiting`
* `aria2.tellStopped`
* `aria2.getFiles`
* `aria2.getGlobalStat`
* `system.listMethods`
* `system.multicall`

### BT

* magnet
* torrent
* 基础下载
* 基础 seeding
* 基础状态统计

### ED2K

* ed2k link 解析
* 任务创建
* 基础下载框架
* 基础 source 管理
* 断点恢复骨架

---

## 15.2 可后置

* WebSocket 通知
* `getPeers`
* Kad
* AICH
* source exchange
* 上传队列
* 完整文件选择下载
* 高级限速策略

---

# 十六、第二阶段目标

* WebSocket JSON-RPC
* aria2 通知事件
* 更完整 `tellStatus`
* `aria2.getPeers`
* `aria2.getOption`
* `aria2.changeOption`
* `aria2.getGlobalOption`
* `aria2.changeGlobalOption`
* BT 文件选择
* ED2K server 管理
* native 扩展 RPC

---

# 十七、第三阶段目标

* Kad
* AICH
* source exchange
* 上传积分 / 队列
* 更完善的 ED2K 行为模拟
* XML-RPC 兼容
* 更高 aria2 兼容度

---

# 十八、开发约束

---

## 18.1 代码要求

* 使用 Go
* 模块化清晰
* 不允许把 BT / ED2K 逻辑写进 aria2 兼容层
* 不允许在 handler 里直接拼复杂业务逻辑
* 必须有接口抽象，便于单测
* 返回结构清晰，避免 map 到处乱飞
* 持久化格式稳定，可向后兼容

---

## 18.2 性能要求

第一版不追求极致性能，但要求：

* 500 个任务以内结构仍清晰可维护
* 状态查询不能阻塞下载主流程
* session 保存要可控，避免频繁写盘
* 对大量 peer/source 状态更新做增量聚合

---

# 十九、验收标准

---

## 19.1 基础验收

* 能通过 RPC 添加 magnet 任务
* 能通过 RPC 添加 torrent 任务
* 能通过 RPC 添加 ed2k 任务
* `tellStatus` 能正常返回
* 暂停/恢复/删除可用
* 重启后未完成任务可恢复
* 配置文件可读取
* `rpc-secret` 生效

---

## 19.2 兼容验收

* 常见 aria2 GUI 能连接
* 能看见 active/waiting/stopped 列表
* 能正常添加/暂停/删除任务
* `system.multicall` 可用
* 绝大多数数字字段以字符串返回


## 20 开源库使用
* bt使用 https://github.com/anacrolix/torrent
* ed2k使用 https://github.com/chenjia404/goed2k
* aria2的文档 https://aria2.github.io/manual/en/html/aria2c.html