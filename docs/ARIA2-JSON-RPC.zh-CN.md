# go-aria2：aria2 兼容 JSON-RPC API 说明

本文描述 **当前已实现** 的 JSON-RPC 2.0 接口，与 aria2 常用方法对齐。传输为 **HTTP POST**，路径 **`/jsonrpc`**（默认与进程监听端口组合，例如 `http://127.0.0.1:6800/jsonrpc`）。另提供 **`/ws`** WebSocket（事件推送，非本文重点）。

---

## 1. 通用约定

### 1.1 请求体（单条调用）

| 字段 | 类型 | 说明 |
|------|------|------|
| `jsonrpc` | string | 固定 `"2.0"`（可选；非 `2.0` 会报错） |
| `method` | string | 方法名，如 `aria2.addUri` |
| `params` | array | **必须是 JSON 数组**。见各方法 |
| `id` | any | 请求关联 ID，原样出现在响应中 |

### 1.2 批量请求

请求体为 **JSON 数组**，元素为多个与上表相同结构的请求对象；响应为与请求等长的响应对象数组。

### 1.3 鉴权（`rpc-secret`）

- 若进程 **未** 配置 `rpc-secret`：  
  - `params` 可直接为方法参数；  
  - 若首元素为形如 `token:...` 的字符串，会被剥掉，其余元素作为方法参数（兼容带 token 的客户端）。
- 若进程 **已** 配置 `rpc-secret`：  
  - `params` **第一个元素** 必须为字符串 `token:<与配置一致的密钥>`；  
  - 其后元素才是方法参数。

示例（已配置 secret 时）：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "aria2.getVersion",
  "params": ["token:YOUR_SECRET"]
}
```

### 1.4 成功响应

| 字段 | 类型 | 说明 |
|------|------|------|
| `jsonrpc` | string | `"2.0"` |
| `id` | any | 与请求一致 |
| `result` | any | 方法返回值 |

### 1.5 错误响应

| 字段 | 类型 | 说明 |
|------|------|------|
| `jsonrpc` | string | `"2.0"` |
| `id` | any | 与请求一致 |
| `error` | object | 见下表 |

`error` 对象：

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | number | 例如 `-32601` 方法不存在，`-32602` 参数无效 |
| `message` | string | 错误描述 |

常用 `code`：

| 值 | 含义 |
|----|------|
| `-32700` | 解析错误（非法 JSON） |
| `-32600` | 无效请求 |
| `-32601` | 方法未找到 |
| `-32602` | 无效参数 |
| `-32603` | 内部错误 |

### 1.6 `options` 对象（键值对）

在 `addUri`、`addTorrent`、`changeOption`、`changeGlobalOption` 等中，选项为 **JSON 对象**，键为字符串；实现侧会将值 **`fmt` 转为字符串** 存入内部选项表。

常见键（与配置/aria2 命名兼容，未列出的键可能被接受但部分尚未实现语义）包括：`dir`、`pause`、`max-concurrent-downloads`、`http-user-agent`、`http-referer`、`bt-tracker`、`bt-exclude-tracker`、**`select-file`**（BT：逗号分隔的 1-based 索引与闭区间 `a-b`，与 aria2 一致；可在 `addUri`/`addTorrent` 与 `changeOption` 中使用；空或未设置表示下载全部文件）等。具体以 `internal/app/daemon.go` 中 `buildGlobalOptions` 与驱动实现为准。

---

## 2. 任务状态 `status`（字符串）

| 值 | 说明 |
|----|------|
| `waiting` | 等待中 |
| `active` | 下载/运行中 |
| `paused` | 已暂停 |
| `complete` | 已完成 |
| `error` | 错误 |
| `removed` | 已移除（若任务仍存在于列表时；**成功 `remove` 后任务会从管理器删除，一般不再出现在列表中**） |

---

## 3. `aria2.tellStatus` 等返回中的「状态对象」字段

当 `keys` 为空或未指定时，返回 **全部** 下列字段（若适用）；若传入 `keys` 数组，则 **仅返回** 列出的键。

**注意：下列数值型量在 JSON 中多为字符串**（与 aria2 风格一致）。

| 字段 | JSON 类型 | 说明 |
|------|-----------|------|
| `gid` | string | 任务 GID |
| `status` | string | 见上表 |
| `totalLength` | string | 总长度（十进制字符串） |
| `completedLength` | string | 已完成长度 |
| `uploadLength` | string | 已上传量 |
| `downloadSpeed` | string | 下载速度（字节/秒） |
| `uploadSpeed` | string | 上传速度（字节/秒） |
| `connections` | string | 连接数 |
| `numSeeders` | string | 做种端数量 |
| `pieceLength` | string | 分片长度 |
| `verifiedLength` | string | 已校验长度 |
| `verifyIntegrityPending` | string | `"true"` / `"false"` |
| `seeder` | string | 是否做种 |
| `infoHash` | string | BT InfoHash 十六进制（非 BT 可能为空） |
| `dir` | string | 保存目录（任务级） |
| `files` | array | 文件列表，结构见第 4 节 |
| `errorCode` | string | 错误码 |
| `errorMessage` | string | 错误信息 |
| `bittorrent` | object | 仅 BT 任务可能存在，见第 5 节 |

---

## 4. `files` 数组元素（`getFiles` / 状态中内嵌）

| 字段 | JSON 类型 | 说明 |
|------|-----------|------|
| `index` | string | 文件序号（从 1 起，字符串） |
| `path` | string | 路径 |
| `length` | string | 文件长度 |
| `completedLength` | string | 已完成长度 |
| `selected` | string | `"true"` / `"false"` |
| `uris` | array | `{ "uri", "status" }` 列表，见第 8 节 |

---

## 5. `bittorrent` 对象（BT 任务）

| 字段 | JSON 类型 | 说明 |
|------|-----------|------|
| `mode` | string | 来自内部 meta |
| `info` | object | 含 `name`（字符串）等 |
| `announceList` | array | 分层 tracker：`[[ "url1" ], [ "url2" ], ...]` |
| `comment` | string | 可选 |
| `createdBy` | string | 可选 |
| `creationDate` | string | 可选 |

---

## 6. 方法列表

以下 **`params` 均为去掉 `token:` 后的数组**（未配置 secret 时与原始 `params` 相同）。

### 6.1 `aria2.addUri`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | uris | array[string] | 是 | URI 列表，如 `http(s)://`、`magnet:`、`ed2k://` 等，每项非空字符串 |
| `1` | options | object | 否 | 选项，如 `dir` |

**响应 `result`：** `string`，新任务的 **GID**。

---

### 6.2 `aria2.addTorrent`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | torrent | string | 是 | `.torrent` 文件内容的 **Base64** |
| `1` | uris | any | 否 | 与官方 aria2 位置对齐；**当前实现未使用** |
| `2` | options | object | 否 | **仅当 `params` 长度 ≥ 3 时读取**。选项对象 |

**说明：** 若只传两个参数 `[torrent, options]`，实现 **不会** 把第二项当作 options；需与官方一致传三项时第三项才是 options。

**响应 `result`：** `string`，GID。

---

### 6.3 `aria2.remove` / `aria2.forceRemove`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** `string`，被移除任务的 GID（`forceRemove` 在部分协议下会额外删除本地文件，语义见驱动实现）。

---

### 6.4 `aria2.pause` / `aria2.forcePause`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** `string`，GID。

---

### 6.5 `aria2.unpause`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** `string`，GID。

---

### 6.6 `aria2.pauseAll` / `aria2.unpauseAll`

**params：** 无额外参数（仅有 token 时数组可为 `["token:..."]` 或 `[]` 视鉴权而定）。

**响应 `result`：** `string`，`"OK"`。

---

### 6.7 `aria2.removeDownloadResult`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**说明：** 删除任务关联的 **本地结果文件**，不删除任务本身。

**响应 `result`：** `string`，`"OK"`。

---

### 6.8 `aria2.tellStatus`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |
| `1` | keys | array[string] | 否 | 若省略或空，返回全部状态字段；否则只返回指定键 |

**响应 `result`：** **object**，见第 3 节（及嵌套 `files`、`bittorrent`）。

---

### 6.9 `aria2.tellActive`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | keys | array[string] | 否 | 每个任务的状态过滤键，规则同 `tellStatus` |

**响应 `result`：** **array[object]**，每项为状态对象。

---

### 6.10 `aria2.tellWaiting`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | offset | number / string | 是 | 偏移 |
| `1` | num | number / string | 是 | 条数 |
| `2` | keys | array[string] | 否 | 状态字段过滤 |

**响应 `result`：** **array[object]**，`waiting` 与 `paused` 任务分页列表。

---

### 6.11 `aria2.tellStopped`

参数同 `tellWaiting`。

**响应 `result`：** **array[object]**，状态为 `complete`、`error`、`removed` 的任务分页列表（**已从管理器 `remove` 的任务通常不再出现**）。

---

### 6.12 `aria2.getFiles`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** **array[object]**，元素字段见第 4 节。

---

### 6.13 `aria2.getPeers`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** **array[object]**：

| 字段 | JSON 类型 | 说明 |
|------|-----------|------|
| `peerId` | string | Peer ID（URL 编码等形式） |
| `ip` | string | IP |
| `port` | string | 端口 |
| `bitfield` | string | 位域十六进制 |
| `amChoking` | string | `"true"` / `"false"` |
| `peerChoking` | string | `"true"` / `"false"` |
| `downloadSpeed` | string | 下载速度 |
| `uploadSpeed` | string | 上传速度 |
| `seeder` | string | `"true"` / `"false"` |

非 BT/ED2K 等无 peer 列表的实现可能返回 `[]`。

---

### 6.14 `aria2.getServers`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** **array[object]**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `index` | string | 文件索引 |
| `servers` | array | 元素：`uri`、`currentUri`、`downloadSpeed`（字符串） |

---

### 6.15 `aria2.getUris`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** **array[object]**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `uri` | string | URI |
| `status` | string | 如 `"used"` |

---

### 6.16 `aria2.getOption`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |

**响应 `result`：** **object**，键值均为 **string**。在任务选项基础上可能合并：`dir`、`pause`（`true`/`false`）、`out`（任务名）等。

---

### 6.17 `aria2.changeOption`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | gid | string | 是 | 任务 GID |
| `1` | options | object | 是 | 要设置的选项，不能为空对象 |

**响应 `result`：** `string`，GID。

---

### 6.18 `aria2.getGlobalOption`

**params：** 无业务参数。

**响应 `result`：** **object[string]string**，当前全局选项快照。

---

### 6.19 `aria2.changeGlobalOption`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | options | object | 是 | 要合并进全局的键值，不能为空对象 |

**响应 `result`：** **object[string]string**，更新后的全局选项。

---

### 6.20 `aria2.getGlobalStat`

**响应 `result`：** **object**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `numActive` | string | 活动任务数 |
| `numWaiting` | string | 等待/暂停等 |
| `numStopped` | string | 已停止相关计数 |
| `downloadSpeed` | string | 总下载速度 |
| `uploadSpeed` | string | 总上传速度 |

---

### 6.21 `aria2.getVersion`

**响应 `result`：** **object**（示例字段，版本号随发布变化）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `version` | string | 短版本 |
| `fullVersion` | string | 完整版本字符串 |
| `releaseDate` | string | 日期 |
| `organization` | string | 组织/项目 |
| `copyright` | string | 版权 |
| `enabledFeatures` | array[string] | 功能列表 |
| `enabledProtocols` | array[string] | 已启用协议 |
| `supportedProtocols` | array[string] | 支持协议 |

---

### 6.22 `aria2.getSessionInfo`

**响应 `result`：** **object**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `sessionId` | string | 会话 ID |
| `startTime` | string | RFC3339 启动时间 |
| `uptimeSecs` | number | 运行秒数 |
| `aria2Style` | bool | 兼容标记 |
| `server` | string | 服务标识 |
| `generatedAt` | string | RFC3339 |

---

### 6.23 `system.listMethods`

**响应 `result`：** **array[string]**，已实现的方法名列表。

---

### 6.24 `system.multicall`

| 索引 | 名称 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `0` | batch | array | 是 | 每个元素为对象，见下表 |

batch 元素：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `methodName` | string | 是 | 方法名 |
| `params` | array | 否 | 该方法参数（**不含** multicall 外层的 token 时，子调用需自行带 token 或依赖未启用 secret） |

**响应 `result`：** **array**，每项为 **单元素数组** `[ subResult ]`，与 aria2 行为一致。任一子调用失败则整次 multicall 返回错误。

---

## 7. 请求示例

```json
{
  "jsonrpc": "2.0",
  "id": "q1",
  "method": "aria2.addUri",
  "params": [
    "token:YOUR_SECRET",
    ["https://example.com/file.zip"],
    { "dir": "/downloads" }
  ]
}
```

```json
{
  "jsonrpc": "2.0",
  "id": "q2",
  "method": "aria2.tellStatus",
  "params": ["token:YOUR_SECRET", "YOUR_GID", ["gid", "status", "totalLength"]]
}
```

---

## 8. 内嵌 `uris`（文件项下）

| 字段 | 类型 | 说明 |
|------|------|------|
| `uri` | string | URI |
| `status` | string | 如 `"used"` |

---

*文档版本随代码演进；如有出入以 `internal/compat/aria2` 与 `internal/core/manager` 实现为准。*
