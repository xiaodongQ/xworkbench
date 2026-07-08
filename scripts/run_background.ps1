# run_background.ps1 - Windows 后台运行脚本（不使用 Windows Service）
# 用法：
#   .\run_background.ps1        # 启动（后台运行，可关闭终端）
#   .\run_background.ps1 -stop  # 停止
#   .\run_background.ps1 -status # 查看状态

param(
    [string]$Action = "start"
)

$ErrorActionPreference = "Stop"
$PROJECT_ROOT = Split-Path $PSScriptRoot -Parent
$BIN = Join-Path $PROJECT_ROOT "bin\xworkbench-windows-amd64.exe"
$LOG_DIR = Join-Path $PROJECT_ROOT "data\logs"
$LOG_FILE = Join-Path $LOG_DIR "xworkbench.log"
$PID_FILE = Join-Path $PROJECT_ROOT "data\xworkbench.pid"
$ADDR = if ($env:ADDR) { $env:ADDR } else { ":8902" }
$DB_PATH = if ($env:DB_PATH) { $env:DB_PATH } else { Join-Path $PROJECT_ROOT "data\xworkbench.db" }
$CONFIG_PATH = if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { Join-Path $PROJECT_ROOT "data\config.json" }

function Get-XworkbenchPid {
    $port = $ADDR -replace '^:', ''
    $conn = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
    if ($conn) {
        $proc = Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
        if ($proc -and $proc.ProcessName -like "*xworkbench*") {
            return $conn.OwningProcess
        }
    }
    return $null
}

function Start-Server {
    if (-not (Test-Path $BIN)) {
        Write-Host "ERROR: Binary not found: $BIN" -ForegroundColor Red
        Write-Host "  Run: .\scripts\build.sh" -ForegroundColor Cyan
        exit 1
    }

    $existing = Get-XworkbenchPid
    if ($existing) {
        Write-Host "Port $ADDR already in use by PID $existing, stopping first..." -ForegroundColor Yellow
        Stop-Server
        Start-Sleep 1
    }

    if (-not (Test-Path $LOG_DIR)) {
        New-Item -ItemType Directory -Path $LOG_DIR -Force | Out-Null
    }

    Write-Host "Starting xworkbench in background..." -ForegroundColor Cyan
    Write-Host "  Binary: $BIN"
    Write-Host "  Log:    $LOG_FILE"
    Write-Host "  Port:   $ADDR"

    # 通过 -addr 命令行参数传递端口（不依赖环境变量，行为更明确）
    $proc = Start-Process -FilePath $BIN `
        -ArgumentList "-config", "`"$CONFIG_PATH`"", "-addr", $ADDR `
        -NoNewWindow `
        -PassThru `
        -RedirectStandardOutput $LOG_FILE `
        -RedirectStandardError "$LOG_DIR\xworkbench.err.log"

    # Wait briefly and check if started
    Start-Sleep 2
    $started = Get-XworkbenchPid
    if ($started) {
        Write-Host "OK: Started (PID $started)" -ForegroundColor Green
        Write-Host "  Browser: http://localhost$ADDR" -ForegroundColor Cyan
        Write-Host "  Log:     $LOG_FILE" -ForegroundColor Cyan
    } else {
        Write-Host "ERROR: Process may have exited. Check log:" -ForegroundColor Red
        Write-Host "  $LOG_FILE" -ForegroundColor Cyan
        if (Test-Path "$LOG_DIR\xworkbench.err.log") {
            Get-Content "$LOG_DIR\xworkbench.err.log" | Select-Object -Last 10
        }
        exit 1
    }
}

function Stop-Server {
    $pid = Get-XworkbenchPid
    if (-not $pid) {
        Write-Host "xworkbench is not running" -ForegroundColor Yellow
        return
    }

    Write-Host "Stopping xworkbench (PID $pid)..." -ForegroundColor Yellow
    Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
    Start-Sleep 1

    $remaining = Get-XworkbenchPid
    if ($remaining) {
        Write-Host "WARNING: Process still running (PID $remaining), trying SIGKILL..." -ForegroundColor Yellow
        Stop-Process -Id $remaining -Force -ErrorAction SilentlyContinue
        Start-Sleep 1
    }

    $final = Get-XworkbenchPid
    if ($final) {
        Write-Host "ERROR: Failed to stop (PID $final still alive)" -ForegroundColor Red
    } else {
        Write-Host "OK: Stopped" -ForegroundColor Green
    }
}

function Get-ServerStatus {
    $pid = Get-XworkbenchPid
    if ($pid) {
        Write-Host "Running: PID $pid" -ForegroundColor Green
        Write-Host "  Port: $ADDR"
        Write-Host "  Binary: $BIN"
    } else {
        Write-Host "Not running" -ForegroundColor Yellow
        Write-Host "  Port: $ADDR"
        Write-Host "  Binary: $BIN"
    }
}

switch ($Action.ToLower()) {
    "start"  { Start-Server }
    "stop"   { Stop-Server }
    "status" { Get-ServerStatus }
    default  {
        Write-Host "Usage: .\run_background.ps1 [-Action start|stop|status]" -ForegroundColor Yellow
        exit 1
    }
}
