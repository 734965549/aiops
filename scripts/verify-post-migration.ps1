<#
.SYNOPSIS
    在 0032 破坏性迁移应用后，对数据库完整性进行验证的 PowerShell 脚本。

.DESCRIPTION
    本脚本用于在应用 0032（及相关后续）迁移之后，验证数据完整性是否满足预期。
    所有检查项通过 psql 命令行执行 SQL，结果以 PASS/FAIL 形式输出。

    检查项：
      a. schema_migrations 最新版本是否为 0043。
      b. SELECT count(*) FROM v_asset_app_ref_integrity 应返回 0
         （该视图汇总资产应用引用完整性异常，0 表示无异常）。
      c. 检查 asset_application 中是否还存在旧格式 cloud-<account_id>
         （以 cloud- 开头但不包含第二个 '-' 的 ID，说明未被 0032 改写）。
      d. 检查 alert_alert 中是否有 application_id 不为空但不存在于
         asset_application 的记录（悬空引用）。
      e. 检查 inspection_policy.scope->'application_ids' 中是否有不存在于
         asset_application 的元素（JSON 数组悬空引用）。

    全部通过输出 "ALL CHECKS PASSED" 并返回 0；
    任意失败输出 "MIGRATION VERIFICATION FAILED" 并返回 1。

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

.EXAMPLE
    .\scripts\verify-post-migration.ps1
    使用默认参数执行验证。

.EXAMPLE
    .\scripts\verify-post-migration.ps1 -Host 127.0.0.1 -Port 5432 -DbName aiops -User aiops -Password aiops
    指定连接参数执行验证。

.NOTES
    依赖：psql（PostgreSQL 客户端工具）。
    兼容：Windows PowerShell 5.1。
#>

param(
    [string]$Host = "127.0.0.1",
    [int]$Port = 5432,
    [string]$DbName = "aiops",
    [string]$User = "aiops",
    [string]$Password = "aiops"
)

$ErrorActionPreference = "Stop"

# -----------------------------------------------------------------------------
# 工具函数
# -----------------------------------------------------------------------------

function Write-Step {
    param([string]$Message)
    Write-Host ("==> " + $Message)
}

function Write-Pass {
    param([string]$Message)
    Write-Host ("    [PASS] " + $Message) -ForegroundColor Green
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

# 调用 psql 执行单条 SQL，返回标量结果（tuples only, 无对齐，无字段头）。
function Invoke-SqlScalar {
    param([string]$Sql)
    $env:PGPASSWORD = $Password
    $args = @(
        "--host", $Host,
        "--port", $Port,
        "--username", $User,
        "--dbname", $DbName,
        "--no-align",
        "--tuples-only",
        "--no-psqlrc",
        "--quiet",
        "--command", $Sql
    )
    $result = & psql @args
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        $env:PGPASSWORD = $null
        throw ("psql execution failed (exit=" + $exitCode + ") for SQL: " + $Sql)
    }
    $env:PGPASSWORD = $null
    if ($null -eq $result) { return "" }
    return ($result -join " ").Trim()
}

# -----------------------------------------------------------------------------
# 前置检查
# -----------------------------------------------------------------------------

Write-Step "checking prerequisites"

if (-not (Test-Command "psql")) {
    Write-Fail "psql not found in PATH. Please install PostgreSQL client tools and ensure psql is available."
    exit 1
}

$psqlVersion = & psql --version 2>&1
Write-Host ("    psql available: " + $psqlVersion)

# 统计失败数
$failureCount = 0

# -----------------------------------------------------------------------------
# 检查 a: schema_migrations 最新版本是否为 0043
# -----------------------------------------------------------------------------

Write-Step "check (a): schema_migrations latest version == 0043"

$sqlA = "SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1;"
try {
    $latestVersion = Invoke-SqlScalar -Sql $sqlA
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $latestVersion = $null
}

if ($null -ne $latestVersion) {
    if ($latestVersion -eq "0043") {
        Write-Pass ("latest migration version is 0043 (got: " + $latestVersion + ")")
    } else {
        Write-Fail ("expected 0043 but got: " + $latestVersion)
        $failureCount++
    }
}

# -----------------------------------------------------------------------------
# 检查 b: v_asset_app_ref_integrity count == 0
# -----------------------------------------------------------------------------

Write-Step "check (b): v_asset_app_ref_integrity count == 0"

$sqlB = "SELECT count(*) FROM v_asset_app_ref_integrity;"
try {
    $countB = Invoke-SqlScalar -Sql $sqlB
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $countB = $null
}

if ($null -ne $countB) {
    if ($countB -eq "0") {
        Write-Pass ("v_asset_app_ref_integrity returned 0 rows (count=" + $countB + ")")
    } else {
        Write-Fail ("v_asset_app_ref_integrity returned " + $countB + " rows, expected 0")
        $failureCount++
    }
}

# -----------------------------------------------------------------------------
# 检查 c: asset_application 中是否还有旧格式 cloud-<account_id>
#        旧格式定义：以 'cloud-' 开头，但去掉 'cloud-' 前缀后不含 '-'。
#        即 application_id ~ '^cloud-' 且 application_id !~ '^cloud-.*-.*'
# -----------------------------------------------------------------------------

Write-Step "check (c): no legacy cloud-<account_id> application ids in asset_application"

$sqlC = @"
SELECT count(*) FROM asset_application
WHERE application_id LIKE 'cloud-%'
  AND application_id NOT LIKE 'cloud-%-%';
"@
try {
    $countC = Invoke-SqlScalar -Sql $sqlC
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $countC = $null
}

if ($null -ne $countC) {
    if ($countC -eq "0") {
        Write-Pass ("no legacy cloud-<account_id> application ids found (count=" + $countC + ")")
    } else {
        Write-Fail ("found " + $countC + " legacy cloud-<account_id> application ids, expected 0")
        $failureCount++
    }
}

# -----------------------------------------------------------------------------
# 检查 d: alert_alert 中是否有 application_id 不为空但不存在于 asset_application
# -----------------------------------------------------------------------------

Write-Step "check (d): no dangling application_id references in alert_alert"

$sqlD = @"
SELECT count(*) FROM alert_alert a
WHERE a.application_id IS NOT NULL
  AND a.application_id <> ''
  AND NOT EXISTS (
      SELECT 1 FROM asset_application app WHERE app.application_id = a.application_id
  );
"@
try {
    $countD = Invoke-SqlScalar -Sql $sqlD
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $countD = $null
}

if ($null -ne $countD) {
    if ($countD -eq "0") {
        Write-Pass ("no dangling application_id in alert_alert (count=" + $countD + ")")
    } else {
        Write-Fail ("found " + $countD + " dangling application_id references in alert_alert, expected 0")
        $failureCount++
    }
}

# -----------------------------------------------------------------------------
# 检查 e: inspection_policy.scope->'application_ids' 中是否有不存在于 asset_application 的元素
# -----------------------------------------------------------------------------

Write-Step "check (e): no dangling application_ids in inspection_policy.scope"

$sqlE = @"
SELECT count(*) FROM inspection_policy p,
     jsonb_array_elements_text(COALESCE(p.scope->'application_ids', '[]'::jsonb)) AS elem(app_id)
WHERE NOT EXISTS (
    SELECT 1 FROM asset_application app WHERE app.application_id = elem.app_id
);
"@
try {
    $countE = Invoke-SqlScalar -Sql $sqlE
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $countE = $null
}

if ($null -ne $countE) {
    if ($countE -eq "0") {
        Write-Pass ("no dangling application_ids in inspection_policy.scope (count=" + $countE + ")")
    } else {
        Write-Fail ("found " + $countE + " dangling application_ids in inspection_policy.scope, expected 0")
        $failureCount++
    }
}

# -----------------------------------------------------------------------------
# 汇总
# -----------------------------------------------------------------------------

Write-Step "verification summary"

if ($failureCount -eq 0) {
    Write-Host ""
    Write-Host "ALL CHECKS PASSED" -ForegroundColor Green
    $env:PGPASSWORD = $null
    exit 0
} else {
    Write-Host ""
    Write-Host ("MIGRATION VERIFICATION FAILED (" + $failureCount + " check(s) failed)") -ForegroundColor Red
    $env:PGPASSWORD = $null
    exit 1
}
