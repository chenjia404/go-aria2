# 端到端冒烟（PowerShell）：与 scripts/e2e-core.sh 行为对齐。
# 在仓库根目录执行: .\scripts\e2e-core.ps1
# 环境变量: E2E_RPC_PORT, E2E_RPC_SECRET, E2E_SKIP_HTTP, E2E_HTTP_URL, E2E_BIN
# Windows 下守护进程使用 `go run` 启动，避免部分环境对临时 .exe 的 Start-Process 失败。

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $repoRoot

$rpcPort = if ($env:E2E_RPC_PORT) { [int]$env:E2E_RPC_PORT } else { 16880 }
$secret = if ($env:E2E_RPC_SECRET) { $env:E2E_RPC_SECRET } else { "e2e-test-secret" }
$skipHttp = $env:E2E_SKIP_HTTP -eq "1"
$httpUrl = if ($env:E2E_HTTP_URL) { $env:E2E_HTTP_URL } else { "https://www.ietf.org/archive/id/draft-ietf-quic-http-34.txt" }

$goExe = (Get-Command go).Source
$cmdDir = Join-Path $repoRoot "cmd\go-aria2"

$work = Join-Path $env:TEMP ("go-aria2-e2e-w-" + [Guid]::NewGuid().ToString("n"))
New-Item -ItemType Directory -Path (Join-Path $work "downloads") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $work "data") -Force | Out-Null
$dl = Join-Path $work "downloads"
$data = Join-Path $work "data"
$session = Join-Path $data "session.json"
$conf = Join-Path $work "aria2.conf"

$confText = @"
enable-rpc=true
rpc-listen-port=$rpcPort
rpc-listen-all=false
rpc-secret=$secret
enable-websocket=false
dir=$($dl.Replace('\','/'))
data-dir=$($data.Replace('\','/'))
max-concurrent-downloads=2
save-session=$($session.Replace('\','/'))
save-session-interval=0
ed2k-enable=false
listen-port=0
enable-dht=false
"@
Set-Content -Path $conf -Value $confText -Encoding UTF8

$base = "http://127.0.0.1:$rpcPort"
$jsonrpc = "$base/jsonrpc"

if ($null -ne $env:E2E_BIN -and $env:E2E_BIN -ne "" -and (Test-Path -LiteralPath $env:E2E_BIN)) {
    $p = Start-Process -FilePath $env:E2E_BIN -ArgumentList @("daemon", "-conf", $conf) -WorkingDirectory $work -PassThru -WindowStyle Hidden
} else {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $goExe
    $psi.Arguments = "run . daemon -conf `"$conf`""
    $psi.WorkingDirectory = $cmdDir
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $proc = New-Object System.Diagnostics.Process
    $proc.StartInfo = $psi
    [void]$proc.Start()
    $p = $proc
}

try {
    $deadline = (Get-Date).AddSeconds(45)
    $ok = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-WebRequest -Uri "$base/healthz" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200 -and $r.Content -eq "ok") { $ok = $true; break }
        } catch { }
        if ($p.HasExited) {
            $errLog = @()
            if (Test-Path (Join-Path $work "daemon.err")) { $errLog += Get-Content (Join-Path $work "daemon.err") -Raw }
            throw "daemon exited early: $errLog"
        }
        Start-Sleep -Milliseconds 100
    }
    if (-not $ok) {
        $errLog = ""
        if (Test-Path (Join-Path $work "daemon.err")) { $errLog = Get-Content (Join-Path $work "daemon.err") -Raw }
        throw "healthz timeout. stderr: $errLog"
    }

    function Invoke-Rpc($body) {
        return Invoke-RestMethod -Uri $jsonrpc -Method Post -ContentType "application/json" -Body $body
    }

    Write-Host "[e2e] unauthenticated (expect error)"
    $r1 = Invoke-Rpc '{"jsonrpc":"2.0","id":1,"method":"system.listMethods","params":[]}'
    if (-not $r1.error) { throw "expected error for missing token" }

    Write-Host "[e2e] system.listMethods"
    $r2 = Invoke-Rpc "{`"jsonrpc`":`"2.0`",`"id`":2,`"method`":`"system.listMethods`",`"params`":[`"token:$secret`"]}"
    if (-not $r2.result) { throw "listMethods failed: $r2" }

    Write-Host "[e2e] aria2.getVersion / getGlobalStat"
    $r3 = Invoke-Rpc "{`"jsonrpc`":`"2.0`",`"id`":3,`"method`":`"aria2.getVersion`",`"params`":[`"token:$secret`"]}"
    if (-not $r3.result) { throw "getVersion failed" }
    $r4 = Invoke-Rpc "{`"jsonrpc`":`"2.0`",`"id`":4,`"method`":`"aria2.getGlobalStat`",`"params`":[`"token:$secret`"]}"
    if (-not $r4.result) { throw "getGlobalStat failed" }

    Write-Host "[e2e] ctl subprocess"
    if ($env:E2E_BIN) {
        $ctlOut = & $env:E2E_BIN ctl -endpoint $jsonrpc -secret $secret -method aria2.getSessionInfo -params "[]" 2>&1 | Out-String
    } else {
        Push-Location $cmdDir
        try {
            $ctlOut = & $goExe @("run", ".", "ctl", "-endpoint", $jsonrpc, "-secret", $secret, "-method", "aria2.getSessionInfo", "-params", "[]") 2>&1 | Out-String
        } finally {
            Pop-Location
        }
    }
    if ($ctlOut -notmatch "sessionId") { throw "ctl output missing sessionId: $ctlOut" }

    if ($skipHttp) {
        Write-Host "[e2e] E2E_SKIP_HTTP=1 - skip addUri"
    } else {
        Write-Host "[e2e] addUri -> tellStatus -> remove"
        $dlEsc = ($dl -replace '\\', '/')
        $addBody = "{`"jsonrpc`":`"2.0`",`"id`":5,`"method`":`"aria2.addUri`",`"params`":[`"token:$secret`",[`"$httpUrl`"],{`"dir`":`"$dlEsc`"}]}"
        $r5 = Invoke-Rpc $addBody
        if ($r5.error) { throw "addUri: $($r5.error)" }
        $gid = [string]$r5.result
        if (-not $gid) { throw "no gid" }
        Start-Sleep -Milliseconds 500
        $r6 = Invoke-Rpc "{`"jsonrpc`":`"2.0`",`"id`":6,`"method`":`"aria2.tellStatus`",`"params`":[`"token:$secret`",`"$gid`"]}"
        if (-not $r6.result) { throw "tellStatus failed" }
        $r7 = Invoke-Rpc "{`"jsonrpc`":`"2.0`",`"id`":7,`"method`":`"aria2.remove`",`"params`":[`"token:$secret`",`"$gid`"]}"
        if (-not $r7.result) { throw "remove failed" }
    }

    Write-Host "[e2e] all checks passed"
}
finally {
    if ($p -and -not $p.HasExited) {
        if ($env:OS -match "Windows") {
            & taskkill /F /T /PID $p.Id 2>$null | Out-Null
        } else {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        }
        Start-Sleep -Milliseconds 800
    }
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
