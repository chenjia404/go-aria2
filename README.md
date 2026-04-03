# go-aria2

Go module path: `github.com/chenjia404/go-aria2` (clone: <https://github.com/chenjia404/go-aria2>).

`go-aria2` is a multi-protocol download engine written in Go. It aims to support common `aria2`-style JSON-RPC APIs through a layered design and gradually replace the original `aria2` daemon.

Current focus:

- BitTorrent
- ED2K
- HTTP / HTTPS
- aria2-style configuration files
- Common aria2 JSON-RPC APIs
- Session persistence and recovery after restart
- Importing incomplete tasks from aria2 `save-session` files

## Features

- Domain model separated from protocol compatibility; aria2 compatibility does not pollute the core task model.
- Supports `rpc-secret` and `token:xxx` authentication.
- Supports preserving `gid` so existing RPC identifiers keep working after migration.
- Supports BT progress reconstruction and reuse of existing files when migrating.
- Supports periodic `save-session` writes so tasks resume after restart.

## Advantages over aria2

Compared to the classic `aria2` daemon, `go-aria2` aims to offer:

- **BitTorrent v2** — modern v2 torrents (hybrid / v2-only) are supported alongside v1, whereas many aria2 deployments are effectively v1-oriented.
- **ED2K (eDonkey2000)** — built-in ED2K support via a real driver layer; aria2 does not cover this protocol.
- **No artificial thread cap** — concurrency is modeled around goroutines and the Go scheduler, not a fixed worker-thread pool size, so you are not boxed in by a hard thread limit in the same way as typical native download daemons.
- **Active maintenance** — the project is under continuous development with regular dependency and feature updates, which helps for long-running deployments.

These points complement (not replace) aria2’s strengths; see [Known limitations](#known-limitations) for gaps in parity.

## Layout

```text
/cmd/go-aria2       unified entrypoint
/internal/core      core domain layer
/internal/protocol  protocol drivers
/internal/compat    aria2 compatibility layer
/internal/config    aria2.conf-style parsing
/internal/rpc       JSON-RPC / WebSocket server
/internal/migrate   aria2 migration logic
/docs               migration docs
```

## Quick start

### 1. Run the daemon

By default reads `aria2.conf` in the current directory:

```bash
go run ./cmd/go-aria2 daemon
```

Specify a config file:

```bash
go run ./cmd/go-aria2 daemon -conf /path/to/aria2.conf
```

Daemon flags support common aria2-style aliases, for example:

```bash
go run ./cmd/go-aria2 -d /downloads -j 4 --rpc-secret secret --enable-rpc
```

You can also pass URIs or `-i` input files like `aria2c`:

```bash
go run ./cmd/go-aria2 https://example.com/file.torrent
go run ./cmd/go-aria2 -i ./input.txt
```

`-i/--input-file` has two behaviors:

- `-i *.txt` or any non-`.json` file: parsed as an aria2-style `input-file` text task list
- `-i *.json`: treated as go-aria2’s own `session.json` and loaded for session recovery

Examples:

```bash
go run ./cmd/go-aria2 -i ./input.txt
go run ./cmd/go-aria2 -i ./data/session.json
```

Do not use go-aria2’s `session.json` as aria2’s plain-text `input-file`. The former is an internal JSON session file; the latter is a plain-text task list with a different format.

### 2. Call JSON-RPC

Use the debug CLI:

```bash
go run ./cmd/go-aria2 ctl -method system.listMethods
go run ./cmd/go-aria2 ctl -method aria2.getGlobalStat
go run ./cmd/go-aria2 ctl -secret your-secret -method aria2.tellActive
```

`ctl` also accepts `--rpc-secret`, aligned with common aria2 flag names.

Custom parameters:

```bash
go run ./cmd/go-aria2 ctl \
  -method aria2.addUri \
  -params '[[\"https://example.com/file.torrent\"],{\"dir\":\"/downloads\"}]'
```

**If RPC fails, check:** the process finished initialization (if BT/ED2K init fails it exits with no RPC); `enable-rpc=true`; client URL is `http://<host>:<port>/jsonrpc` (path must be `/jsonrpc`); port matches `rpc-listen-port`. With default **`rpc-listen-all=false`**, only **127.0.0.1** is listened on—access from the Docker host or other LAN machines will fail; use **`rpc-listen-all=true`** and open the firewall. With **`rpc-secret`**, the first parameter must be **`token:<secret>`** (`ctl` can add it via `-secret`). You can probe with `curl http://127.0.0.1:<port>/healthz` and expect `ok`.

## Build

### Native build

Linux / macOS:

```bash
go build -trimpath -o go-aria2 ./cmd/go-aria2
```

Windows PowerShell:

```powershell
go build -trimpath -o .\go-aria2.exe .\cmd\go-aria2
```

### Cross-compile for Windows

Prefer a pure Go binary with **`CGO` disabled** to avoid runtime DLLs such as `libgcc_s_seh-1.dll`, and for easier cross-compilation from WSL or Linux.

64-bit Windows:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o go-aria2.exe ./cmd/go-aria2
```

32-bit Windows:

```bash
GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -trimpath -o go-aria2.exe ./cmd/go-aria2
```

Windows PowerShell:

```powershell
$env:GOOS="windows"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -trimpath -o .\go-aria2.exe .\cmd\go-aria2
```

### WSL cross-compile notes

When building in WSL for Windows, keep:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0
```

If you omit `CGO_ENABLED=0`, Go may use external linking and produce a Windows binary that needs extra DLLs. Under PowerShell or `Start-Process` you may see:

```text
The specified executable is not a valid application for this OS platform.
```

Common causes:

- The binary depends on external runtime DLLs missing on the target machine
- Architecture mismatch, e.g. running an `amd64` build on 32-bit Windows

Check OS bitness in PowerShell:

```powershell
[Environment]::Is64BitOperatingSystem
```

If `False`, rebuild with `GOARCH=386`.

## Migrate aria2 tasks

Import tasks from aria2’s `save-session` file:

```bash
go run ./cmd/go-aria2 migrate-from-aria2 --session /path/to/session.txt
```

Options:

- `--conf`: path to aria2-style config (default `aria2.conf`)
- `--session`: path to aria2 `save-session` file (required)
- `--strict`: enable strict BT recovery mode

See [docs/migrate-from-aria2.md](docs/migrate-from-aria2.md).

If you are replacing an existing `aria2` daemon with this program, read the full “stop old service → backup → import old save-session → start go-aria2” flow in [docs/migrate-from-aria2.md](docs/migrate-from-aria2.md).

## Configuration

Config uses `key=value` lines, with blank lines and `#` comments allowed.

Example:

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

### Main options

- RPC
  - `enable-rpc`
  - `rpc-listen-port`
  - `rpc-listen-all`
  - `rpc-allow-origin-all`
  - `rpc-max-request-size`
  - `rpc-secret`
  - `enable-websocket`
- Directories and scheduling
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

Unknown keys do not abort startup; they are logged as warnings.

## JSON-RPC coverage

Phase-one and migration-related methods are implemented, including:

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

Conventions:

- `gid` is a 16-character hex string
- Numeric fields in `tellStatus` are serialized as strings
- `token:xxx` auth is supported
- WebSocket notifications honor `rpc-secret` via `?token=...`, `Authorization: token:...`, or `X-Auth-Token`

## Protocol support

### BitTorrent

- Magnet links
- `.torrent` files / `.torrent` URLs
- Session recovery and progress reconstruction
- Migration can scan local files and rebuild piece completion

### ED2K

- `ed2k://` links
- Real driver layer wired up
- Migration preserves task metadata; recovery can be extended further

### HTTP / HTTPS

- Plain downloads
- Range resume
- Split/concurrent segments
- Per-task option overrides

## Task persistence

Task state is written to a session file and restored on startup.

Default session path comes from `save-session`; if unset, it falls under `data-dir`.

## Known limitations

- Not a full aria2 replacement; compatibility is still growing.
- Some niche aria2 RPC methods are not implemented yet.
- Strict BT recovery needs torrent metadata to be available.
- Full ED2K recovery is still being extended.

## Development

Run tests:

```bash
go test ./...
```

End-to-end smoke (requires `go` locally; Bash script also needs `curl`):

- Bash (Git Bash / Linux / macOS): `./scripts/e2e-core.sh`
- Windows PowerShell: `.\scripts\e2e-core.ps1`

Environment variables: `E2E_RPC_PORT` (default `16880`), `E2E_RPC_SECRET`, `E2E_BIN` (prebuilt binary to skip in-script `go build`), `E2E_SKIP_HTTP=1` (skip public `addUri`), `E2E_HTTP_URL` (custom HTTP test URL).

Process-level integration tests (no `curl`; local HTTP via `httptest`):

```bash
go test -tags=integration -timeout 5m ./internal/app/...
```

## License

This repository does not declare a license explicitly. Add a license file before a public release.
