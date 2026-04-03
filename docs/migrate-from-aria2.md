# aria2 迁移说明

本文说明如何把现有 `aria2` 的未完成任务迁移到 `github.com/chenjia404/go-aria2`，并尽量复用已有文件继续下载。

## 目标

- 读取 `aria2` 的 `save-session` 文件
- 解析其中的任务块
- 按协议导入到当前下载内核
- 保留原有 `gid`
- 自动识别已存在文件并恢复进度
- 对 BT 任务进行 piece 级恢复
- 单任务失败不影响其他任务

## 使用方式

命令如下：

```bash
go-aria2 migrate-from-aria2 --session /path/to/session.txt
```

可选参数：

- `--conf`：指定 aria2 风格配置文件，默认 `aria2.conf`
- `--session`：指定 aria2 `save-session` 文件路径，必填
- `--strict`：启用 BT 严格校验模式，默认关闭

## 迁移流程

迁移过程分为四步：

1. 读取 `aria2 save-session` 文件。
2. 解析每个任务块，得到 URI 和相关选项。
3. 创建内部任务并导入任务管理器。
4. 对 BT 任务重建 piece 完成状态，并把结果写回 session。

## session 文件格式

`aria2` 的 session 文件按“任务块”组织：

- 每个任务以一行 URI 开头
- 后续缩进行是该任务的选项
- 空行忽略
- `#` 开头的行视为注释

示例：

```text
magnet:?xt=urn:btih:xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
 dir=/downloads
 out=movie.mkv

http://example.com/file.torrent
 dir=/downloads/torrents

ed2k://|file|sample.iso|123456789|abcdef0123456789abcdef0123456789|/
```

### 支持的选项

解析器会保留所有 `key=value` 形式的选项，并把 key 统一成小写。

其中迁移逻辑会重点使用这些字段：

- `dir`
- `out`
- `gid`

其他选项会保留在任务 `Options` 中，便于后续兼容扩展。

## 协议路由规则

导入时会根据 URI 自动判断协议：

- `magnet:?` -> BT
- `http(s)://... .torrent` -> BT
- `ed2k://...` -> ED2K
- 普通 `http(s)` 文件 -> HTTP

当前迁移重点是 BT 任务恢复。ED2K 和 HTTP 任务会先导入到任务系统，后续可继续补充更完整的恢复策略。

## BT 进度恢复

BT 任务导入后会尝试重建 piece 完成状态。

恢复策略分两种：

- 默认快速模式
  - 先按“已完成”乐观恢复
  - 后台再做校验
- 严格模式
  - 逐 piece 读取磁盘文件
  - 计算 SHA1 hash
  - 和 torrent 元数据中的 piece hash 比较

### 文件定位规则

BT 文件路径按 aria2 的目录语义拼接：

```text
path = dir + "/" + torrent_name + "/" + file_path
```

对于单文件 torrent，`torrent_name` 可能不会出现，实际以 torrent 实现给出的文件布局为准。

### 缺失和异常处理

- 文件不存在：记为未完成，不报错
- 文件大小不一致：进入重新校验
- hash 校验失败：该 piece 记为未完成
- 权限错误：记录 warning，继续处理其他 piece

### 恢复结果

恢复完成后会写入任务元数据：

- `bitfield`
- `bt.totalPieces`
- `bt.completedPieces`

同时会更新任务的：

- `CompletedLength`
- `VerifiedLength`
- `PieceLength`

## GID 处理

迁移时会尽量保留 aria2 原始 `gid`：

- 如果 session 里带有 `gid`，会优先复用
- 如果没有 `gid`，会生成新的 16 位十六进制字符串

这样可以继续兼容 RPC 层中基于 `gid` 的任务访问方式。

## 日志输出

迁移命令会输出这些关键信息：

- 读取 session 文件
- 解析到的任务数量
- 每个任务的导入结果
- BT 恢复进度
- 错误和 warning

示例：

```text
[INFO] Reading session file: /path/to/session.txt
[INFO] Parsed 12 session tasks
[INFO] Importing task: movie.mkv
[INFO] Found existing file: /downloads/movie.mkv
[INFO] Rebuilding BT progress...
[INFO] Completed pieces: 180/256
[INFO] Task imported successfully
```

## 错误处理策略

- 单个任务失败不会中断整批导入
- 最终会返回成功任务列表和错误列表
- 只有当所有任务都失败时，命令才会返回非零退出码

常见错误包括：

- session 文件不存在
- session 格式错误
- torrent 元数据无法解析
- 文件权限不足
- hash 校验失败

## 迁移建议

建议按以下顺序执行：

1. 先备份原 `aria2` 的 `save-session` 文件和下载目录。
2. 启动本程序（模块 `github.com/chenjia404/go-aria2`），确认配置中的 `dir` 和 `data-dir` 与原环境一致。
3. 运行迁移命令。
4. 检查日志中的导入结果。
5. 继续运行下载内核，观察 BT 任务是否恢复到预期进度。

如果你有大量 BT 任务，建议先用默认快速模式验证一轮，再对少数关键任务启用 `--strict` 做精确校验。

## 当前限制

- 迁移功能主要面向 BT / ED2K / HTTP 任务的导入和重建
- BT 严格恢复依赖 torrent 元数据可用
- 如果原文件目录结构和当前配置不一致，恢复结果会受影响
- `aria2` 的全部 session 细节并不一定都能 100% 对齐

## 后续扩展

后续可以继续补充：

- 从 aria2 RPC 直接导入任务
- 更完整的 ED2K 恢复
- 更多 aria2 session 字段兼容
- `--dry-run` 预览模式
- JSON 输出模式，便于自动化迁移脚本使用
