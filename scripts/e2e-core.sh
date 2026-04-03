#!/usr/bin/env bash
# 端到端冒烟：拉起 go-aria2 daemon，检查 healthz、JSON-RPC、鉴权与可选 HTTP 任务。
# 用法：在仓库根目录执行 ./scripts/e2e-core.sh
# 环境变量：
#   E2E_RPC_PORT      默认 16880
#   E2E_RPC_SECRET    默认 e2e-test-secret
#   E2E_SKIP_HTTP=1   跳过 addUri/tellStatus/remove
#   E2E_HTTP_URL      默认可访问的小文件 HTTPS URL（用于 HTTP 任务）

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

E2E_RPC_PORT="${E2E_RPC_PORT:-16880}"
E2E_RPC_SECRET="${E2E_RPC_SECRET:-e2e-test-secret}"
E2E_HTTP_URL="${E2E_HTTP_URL:-https://www.ietf.org/archive/id/draft-ietf-quic-http-34.txt}"
SKIP_HTTP="${E2E_SKIP_HTTP:-0}"

BIN="${E2E_BIN:-}"
TMPBIN=""
if [[ -z "$BIN" ]]; then
  TMPBIN="$(mktemp -d)"
  echo "[e2e] building go-aria2 -> $TMPBIN/go-aria2"
  go build -o "$TMPBIN/go-aria2" ./cmd/go-aria2
  BIN="$TMPBIN/go-aria2"
fi

WORKDIR="$(mktemp -d)"
cleanup() {
  if [[ -n "${DAEMON_PID:-}" ]] && kill -0 "$DAEMON_PID" 2>/dev/null; then
    kill "$DAEMON_PID" 2>/dev/null || true
    wait "$DAEMON_PID" 2>/dev/null || true
  fi
  rm -rf "$WORKDIR"
  if [[ -n "$TMPBIN" ]]; then
    rm -rf "$TMPBIN"
  fi
}
trap cleanup EXIT

DL="$WORKDIR/downloads"
DATA="$WORKDIR/data"
mkdir -p "$DL" "$DATA"

CONF="$WORKDIR/aria2.conf"
SESSION="$DATA/session.json"
cat > "$CONF" <<EOF
enable-rpc=true
rpc-listen-port=${E2E_RPC_PORT}
rpc-listen-all=false
rpc-secret=${E2E_RPC_SECRET}
enable-websocket=false
dir=${DL}
data-dir=${DATA}
max-concurrent-downloads=2
save-session=${SESSION}
save-session-interval=0
ed2k-enable=false
listen-port=0
enable-dht=false
EOF

BASE_URL="http://127.0.0.1:${E2E_RPC_PORT}"
JSONRPC_URL="${BASE_URL}/jsonrpc"
HEALTH_URL="${BASE_URL}/healthz"

rpc_post() {
  local body="$1"
  curl -sS --connect-timeout 2 --max-time 30 -X POST "$JSONRPC_URL" \
    -H 'Content-Type: application/json' \
    -d "$body"
}

json_has_result() {
  local json="$1"
  if command -v jq >/dev/null 2>&1; then
    echo "$json" | jq -e '.result != null' >/dev/null
  else
    echo "$json" | grep -q '"result"'
  fi
}

json_has_error() {
  local json="$1"
  if command -v jq >/dev/null 2>&1; then
    echo "$json" | jq -e '.error != null' >/dev/null
  else
    echo "$json" | grep -q '"error"'
  fi
}

extract_gid() {
  local json="$1"
  if command -v jq >/dev/null 2>&1; then
    echo "$json" | jq -r '.result'
  elif command -v python3 >/dev/null 2>&1; then
    echo "$json" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("result") or "")'
  else
    echo "$json" | sed -n 's/.*"result"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1
  fi
}

build_add_uri_json() {
  local url="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -n \
      --arg tok "token:${E2E_RPC_SECRET}" \
      --arg u "$url" \
      --arg d "$DL" \
      '{jsonrpc:"2.0", id:5, method:"aria2.addUri", params:[$tok,[$u],{dir:$d}]}'
  elif command -v python3 >/dev/null 2>&1; then
    E2E_RPC_SECRET="$E2E_RPC_SECRET" E2E_HTTP_URL="$url" E2E_DL="$DL" python3 -c '
import json, os
print(json.dumps({
  "jsonrpc": "2.0",
  "id": 5,
  "method": "aria2.addUri",
  "params": ["token:" + os.environ["E2E_RPC_SECRET"], [os.environ["E2E_HTTP_URL"]], {"dir": os.environ["E2E_DL"]}],
}))
'
  else
    printf '{"jsonrpc":"2.0","id":5,"method":"aria2.addUri","params":["token:%s",["%s"],{"dir":"%s"}]}' \
      "${E2E_RPC_SECRET}" "$url" "$DL"
  fi
}

echo "[e2e] starting daemon (port ${E2E_RPC_PORT})..."
(
  cd "$WORKDIR"
  "$BIN" daemon -conf "$CONF" >>"$WORKDIR/daemon.log" 2>&1 &
  echo $!
) >"$WORKDIR/pidfile"
DAEMON_PID="$(cat "$WORKDIR/pidfile")"

for i in $(seq 1 50); do
  if curl -sf --connect-timeout 1 --max-time 2 "$HEALTH_URL" | grep -q '^ok$'; then
    echo "[e2e] healthz ok"
    break
  fi
  if ! kill -0 "$DAEMON_PID" 2>/dev/null; then
    echo "[e2e] daemon exited early. log:" >&2
    cat "$WORKDIR/daemon.log" >&2 || true
    exit 1
  fi
  if [[ "$i" -eq 50 ]]; then
    echo "[e2e] timeout waiting for healthz" >&2
    cat "$WORKDIR/daemon.log" >&2 || true
    exit 1
  fi
  sleep 0.1
done

echo "[e2e] system.listMethods (no token, expect error when secret set)"
RESP="$(rpc_post '{"jsonrpc":"2.0","id":1,"method":"system.listMethods","params":[]}')"
if ! json_has_error "$RESP"; then
  echo "$RESP" >&2
  echo "[e2e] expected error for unauthenticated request" >&2
  exit 1
fi

echo "[e2e] system.listMethods (with token)"
RESP="$(rpc_post "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"system.listMethods\",\"params\":[\"token:${E2E_RPC_SECRET}\"]}")"
if ! json_has_result "$RESP"; then
  echo "$RESP" >&2
  exit 1
fi

echo "[e2e] aria2.getVersion"
RESP="$(rpc_post "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"aria2.getVersion\",\"params\":[\"token:${E2E_RPC_SECRET}\"]}")"
if ! json_has_result "$RESP"; then
  echo "$RESP" >&2
  exit 1
fi

echo "[e2e] aria2.getGlobalStat"
RESP="$(rpc_post "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"aria2.getGlobalStat\",\"params\":[\"token:${E2E_RPC_SECRET}\"]}")"
if ! json_has_result "$RESP"; then
  echo "$RESP" >&2
  exit 1
fi

echo "[e2e] ctl subprocess"
if ! "$BIN" ctl -endpoint "$JSONRPC_URL" -secret "$E2E_RPC_SECRET" -method aria2.getSessionInfo -params '[]' | grep -q sessionId; then
  echo "[e2e] ctl output missing sessionId" >&2
  exit 1
fi

if [[ "$SKIP_HTTP" == "1" ]]; then
  echo "[e2e] E2E_SKIP_HTTP=1 — skipping HTTP addUri flow"
  echo "[e2e] all checks passed"
  exit 0
fi

echo "[e2e] aria2.addUri -> tellStatus -> remove (url: ${E2E_HTTP_URL})"
ADD_BODY="$(build_add_uri_json "$E2E_HTTP_URL")"
RESP="$(rpc_post "$ADD_BODY")"
if ! json_has_result "$RESP"; then
  echo "$RESP" >&2
  echo "[e2e] addUri failed (network or URL? set E2E_SKIP_HTTP=1 to skip)" >&2
  exit 1
fi
GID="$(extract_gid "$RESP")"
if [[ -z "$GID" || "$GID" == "null" ]]; then
  echo "$RESP" >&2
  echo "[e2e] could not parse gid" >&2
  exit 1
fi
echo "[e2e] gid=$GID"

sleep 0.5
RESP="$(rpc_post "{\"jsonrpc\":\"2.0\",\"id\":6,\"method\":\"aria2.tellStatus\",\"params\":[\"token:${E2E_RPC_SECRET}\",\"${GID}\"]}")"
if ! json_has_result "$RESP"; then
  echo "$RESP" >&2
  exit 1
fi

RESP="$(rpc_post "{\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"aria2.remove\",\"params\":[\"token:${E2E_RPC_SECRET}\",\"${GID}\"]}")"
if ! json_has_result "$RESP"; then
  echo "$RESP" >&2
  exit 1
fi

echo "[e2e] all checks passed"
