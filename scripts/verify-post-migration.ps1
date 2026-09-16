# 迁移后完整性验证（PowerShell 5.1+，依赖 psql）。默认期望最新迁移 0045。
# 用法示例：./scripts/verify-post-migration.ps1 -ExpectedMigrationVersion 0045

param(
    [string]$PgHost = "127.0.0.1",
    [int]$PgPort = 5432,
    [string]$PgDbName = "aiops",
    [string]$PgUser = "aiops",
    [string]$PgPassword = "aiops",
    [string]$ExpectedMigrationVersion = "0045"
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
    $env:PGPASSWORD = $PgPassword
    $psqlArgs = @(
        "--host", $PgHost,
        "--port", $PgPort,
        "--username", $PgUser,
        "--dbname", $PgDbName,
        "--no-align",
        "--tuples-only",
        "--no-psqlrc",
        "--quiet",
        "--command", $Sql
    )
    $result = & psql @psqlArgs
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
# 检查 a: schema_migrations 最新版本
# -----------------------------------------------------------------------------

Write-Step ("check (a): schema_migrations latest version == " + $ExpectedMigrationVersion)

$sqlA = "SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1;"
try {
    $latestVersion = Invoke-SqlScalar -Sql $sqlA
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $latestVersion = $null
}

if ($null -ne $latestVersion) {
    if ($latestVersion -eq $ExpectedMigrationVersion) {
        Write-Pass ("latest migration version is " + $ExpectedMigrationVersion + " (got: " + $latestVersion + ")")
    } else {
        Write-Fail ("expected " + $ExpectedMigrationVersion + " but got: " + $latestVersion)
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
#        与 0032/0041 一致：按 application_id = 'cloud-' || trim(account_id)
#        精确关联 integration_account（覆盖 account_id 含连字符的情况）。
# -----------------------------------------------------------------------------

Write-Step "check (c): no legacy cloud-<account_id> application ids in asset_application"

$sqlC = @"
SELECT count(*)
FROM asset_application aa
JOIN integration_account ia
  ON aa.application_id = 'cloud-' || trim(ia.account_id);
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
# 检查 f: 管理员安全态（支持发布链路重复执行）
#   - 首发 / provision 前：0044 锁定的默认 admin（locked + 空 password_hash）
#   - 二次发布 / 已 provision：至少一个 active 且绑定 admin 角色、密码非空的管理员
# -----------------------------------------------------------------------------

Write-Step "check (f): admin security state (locked pre-provision OR active provisioned admin)"

$sqlF = @"
SELECT CASE
  WHEN EXISTS (
    SELECT 1 FROM iam_user
    WHERE username = 'admin'
      AND status = 'locked'
      AND COALESCE(password_hash, '') = ''
  ) THEN 1
  WHEN EXISTS (
    SELECT 1
    FROM iam_user u
    JOIN iam_user_role ur ON ur.user_id = u.user_id
    JOIN iam_role r ON r.role_id = ur.role_id
    WHERE r.code = 'admin'
      AND u.status = 'active'
      AND COALESCE(u.password_hash, '') <> ''
  ) THEN 1
  ELSE 0
END;
"@
try {
    $countF = Invoke-SqlScalar -Sql $sqlF
} catch {
    Write-Fail ("query failed: " + $_.Exception.Message)
    $failureCount++
    $countF = $null
}

if ($null -ne $countF) {
    if ($countF -eq "1") {
        Write-Pass ("admin security state ok (locked pre-provision or active provisioned admin; result=" + $countF + ")")
    } else {
        Write-Fail ("admin security state invalid (got=" + $countF + "); expect 0044 locked admin OR an active admin-role user with password; run migrations through 0044 / provision-prod-admin.ps1")
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
