# 生产首个安全管理员初始化（0044 锁定默认 admin 后使用）。
# 用法示例：
#   ./scripts/provision-prod-admin.ps1 -PgPassword '<db-password>'
#   ./scripts/provision-prod-admin.ps1 -Username ops-admin -GeneratePassword
#
# 默认重新激活被 0044 锁定的 username=admin 账号并绑定 admin 角色；
# 指定 -Username 且非 admin 时创建新用户并绑定 admin 角色。
# 可重复执行：目标用户已是 active + 非空密码 + admin 角色时幂等跳过；-Force 才重置密码。

param(
    [string]$PgHost = "127.0.0.1",
    [int]$PgPort = 5432,
    [string]$PgDbName = "aiops",
    [string]$PgUser = "aiops",
    [string]$PgPassword = "",
    [string]$Username = "admin",
    [string]$DisplayName = "Administrator",
    [string]$Password = "",
    [switch]$GeneratePassword,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

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
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Escape-SqlLiteral {
    param([string]$Value)
    return ($Value -replace "'", "''")
}

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

function Invoke-SqlExec {
    param([string]$Sql)
    $env:PGPASSWORD = $PgPassword
    $psqlArgs = @(
        "--host", $PgHost,
        "--port", $PgPort,
        "--username", $PgUser,
        "--dbname", $PgDbName,
        "--no-psqlrc",
        "--quiet",
        "--command", $Sql
    )
    & psql @psqlArgs | Out-Null
    $exitCode = $LASTEXITCODE
    $env:PGPASSWORD = $null
    if ($exitCode -ne 0) {
        throw ("psql execution failed (exit=" + $exitCode + ")")
    }
}

function New-RandomPassword {
    if (Test-Command "openssl") {
        return (& openssl rand -base64 24).Trim()
    }
    $bytes = New-Object byte[] 24
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return [Convert]::ToBase64String($bytes)
}

function Get-BcryptHash {
    param([string]$Plain)
    $repoRoot = Split-Path -Parent $PSScriptRoot
    Push-Location $repoRoot
    try {
        $hash = & go run ./scripts/tools/hash-password $Plain 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw ("hash-password failed: " + ($hash -join "`n"))
        }
        return ($hash | Out-String).Trim()
    } finally {
        Pop-Location
    }
}

Write-Step "checking prerequisites"

if (-not (Test-Command "psql")) {
    Write-Fail "psql not found in PATH"
    exit 1
}
if (-not (Test-Command "go")) {
    Write-Fail "go not found in PATH (required for bcrypt cost 12 via pkg/auth)"
    exit 1
}
if ([string]::IsNullOrWhiteSpace($PgPassword)) {
    Write-Fail "PgPassword is required (do not commit database credentials)"
    exit 1
}

Write-Step "checking migration 0044 state for username=$Username"

$escapedUser = Escape-SqlLiteral $Username
$userStatus = Invoke-SqlScalar -Sql ("SELECT status FROM iam_user WHERE username = '" + $escapedUser + "' LIMIT 1;")
$userHash = Invoke-SqlScalar -Sql ("SELECT password_hash FROM iam_user WHERE username = '" + $escapedUser + "' LIMIT 1;")
$userIdExisting = Invoke-SqlScalar -Sql ("SELECT user_id FROM iam_user WHERE username = '" + $escapedUser + "' LIMIT 1;")
$roleBoundExisting = "0"
if (-not [string]::IsNullOrWhiteSpace($userIdExisting)) {
    $escapedUserIdExisting = Escape-SqlLiteral $userIdExisting
    $roleBoundExisting = Invoke-SqlScalar -Sql @"
SELECT count(*)
FROM iam_user_role ur
JOIN iam_role r ON r.role_id = ur.role_id
WHERE ur.user_id = '$escapedUserIdExisting' AND r.code = 'admin';
"@
}

# 可重复执行：已有安全管理员（active + 密码哈希 + admin 角色）时直接成功退出，不改密码。
# 需要重置密码时显式传 -Force。
if (-not $Force -and $userStatus -eq "active" -and -not [string]::IsNullOrWhiteSpace($userHash) -and $roleBoundExisting -eq "1") {
    Write-Pass ("admin already provisioned: username=" + $Username + ", user_id=" + $userIdExisting + " (skip; use -Force to reset password)")
    exit 0
}

if ($Username -eq "admin") {
    if ([string]::IsNullOrWhiteSpace($userStatus)) {
        Write-Fail "username=admin not found; run migrations through 0016 first"
        exit 1
    }
    if ($userStatus -ne "locked" -and -not $Force) {
        Write-Fail ("expected admin status=locked after 0044 (or already-provisioned active admin), got status=" + $userStatus + "; use -Force to override")
        exit 1
    }
} else {
    if (-not [string]::IsNullOrWhiteSpace($userStatus) -and -not $Force) {
        Write-Fail ("username already exists: " + $Username + "; use -Force to reset password")
        exit 1
    }
}

$plainPassword = $Password
if ($GeneratePassword) {
    $plainPassword = New-RandomPassword
} elseif ([string]::IsNullOrWhiteSpace($plainPassword)) {
    $secure = Read-Host "Enter admin password (input hidden)" -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $plainPassword = [Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
}

if ([string]::IsNullOrWhiteSpace($plainPassword)) {
    Write-Fail "password must not be empty"
    exit 1
}
if ($plainPassword.Length -lt 12) {
    Write-Fail "password too short (minimum 12 characters recommended)"
    exit 1
}

Write-Step "hashing password (bcrypt cost 12 via pkg/auth)"
$passwordHash = Get-BcryptHash -Plain $plainPassword
if (-not $passwordHash.StartsWith("`$2a`$12`$")) {
    Write-Fail ("unexpected bcrypt hash prefix: " + $passwordHash.Substring(0, [Math]::Min(7, $passwordHash.Length)))
    exit 1
}

Write-Step "writing admin account to database"
$escapedHash = Escape-SqlLiteral $passwordHash
$escapedDisplay = Escape-SqlLiteral $DisplayName

if ($Username -eq "admin") {
    $sql = @"
UPDATE iam_user
SET password_hash = '$escapedHash',
    status = 'active',
    display_name = '$escapedDisplay',
    updated_at = NOW()
WHERE username = 'admin';
"@
    Invoke-SqlExec -Sql $sql
    $userId = Invoke-SqlScalar -Sql "SELECT user_id FROM iam_user WHERE username = 'admin' LIMIT 1;"
} else {
    $sql = @"
INSERT INTO iam_user (user_id, username, display_name, email, password_hash, status, created_at, updated_at)
VALUES (
    gen_random_uuid()::text,
    '$escapedUser',
    '$escapedDisplay',
    '',
    '$escapedHash',
    'active',
    NOW(),
    NOW()
)
ON CONFLICT (username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    status = 'active',
    display_name = EXCLUDED.display_name,
    updated_at = NOW();
"@
    Invoke-SqlExec -Sql $sql
    $userId = Invoke-SqlScalar -Sql ("SELECT user_id FROM iam_user WHERE username = '" + $escapedUser + "' LIMIT 1;")
}

if ([string]::IsNullOrWhiteSpace($userId)) {
    Write-Fail "failed to resolve user_id after upsert"
    exit 1
}

$escapedUserId = Escape-SqlLiteral $userId
$bindSql = @"
INSERT INTO iam_user_role (user_id, role_id, source, created_at, updated_at)
SELECT '$escapedUserId', r.role_id, 'manual', NOW(), NOW()
FROM iam_role r
WHERE r.code = 'admin'
ON CONFLICT (user_id, role_id) DO UPDATE SET
    source = EXCLUDED.source,
    updated_at = NOW();
"@
Invoke-SqlExec -Sql $bindSql

$roleBound = Invoke-SqlScalar -Sql @"
SELECT count(*)
FROM iam_user_role ur
JOIN iam_role r ON r.role_id = ur.role_id
WHERE ur.user_id = '$escapedUserId' AND r.code = 'admin';
"@

if ($roleBound -ne "1") {
    Write-Fail ("admin role binding missing (count=" + $roleBound + ")")
    exit 1
}

Write-Pass ("admin account ready: username=" + $Username + ", user_id=" + $userId)
if ($GeneratePassword) {
    Write-Host ""
    Write-Host "Generated password (store in a secret manager; shown once):" -ForegroundColor Yellow
    Write-Host $plainPassword
}

exit 0
