<#
.SYNOPSIS
    生产发布包装脚本：先校验 AIOPS_IMAGE，再启动 docker-compose.prod.yml。

.DESCRIPTION
    强制在 docker compose up 之前运行 verify-prod-version.ps1，杜绝 latest / 空 /
    非完整引用绕过。校验失败则中止，不会执行 up。

    本脚本不替你执行数据库迁移；请先按 docker-compose.prod.yml 顶部注释完成
    中间件启动与数据库迁移，再运行本脚本启动 API。

.PARAMETER EnvFile
    .env.production 文件路径，默认 deployments/.env.production。
    AIOPS_IMAGE 优先从该文件读取，回退到进程环境变量 AIOPS_IMAGE。

.PARAMETER SkipUp
    仅校验，不执行 docker compose up（用于 CI dry-run）。

.EXAMPLE
    .\scripts\deploy-prod.ps1
    # 校验 AIOPS_IMAGE 后启动 api

.EXAMPLE
    .\scripts\deploy-prod.ps1 -SkipUp
    # 仅校验，不 up（CI 用）

.NOTES
    依赖：docker compose、PowerShell 5.1+。
#>

param(
    [string]$EnvFile = "deployments/.env.production",
    [switch]$SkipUp
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host ("==> " + $Message)
}

function Write-Fail {
    param([string]$Message)
    Write-Host ("    [FAIL] " + $Message) -ForegroundColor Red
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$verifyScript = Join-Path $repoRoot (Join-Path 'scripts' 'verify-prod-version.ps1')
$composeFile = Join-Path $repoRoot (Join-Path 'deployments' 'docker-compose.prod.yml')

# 1) 解析 AIOPS_IMAGE：优先进程环境变量 AIOPS_IMAGE，回退 .env.production
$aiopsImage = $env:AIOPS_IMAGE
$envFileFull = Join-Path $repoRoot $EnvFile
if ([string]::IsNullOrWhiteSpace($aiopsImage) -and (Test-Path $envFileFull)) {
    Get-Content $envFileFull | ForEach-Object {
        $line = $_.Trim()
        if ($line -match '^AIOPS_IMAGE\s*=\s*(.*)$') {
            $val = $Matches[1].Trim().Trim('"').Trim("'")
            if (-not [string]::IsNullOrWhiteSpace($val)) { $aiopsImage = $val }
        }
    }
}

Write-Step "step 1/2: verify AIOPS_IMAGE"
& $verifyScript -Image $aiopsImage
if ($LASTEXITCODE -ne 0) {
    Write-Fail "AIOPS_IMAGE 校验失败，中止部署。"
    exit 1
}

if ($SkipUp) {
    Write-Step "step 2/2: -SkipUp set, skipping docker compose up"
    Write-Host "    OK: verification passed, no up performed." -ForegroundColor Green
    exit 0
}

Write-Step "step 2/2: docker compose up -d --no-build api"
docker compose --env-file $envFileFull -f $composeFile up -d --no-build api
if ($LASTEXITCODE -ne 0) {
    Write-Fail ("docker compose up 失败，exit=" + $LASTEXITCODE)
    exit $LASTEXITCODE
}

Write-Host "    OK: api started." -ForegroundColor Green
exit 0
