<#
.SYNOPSIS
    在执行 0032 破坏性迁移前备份关键表的 PowerShell 脚本。

.DESCRIPTION
    本脚本用于在应用 0032 破坏性数据库迁移之前，对涉及的关键业务表进行
    数据级备份（--data-only），同时再做一次完整数据库的 custom 格式备份，
    以便在迁移失败或数据异常时可以快速回滚。

    涉及的关键表：
      - asset_application   资产应用表（0032 主要改写对象）
      - asset_resource      资产资源表
      - asset_match_rule    资产匹配规则表
      - alert_alert         告警表（application_id 引用资产应用）
      - inspection_policy   巡检策略表（scope 中引用 application_ids）

    备份策略：
      1. 对上述每张表执行 pg_dump --data-only --table=<table>，输出纯文本 SQL。
      2. 对整个数据库执行 pg_dump --format=custom，输出自定义压缩格式文件。
      3. 备份文件名包含时间戳，避免覆盖。
      4. 任何步骤失败立即退出（非零退出码）。

.PARAMETER Host
    PostgreSQL 主机地址，默认 127.0.0.1。

.PARAMETER Port
    PostgreSQL 端口，默认 5432。

.PARAMETER DbName
    目标数据库名，默认 aiops。

.PARAMETER User
    数据库用户名，默认 aiops。

.PARAMETER Password
    数据库密码，默认 aiops。

.PARAMETER BackupDir
    备份文件输出目录，默认当前目录下的 backups 子目录。

.EXAMPLE
    .\scripts\backup-pre-migration.ps1
    使用默认参数执行备份。

.EXAMPLE
    .\scripts\backup-pre-migration.ps1 -Host 127.0.0.1 -Port 5432 -DbName aiops -User aiops -Password aiops -BackupDir D:\backups\aiops
    指定全部参数执行备份。

.NOTES
    依赖：pg_dump（PostgreSQL 客户端工具）。
    兼容：Windows PowerShell 5.1。
#>

param(
    [string]$Host = "127.0.0.1",
    [int]$Port = 5432,
    [string]$DbName = "aiops",
    [string]$User = "aiops",
    [string]$Password = "aiops",
    [string]$BackupDir = ".\backups"
)

$ErrorActionPreference = "Stop"

# 表 pg_dump --data-only 备份的目标表清单。
$TargetTables = @(
    "asset_application",
    "asset_resource",
    "asset_match_rule",
    "alert_alert",
    "inspection_policy"
)

# -----------------------------------------------------------------------------
# 工具函数
# -----------------------------------------------------------------------------

function Write-Step {
    param([string]$Message)
    Write-Host ("==> " + $Message)
}

function Write-Ok {
    param([string]$Message)
    Write-Host ("    [OK] " + $Message) -ForegroundColor Green
}

function Write-Fail {
    param([string]$Message)
    Write-Host ("    [FAIL] " + $Message) -ForegroundColor Red
}

function Test-Command {
    param([string]$Name)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    return $null -ne $cmd
}

function Format-FileSize {
    param([long]$Bytes)
    if ($Bytes -ge 1GB) { return ("{0:N2} GB" -f ($Bytes / 1GB)) }
    if ($Bytes -ge 1MB) { return ("{0:N2} MB" -f ($Bytes / 1MB)) }
    if ($Bytes -ge 1KB) { return ("{0:N2} KB" -f ($Bytes / 1KB)) }
    return ("{0} B" -f $Bytes)
}

# -----------------------------------------------------------------------------
# 前置检查
# -----------------------------------------------------------------------------

Write-Step "checking prerequisites"

if (-not (Test-Command "pg_dump")) {
    Write-Fail "pg_dump not found in PATH. Please install PostgreSQL client tools and ensure pg_dump is available."
    exit 1
}

$pgDumpVersion = & pg_dump --version 2>&1
Write-Ok ("pg_dump available: " + $pgDumpVersion)

# 创建备份目录
if (-not (Test-Path $BackupDir)) {
    New-Item -ItemType Directory -Path $BackupDir -Force | Out-Null
    Write-Ok ("created backup dir: " + (Resolve-Path $BackupDir).Path)
} else {
    Write-Ok ("backup dir exists: " + (Resolve-Path $BackupDir).Path)
}

# 设置密码环境变量
$env:PGPASSWORD = $Password

# 构建公共连接参数
$connParams = @(
    "--host", $Host,
    "--port", $Port,
    "--username", $User,
    "--dbname", $DbName
)

# 生成时间戳
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"

# -----------------------------------------------------------------------------
# 表级 data-only 备份
# -----------------------------------------------------------------------------

Write-Step "starting per-table data-only backups"

foreach ($table in $TargetTables) {
    $backupFile = Join-Path $BackupDir ("aiops_pre_0032_{0}_{1}.sql" -f $table, $timestamp)
    Write-Host ("    backing up table: " + $table)

    $tableParams = @("--data-only", "--table=" + $table) + $connParams

    & pg_dump @tableParams -f $backupFile
    $exitCode = $LASTEXITCODE

    if ($exitCode -ne 0 -or -not (Test-Path $backupFile)) {
        Write-Fail ("pg_dump failed for table " + $table + " (exit=" + $exitCode + ")")
        $env:PGPASSWORD = $null
        exit 1
    }

    $fileInfo = Get-Item $backupFile
    Write-Ok ("table " + $table + " -> " + $fileInfo.FullName + " (" + (Format-FileSize $fileInfo.Length) + ")")
}

# -----------------------------------------------------------------------------
# 完整数据库 custom 格式备份
# -----------------------------------------------------------------------------

Write-Step "starting full database custom-format backup"

$fullBackupFile = Join-Path $BackupDir ("aiops_pre_0032_full_{0}.dump" -f $timestamp)
$fullParams = @("--format=custom") + $connParams

& pg_dump @fullParams -f $fullBackupFile
$exitCode = $LASTEXITCODE

if ($exitCode -ne 0 -or -not (Test-Path $fullBackupFile)) {
    Write-Fail ("full database pg_dump failed (exit=" + $exitCode + ")")
    $env:PGPASSWORD = $null
    exit 1
}

$fullInfo = Get-Item $fullBackupFile
Write-Ok ("full db -> " + $fullInfo.FullName + " (" + (Format-FileSize $fullInfo.Length) + ")")

# -----------------------------------------------------------------------------
# 清理与汇总
# -----------------------------------------------------------------------------

$env:PGPASSWORD = $null

Write-Step "backup summary"
Write-Host ("    timestamp : " + $timestamp)
Write-Host ("    backup dir: " + (Resolve-Path $BackupDir).Path)
Write-Host ("    table dumps: " + $TargetTables.Count)
Write-Host ("    full dump  : " + $fullBackupFile)

Write-Ok "pre-migration backup completed successfully"
exit 0
