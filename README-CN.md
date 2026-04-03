# go-aria2

Go 模块路径：`github.com/chenjia404/go-aria2`（克隆地址：<https://github.com/chenjia404/go-aria2>）。

`go-aria2` 是一个用 Go 编写的多协议下载内核，目标是用分层架构兼容 `aria2` 的常用 JSON-RPC 接口，并逐步替代原有 `aria2` 守护进程。

当前实现重点支持：

- BitTorrent
- ED2K
- HTTP / HTTPS
- aria2 风格配置文件
- aria2 常用 JSON-RPC 接口
- session 持久化与重启恢复
- 从 aria2 `save-session` 文件导入未完成任务

## 特点

- 领域模型与协议兼容层分离，`aria2` 兼容逻辑不会污染核心任务模型。
- 支持 `rpc-secret` 和 `token:xxx` 鉴权格式。
- 支持 `gid` 保留，方便迁移后继续使用原有 RPC 标识。
- 支持 BT 任务的进度重建，迁移时会尽量复用已有文件。
- 支持 `save-session` 周期性落盘，重启后继续恢复任务。

## 目录结构

```text
/cmd/go-aria2       统一入口
/internal/core      核心领域层
/internal/protocol  协议驱动
/internal/compat    aria2 兼容层
/internal/config    aria2.conf 风格配置解析
/internal/rpc       JSON-RPC / WebSocket 服务
/internal/migrate   aria2 迁移逻辑
/docs               迁移文档
```

## 快速开始

### 1. 启动守护进程

默认读取当前目录下的 `aria2.conf`：

```bash
go run ./cmd/go-aria2 daemon
```

指定配置文件：

```bash
go run ./cmd/go-aria2 daemon -conf /path/to/aria2.conf
```

守护进程参数支持常见 aria2 风格别名，例如：

```bash
go run ./cmd/go-aria2 -d /downloads -j 4 --rpc-secret secret --enable-rpc
```

也可以像 `aria2c` 一样直接跟 URI 或 `-i` 输入文件：

```bash
go run ./cmd/go-aria2 https://example.com/file.torrent
go run ./cmd/go-aria2 -i ./input.txt
```

`-i/--input-file` 现在有两种行为：

- `-i *.txt` 或其他非 `.json` 文件：按 aria2 风格 `input-file` 文本任务列表解析
- `-i *.json`：按 go-aria2 自己的 `session.json` 处理，作为会话恢复文件加载

例如：

```bash
go run ./cmd/go-aria2 -i ./input.txt
go run ./cmd/go-aria2 -i ./data/session.json
```

不要把 go-aria2 的 `session.json` 当成 aria2 的文本 `input-file`。前者是内部 JSON 会话文件，后者是纯文本任务列表，格式不同。

### 2. 发起 JSON-RPC 请求

使用调试 CLI 直接调用接口：

```bash
go run ./cmd/go-aria2 ctl -method system.listMethods
go run ./cmd/go-aria2 ctl -method aria2.getGlobalStat
go run ./cmd/go-aria2 ctl -secret your-secret -method aria2.tellActive
```

`ctl` 同时接受 `--rpc-secret`，与 aria2 常见参数名对齐。

自定义参数：

```bash
go run ./cmd/go-aria2 ctl \
  -method aria2.addUri \
  -params '[[\"https://example.com/file.torrent\"],{\"dir\":\"/downloads\"}]'
```

**RPC 调不通时请先核对：** 进程是否已正常跑完初始化（若 BT/ED2K 初始化失败会直接退出，没有 RPC）；`enable-rpc=true`；客户端 URL 是否为 `http://<主机>:<端口>/jsonrpc`（路径必须是 `/jsonrpc`）；端口与 `rpc-listen-port` 一致。默认 **`rpc-listen-all=false`** 时只监听 **127.0.0.1**，从 Docker 宿主机、局域网其它机器访问会失败，需 **`rpc-listen-all=true`** 并放行防火墙。配置了 **`rpc-secret`** 时，参数首项需为 **`token:<密钥>`**（`ctl` 可用 `-secret` 自动加）。可先 `curl http://127.0.0.1:<端口>/healthz` 应返回 `ok`。

## 编译

### 当前平台直接编译

Linux / macOS：

```bash
go build -trimpath -o go-aria2 ./cmd/go-aria2
```

Windows PowerShell：

```powershell
go build -trimpath -o .\go-aria2.exe .\cmd\go-aria2
```

### 为 Windows 编译

建议优先生成纯 Go 可执行文件，显式关闭 `CGO`。这样可以避免额外依赖 `libgcc_s_seh-1.dll` 一类运行库，也更适合在 WSL 或 Linux 下交叉编译。

64 位 Windows：

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o go-aria2.exe ./cmd/go-aria2
```

32 位 Windows：

```bash
GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -trimpath -o go-aria2.exe ./cmd/go-aria2
```

Windows PowerShell：

```powershell
$env:GOOS="windows"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -trimpath -o .\go-aria2.exe .\cmd\go-aria2
```

### WSL 交叉编译说明

如果你在 WSL 里编译，产物会在 Windows 下运行，建议固定使用：

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0
```

如果省略 `CGO_ENABLED=0`，Go 可能会走外部链接，生成依赖额外 DLL 的 Windows 可执行文件。此时在 PowerShell 或 `Start-Process` 下，可能出现下面的错误：

```text
The specified executable is not a valid application for this OS platform.
```

这个报错常见原因有两个：

- 生成的是带外部运行库依赖的 Windows 可执行文件，目标机器缺少对应 DLL
- 可执行文件架构与目标系统不匹配，例如把 `amd64` 产物放到 32 位 Windows 上运行

可以在 PowerShell 检查系统位数：

```powershell
[Environment]::Is64BitOperatingSystem
```

如果结果是 `False`，请改用 `GOARCH=386` 重新编译。

## 迁移 aria2 任务

支持从 aria2 的 `save-session` 文件导入任务：

```bash
go run ./cmd/go-aria2 migrate-from-aria2 --session /path/to/session.txt
```

可选参数：

- `--conf`：aria2 风格配置文件路径，默认 `aria2.conf`
- `--session`：aria2 `save-session` 文件路径，必填
- `--strict`：启用 BT 严格恢复模式

迁移说明见 [docs/migrate-from-aria2.md](docs/migrate-from-aria2.md)。

如果你是把本程序直接替代现有 `aria2` 守护进程使用，建议先阅读 [docs/migrate-from-aria2.md](docs/migrate-from-aria2.md) 中的“停旧服务 -> 备份 -> 导入旧 save-session -> 启动 go-aria2”完整切换流程。

## 配置文件

配置文件采用 `key=value` 格式，支持空行和 `#` 注释。

示例：

```ini
enable-rpc=true
rpc-listen-port=6800
rpc-secret=change-me
dir=./downloads
data-dir=./data
max-concurrent-downloads=3
max-overall-download-limit=0
max-overall-upload-limit=0
listen-port=6881
enable-dht=true
bt-max-peers=50
seed-ratio=1.0
save-session=./data/session.json
save-session-interval=30
ed2k-enable=true
ed2k-listen-port=4662
ed2k-server-port=4661
ed2k-max-sources=200
ed2k-kad-enable=true
ed2k-server-enable=true
ed2k-aich-enable=true
ed2k-source-exchange=true
ed2k-upload-slots=3
```

### 主要配置项

- RPC
  - `enable-rpc`
  - `rpc-listen-port`
  - `rpc-listen-all`
  - `rpc-allow-origin-all`
  - `rpc-max-request-size`
  - `rpc-secret`
  - `enable-websocket`
- 下载目录与调度
  - `dir`
  - `data-dir`
  - `max-concurrent-downloads`
  - `pause`
  - `continue`
  - `save-session`
  - `save-session-interval`
- HTTP
  - `http-user-agent`
  - `http-referer`
  - `http-proxy`
  - `https-proxy`
  - `all-proxy`
  - `max-connection-per-server`
  - `split`
  - `check-certificate`
- BT
  - `listen-port`
  - `enable-dht`
  - `bt-max-peers`
  - `seed-ratio`
- ED2K
  - `ed2k-enable`
  - `ed2k-listen-port`
  - `ed2k-server-port`
  - `ed2k-max-sources`
  - `ed2k-kad-enable`
  - `ed2k-server-enable`
  - `ed2k-aich-enable`
  - `ed2k-source-exchange`
  - `ed2k-upload-slots`

未识别配置项不会直接退出，会记录 warning。

## JSON-RPC 兼容范围

当前第一阶段和迁移相关接口已实现，常用方法包括：

- `aria2.addUri`
- `aria2.addTorrent`
- `aria2.remove`
- `aria2.forceRemove`
- `aria2.pause`
- `aria2.forcePause`
- `aria2.pauseAll`
- `aria2.unpause`
- `aria2.unpauseAll`
- `aria2.tellStatus`
- `aria2.tellActive`
- `aria2.tellWaiting`
- `aria2.tellStopped`
- `aria2.getFiles`
- `aria2.getPeers`
- `aria2.getServers`
- `aria2.getUris`
- `aria2.getOption`
- `aria2.changeOption`
- `aria2.getGlobalOption`
- `aria2.changeGlobalOption`
- `aria2.getGlobalStat`
- `aria2.getVersion`
- `aria2.getSessionInfo`
- `aria2.removeDownloadResult`
- `system.listMethods`
- `system.multicall`

兼容约定：

- `gid` 使用 16 位十六进制字符串
- `tellStatus` 中的数值字段按字符串序列化
- 支持 `token:xxx` 形式的鉴权参数
- WebSocket 通知端点同样遵循 `rpc-secret`，可通过 `?token=...`、`Authorization: token:...` 或 `X-Auth-Token` 传入

## 协议支持

### BitTorrent

- 支持 magnet 链接
- 支持 `.torrent` 文件 / `.torrent` URL
- 支持 session 恢复与进度重建
- 迁移时可扫描本地文件并重建 piece 完成状态

### ED2K

- 支持 `ed2k://` 链接
- 已接入真实驱动层
- 迁移时会保留任务元数据，后续可继续扩展恢复逻辑

### HTTP / HTTPS

- 支持普通下载
- 支持 Range 续传
- 支持分片并发
- 支持任务级选项覆盖

## 任务持久化

当前会将任务状态写入 session 文件，并在启动时恢复。

默认 session 路径由配置中的 `save-session` 决定，若未显式指定，会落到 `data-dir` 对应目录下。

## 已知限制

- 不是 aria2 的完整替代品，仍在逐步补齐兼容面。
- 某些 aria2 边角 RPC 还未实现。
- BT 严格恢复依赖 torrent 元数据可用。
- ED2K 的完整恢复能力仍在扩展中。

## 开发

运行测试：

```bash
go test ./...
```

端到端冒烟（需本机已安装 `go`；Bash 脚本另需 `curl`）：

- Bash（Git Bash / Linux / macOS）：`./scripts/e2e-core.sh`
- Windows PowerShell：`.\scripts\e2e-core.ps1`

环境变量：`E2E_RPC_PORT`（默认 `16880`）、`E2E_RPC_SECRET`、`E2E_BIN`（可指向已编译二进制，跳过脚本内 `go build`）、`E2E_SKIP_HTTP=1`（跳过公网 `addUri`）、`E2E_HTTP_URL`（自定义 HTTP 测试 URL）。

进程级集成测试（不依赖 `curl`，使用 `httptest` 本地 HTTP）：

```bash
go test -tags=integration -timeout 5m ./internal/app/...
```

## 许可证

当前仓库未显式声明许可证。若准备对外发布，建议先补充许可证文件。
