# 从 aria2 迁移到 go-aria2

本文面向“直接用 `go-aria2` 替代现有 `aria2` 守护进程”的场景，重点说明如何迁移旧的 `save-session`、尽量复用原有下载目录，并把切换风险降到最低。

## 适用场景

适用于以下情况：

- 你当前已经在使用 `aria2c --enable-rpc ...`
- 你有一个正在使用中的 `aria2.conf`
- 你保留了 aria2 的 `save-session` 文件
- 你希望把原来的未完成任务迁移到 `go-aria2`

不适用于以下情况：

- 你没有 `save-session` 文件，也没有办法从原 aria2 导出任务
- 你希望完整复刻 aria2 的全部参数行为
- 你依赖一些当前仍未在 `go-aria2` 中落地的高级 BT/DHT 细节

## 迁移前要先知道的事

### 1. `go-aria2` 读取的是 aria2 风格配置，但不是 100% 全量实现

也就是说：

- 常见参数会被解析
- 一部分参数已经真正生效
- 还有一部分参数只是“兼容接受，不报错，但当前只打印 warning”

所以迁移时不要假设所有 aria2 选项都已经完全等价。

### 2. 旧的 aria2 session 文件和新的 go-aria2 session 文件不是同一种格式

通常你会同时看到两类文件：

- aria2 的 `save-session` 文本文件
- go-aria2 的内部 session 文件，默认是 `data/session.json`

迁移命令读取的是前者：

```text
aria2 save-session -> 导入 -> go-aria2 data/session.json
```

迁移完成后，后续运行 `go-aria2` 时使用的是它自己的 `session.json`，不再继续写回 aria2 的 `save-session` 文本格式。

### 3. 正式迁移前，必须先停掉旧 aria2

不要让 `aria2` 和 `go-aria2` 同时写同一个下载目录和同一批临时文件，否则会出现这些问题：

- 断点状态互相覆盖
- 临时文件被重复改写
- BT 任务状态混乱
- session 内容不一致

## 建议的迁移流程

建议按下面的顺序操作，不要跳步骤。

### 第一步：停掉旧 aria2

先确保旧的 aria2 守护进程已经完全退出。

如果你是手工运行：

```bash
pkill aria2c
```

如果是 systemd / 服务管理器启动，请用你当前的方式停服务。

重点不是命令形式，而是要确认旧 aria2 已经不再写磁盘。

### 第二步：备份关键文件

至少备份这三类内容：

- 原 `aria2.conf`
- 原 `save-session` 文件
- 原下载目录

例如：

```bash
cp /path/to/aria2.conf /path/to/aria2.conf.bak
cp /path/to/session.txt /path/to/session.txt.bak
```

下载目录如果很大，不一定非要完整复制一份，但至少要保证你知道原目录在哪里，并且不要在迁移前人为移动文件。

### 第三步：确认目录映射关系

迁移是否顺利，最关键的是目录一致性。

你至少要确认这几个值：

- aria2 原来的 `dir`
- aria2 原来的 `save-session`
- go-aria2 当前配置里的 `dir`
- go-aria2 当前配置里的 `data-dir`

建议：

- `dir` 尽量和旧 aria2 保持一致
- `data-dir` 可以单独给 go-aria2 一个新目录

例如：

```ini
dir=/downloads
data-dir=/downloads/.go-aria2
save-session=/downloads/.go-aria2/session.json
```

这样做的好处是：

- 下载文件继续复用原目录
- go-aria2 的内部状态单独存放
- 出问题时更容易排查和回滚

## 推荐的目录结构

假设你原来的环境是：

```text
/downloads
/etc/aria2/aria2.conf
/etc/aria2/session.txt
```

建议迁移后使用：

```text
/downloads
/downloads/.go-aria2/session.json
/downloads/.go-aria2/bt
/downloads/.go-aria2/ed2k-state.json
/etc/go-aria2/aria2.conf
```

核心原则：

- 下载内容继续放原目录
- go-aria2 运行时状态放单独目录

## 准备 go-aria2 配置

如果你已经有一份从 aria2 改过来的配置，至少检查下面这些项目：

```ini
enable-rpc=true
rpc-listen-port=6800
rpc-secret=your-secret

dir=/downloads
data-dir=/downloads/.go-aria2
save-session=/downloads/.go-aria2/session.json
save-session-interval=30

listen-port=6881
enable-dht=true
max-concurrent-downloads=5
```

如果你是从现有 aria2 配置直接迁移，建议先保留常见核心项，不要一开始就带上太多边缘参数。

## 执行迁移

迁移命令如下：

```bash
go-aria2 migrate-from-aria2 --conf /path/to/aria2.conf --session /path/to/session.txt
```

如果你还没安装到系统路径，也可以：

```bash
go run ./cmd/go-aria2 migrate-from-aria2 --conf /path/to/aria2.conf --session /path/to/session.txt
```

如果你担心 BT 恢复不够严格，可以使用：

```bash
go-aria2 migrate-from-aria2 --conf /path/to/aria2.conf --session /path/to/session.txt --strict
```

`--strict` 的含义是：

- BT 任务会更严格地校验 piece
- 迁移耗时会更长

建议做法：

- 先不用 `--strict` 跑一遍，确认整体流程可通
- 如果有关键 BT 任务，再考虑用 `--strict`

## 迁移时程序实际做了什么

迁移命令会做这些事：

1. 读取 aria2 的 `save-session` 文本文件
2. 解析每个任务块
3. 根据 URI 自动识别协议
4. 把任务导入 go-aria2 的任务管理器
5. 尝试复用已有文件和已有进度
6. 把导入结果写入 go-aria2 自己的 `session.json`

协议路由规则如下：

- `magnet:?` -> BT
- `http(s)://...torrent` -> BT
- `ed2k://...` -> ED2K
- 普通 `http(s)` 文件 -> HTTP

## 迁移成功后看什么

成功迁移后，你应该重点检查以下几点。

### 1. 日志里是否出现 Imported N tasks

示例：

```text
[INFO] Parsed 12 session tasks
[INFO] Imported 12 tasks
[INFO] Session saved to /downloads/.go-aria2/session.json
```

如果解析到了任务，但导入数为 0，说明迁移没有真正落地。

### 2. `session.json` 是否已生成

例如：

```text
/downloads/.go-aria2/session.json
```

如果这个文件没有生成，后续重启就不会保留迁移结果。

### 3. 下载目录里的已有文件是否被识别

尤其是：

- HTTP 断点续传文件
- BT 已完成或部分完成的数据文件

### 4. RPC 是否正常

启动守护进程后，先检查健康接口：

```bash
curl http://127.0.0.1:6800/healthz
```

如果返回：

```text
ok
```

说明服务已正常起来。

## 正式切换到 go-aria2

迁移命令成功后，再启动守护进程：

```bash
go-aria2 daemon --conf /path/to/aria2.conf
```

或者：

```bash
go run ./cmd/go-aria2 daemon --conf /path/to/aria2.conf
```

如果你之前的客户端是通过 aria2 RPC 调用的，建议先验证下面几个接口：

- `system.listMethods`
- `aria2.getGlobalStat`
- `aria2.tellActive`
- `aria2.tellWaiting`
- `aria2.tellStopped`

## 推荐的首次验证顺序

正式替换时，建议这样验证：

1. 先停掉旧 aria2
2. 跑一次迁移命令
3. 启动 go-aria2
4. 检查 `healthz`
5. 检查 RPC 基本接口
6. 检查一两个 HTTP 任务是否能继续
7. 检查一两个 BT 任务是否能恢复
8. 确认没有明显异常后，再让业务侧切换到新服务

## 迁移失败时如何回滚

回滚思路应该很直接：

1. 停掉 `go-aria2`
2. 恢复你备份的 aria2 配置
3. 恢复你备份的 `save-session`
4. 重新启动旧 aria2

只要你没有让两个程序同时写同一批文件，回滚通常是可控的。

## 常见问题

### 1. 迁移后任务数变少了

常见原因：

- 原 `save-session` 文件里有损坏或不完整任务块
- 某些任务协议当前还没有完全恢复成功
- 某些导入失败，但其他任务仍然导入成功

先看迁移日志，不要凭界面感觉判断。

### 2. BT 进度和原 aria2 不完全一致

这是可能出现的。

原因通常有：

- 原文件目录和当前 `dir` 不一致
- 部分 BT 元数据不可用
- 你没有使用 `--strict`

### 3. HTTP 任务为什么会重新校验或重新下载

可能原因：

- 文件名或目录和原任务不一致
- 任务选项导致走了不同的覆盖/续传策略
- 原临时文件状态不完整

### 4. 某些参数虽然不报错，但日志里有 warning

这表示：

- 参数已经被兼容接受
- 但当前版本还没有把该行为完整接到底层实现

这类 warning 不会阻止程序启动，但你不应该把它当成“和 aria2 完全等价”。

## 当前版本下的实用建议

如果你现在的目标是“尽快替换 aria2 并稳定跑起来”，建议遵循下面这几条：

1. 保持原 `dir` 不变
2. 给 `go-aria2` 单独设置 `data-dir`
3. 先迁移，再启动，不要并行运行
4. 先验证 HTTP 和 RPC，再验证 BT 细节
5. 对 warning 保持敏感，不要忽略

## 一个完整示例

假设：

- 原 aria2 下载目录：`/data/downloads`
- 原 aria2 session：`/data/aria2/session.txt`
- 新配置文件：`/etc/go-aria2/aria2.conf`

配置：

```ini
enable-rpc=true
rpc-listen-port=6800
rpc-secret=change-me

dir=/data/downloads
data-dir=/data/downloads/.go-aria2
save-session=/data/downloads/.go-aria2/session.json
save-session-interval=30
```

迁移：

```bash
go-aria2 migrate-from-aria2 \
  --conf /etc/go-aria2/aria2.conf \
  --session /data/aria2/session.txt
```

启动：

```bash
go-aria2 daemon --conf /etc/go-aria2/aria2.conf
```

检查：

```bash
curl http://127.0.0.1:6800/healthz
```

## 结论

正确的迁移方式不是“直接拿着旧参数启动新程序”，而是：

1. 先停旧服务
2. 先备份
3. 对齐目录
4. 导入旧 `save-session`
5. 再启动 `go-aria2`
6. 最后做 RPC 和任务恢复验证

如果你的目标是用 `go-aria2` 长期替代 aria2，这个顺序不能反。
