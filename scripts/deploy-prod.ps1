<#
.SYNOPSIS
    生产发布包装脚本：完整覆盖 docs/release-checklist.md §9 发布顺序。

.DESCRIPTION
    原脚本只做镜像校验 + docker compose up api。本版本扩展为完整链路：

        1. 解析 .env.production（AIOPS_IMAGE / POSTGRES_* 等键）
        2. 校验 AIOPS_IMAGE（保留原 verify-prod-version.ps1 调用）
        --- -SkipUp 在此结束（CI dry-run 语义不变）---
        3. 前置依赖检查 + 必要时启动 postgres / redis（container 相关步骤依赖）
        4. 备份关键表（除非 -SkipBackup）
        5. 执行数据库迁移（除非 -SkipMigrate）
        6. 迁移后引用完整性验证（除非 -SkipVerify）
        7. 创建 / 重置安全管理员（除非 -SkipProvisionAdmin）
        8. docker compose up -d --no-build api
        9. 轮询 /readyz，确认 data.status == "ready"

    每一步独立 Write-Step / 失败立即 exit 1。任何子脚本退出码非 0 都会中止后续步骤。

    本脚本不改 schema / 业务代码；若新版逻辑有 bug，回滚方法是 git checkout 旧版，
    或按 release-checklist §9 手工分步执行（每个子脚本均可独立调用）。

.PARAMETER EnvFile
    .env.production 路径，默认 deployments/.env.production。
    用途：docker compose --env-file；解析 AIOPS_IMAGE / POSTGRES_PASSWORD / POSTGRES_USER / POSTGRES_DB。

.PARAMETER SkipUp
    仅做 step 1+2（解析 + 镜像校验），不连 DB、不启容器、不轮询。CI dry-run 用。

.PARAMETER DbMode
    DB 访问模式：host（默认）或 container。
    host      通过 -PgHost/-PgPort/-PgPassword 直连 backup / verify / provision-admin 三个现有脚本。
              要求操作机能直接连到生产 PG（SSH 隧道、同机部署、外部 PG 均可）。
    container 通过 docker compose exec postgres 执行 pg_dump / psql；备份走容器内 pg_dump + docker cp
              拷出（避免 PowerShell 重定向的编码坑），verify 重放 verify-post-migration.ps1 的 6 项检查。
              注意：provision-admin 在 container 模式下不支持（依赖宿主 psql + go run bcrypt），
              会显式报错。请改用 -SkipProvisionAdmin 后单独 host 模式执行 provision-prod-admin.ps1。

.PARAMETER PgHost / PgPort / PgDbName / PgUser / PgPassword
    host 模式下传给 backup / verify / provision-admin 的连接参数。
    PgPassword 解析优先级：-PgPassword > $env:AIOPS_PG_PASSWORD > .env.production 的 POSTGRES_PASSWORD。
    未显式传入 -PgDbName / -PgUser 时，回退到 .env.production 的 POSTGRES_DB / POSTGRES_USER。
    container 模式下 psql / pg_dump 使用 POSTGRES_DB / POSTGRES_USER（经 Unix socket，无密码）。

.PARAMETER MigrateMode
    迁移执行方式：container（默认，对齐 docker-compose.prod.yml 顶部注释推荐方式 A）或 local。
    container  docker compose run --rm --entrypoint /app/aiops-migrate api（镜像内 migrate + env 注入）。
    local      go run ./cmd/migrate -config $MigrateConfig（要求宿主 Go 工具链且 config 指向生产 DB）。

.PARAMETER MigrateConfig
    -MigrateMode local 时的 -config 参数，默认 configs/config.yaml。

.PARAMETER ExpectedMigrationVersion
    verify-post-migration.ps1 期望的最新迁移版本，默认 0045。

.PARAMETER BackupDir
    备份目录，默认 ./backups。

.PARAMETER SkipProvisionAdmin
    跳过 provision-prod-admin.ps1。container 模式或已单独跑过该步骤时使用。

.PARAMETER AdminUsername / AdminDisplayName / AdminPassword / GenerateAdminPassword
    传给 provision-prod-admin.ps1 的参数。AdminPassword 留空且未指定 -GenerateAdminPassword 时，
    由 provision 脚本交互提示（隐藏输入）。本脚本不传 -Force：已 provision 的管理员会幂等跳过；
    需要重置密码时请单独对 provision-prod-admin.ps1 传 -Force。

.PARAMETER SkipBackup / SkipMigrate / SkipVerify
    跳过对应子步骤。罕见用法，主要给已单独跑过某步的场景。

.PARAMETER ReadyzUrl / ReadyzTimeoutS / ReadyzIntervalS
    up api 后轮询 /readyz 的参数。用 ConvertFrom-Json 取 data.status，不用 grep。

.EXAMPLE
    .\scripts\deploy-prod.ps1 -SkipUp
    # CI dry-run：仅解析 env + 校验 AIOPS_IMAGE

.EXAMPLE
    .\scripts\deploy-prod.ps1 -DbMode host -MigrateMode container -PgPassword '<db-password>' -GenerateAdminPassword
    # 完整发布：备份 → 容器内 migrate → verify → 生成随机密码并建管理员 → up api → readyz

.EXAMPLE
    .\scripts\deploy-prod.ps1 -DbMode container -MigrateMode container -SkipProvisionAdmin
    # PG 端口不映射的场景：备份 / 迁移 / 验证走容器；管理员单独 host 模式执行

.NOTES
    依赖：docker compose；host 模式额外需要 pg_dump + psql；local 迁移模式额外需要宿主 Go 工具链；
          provision 步骤额外需要宿主 Go（bcrypt cost 12 via pkg/auth）。
#>

param(
    [string]$EnvFile = "deployments/.env.production",
    [switch]$SkipUp,

    [ValidateSet("host", "container")]
    [string]$DbMode = "host",

    [string]$PgHost = "127.0.0.1",
    [int]$PgPort = 5432,
    [string]$PgDbName = "aiops",
    [string]$PgUser = "aiops",
    [string]$PgPassword = "",

    [ValidateSet("container", "local")]
    [string]$MigrateMode = "container",
    [string]$MigrateConfig = "configs/config.yaml",
    [string]$ExpectedMigrationVersion = "0045",

    [string]$BackupDir = "./backups",

    [switch]$SkipProvisionAdmin,
    [string]$AdminUsername = "admin",
    [string]$AdminDisplayName = "Administrator",
    [string]$AdminPassword = "",
    [switch]$GenerateAdminPassword,

    [switch]$SkipBackup,
    [switch]$SkipMigrate,
    [switch]$SkipVerify,

    [int]$ReadyzTimeoutS = 120,
    [int]$ReadyzIntervalS = 5,
    [string]$ReadyzUrl = "http://127.0.0.1:8080/readyz"
)

$ErrorActionPreference = "Stop"

# -----------------------------------------------------------------------------
# 路径常量
# -----------------------------------------------------------------------------

$repoRoot = Split-Path -Parent $PSScriptRoot
$verifyImageScript = Join-Path $repoRoot 'scripts/verify-prod-version.ps1'
$backupScript = Join-Path $repoRoot 'scripts/backup-pre-migration.ps1'
$verifyMigScript = Join-Path $repoRoot 'scripts/verify-post-migration.ps1'
$provisionScript = Join-Path $repoRoot 'scripts/provision-prod-admin.ps1'
$composeFile = Join-Path $repoRoot 'deployments/docker-compose.prod.yml'
$envFileFull = Join-Path $repoRoot $EnvFile

# postgres 服务在 docker-compose.prod.yml 中固定 container_name；备份时 docker cp 使用。
$pgContainerName = "aiops-postgres-prod"

# 备份目标表清单（与 backup-pre-migration.ps1 保持一致）。
$BackupTables = @(
    "asset_application",
    "asset_resource",
    "asset_match_rule",
    "alert_alert",
    "inspection_policy"
)

# -----------------------------------------------------------------------------
# 工具函数（与 backup / verify / provision 风格一致：每个脚本自带 helpers）
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

function Write-Warn {
    param([string]$Message)
    Write-Host ("    [WARN] " + $Message) -ForegroundColor Yellow
}

function Test-Command {
    param([string]$Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

# 解析 .env 文件为 hashtable；忽略注释行 / 空行，剥离首尾引号。
function Read-EnvFile {
    param([string]$Path)
    $result = @{}
    if (-not (Test-Path $Path)) { return $result }
    Get-Content $Path | ForEach-Object {
        $line = $_.Trim()
        if ($line -eq "" -or $line.StartsWith("#")) { return }
        if ($line -match '^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
            $key = $Matches[1]
            $val = $Matches[2].Trim().Trim('"').Trim("'")
            $result[$key] = $val
        }
    }
    return $result
}

# 容器内执行 psql 取标量（container 模式专用）。
# postgres 容器通过 Unix socket + peer/trust 认证，无需 PGPASSWORD；用户名/库名取自 POSTGRES_USER / POSTGRES_DB。
function Invoke-ContainerSqlScalar {
    param(
        [string]$Sql,
        [string]$PgUserInContainer,
        [string]$PgDbInContainer
    )
    $psqlArgs = @(
        "exec", "-T", "postgres", "psql",
        "--username", $PgUserInContainer,
        "--dbname", $PgDbInContainer,
        "--no-align", "--tuples-only", "--no-psqlrc", "--quiet",
        "--command", $Sql
    )
    $output = & docker compose --env-file $script:envFileFull -f $script:composeFile @psqlArgs 2>&1
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw ("docker compose exec psql 失败 (exit=" + $exitCode + "): " + ($output -join "`n"))
    }
    if ($null -eq $output) { return "" }
    return ($output -join " ").Trim()
}

# 容器内 pg_dump 并 docker cp 拷出（避免 PowerShell 重定向把 UTF-8 / 二进制改写成 UTF-16）。
# $DumpSpec 例如 "--data-only --table=asset_application" 或 "--format=custom"。
function Invoke-ContainerBackup {
    param(
        [string]$OutputFile,
        [string]$PgUserInContainer,
        [string]$PgDbInContainer,
        [string]$DumpSpec
    )
    $inContainerPath = "/tmp/aiops_dump_" + ([System.Guid]::NewGuid().ToString("N").Substring(0, 8))
    $innerCmd = "pg_dump --username " + $PgUserInContainer + " --dbname " + $PgDbInContainer + " " + $DumpSpec + " > " + $inContainerPath

    & docker compose --env-file $script:envFileFull -f $script:composeFile exec -T postgres sh -c $innerCmd
    if ($LASTEXITCODE -ne 0) {
        Write-Fail ("container pg_dump 失败 (exit=" + $LASTEXITCODE + ", spec=" + $DumpSpec + ")")
        & docker compose --env-file $script:envFileFull -f $script:composeFile exec -T postgres rm -f $inContainerPath 2>&1 | Out-Null
        return $false
    }

    & docker cp ($script:pgContainerName + ":" + $inContainerPath) $OutputFile
    $cpExit = $LASTEXITCODE

    # 无论 cp 成功与否都清理容器内临时文件
    & docker compose --env-file $script:envFileFull -f $script:composeFile exec -T postgres rm -f $inContainerPath 2>&1 | Out-Null

    if ($cpExit -ne 0) {
        Write-Fail ("docker cp 失败 (exit=" + $cpExit + ", " + $script:pgContainerName + ":" + $inContainerPath + " -> " + $OutputFile + ")")
        return $false
    }
    return $true
}

# 等待 /readyz 返回 data.status == "ready"，超时返回 $false。
# 用 ConvertFrom-Json 取嵌套字段，规避 grep 误中嵌套子串的问题。
function Wait-Ready {
    param([string]$Url, [int]$TimeoutS, [int]$IntervalS)
    $deadline = (Get-Date).AddSeconds($TimeoutS)
    $attempt = 0
    while ((Get-Date) -lt $deadline) {
        $attempt++
        try {
            $resp = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 5
            $json = $resp.Content | ConvertFrom-Json
            $status = $json.data.status
            if ($status -eq "ready") {
                Write-Pass ("ready after " + $attempt + " attempt(s): data.status=" + $status)
                return $true
            }
            Write-Warn ("attempt #" + $attempt + ": data.status=" + $status + ", keep waiting")
        } catch {
            Write-Warn ("attempt #" + $attempt + ": " + $_.Exception.Message)
        }
        Start-Sleep -Seconds $IntervalS
    }
    return $false
}

# -----------------------------------------------------------------------------
# step 1: 解析 .env.production
# -----------------------------------------------------------------------------

Write-Step "step 1/9: parse env file"

if (-not (Test-Path $envFileFull)) {
    Write-Fail ("env file not found: " + $envFileFull)
    exit 1
}

$envVars = Read-EnvFile -Path $envFileFull

# AIOPS_IMAGE：进程环境变量优先（保留原 deploy-prod.ps1 行为），回退到 .env 文件
$aiopsImage = $env:AIOPS_IMAGE
if ([string]::IsNullOrWhiteSpace($aiopsImage)) {
    $aiopsImage = $envVars['AIOPS_IMAGE']
}

$envPgUser = $envVars['POSTGRES_USER']
if ([string]::IsNullOrWhiteSpace($envPgUser)) { $envPgUser = "aiops" }
$envPgDb = $envVars['POSTGRES_DB']
if ([string]::IsNullOrWhiteSpace($envPgDb)) { $envPgDb = "aiops" }
$envPgPassword = $envVars['POSTGRES_PASSWORD']

# 未显式传 -PgDbName / -PgUser 时，与 .env.production 对齐，避免自定义库名/用户名漂移。
if (-not $PSBoundParameters.ContainsKey('PgDbName')) { $PgDbName = $envPgDb }
if (-not $PSBoundParameters.ContainsKey('PgUser')) { $PgUser = $envPgUser }

Write-Pass ("env file parsed: " + $envFileFull + " (" + $envVars.Count + " entries; db=" + $envPgDb + ", user=" + $envPgUser + ")")

# -----------------------------------------------------------------------------
# step 2: 校验 AIOPS_IMAGE
# -----------------------------------------------------------------------------

Write-Step "step 2/9: verify AIOPS_IMAGE"
& $verifyImageScript -Image $aiopsImage
if ($LASTEXITCODE -ne 0) {
    Write-Fail "AIOPS_IMAGE 校验失败，中止部署。"
    exit 1
}

if ($SkipUp) {
    Write-Step "-SkipUp set, exiting after image verification (CI dry-run)"
    Write-Host "    OK: verification passed, no further actions performed." -ForegroundColor Green
    exit 0
}

# -----------------------------------------------------------------------------
# step 3: 前置依赖检查 + 必要时启动中间件
# -----------------------------------------------------------------------------

Write-Step "step 3/9: check prerequisites"

if (-not (Test-Command "docker")) {
    Write-Fail "docker not found in PATH"
    exit 1
}
Write-Pass "docker available"

# host 模式下，按需检查 pg_dump / psql
$needHostPgDump = (-not $SkipBackup) -and ($DbMode -eq "host")
$needHostPsql = ((-not $SkipVerify) -or (-not $SkipProvisionAdmin)) -and ($DbMode -eq "host")
if ($needHostPgDump -and -not (Test-Command "pg_dump")) {
    Write-Fail "pg_dump not found in PATH (host 模式 + 备份需要 pg_dump)"
    exit 1
}
if ($needHostPsql -and -not (Test-Command "psql")) {
    Write-Fail "psql not found in PATH (host 模式 + verify/provision 需要 psql)"
    exit 1
}
if ($needHostPgDump) { Write-Pass "pg_dump available" }
if ($needHostPsql)   { Write-Pass "psql available" }

# provision 步骤：需要宿主 Go（go run ./scripts/tools/hash-password bcrypt cost 12）+ host DB 模式
if (-not $SkipProvisionAdmin) {
    if (-not (Test-Command "go")) {
        Write-Fail "go not found in PATH (provision 需要 go run ./scripts/tools/hash-password 做 bcrypt)"
        exit 1
    }
    Write-Pass "go available (provision bcrypt)"

    if ($DbMode -eq "container") {
        Write-Fail "provision-prod-admin.ps1 不支持 container 模式（依赖宿主 psql + go run bcrypt）。"
        Write-Fail "  方式 1：用 -DbMode host 重新执行完整链路；"
        Write-Fail "  方式 2：加 -SkipProvisionAdmin 跳过本步，再单独 host 模式运行 provision-prod-admin.ps1。"
        exit 1
    }
}

# PgPassword 解析（仅 host 模式 + 有 DB 操作时需要）
$resolvedPgPassword = $PgPassword
if ([string]::IsNullOrWhiteSpace($resolvedPgPassword)) {
    $resolvedPgPassword = $env:AIOPS_PG_PASSWORD
}
if ([string]::IsNullOrWhiteSpace($resolvedPgPassword)) {
    $resolvedPgPassword = $envPgPassword
}
if (($needHostPgDump -or $needHostPsql) -and [string]::IsNullOrWhiteSpace($resolvedPgPassword)) {
    Write-Fail "PgPassword 未提供。请用 -PgPassword 传入、设置 AIOPS_PG_PASSWORD 环境变量，或在 .env.production 中填写 POSTGRES_PASSWORD。"
    exit 1
}

# local 迁移模式：要求宿主 Go 工具链 + 配置文件指向生产 DB
if ((-not $SkipMigrate) -and ($MigrateMode -eq "local")) {
    if (-not (Test-Command "go")) {
        Write-Fail "go not found in PATH (local 迁移模式需要宿主 Go 工具链)"
        exit 1
    }
    Write-Warn ("local 迁移模式：确保 -MigrateConfig (" + $MigrateConfig + ") 指向生产 DB")
}

# 必要时启动中间件（container 模式相关步骤都依赖 postgres 已起）
$containerBackupNeeded = (-not $SkipBackup) -and ($DbMode -eq "container")
$containerMigrateNeeded = (-not $SkipMigrate) -and ($MigrateMode -eq "container")
$containerVerifyNeeded = (-not $SkipVerify) -and ($DbMode -eq "container")
$needsMiddleware = $containerBackupNeeded -or $containerMigrateNeeded -or $containerVerifyNeeded
if ($needsMiddleware) {
    Write-Step "step 3: ensure postgres + redis are up (container mode)"
    docker compose --env-file $envFileFull -f $composeFile up -d --no-build postgres redis
    if ($LASTEXITCODE -ne 0) {
        Write-Fail ("up postgres redis 失败, exit=" + $LASTEXITCODE)
        exit $LASTEXITCODE
    }
    Write-Pass "postgres + redis up"
}

# -----------------------------------------------------------------------------
# step 4: 备份
# -----------------------------------------------------------------------------

if (-not $SkipBackup) {
    Write-Step "step 4/9: backup critical tables"
    if (-not (Test-Path $BackupDir)) {
        New-Item -ItemType Directory -Path $BackupDir -Force | Out-Null
        Write-Pass ("created backup dir: " + (Resolve-Path $BackupDir).Path)
    }
    $ts = Get-Date -Format "yyyyMMdd_HHmmss"

    if ($DbMode -eq "host") {
        & $backupScript -PgHost $PgHost -PgPort $PgPort -PgDbName $PgDbName -PgUser $PgUser -PgPassword $resolvedPgPassword -BackupDir $BackupDir
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "backup-pre-migration.ps1 失败"
            exit $LASTEXITCODE
        }
    } else {
        # container 模式：复用 backup-pre-migration.ps1 的目标表清单与文件名约定，
        # 但 prefix 用本次目标版本（脚本自带 -BackupDir 创建已在上面完成）。
        foreach ($table in $BackupTables) {
            $backupFile = Join-Path $BackupDir ("aiops_pre_{0}_{1}_{2}.sql" -f $ExpectedMigrationVersion, $table, $ts)
            Write-Host ("    backing up table: " + $table)
            $ok = Invoke-ContainerBackup -OutputFile $backupFile -PgUserInContainer $envPgUser -PgDbInContainer $envPgDb -DumpSpec ("--data-only --table=" + $table)
            if (-not $ok) { exit 1 }
            $sz = (Get-Item $backupFile).Length
            Write-Pass ("table " + $table + " -> " + $backupFile + " (" + $sz + " B)")
        }
        $fullFile = Join-Path $BackupDir ("aiops_pre_{0}_full_{1}.dump" -f $ExpectedMigrationVersion, $ts)
        Write-Host "    backing up full database (custom format)"
        $ok = Invoke-ContainerBackup -OutputFile $fullFile -PgUserInContainer $envPgUser -PgDbInContainer $envPgDb -DumpSpec "--format=custom"
        if (-not $ok) { exit 1 }
        $szFull = (Get-Item $fullFile).Length
        Write-Pass ("full db -> " + $fullFile + " (" + $szFull + " B)")
    }
} else {
    Write-Step "step 4/9: backup skipped (-SkipBackup)"
}

# -----------------------------------------------------------------------------
# step 5: 迁移
# -----------------------------------------------------------------------------

if (-not $SkipMigrate) {
    Write-Step ("step 5/9: run database migrations (mode=" + $MigrateMode + ")")
    if ($MigrateMode -eq "container") {
        # docker-compose.prod.yml 顶部注释推荐方式 A：镜像内置 aiops-migrate 二进制。
        # `run` 会通过 depends_on 自动启动 postgres + redis（若未起）。
        docker compose --env-file $envFileFull -f $composeFile run --rm --entrypoint /app/aiops-migrate api
        if ($LASTEXITCODE -ne 0) {
            Write-Fail ("container migrate 失败, exit=" + $LASTEXITCODE)
            exit $LASTEXITCODE
        }
    } else {
        # 方式 C：本地 go run。要求宿主 Go 工具链且 $MigrateConfig 指向生产 DB。
        Push-Location $repoRoot
        try {
            & go run ./cmd/migrate -config $MigrateConfig
            $migExit = $LASTEXITCODE
        } finally {
            Pop-Location
        }
        if ($migExit -ne 0) {
            Write-Fail ("local migrate 失败, exit=" + $migExit)
            exit $migExit
        }
    }
    Write-Pass "migrate done"
} else {
    Write-Step "step 5/9: migrate skipped (-SkipMigrate)"
}

# -----------------------------------------------------------------------------
# step 6: 迁移后引用完整性验证
# -----------------------------------------------------------------------------

if (-not $SkipVerify) {
    Write-Step "step 6/9: verify post-migration integrity"
    if ($DbMode -eq "host") {
        & $verifyMigScript -PgHost $PgHost -PgPort $PgPort -PgDbName $PgDbName -PgUser $PgUser -PgPassword $resolvedPgPassword -ExpectedMigrationVersion $ExpectedMigrationVersion
        if ($LASTEXITCODE -ne 0) {
            Write-Fail "verify-post-migration.ps1 失败"
            exit $LASTEXITCODE
        }
    } else {
        # container 模式：重放 verify-post-migration.ps1 的 6 项检查（a-f）。
        # 任何 verify 检查的语义变化须同步更新 backup-pre-migration.ps1 与此处。
        $checks = @(
            @{ Name = "(a) schema_migrations latest == " + $ExpectedMigrationVersion;
               Sql  = "SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1;";
               Want = $ExpectedMigrationVersion },
            @{ Name = "(b) v_asset_app_ref_integrity count == 0";
               Sql  = "SELECT count(*) FROM v_asset_app_ref_integrity;";
               Want = "0" },
            @{ Name = "(c) no legacy cloud-<account_id> application ids in asset_application";
               Sql  = "SELECT count(*) FROM asset_application aa JOIN integration_account ia ON aa.application_id = 'cloud-' || trim(ia.account_id);";
               Want = "0" },
            @{ Name = "(d) no dangling application_id in alert_alert";
               Sql  = "SELECT count(*) FROM alert_alert a WHERE a.application_id IS NOT NULL AND a.application_id <> '' AND NOT EXISTS (SELECT 1 FROM asset_application app WHERE app.application_id = a.application_id);";
               Want = "0" },
            @{ Name = "(e) no dangling application_ids in inspection_policy.scope";
               Sql  = "SELECT count(*) FROM inspection_policy p, jsonb_array_elements_text(COALESCE(p.scope->'application_ids', '[]'::jsonb)) AS elem(app_id) WHERE NOT EXISTS (SELECT 1 FROM asset_application app WHERE app.application_id = elem.app_id);";
               Want = "0" },
            @{ Name = "(f) admin security state (locked pre-provision OR active provisioned admin)";
               Sql  = "SELECT CASE WHEN EXISTS (SELECT 1 FROM iam_user WHERE username = 'admin' AND status = 'locked' AND COALESCE(password_hash, '') = '') THEN 1 WHEN EXISTS (SELECT 1 FROM iam_user u JOIN iam_user_role ur ON ur.user_id = u.user_id JOIN iam_role r ON r.role_id = ur.role_id WHERE r.code = 'admin' AND u.status = 'active' AND COALESCE(u.password_hash, '') <> '') THEN 1 ELSE 0 END;";
               Want = "1" }
        )
        $failures = 0
        foreach ($c in $checks) {
            try {
                $got = Invoke-ContainerSqlScalar -Sql $c.Sql -PgUserInContainer $envPgUser -PgDbInContainer $envPgDb
            } catch {
                Write-Fail ($c.Name + " query failed: " + $_.Exception.Message)
                $failures++
                continue
            }
            if ($got -eq $c.Want) {
                Write-Pass ($c.Name + " (got=" + $got + ")")
            } else {
                Write-Fail ($c.Name + " expected '" + $c.Want + "' got '" + $got + "'")
                $failures++
            }
        }
        if ($failures -gt 0) {
            Write-Fail ("container verify 失败, " + $failures + " check(s) failed")
            exit 1
        }
    }
} else {
    Write-Step "step 6/9: verify skipped (-SkipVerify)"
}

# -----------------------------------------------------------------------------
# step 7: 创建 / 重置安全管理员
# -----------------------------------------------------------------------------

if (-not $SkipProvisionAdmin) {
    Write-Step ("step 7/9: provision admin (username=" + $AdminUsername + ")")
    # 不传 -Force：已 provision 的管理员会幂等跳过；需要重置密码时单独对 provision 传 -Force。
    $provArgs = @(
        "-PgHost", $PgHost,
        "-PgPort", $PgPort,
        "-PgDbName", $PgDbName,
        "-PgUser", $PgUser,
        "-PgPassword", $resolvedPgPassword,
        "-Username", $AdminUsername,
        "-DisplayName", $AdminDisplayName
    )
    if ($GenerateAdminPassword) {
        $provArgs += "-GeneratePassword"
    } elseif (-not [string]::IsNullOrWhiteSpace($AdminPassword)) {
        $provArgs += @("-Password", $AdminPassword)
    }
    & $provisionScript @provArgs
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "provision-prod-admin.ps1 失败"
        exit $LASTEXITCODE
    }
} else {
    Write-Step "step 7/9: provision admin skipped (-SkipProvisionAdmin)"
}

# -----------------------------------------------------------------------------
# step 8: 启动 API
# -----------------------------------------------------------------------------

Write-Step "step 8/9: docker compose up -d --no-build api"
docker compose --env-file $envFileFull -f $composeFile up -d --no-build api
if ($LASTEXITCODE -ne 0) {
    Write-Fail ("docker compose up api 失败, exit=" + $LASTEXITCODE)
    exit $LASTEXITCODE
}
Write-Pass "api started"

# -----------------------------------------------------------------------------
# step 9: 轮询 /readyz
# -----------------------------------------------------------------------------

Write-Step ("step 9/9: wait for /readyz (timeout=" + $ReadyzTimeoutS + "s, url=" + $ReadyzUrl + ")")
$ready = Wait-Ready -Url $ReadyzUrl -TimeoutS $ReadyzTimeoutS -IntervalS $ReadyzIntervalS
if (-not $ready) {
    Write-Fail ("/readyz 未在 " + $ReadyzTimeoutS + "s 内返回 data.status=ready；检查容器日志：docker compose -f " + $composeFile + " logs api")
    exit 1
}

# -----------------------------------------------------------------------------
# 收尾：提示剩余手工步骤（release-checklist §9 步骤 7-9）
# -----------------------------------------------------------------------------

Write-Host ""
Write-Host "DEPLOY COMPLETE" -ForegroundColor Green
Write-Host "    剩余手工步骤（docs/release-checklist.md §9）：" -ForegroundColor Cyan
Write-Host "    7. 部署前端静态资源"
Write-Host "    8. 执行 E2E 脚本或手工抽检（docs/demo-flow.md）"
Write-Host "    9. 观察日志与告警 15-30 分钟"
exit 0
