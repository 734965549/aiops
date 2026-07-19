# 0032 破坏性迁移前备份关键表（PowerShell 5.1+，依赖 pg_dump）。
# 用法示例：./scripts/backup-pre-migration.ps1 -PgHost 127.0.0.1 -PgPort 5432

param(
    [string]$PgHost = "127.0.0.1",
    [int]$PgPort = 5432,
    [string]$PgDbName = "aiops",
    [string]$PgUser = "aiops",
    [string]$PgPassword = "aiops",
    [string]$BackupDir = "./backups"
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
$env:PGPASSWORD = $PgPassword

# 构建公共连接参数
$connParams = @(
    "--host", $PgHost,
    "--port", $PgPort,
    "--username", $PgUser,
    "--dbname", $PgDbName
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
