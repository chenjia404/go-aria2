# go-aria2: aria2-compatible JSON-RPC API

This document describes the **currently implemented** JSON-RPC 2.0 surface aligned with common aria2 methods. Transport is **HTTP POST** at **`/jsonrpc`** (e.g. `http://127.0.0.1:6800/jsonrpc` with the daemon listen port). A **`/ws`** WebSocket endpoint exists for notifications (out of scope here).

---

## 1. General conventions

### 1.1 Request body (single call)

| Field | Type | Description |
|-------|------|-------------|
| `jsonrpc` | string | Should be `"2.0"` (optional; other values are rejected) |
| `method` | string | Method name, e.g. `aria2.addUri` |
| `params` | array | **Must be a JSON array**. See each method |
| `id` | any | Correlation id echoed in the response |

### 1.2 Batch requests

The body is a **JSON array** of request objects as above; the response is an array of response objects of the same length.

### 1.3 Authentication (`rpc-secret`)

- If **no** `rpc-secret` is configured:  
  - `params` are the method parameters directly;  
  - If the first element is a string like `token:...`, it is stripped and the rest are used as parameters (client compatibility).
- If `rpc-secret` **is** configured:  
  - The **first** `params` element must be `token:<secret>` matching the configured value;  
  - Following elements are the method parameters.

Example (with secret):

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "aria2.getVersion",
  "params": ["token:YOUR_SECRET"]
}
```

### 1.4 Success response

| Field | Type | Description |
|-------|------|-------------|
| `jsonrpc` | string | `"2.0"` |
| `id` | any | Same as request |
| `result` | any | Method return value |

### 1.5 Error response

| Field | Type | Description |
|-------|------|-------------|
| `jsonrpc` | string | `"2.0"` |
| `id` | any | Same as request |
| `error` | object | See below |

`error` object:

| Field | Type | Description |
|-------|------|-------------|
| `code` | number | e.g. `-32601` method not found, `-32602` invalid params |
| `message` | string | Human-readable message |

Common `code` values:

| Value | Meaning |
|-------|---------|
| `-32700` | Parse error (invalid JSON) |
| `-32600` | Invalid request |
| `-32601` | Method not found |
| `-32602` | Invalid params |
| `-32603` | Internal error |

### 1.6 `options` object

For `addUri`, `addTorrent`, `changeOption`, `changeGlobalOption`, etc., options are a **JSON object** with string keys; values are converted to **strings** internally (`fmt.Sprint`).

Common keys (aria2-style; others may be accepted but not fully implemented) include: `dir`, `pause`, `max-concurrent-downloads`, `http-user-agent`, `http-referer`, `bt-tracker`, `bt-exclude-tracker`, **`select-file`** (BT: comma-separated 1-based indices and inclusive ranges `a-b`, same as aria2; usable in `addUri`/`addTorrent` and `changeOption`; empty or omitted means download all files), etc. See `buildGlobalOptions` in `internal/app/daemon.go` and driver code for the source of truth.

---

## 2. Task `status` (string)

| Value | Meaning |
|-------|---------|
| `waiting` | Waiting |
| `active` | Active / downloading |
| `paused` | Paused |
| `complete` | Completed |
| `error` | Error |
| `removed` | Removed (if still listed; **after a successful `remove`, the task is dropped from the manager and usually no longer appears in lists**) |

---

## 3. Status object fields (`aria2.tellStatus`, etc.)

When `keys` is omitted or empty, **all** fields below are returned (when applicable). If `keys` is a non-empty array, **only** those keys are returned.

**Note: many numeric fields are JSON strings** (aria2-style).

| Field | JSON type | Description |
|-------|-----------|-------------|
| `gid` | string | Task GID |
| `status` | string | See table above |
| `totalLength` | string | Total length (decimal string) |
| `completedLength` | string | Completed length |
| `uploadLength` | string | Uploaded bytes |
| `downloadSpeed` | string | Download speed (bytes/s) |
| `uploadSpeed` | string | Upload speed (bytes/s) |
| `connections` | string | Connection count |
| `numSeeders` | string | Seeder count |
| `pieceLength` | string | Piece length |
| `verifiedLength` | string | Verified length |
| `verifyIntegrityPending` | string | `"true"` / `"false"` |
| `seeder` | string | Seeder flag |
| `infoHash` | string | BT info hash hex (may be empty) |
| `dir` | string | Save directory |
| `files` | array | File entries; see §4 |
| `errorCode` | string | Error code |
| `errorMessage` | string | Error message |
| `bittorrent` | object | Present for BT tasks; see §5 |

---

## 4. `files[]` entry (`getFiles` / embedded in status)

| Field | JSON type | Description |
|-------|-----------|-------------|
| `index` | string | File index (1-based, as string) |
| `path` | string | Path |
| `length` | string | File size |
| `completedLength` | string | Completed length |
| `selected` | string | `"true"` / `"false"` |
| `uris` | array | `{ "uri", "status" }`; see §8 |

---

## 5. `bittorrent` object (BT tasks)

| Field | JSON type | Description |
|-------|-----------|-------------|
| `mode` | string | From internal meta |
| `info` | object | Includes `name` (string), etc. |
| `announceList` | array | Tiered trackers: `[["url1"],["url2"],...]` |
| `comment` | string | Optional |
| `createdBy` | string | Optional |
| `creationDate` | string | Optional |

---

## 6. Methods

Below, **`params` are after stripping the optional `token:` prefix** (when secret is disabled, this is the raw `params` array).

### 6.1 `aria2.addUri`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | uris | array[string] | yes | URI list: `http(s)://`, `magnet:`, `ed2k://`, etc.; each non-empty string |
| `1` | options | object | no | Options, e.g. `dir` |

**`result`:** `string` — new task **GID**.

---

### 6.2 `aria2.addTorrent`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | torrent | string / object / array | yes | **Base64** (aria2 standard); or **Node `Buffer` JSON** `{"type":"Buffer","data":[0–255,...]}`; or **numeric byte array** (e.g. `Uint8Array` serialized). Must **not** be a plain task-options object |
| `1` | uris / options | any | no | **Two args:** if **object**, it is `options`; if **array**, it is the URI list. **Three args:** second is the URI list; may be **`[]` or JSON `null`** (common with older aria2 clients). |
| `2` | options | object | no | Third argument in `[torrent, uris, options]`; valid even when `uris` is `null` |

**Note:** Classic three-arg calls include `[torrent, null, options]` or `[torrent, [], options]`. Two-arg `[torrent, options]` is also accepted. For an **existing** task, use `aria2.changeOption(gid, options)`; do not pass an options map as the first argument to `addTorrent`.

**`result`:** `string` — GID.

---

### 6.3 `aria2.remove` / `aria2.forceRemove`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** `string` — GID of the removed task (`forceRemove` may delete files depending on the driver).

---

### 6.4 `aria2.pause` / `aria2.forcePause`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** `string` — GID.

---

### 6.5 `aria2.unpause`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** `string` — GID.

---

### 6.6 `aria2.pauseAll` / `aria2.unpauseAll`

**params:** none beyond optional token.

**`result`:** `string` — `"OK"`.

---

### 6.7 `aria2.removeDownloadResult`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

Deletes **downloaded files** for the task; does not remove the task.

**`result`:** `string` — `"OK"`.

---

### 6.8 `aria2.tellStatus`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |
| `1` | keys | array[string] | no | If omitted/empty, return full status; else only listed keys |

**`result`:** **object** — §3 (including nested `files`, `bittorrent`).

---

### 6.9 `aria2.tellActive`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | keys | array[string] | no | Per-task key filter (same as `tellStatus`) |

**`result`:** **array[object]** — status objects.

---

### 6.10 `aria2.tellWaiting`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | offset | number / string | yes | Offset |
| `1` | num | number / string | yes | Limit |
| `2` | keys | array[string] | no | Field filter |

**`result`:** **array[object]** — paginated `waiting` and `paused` tasks.

---

### 6.11 `aria2.tellStopped`

Same parameters as `tellWaiting`.

**`result`:** **array[object]** — tasks in `complete`, `error`, or `removed` (tasks successfully **`remove`d are usually not present**).

---

### 6.12 `aria2.getFiles`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** **array[object]** — §4.

---

### 6.13 `aria2.getPeers`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** **array[object]**:

| Field | JSON type | Description |
|-------|-----------|-------------|
| `peerId` | string | Peer id |
| `ip` | string | IP |
| `port` | string | Port |
| `bitfield` | string | Bitfield hex |
| `amChoking` | string | `"true"` / `"false"` |
| `peerChoking` | string | `"true"` / `"false"` |
| `downloadSpeed` | string | Download speed |
| `uploadSpeed` | string | Upload speed |
| `seeder` | string | `"true"` / `"false"` |

May be `[]` for protocols without peers.

---

### 6.14 `aria2.getServers`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** **array[object]**:

| Field | Type | Description |
|-------|------|-------------|
| `index` | string | File index |
| `servers` | array | Entries: `uri`, `currentUri`, `downloadSpeed` (string) |

---

### 6.15 `aria2.getUris`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** **array[object]**:

| Field | Type | Description |
|-------|------|-------------|
| `uri` | string | URI |
| `status` | string | e.g. `"used"` |

---

### 6.16 `aria2.getOption`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string | yes | Task GID |

**`result`:** **object** with **string** values — task options plus synthesized keys such as `dir`, `pause` (`true`/`false`), `out` (name).

---

### 6.17 `aria2.changeOption`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | gid | string / number | yes | Task GID (string preferred; JSON integers accepted as decimal gid) |
| `1` | options | object | yes | Non-empty option map |

**`result`:** `string` — GID.

---

### 6.18 `aria2.getGlobalOption`

**params:** none.

**`result`:** **object[string]string** — global options snapshot.

---

### 6.19 `aria2.changeGlobalOption`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | options | object | yes | Non-empty map merged into globals |

**`result`:** **object[string]string** — updated globals.

---

### 6.20 `aria2.getGlobalStat`

**`result`:** **object**:

| Field | Type | Description |
|-------|------|-------------|
| `numActive` | string | Active count |
| `numWaiting` | string | Waiting/paused-related count |
| `numStopped` | string | Stopped-related count |
| `downloadSpeed` | string | Aggregate download speed |
| `uploadSpeed` | string | Aggregate upload speed |

---

### 6.21 `aria2.getVersion`

**`result`:** **object** (representative fields; values change by release):

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Short version |
| `fullVersion` | string | Full version string |
| `releaseDate` | string | Date |
| `organization` | string | Project |
| `copyright` | string | Copyright |
| `enabledFeatures` | array[string] | Features |
| `enabledProtocols` | array[string] | Enabled protocols |
| `supportedProtocols` | array[string] | Supported protocols |

---

### 6.22 `aria2.getSessionInfo`

**`result`:** **object**:

| Field | Type | Description |
|-------|------|-------------|
| `sessionId` | string | Session id |
| `startTime` | string | RFC3339 start time |
| `uptimeSecs` | number | Uptime seconds |
| `aria2Style` | bool | Compatibility flag |
| `server` | string | Server id |
| `generatedAt` | string | RFC3339 |

---

### 6.23 `system.listMethods`

**`result`:** **array[string]** — implemented method names.

---

### 6.24 `system.multicall`

| Index | Name | Type | Required | Description |
|-------|------|------|----------|-------------|
| `0` | batch | array | yes | Each element: |

Batch element:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `methodName` | string | yes | Method name |
| `params` | array | no | Parameters for that method (sub-calls must include `token:` themselves if secret is enabled) |

**`result`:** **array** — each item is a **single-element array** `[ subResult ]` (aria2-style). If any sub-call fails, the whole multicall errors.

---

## 7. Examples

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

## 8. Nested `uris` (under each file)

| Field | Type | Description |
|-------|------|-------------|
| `uri` | string | URI |
| `status` | string | e.g. `"used"` |

---

*This document tracks the implementation in `internal/compat/aria2` and `internal/core/manager`; when in doubt, refer to the source.*
